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

### Go tests
- Use the **standard library only** (`testing` package). Do not add testify or any other assertion library.
- Use **in-memory SQLite** (`db.Open(":memory:")`) for all DB tests — never touch `data/vocab.db`.
- When you change a function, update or add tests in the same package (`_test.go` alongside the source file).
- When you add or change an HTTP endpoint, update `handlers/handlers_test.go`.
- Run `go test ./... -count=1` before considering a task done.

### JS tests (Vitest)
- Add or update tests **only for pure/utility functions** (e.g. `escHtml`, `renderProgress`, `buildFormPayload`,
  `normalize`, answer-checking helpers). Skip DOM event handlers and async fetch flows.
- Test files live in `frontend/` as `*.test.js`.
- Run `npm test` to verify.

## README

Update `README.md` whenever:
- A user-visible behaviour changes (new quiz rule, new UI element, new API endpoint).
- A new Makefile target or CLI flag is added.
- The deployment or configuration process changes.

## Code style

- **No extra abstractions.** Don't add interfaces, wrapper types, middleware layers, or utility helpers
  unless they are used in at least two places.
- Match the style of the surrounding code exactly (package layout, error handling pattern, SQL style).
- SQL queries stay in `db/db.go` — no SQL anywhere else.
- All datetime columns are scanned as `string` and parsed with `parseDateTime()` — never scan directly into `time.Time`.
- `db.SetMaxOpenConns(1)` is intentional (SQLite WAL). Collect all rows and call `rows.Close()` **before**
  issuing any follow-up query in the same function to avoid deadlocks.
- Do not add docstrings or comments to code you didn't change.

## Data invariants

- Every zh word **must have at least one English translation**. `CreateWord` and `UpdateWord` enforce this
  at the handler layer. Do not relax this constraint.
- SM-2 progress rows are initialised for every word (zh and en) at creation time via `initSM2`.
  Quiz logic only reads/writes progress for zh words.

## Schema changes

The schema is managed by a version-based migration system in `db/migrate.go`.
A `schema_version` table tracks the current version. Each migration has a version number,
optional SQL, and an optional Go function. Migrations run in order on startup.

To add a schema change, append a new `migration` entry to the `migrations` slice in
`db/migrate.go` with the next version number. Use `CREATE ... IF NOT EXISTS` and
`ALTER TABLE ... ADD COLUMN` with duplicate-column guards for idempotency.
Never rename or drop columns/tables.

## Off-limits — do not change without explicit instruction

- **SM-2 algorithm parameters:** `QualityCorrect = 4`, `QualityWrong = 0`, and the EF formula in `sm2.Update`.
  These are calibrated values — don't adjust them speculatively.

## Key architecture decisions

- SM-2 progress is always tracked on the **zh word** (canonical unit). `word_id` in quiz responses is always the zh word ID.
- `GetNextCard` must filter `WHERE w.language = 'zh'` — EN words must never be returned as quiz prompts.
- Answer normalisation lives in `sm2/sm2.go` (`normalize`, `expandVariants`, `CheckAnswer`).
  Rules applied in order: lowercase + trim whitespace → strip trailing sentence punctuation (`。.！!？?`) →
  strip optional parenthesised segments → split on `/` for alternatives.
- Static frontend files are embedded in the binary via `//go:embed frontend`. No separate build step.
- The import tools (`cmd/import`, `cmd/import-hsk`) call `db.Migrate()` for schema setup and can run
  independently of the main server.

## File map

| Path | Purpose |
|---|---|
| `main.go` | Router setup, embed directive, `DB_PATH` env var |
| `db/migrate.go` | Version-based schema migrations (`Migrate()`, `migrations` slice) |
| `db/db.go` | All SQL — Store methods, `parseDateTime`, `upsertWord`, `initSM2` |
| `handlers/words.go` | CRUD + `AddTranslation` handler, shared `writeJSON`/`writeError`/`parseID` |
| `handlers/quiz.go` | `Next`, `Answer`, `Stats` handlers |
| `models/models.go` | All shared structs and mode constants |
| `sm2/sm2.go` | SM-2 algorithm, `CheckAnswer`, `expandVariants`, `normalize` |
| `cmd/import/main.go` | Standalone vocabulary import tool |
| `frontend/app.js` | `apiFetch`, `escHtml`, DOM helpers (`$`, `show`, `hide`, `setText`) |
| `frontend/train.js` | Training page state machine |
| `frontend/vocab.js` | Vocabulary management logic |
| `deploy/nginx.conf` | Sample nginx reverse-proxy config |
| `deploy/vocab-trainer.service` | systemd unit (auto-restarts on binary change via `WatchPaths`) |
| `.github/workflows/test.yml` | CI: runs Go + JS tests on every push/PR |
