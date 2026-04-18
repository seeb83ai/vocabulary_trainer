# CLAUDE.md — Agent instructions for vocabulary_trainer

## Project overview

A self-hosted Chinese–English vocabulary trainer (Go backend + vanilla JS frontend, SQLite, SM-2 spaced repetition).
See README.md for full feature description and API reference.

## Workflow

**Always plan before implementing.**
For any non-trivial change (new feature, multi-file edit, architectural decision), propose an approach and
wait for approval before writing code. For small isolated fixes (typo, obvious one-liner bug) you may proceed directly.

**Bugs discovered during a task:** point them out and ask before fixing. Never silently fix code outside
the scope of the current task.

## Testing rules

**Mandatory:** Every code change that adds or modifies a function, DB query, or HTTP endpoint **must** include
corresponding test additions or updates in the same commit. Do not consider a task done until new tests cover
the changed behaviour.

### Go tests
- Use the **standard library only** (`testing` package). Do not add testify or any other assertion library.
- Use **in-memory SQLite** (`db.Open(":memory:")`) for all DB tests — never touch `data/vocab.db`.
- When you change a function, update or add tests in the same package (`_test.go` alongside the source file).
- When you add or change an HTTP endpoint, update `service/handlers/handlers_test.go`.
  - Also register any new route in the `newRouter()` helper inside that file.
- Run `cd service && go test ./... -count=1` before considering a task done.

### JS tests (Vitest)
- Add or update tests **only for pure/utility functions** (e.g. `escHtml`, `renderProgress`, `buildFormPayload`,
  `normalize`, answer-checking helpers). Skip DOM event handlers and async fetch flows.
- Inline the function under test directly in the test file (do not import from source) so tests stay
  self-contained and do not depend on module bundling.
- Test files live in `service/frontend/` as `*.test.js`.
- Run `npm test` to verify.

### Pre-flight checklist

Before marking any task done:
1. `cd service && go test ./... -count=1` — no failures.
2. `npm test` — no failures (if JS was changed).
3. New route registered in **both** `service/main.go` and `newRouter()` in `handlers_test.go`.
4. `README.md` updated if user-visible behaviour changed.
5. No SQL outside `service/db/db.go`.
6. New env var? Read in `main.go`, default documented, logged with `log.Printf`.

### What must be tested
| Change type | Required test |
|---|---|
| New DB query / Store method | Unit test in `service/db/db_test.go` |
| Changed DB query | Update existing test or add regression test |
| New HTTP endpoint | Integration test in `service/handlers/handlers_test.go` + register route in `newRouter()` |
| Changed HTTP endpoint behaviour | Update or extend existing handler test |
| New pure JS utility function | Unit test in the relevant `*.test.js` file |
| Changed pure JS utility function | Update or extend existing JS test |

## README

Update `README.md` whenever:
- A user-visible behaviour changes (new quiz rule, new UI element, new API endpoint).
- A new Makefile target or CLI flag is added.
- The deployment or configuration process changes.

## Code style

- **No extra abstractions.** Don't add interfaces, wrapper types, middleware layers, or utility helpers
  unless they are used in at least two places.
- Match the style of the surrounding code exactly (package layout, error handling pattern, SQL style).
- SQL queries stay in `service/db/db.go` — no SQL anywhere else.
- All datetime columns are scanned as `string` and parsed with `parseDateTime()` — never scan directly into `time.Time`.
- `db.SetMaxOpenConns(1)` is intentional (SQLite WAL). Collect all rows and call `rows.Close()` **before**
  issuing any follow-up query in the same function to avoid deadlocks.
- Do not add docstrings or comments to code you didn't change.

## Error handling

- Use `writeError(w, status, "message")` for all error responses (JSON shape: `{"error":"..."}`).
- Status codes: `400` malformed/missing input, `404` resource not found, `500` unexpected DB/IO failure, `503` optional feature not configured.
- Never return `200 OK` with an error body.

## Data invariants

- Every zh word **must have at least one translation in any language (EN or DE)**. `CreateWord` and `UpdateWord` enforce this
  at the handler layer. A word with only DE translations and no EN is valid.
- SM-2 progress rows are initialised for every word (zh and en) at creation time via `initSM2`.
  Quiz logic only reads/writes progress for zh words.

## Schema changes

The schema is managed by a version-based migration system in `service/db/migrate.go`.
A `schema_version` table tracks the current version. Each migration has a version number,
optional SQL, and an optional Go function. Migrations run in order on startup.

