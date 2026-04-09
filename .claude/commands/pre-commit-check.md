Run through this checklist before committing any change to vocabulary_trainer.

Go through each item and confirm it passes:

1. **Go tests** — `cd service && go test ./... -count=1` completes with no failures.

2. **JS tests** — `npm test` completes with no failures (only required if JS files were changed).

3. **Route registration** — If a new HTTP endpoint was added, it is registered in **both**:
   - `service/main.go` (the live router)
   - `newRouter()` inside `service/handlers/handlers_test.go` (the test router)

4. **README updated** — If user-visible behaviour changed (new quiz rule, new UI element, new API endpoint, new Makefile target, new env var), `README.md` has been updated accordingly.

5. **SQL location** — No SQL strings exist outside `service/db/db.go`. (Quick check: `grep -r "SELECT\|INSERT\|UPDATE\|DELETE" service --include="*.go" -l`)

6. **New env var** — If a new environment variable was introduced, it is:
   - Read in `service/main.go`
   - Has a documented default value
   - Logged on startup with `log.Printf`

7. **SM-2 parameters** — `QualityCorrect = 4` and `QualityWrong = 0` in `service/sm2/sm2.go` are unchanged.

8. **Data invariants** — The constraint "every zh word must have at least one English translation" is still enforced at the handler layer in `CreateWord` and `UpdateWord`.

If all items pass, the change is ready to commit.
