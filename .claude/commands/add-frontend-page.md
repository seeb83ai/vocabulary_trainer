Add a new frontend page to vocabulary_trainer following the project conventions.

Work through these steps **in order**:

1. **HTML file** — Create `service/frontend/<name>.html`:
   - Copy the `<head>` block and `<nav>` block verbatim from `service/frontend/stats.html` as a starting point
   - Update the nav link for this page to the active/current state
   - Include `app.js` and `i18n.js` (already in the template); add your page's JS file at the bottom

2. **JS file** — Create `service/frontend/<name>.js` with the page logic. Follow the state-machine pattern used in `train.js` and `pinyin.js` for quiz-like flows, or the simpler CRUD pattern in `vocab.js` for data management pages.

3. **Route** — Add a route in `service/main.go`:
   ```go
   r.Get("/<name>", func(w http.ResponseWriter, r *http.Request) {
       serveFileFromFS(w, r, sub, "<name>.html")
   })
   ```

4. **Nav links** — Add a `<a href="/<name>">Page Name</a>` nav entry to **every** existing HTML page in `service/frontend/` so navigation stays consistent.

5. **CLAUDE.md file map** — Add both new files to the File map table in `CLAUDE.md`.

6. **JS tests (if applicable)** — If the JS file contains pure utility functions (no DOM, no fetch), add a `service/frontend/<name>.test.js` and inline the function under test (do not import from source).

7. **Verify** — Run `cd service && go test ./... -count=1` (route registered) and `npm test` (JS tests if added).

**Note:** The `//go:embed frontend` directive in `main.go` picks up all new files under `service/frontend/` automatically. There is no separate build step.
