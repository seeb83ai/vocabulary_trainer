# Security Review & Remediation Plan — REVIEW1

Scope: full security scan of `vocabulary_trainer` (Go backend, vanilla-JS frontend,
SQLite, multi-user). Areas covered: authentication/sessions, DB-layer SQL injection
and multi-tenant isolation, SSRF/outbound requests, file upload & serving, crypto/
secrets, and frontend XSS.

Branch: `claude/security-vulnerability-scan-nrpgfi` · PR: #116

---

## Part 1 — Already fixed in this PR (commit `ce4d777`)

Each fix added a failing test first, then the production change (per `CLAUDE.md` TDD rules).

| # | Severity | Issue | Fix | Test |
|---|----------|-------|-----|------|
| 1 | High | Cross-tenant IDOR: `AddTranslation` checked word existence without `user_id` — any user could attach translations to another user's word via `POST /api/words/{id}/translations` | `AND user_id = ?` added to the existence check (`db/words.go`) | `TestAddTranslation_UserIsolation` |
| 2 | High | SSRF via user-supplied `llm_local_url` — server issued outbound requests to loopback/link-local/private hosts (e.g. cloud metadata) | `ValidateExternalURL` + dial-time guard blocking non-public IPs (defeats redirects & DNS rebinding); rejected at write time in `PutAPIKeys`; operator `LOCAL_LLM_URL` env stays trusted | `TestValidateExternalURL_*`, `TestPutAPIKeys_RejectsInternalLLMURL` |
| 5 | Medium | Cross-tenant stats leak: `RecordDailyStat` aggregated `words_seen`/bucket counts across all users into the caller's row | Both aggregate queries scoped by `user_id` (`db/stats.go`) | `TestRecordDailyStat_UserIsolation` |
| 3 | Medium | Session/settings cookies lacked the `Secure` flag | `Secure` set in production, gated off when `APP_ENV=dev`; e2e setup passes `APP_ENV=dev`; README updated | `TestSessionCookie_SecureInProduction` / `_NotSecureInDev` |

Verification at time of commit: `go test ./...`, `go vet`, `npm test` (132) all green;
e2e auth/login flows pass. (One unrelated settings-toggle e2e test is pre-existing
shared-state flakiness — passes 5/5 in isolation.)

---

## Part 2 — Remaining findings & remediation plan

Ordered by recommended priority. Items needing a product/deployment decision are
flagged; the rest are ready to implement.

### A. Global component-dictionary poisoning — Medium (needs decision)

- **Where:** `PUT /api/components/{char}/translation` → `handlers/components.go:336` →
  `db/components.go:163` (`StoreComponentTranslation`).
- **Problem:** `hanzi_decomposition_translation` is a global, non-user-scoped table.
  Any authenticated user can overwrite component (radical) definitions for **all**
  users. Not stored XSS (frontend escapes via `escHtml`), but a cross-tenant
  data-integrity defect.
- **Decision required:** Is this dictionary meant to be shared/curated (admin-only)
  or personal (per-user)?
  - *Option A (admin-only, recommended):* gate the write on `GetUserRole(...) == "admin"`;
    return `403` otherwise. Smallest change, preserves the shared dictionary.
  - *Option B (per-user):* add a `user_id` column + migration and scope reads/writes.
    Larger; only if personal overrides are a real feature.
- **Plan (Option A):** failing handler test `TestUpdateComponentTranslation_NonAdminForbidden`
  → add role check in handler → `TestUpdateComponentTranslation_AdminAllowed`.

### B. `X-Forwarded-For` trusted unconditionally — Medium (needs decision)

- **Where:** `handlers/ratelimit.go:93-104` (`ClientIP`).
- **Problem:** XFF is read from any client. Since the login/register/verify IP rate
  limiter and the audit log both key on it, an attacker rotates the header to bypass
  the per-IP brute-force/enumeration budget and to poison audit logs. (Per-account
  lockout still limits credential brute-force.)
- **Decision required:** deployment topology — is the app always behind a known
  reverse proxy?
  - *Recommended:* introduce `TRUSTED_PROXIES` (CIDR list, env var read in `main.go`,
    logged on startup, default empty). Only honor XFF when `RemoteAddr` is in the
    trusted set; otherwise use `RemoteAddr`. Take the right-most untrusted hop.
- **Plan:** failing `TestClientIP_IgnoresXFFFromUntrustedSource` and
  `TestClientIP_HonorsXFFFromTrustedProxy` → implement trusted-proxy parsing →
  wire env var + README.

### C. Unbounded CSV upload (DoS) — Medium (ready)

