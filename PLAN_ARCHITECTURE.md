# Architecture Review & Improvement Roadmap

_Solution-architect review of **vocabulary_trainer** — a self-hosted Chinese–English
vocabulary trainer (Go backend, vanilla JS frontend, SQLite, SM-2 spaced repetition)._

## Verdict

This is a healthy, well-engineered codebase. The layering is clean
(`handlers → Store → SQLite`), error handling is centralized, the security posture is
strong, and the test suites are large (`handlers_test.go` ~5,400 lines, `db_test.go`
~4,000 lines). The recommendations below are **hardening and maintainability**
improvements, not a rewrite.

### Strengths to preserve
- Clean separation of concerns; **all SQL confined to `service/db/`**; transactions use a
  consistent defer-Rollback / explicit-Commit pattern.
- Centralized `writeJSON` / `writeError` / `internalError` helpers; parameterized SQL
  everywhere (including safe dynamic `IN (...)` placeholder building).
- Security done right: timing-safe HMAC session comparison, session invalidation on
  password change, account lockout, bcrypt, AES-GCM-encrypted per-user settings, strict
  CSP, per-IP + per-user rate limiting, consistent `escHtml()` on the frontend.
- Versioned migration system (52+ migrations); pragmatic `SetMaxOpenConns(1)` for SQLite WAL.
- Robust E2E harness: builds the binary, runs against a temp DB, seeds pre-authenticated state.

---

## Findings & roadmap (prioritized)

### P1 — Reliability & operational hardening _(low effort, high value)_

| # | Finding | Where | Action |
|---|---------|-------|--------|
| 1 | No HTTP server timeouts and no per-request context deadline. With a single SQLite connection, one stalled query blocks the whole app. | `service/main.go` | Set `http.Server{ReadHeaderTimeout, ReadTimeout, WriteTimeout, IdleTimeout}`; add a context-deadline middleware (~30s) on `/api` routes. |
| 2 | Request bodies are unbounded — CSV import and JSON bodies can OOM the process. | `service/handlers/upload_csv.go`, JSON handlers | Wrap with `http.MaxBytesReader` (e.g. 1 MB JSON / 10 MB CSV); return 400/413. |
| 3 | Fire-and-forget TTS/audio goroutines swallow errors silently. | `handlers/words.go:130`, `handlers/upload_csv.go:163` | Capture and log errors with word-ID context. |
| 4 | Plain `log.Printf` everywhere — hard to parse/aggregate. | backend-wide | Migrate to stdlib `log/slog` (no new dependency). |

### P2 — Frontend maintainability _(largest structural debt)_

| # | Finding | Where | Action |
|---|---------|-------|--------|
| 5 | No module system — all JS is global scope. `vocab.js` is 1,849 lines, `train.js` 1,466. | `service/frontend/*.js` | Adopt native ES modules (`<script type="module">`). Works with existing `go:embed`, no build step, CSP-compatible. |
| 6 | **Test-drift risk:** unit tests inline *copies* of production functions (~200+ duplicated lines across `app.test.js`, `train.test.js`, `vocab.test.js`). | `service/frontend/*.test.js` | Once ES modules exist, import the real functions; update the CLAUDE.md "inline the function" rule. |
| 7 | Desktop/mobile UI duplicated — every train-page filter exists twice (`.mode-btn` vs `.overlay-mode-btn`, tier pills vs overlay tier buttons), hand-synced in JS. | `train.html`, `train.js` | Render filters once from a shared config; drive both layouts from one state. |
| 8 | `i18n.js` is an 852-line monolith loaded on every page. | `service/frontend/i18n.js` | Consider per-page slices or lazy load (lower priority). |
| 9 | Inconsistent error handling — many `.catch(() => {})` silently swallow failures. | frontend-wide | Introduce a shared toast/notice; surface secondary-API failures. |

### P3 — Backend structure & efficiency

| # | Finding | Where | Action |
|---|---------|-------|--------|
| 10 | User settings loaded from DB twice per quiz round (Next + Answer). | `handlers/quiz.go` | Load once / cache per request. (See `.scratch/architecture-deepening/02`.) |
| 11 | Session validation does a `sessions_invalidated_at` DB lookup on every request. | `handlers/auth.go` | Short-TTL in-memory cache, invalidated on password change. |
| 12 | Daily new-word cap state lives in handler memory under a mutex — fine single-host, but an undocumented deployment constraint. | `handlers/quiz.go` | Record as an ADR (single-instance assumption). |
| 13 | `quiz.go` (884) and `auth.go` (738) are growing; `Store` has 117+ methods. | `handlers/`, `db/` | Already tracked in `.scratch/architecture-deepening/07` (split Store) — defer to that effort. |

### P4 — Test coverage gaps

| # | Finding | Action |
|---|---------|--------|
| 14 | E2E covers only auth/vocab/quiz/settings; no stats, pinyin, mnemonics, or mobile-UI specs. | Add Playwright specs for the missing pages and the mobile overlay UI. |
| 15 | No JS unit tests for `settings.js`, `pinyin.js`, `mnemonics.js`, `index.js`. | Add tests for their pure helpers. |
| 16 | No coverage reporting in CI. | Add `go test -cover` and vitest coverage to `.github/workflows/test.yml` (report-only, no gate yet). |

### Deliberate non-goals
- **`SetMaxOpenConns(1)`** stays — intentional per CLAUDE.md. A read-pool/write-pool split
  is a future scale lever, not a current need.
- **API versioning** — overkill for a self-hosted single-binary app.
- **SM-2 parameters** — frozen per ADR-0002; off-limits.

---

## Suggested sequencing

1. **Phase 1 (P1):** server/request timeouts, body-size limits, TTS error logging, slog.
   Small, isolated, test-driven (failing handler tests first per CLAUDE.md).
2. **Phase 2 (P2 #5–6):** ES-module conversion, then switch tests to import real functions.
   Larger, multi-session; needs sign-off (touches every page + the testing convention).
3. **Phase 3 (P3/P4):** settings/session caching, UI dedup, coverage specs and CI reporting.

## Verification (every change)
- `cd service && go test ./... -count=1`
- `npm test` (if JS changed)
- `make test-e2e` (if a user-visible flow changed)
- Manual: oversized request returns 413/400 (not OOM); a slow request times out cleanly
  rather than hanging the single DB connection.

> Many P3 items overlap with the existing `.scratch/architecture-deepening/` issues
> (Store split, SM-2 encapsulation, tier-classification single source, settings
> projection methods). Cross-reference rather than duplicate when filing follow-ups.
