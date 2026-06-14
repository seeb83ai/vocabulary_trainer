# Project Review — vocabulary_trainer

_Date: 2026-06-13_

A deep review across four dimensions: Go backend, JS frontend, security, and
testing/CI/deployment. Overall this is a **mature, well-engineered project** —
bcrypt auth, AES-GCM-encrypted API keys, per-user query isolation with a
dedicated isolation test suite, version-based migrations, three-layer testing,
ADRs, and a solid deployment story. The items below are prioritized
improvements, not a sign of poor health.

## Verified non-issue (important)

An automated pass flagged "critical cross-user data leaks" in the `hmm_scenes`
and `confusion_pairs` tables, since neither has a `user_id` column. **This was
verified as NOT exploitable.** Every handler path validates word ownership via
`GetWordByID(ctx, userID, ...)` before touching those tables (e.g.
`handlers/quiz.go:375`, `handlers/hmm.go:310`), and because the `words` table is
per-user (migration v21), the word-keyed rows inherit ownership.

It remains a **defense-in-depth gap**: isolation for these two tables relies
entirely on handler-layer checks rather than the schema. A future handler that
forgets the ownership check would leak silently. Worth a note in the schema /
ADRs, but not a live bug.

## Priority 1 — Small, high-value fixes (~1 day total)

1. **Session cookies never set `Secure: true`** — `handlers/auth.go:209` plus 5
   other `SetCookie` sites. Behind nginx with HSTS it's mitigated, but one
   misconfiguration exposes session tokens over HTTP. Gate on an env var
   defaulting to secure.
2. **`llm.go:362` logs the full LLM request payload** (`log.Printf("%v\n", payload)`) —
   user mnemonics/comments end up in server logs. Replace with a redacted
   one-liner.
3. **Bare `http.ListenAndServe` in `main.go:366`** — no read/write/idle timeouts
   (Slowloris exposure) and no graceful shutdown (in-flight requests dropped on
   the systemd watcher's auto-restarts). Switch to `*http.Server` + signal
   handler.
4. **No `http.MaxBytesReader` on request bodies** — CSV upload caps multipart
   parsing at 32 MB but nothing limits other JSON endpoints.
5. **Silently ignored errors in the answer path** — `quiz.go:457, 459, 481, 496`.
   `SaveSM2PrevState` failing means "accept as correct (typo)" silently restores
   nothing. At minimum log these.

## Priority 2 — CI quality gates (~half a day)

CI runs all three test suites with `-race` (good), but has **zero static
analysis**: no `go vet`, no golangci-lint/staticcheck, no eslint, no gofmt
check. For a project largely developed by agents, lint gates are
disproportionately valuable — they catch exactly the class of drift (unused
code, shadowed vars, ignored errors) that accumulates across many small PRs.
Add a lint job to `.github/workflows/test.yml` and a minimal `.golangci.yml`.

## Priority 3 — Data safety and reliability

1. **Backup story is one manual Makefile target** with no schedule, retention,
   or tested restore. For a single-SQLite-file app this is the biggest
   real-world data-loss risk. A systemd timer running `sqlite3 .backup` nightly
   + a documented restore test would close it.
2. **Migration tests cover ~19%** (only v37/v39 of 50+ migrations). Add a test
   that runs the full chain from an empty DB and from a mid-history snapshot —
   table-recreation migrations like v18/v21 are where upgrades break.
3. **Duplicate timestamp columns** `first_seen_date` and `first_seen_at` on
   `sm2_progress` must be updated in lockstep or the new-word cooldown logic
   breaks (`db/quiz.go:102`, `db/words.go:729`). Consolidate to one.
4. **`AcknowledgeRandomWords` loops multi-statement writes without a
   transaction** (`db/quiz.go:121-164`) — a mid-loop failure leaves partial
   state.

## Priority 4 — Maintainability debt (phased, not urgent)

- **Three parallel quiz stacks** (`quiz.go` 884 lines, `pinyin_quiz.go`,
  `hmm_quiz.go`) duplicate tier-bucketing logic three times with identical
  thresholds (`quiz.go:26`, `pinyin_quiz.go:27`, `hmm_quiz.go:20`). Extract one
  `CalculateTier` into `sm2` — bug fixes currently need porting ×3.
- **Giant test files**: `handlers_test.go` (5,396 lines) and `db_test.go`
  (3,982 lines) should be split by domain (`quiz_test.go`, `words_test.go`, …)
  with shared helpers. Go allows multiple test files per package, so this is
  mechanical.
- **`vocab.js` (1,849) and `train.js` (1,466)** need splitting by concern
  (CRUD/form/import; card-render/result/filters/onboarding). Also: a
  double-submit race in the vocab form (no button disabling during `await`),
  event listeners re-attached on every result render in `train.js`, and
  inconsistent error handling — some pages use `apiFetch`, others raw `fetch`
  without checking `res.ok`.
- **Frontend test convention is drifting**: functions are inlined into
  `*.test.js` per CLAUDE.md, so `levenshtein`/`canSubmit` copies in tests can
  silently diverge from production code. Consider relaxing that rule now that
  Vitest is set up — plain ESM imports would work without a bundler.
- **i18n coverage gaps**: ~30 hardcoded user-facing strings (`index.js`,
  `hmm-builder.js`, `settings.js:84`, `train.js:517`) bypass `i18n.js`.

## Priority 5 — Docs and hygiene

- **README API table has drifted ~10%**: at least 6 routes are undocumented
  (`/api/quiz/accept-correct`, `/quiz/langs`, `/quiz/acknowledge-random`,
  `/quiz/advance`, `/words/{id}/review`, `/hmm/breakdown`). A small script that
  diffs `main.go` routes against the README would keep this honest in CI.
- `dictionary.txt` (2.5 MB) bloats every clone — download-on-demand in the
  import tool or git-lfs.
- `docker-compose.yml` lacks a healthcheck; Dockerfile should add an explicit
  non-root `USER`.
- Untested packages: `email/`, `tts/`, and all four CLI import tools (~1,100
  lines at 0% coverage). Coverage elsewhere is good: `sm2` 97%, `llm` 71%,
  `handlers`/`db` ~60%.

## Suggested first PR

Bundle Priority 1 (items 1–4 are each a few lines) plus the CI lint job —
roughly a day of work that closes all the security findings and stops quality
drift going forward. The backup timer is the best second PR.
