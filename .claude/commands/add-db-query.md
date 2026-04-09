Add a new database query / Store method to vocabulary_trainer following the project conventions.

Work through these steps **in order**:

1. **Store method** — Add the method to `service/db/db.go`. All SQL lives here exclusively. Follow the existing style:
   - Datetime columns must be scanned as `string` and parsed with `parseDateTime()` — never scan directly into `time.Time`
   - Collect all rows and call `rows.Close()` **before** issuing any follow-up query in the same function (SQLite WAL + `MaxOpenConns(1)` will deadlock otherwise)
   - Use named parameters (`:param`) for inserts/updates to match the existing patterns

2. **Schema change (if needed)** — If the query requires new tables or columns, append a new `migration` entry to the `migrations` slice in `service/db/migrate.go`:
   - Increment the version number (one past the current highest)
   - Use `CREATE TABLE IF NOT EXISTS` and `ALTER TABLE ... ADD COLUMN` with a duplicate-column guard for idempotency
   - Dropping columns is allowed via `ALTER TABLE ... DROP COLUMN` with an existence guard
   - Never rename or drop tables

3. **Unit test** — Add a test in `service/db/db_test.go`:
   - Use `db.Open(":memory:")` — never touch `data/vocab.db`
   - Use only the standard `testing` package (no testify)
   - Test both the happy path and relevant error/edge cases

4. **Verify** — Run `cd service && go test ./... -count=1` and confirm all tests pass.