- **Where:** `handlers/upload_csv.go:21,102-189`; per-row TTS goroutine fan-out at
  `:163,:185`; route not behind `expensiveLimit` (`main.go:227`).
- **Problem:** no `http.MaxBytesReader`, no row cap, one TTS goroutine per row with no
  concurrency limit/timeout against single-connection SQLite → lock starvation +
  goroutine/FD exhaustion by one authenticated user.
- **Plan:**
  1. Failing `TestUploadCSV_RejectsOversizedBody` and `TestUploadCSV_RejectsTooManyRows`.
  2. Wrap `r.Body` in `http.MaxBytesReader` (e.g. 8 MB, configurable via env).
  3. Cap processed rows (e.g. 5 000) and return `400` past the cap.
  4. Add `.With(expensiveLimit)` to the `/api/words/upload-csv` route.
  5. Bound TTS generation with a worker-pool semaphore; add dial/read deadlines in
     `tts.Synthesize` (`tts/tts.go:44,74`).
  6. README: document new size/row limits and env var.

### D. Reflected upstream error bodies — Medium (ready)

- **Where:** `handlers/translate.go:93,104,230`; `llm/llm.go:382`.
- **Problem:** full DeepL/LLM upstream response bodies are embedded into
  client-visible errors (and logs), leaking upstream detail — and, combined with the
  (now-fixed) SSRF, internal responses into logs.
- **Plan:** failing test asserting the client error is generic →
  replace reflected bodies with generic `writeError` messages per `CLAUDE.md`; keep
  detail in server logs only (and stop logging full upstream bodies / prompts —
  `llm/llm.go:362,170,324`, `handlers/llm.go:181,332`).

### E. Low / hardening (ready, batch as one commit)

- **Audio IDOR** (`handlers/audio.go:30-52`): cached MP3 served without ownership
  check (check only runs on generation). Add the `GetWordByID` ownership check before
  serving an existing file. Test: `TestServeAudio_OtherUserForbidden`.
- **Stateless-session revocation** (`handlers/auth.go:438`): logout only clears the
  cookie; a stolen 24h token stays valid until expiry (only password change
  invalidates). *Decision:* accept (document) or add a server-side token denylist /
  per-user token version bumped on logout. Recommend documenting the trade-off unless
  individual-session revocation is required.
- **Account-lockout DoS** (`auth.go:35-36`): a known email can be kept locked (5 fails
  / 15 min). Trade-off; consider an IP-scoped backoff alternative. Document.
- **Login user-enumeration via timing** (`auth.go:175-189`): bcrypt only runs when the
  email exists. Add a dummy `bcrypt.CompareHashAndPassword` on the `user == nil` path.
  Test asserts the no-user path still does the compare.
- **`escHtml` single-quote** (`frontend/app.js:135-141`): add `'` → `&#39;` so the
  helper is safe in single-quoted attribute contexts (defense-in-depth; no live bug —
  all attributes are currently double-quoted). Add a unit assertion in `app.test.js`.
- **Gemini key in URL** (`llm/llm.go:285`): move the key out of the query string into a
  header if supported, or ensure request URLs are never logged.

---

## Part 3 — Confirmed safe (no action)

- **SQL injection:** all dynamic SQL uses `?` placeholders; the two `fmt.Sprintf`-into-SQL
  spots (`tierFilter`/`validSortExprs`, pinyin tone columns) use only constants or
  clamped integers. `sort`/`order` are whitelisted.
- **Frontend XSS:** `escHtml` applied consistently; no `eval`/`new Function`/
  `document.write`/`insertAdjacentHTML`/`javascript:` sinks with user data.
- **Path traversal:** audio (`int`-only IDs), pinyin serving (`..` and `/` blocked).
- **Arbitrary file write / CSV-formula injection:** none (export emits JSON; writes use
  server-assigned integer filenames).
- **Word CRUD ownership, bcrypt hashing, PBKDF2/AES-GCM key sealing, SMTP header
  injection** (blocked by `net/smtp` line validation): all sound.
- **Secrets:** none committed (only the public Microsoft Edge TTS token).

---

## Suggested execution order

1. **D** (reflected errors) + **E** low-hardening batch — ready, no decisions, quick wins.
2. **C** (CSV DoS) — ready; confirm limit values.
3. **A** and **B** — implement once the admin-only vs per-user (A) and trusted-proxy
   topology (B) decisions are made.

Each item follows the repo TDD loop: failing test → minimal fix → green → update
`README.md` if user-visible / config changes, and register any new route in both
`main.go` and `newRouter()`.
