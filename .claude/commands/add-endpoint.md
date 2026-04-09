Add a new HTTP endpoint to vocabulary_trainer following the project conventions.

Work through these steps **in order**:

1. **Models** — If the endpoint needs new request/response types, add structs to `service/models/models.go`. Reuse existing structs where possible.

2. **DB layer** — Add the required Store method(s) to `service/db/db.go`. All SQL goes here and nowhere else. Remember:
   - Scan datetime columns as `string`, parse with `parseDateTime()`
   - Call `rows.Close()` before any follow-up query in the same function

3. **DB test** — Add a unit test in `service/db/db_test.go` using an in-memory SQLite DB (`db.Open(":memory:")`).

4. **Handler** — Add the handler function in the appropriate `service/handlers/*.go` file. Use the shared helpers from `words.go`:
   - `writeJSON(w, status, payload)` for success responses
   - `writeError(w, status, "message")` for errors (JSON shape: `{"error":"..."}`)
   - `parseID(r)` to extract `{id}` from the URL
   - Status codes: `400` bad/missing input · `404` not found · `500` DB/IO failure · `503` feature not configured

5. **Route — main.go** — Register the route in `service/main.go` inside the appropriate router group.

6. **Route — test router** — Register the **same** route in `newRouter()` inside `service/handlers/handlers_test.go`. Forgetting this is the most common cause of test 404s.

7. **Handler test** — Add an integration test in `service/handlers/handlers_test.go`.

8. **Verify** — Run `cd service && go test ./... -count=1` and confirm all tests pass.

9. **README** — If this endpoint is user-visible, add it to the API reference section in `README.md`.