To add a schema change, append a new `migration` entry to the `migrations` slice in
`service/db/migrate.go` with the next version number. Use `CREATE ... IF NOT EXISTS` and
`ALTER TABLE ... ADD COLUMN` with duplicate-column guards for idempotency.
Dropping columns is allowed via `ALTER TABLE ... DROP COLUMN` with existence guards.
Never rename or drop tables.

## Off-limits — do not change without explicit instruction

- **SM-2 algorithm parameters:** `QualityCorrect = 4`, `QualityWrong = 0`, and the EF formula in `sm2.Update`.
  These are calibrated values — don't adjust them speculatively.

## Key architecture decisions

- SM-2 progress is always tracked on the **zh word** (canonical unit). `word_id` in quiz responses is always the zh word ID.
- `GetNextCard` must filter `WHERE w.language = 'zh'` — EN words must never be returned as quiz prompts.
- Answer normalisation lives in `service/sm2/sm2.go` (`normalize`, `expandVariants`, `CheckAnswer`).
  Rules applied in order: lowercase + trim whitespace → strip trailing sentence punctuation (`。.！!？?`) →
  strip optional parenthesised segments → split on `/` for alternatives.
- Static frontend files are embedded in the binary via `//go:embed frontend` (from `service/main.go`). No separate build step.
- The import tools (`service/cmd/import`, `service/cmd/import-hsk`) call `db.Migrate()` for schema setup and can run
  independently of the main server.
- To add a new frontend page: create `service/frontend/<name>.html` (copy `<head>` + `<nav>` from `stats.html`), create `service/frontend/<name>.js`, add `r.Get("/<name>", ...)` in `main.go`. The `//go:embed frontend` directive picks up new files automatically — no build step. Add both files to the File map below.
- All configuration is env vars read in `service/main.go`. Always provide a documented default and log the effective value on startup. Optional external features (API keys, etc.) gate route registration on a nil/empty check — see the LLM/DeepL/Auth handlers for the pattern. Do not use the `flag` package in `main.go`; it is only for standalone CLI tools in `service/cmd/`.

## File map

| Path | Purpose |
|---|---|
| `service/main.go` | Router setup, embed directive, `DB_PATH` env var |
| `service/db/migrate.go` | Version-based schema migrations (`Migrate()`, `migrations` slice) |
| `service/db/db.go` | All SQL — Store methods, `parseDateTime`, `upsertWord`, `initSM2` |
| `service/handlers/words.go` | CRUD + `AddTranslation` handler, shared `writeJSON`/`writeError`/`parseID` |
| `service/handlers/quiz.go` | `Next`, `Answer`, `Stats` handlers |
| `service/models/models.go` | All shared structs and mode constants |
| `service/sm2/sm2.go` | SM-2 algorithm, `CheckAnswer`, `expandVariants`, `normalize` |
| `service/sm2/pinyin.go` | Pinyin tone mark conversion, answer parsing (`NumberedToToneMark`, `CheckPinyinAnswer`) |
| `service/db/pinyin.go` | Pinyin listening SQL — `GetNextPinyinCard`, distractors, progress, confusions |
| `service/handlers/pinyin_quiz.go` | `PinyinQuizHandler`: `Next`, `Answer`, `Stats`, `ServeAudio` |
| `service/cmd/import/main.go` | Standalone vocabulary import tool |
| `service/cmd/import-pinyin/main.go` | Import pinyin MP3 files + seed `pinyin_sounds` table |
| `service/frontend/app.js` | `apiFetch`, `escHtml`, DOM helpers (`$`, `show`, `hide`, `setText`) |
| `service/frontend/train.js` | Training page state machine |
| `service/frontend/pinyin.js` | Pinyin listening training state machine |
| `service/frontend/pinyin.html` | Pinyin listening training page |
| `service/frontend/vocab.js` | Vocabulary management logic |
| `deploy/nginx.conf` | Sample nginx reverse-proxy config |
| `deploy/vocab-trainer.service` | systemd unit (auto-restarts on binary change via `WatchPaths`) |
| `.github/workflows/test.yml` | CI: runs Go + JS tests on every push/PR |

## Large files — do not read proactively

| File | Why to skip |
|---|---|
| `dictionary.txt` | 2.5 MB hanzi dataset; only needed by `service/cmd/import-hanzi` |
| `package-lock.json` | npm lockfile; never needed for code tasks |
| `chinese_a1.txt` | Sample vocabulary import data; not needed for code tasks |
