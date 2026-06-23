# ADR-0004: Accepted security and single-host trade-offs

## Status

Accepted

## Context

ADR-0003 moved the app from single-user self-hosted to a multi-tenant shared hosted instance. That shift forced several security and architecture decisions where the pragmatic choice carries a known, bounded downside. This ADR records those deliberate trade-offs in one place so they are not re-litigated, and so a future reader does not mistake any of them for an oversight or a bug.

Each trade-off below was verified against the current code at the time of writing.

## Decision

We accept the following four trade-offs.

### 1. Handler-enforced isolation for `hmm_scenes` and `confusion_pairs`

The `hmm_scenes` table is keyed only by `word_id` (`hmm_scenes.word_id INTEGER PRIMARY KEY REFERENCES words(id) ON DELETE CASCADE`, see `service/db/migrate/v13.go`). The `confusion_pairs` table is keyed only by `(zh_word_id, confused_with_id, mode)`, both referencing `words(id)` (see `service/db/migrate/v01.go`). Neither table carries a `user_id` column.

Isolation for these two tables is enforced at the handler layer: a request must first resolve the word via the user-scoped `GetWordByID` check before any scene or confusion-pair row is read or written. Because `words` is owned per `user_id` and the `word_id` is validated against the caller's account before touching these tables, a user cannot reach another user's scenes or confusion pairs.

This was reviewed and is **not exploitable** — it is a defense-in-depth gap only. The accepted downside is that the row-level data is not self-protecting: a new query path that reads `hmm_scenes` or `confusion_pairs` without first going through the handler-layer ownership check could leak data. We accept this rather than back-filling `user_id` columns and a migration onto two tables that are always accessed through an already-scoped word.

### 2. Stateless HMAC session tokens (no per-session revocation)

Sessions are stateless signed tokens, not server-side session records. `mintToken` produces `userID:unixNano:hmac` signed with the server HMAC secret (`service/handlers/auth.go`). There is no stored table of live sessions and therefore no per-session denylist.

Revocation is coarse-grained:
- **Logout** clears the session cookie (`MaxAge: -1`) on the client. It does not invalidate the token server-side; a copy of the cookie would still validate until it expires.
- **Password change** revokes *all* of a user's sessions at once: `sessionUserID` rejects any token minted before the user's `GetSessionsInvalidatedAt` cutoff. Nanosecond mint timestamps are used so a cookie re-minted in the same second as the password change is not accidentally invalidated.

The accepted downside: there is no way to revoke a single leaked session without forcing a password change (which logs the user out everywhere). We accept this to keep auth stateless — no session store, no per-request DB lookup for the common authenticated path — which suits a small shared instance.

### 3. Email-keyed account lockout (no IP-scoped backoff)

Brute-force protection on login is keyed by account. After `MaxFailedLogins` (5) consecutive wrong-password attempts, the account is locked for `LockoutDuration` (15 minutes); a successful login resets the counter (`service/handlers/auth.go`). The failure counter and lock are stored against `user.ID`, resolved from the submitted email.

General per-IP rate limiting still applies to the login endpoint via the in-memory token-bucket limiter (`service/handlers/ratelimit.go`, `IPKey`), but there is **no IP-scoped exponential backoff** dedicated to failed logins.

The accepted downsides:
- An attacker who knows a victim's email can deliberately lock that account (a denial-of-service against one user) by submitting wrong passwords.
- A single source IP spreading attempts across many accounts is governed only by the general rate limiter, not by a failed-login-specific IP backoff.

We accept this because account-keyed lockout is the right primitive for protecting a specific account's password, and the generic IP rate limiter already caps overall request volume from one source. Adding IP-scoped login backoff is a possible future enhancement, not a current requirement.

### 4. Single-instance assumption: in-memory daily new-word cap

The daily new-word cap is maintained in handler process memory, guarded by a mutex on `QuizHandler` (`service/handlers/quiz.go`): the struct holds `mu sync.Mutex`, `capResetDate`, and `newCapBase`, and the cap is computed from these fields under the lock. This state is per-process and is not shared across instances or persisted as the source of truth for the running cap.

The accepted assumption: the server runs as a **single instance**. Running two or more instances behind a load balancer would give each its own in-memory cap state, so a user could exceed the intended daily new-word limit by having requests land on different instances. This matches the single-host deployment model (one systemd unit, one SQLite file with `SetMaxOpenConns(1)` and WAL — see `deploy/vocab-trainer.service`).

We accept this rather than introducing a distributed/persistent cap counter, because horizontal scaling is out of scope for the current deployment.

## Consequences

- These four behaviours are **decisions, not defects**. Do not "fix" them speculatively; reopen this ADR if the deployment model changes.
- New code that reads `hmm_scenes` or `confusion_pairs` **must** first validate word ownership through the handler-layer `GetWordByID` path. Do not add direct, unscoped query paths to these tables.
- A leaked individual session can only be revoked by changing the user's password (which logs them out everywhere).
- Account lockout protects passwords but enables targeted account-level DoS via known emails; IP-scoped login backoff is deferred.
- The app assumes a single running instance. Moving to multiple instances requires revisiting the daily new-word cap (and re-checking any other in-memory handler state) before it can be done safely.
