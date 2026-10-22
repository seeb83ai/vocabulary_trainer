# Configuration

All configuration is read from environment variables (or a `.env` file). Copy `.env.example` to `.env` to start. Every optional feature stays off until its variables are set.

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP port |
| `DB_PATH` | `data/vocab.db` | SQLite database file |
| `APP_ENV` | `production` | `dev` relaxes the security defaults (see below) |
| `SESSION_SECRET` | – | Session cookie key, required in production (`openssl rand -hex 32`) |
| `APP_URL` | `http://localhost:$PORT` | Public URL, used in verification emails |
| `MAX_NEW_WORDS` | `5` | Daily new-word cap for new accounts |
| `AUDIO_DIR` | next to the DB | Cache for generated TTS MP3 files |
| `PINYIN_AUDIO_DIR` / `PINYIN_AUDIO_DIRS` | `data/pinyin-audio` | Pinyin listening audio (`DIRS` takes extra `:`-separated directories) |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM` | – | Email for account verification |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `USER_EMAIL`, `USER_PASSWORD` | – | Seed accounts created by the first migration |
| `RATE_LIMIT_*`, `MAX_BODY_BYTES`, `CSV_MAX_*` | see below | [Production hardening](#production-hardening) |
| `DEEPL_API_KEY`, `DEEPL_TARGET_LANGUAGE`, `DEEPL_API_BASE_URL` | – | [DeepL translation](#auto-translate-free-dictionary-lookup--deepl) |
| `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `LOCAL_LLM_*` | – | [LLM features](#llm-scene-and-sentence-generation) |
| `GITHUB_TOKEN`, `GITHUB_ISSUE_*`, `GITHUB_ASSETS_BRANCH`, `GITHUB_API_BASE_URL` | – | [In-app issue reporting](#in-app-issue-reporting-github) |
| `HIBP_API_URL` | `https://api.pwnedpasswords.com/range/` | [Leaked-password check](#leaked-passwords-have-i-been-pwned); `off` turns it off |
| `IMPRESSUM_NAME`, `IMPRESSUM_ADDRESS`, `IMPRESSUM_EMAIL`, `IMPRESSUM_PHONE` | – | Legal notice at `/impressum` |
| `RSYNC_DEST` | – | Target of `make release` |

## Accounts and authentication

Every page and API endpoint, except the landing page, the demo quiz and login/registration, requires a session. The app redirects unauthenticated page requests to `/login`. Unauthenticated API requests receive `401 Unauthorized`. Sessions expire after 24 hours.

Users register with an email address. When SMTP is configured, the app sends a verification email with a link to `APP_URL`. The first migration can create an admin and a regular account from `ADMIN_EMAIL`/`ADMIN_PASSWORD` and `USER_EMAIL`/`USER_PASSWORD`.

### Leaked passwords (Have I Been Pwned)

The app does not accept a leaked password when you create an account or change your password. It checks the password against the [Have I Been Pwned](https://haveibeenpwned.com/Passwords) Pwned Passwords list. Only the first 5 characters of the password's SHA-1 hash leave the device (k-anonymity), with padding turned on.

- The browser checks first and shows the error at once.
- The server checks again in `/api/register` and `/api/change-password` and returns `400`. So a client that skips the browser check cannot set a leaked password.
- When you log in with a leaked password, the login is successful, but a warning shows. *Change password* opens the password form in **Settings**. *Continue* goes to training.
- When HIBP does not answer in 2 seconds or returns an error, the check is skipped and the password is accepted. The server writes a log line.

```bash
HIBP_API_URL=https://api.pwnedpasswords.com/range/  # default; the server check calls it
HIBP_API_URL=off                                    # no server check, for example without internet access
```

`HIBP_API_URL` changes only the server check. The browser always calls `api.pwnedpasswords.com`, which the `Content-Security-Policy` allows in `connect-src`.

## Production hardening

For any deployment that is not your local dev box, set these values:

```bash
APP_ENV=production            # default; refuses startup if SESSION_SECRET is missing
SESSION_SECRET=<64 hex chars>  # `openssl rand -hex 32`
```

`APP_ENV=dev` is the explicit opt-out for local development. In this mode, the app tolerates a missing `SESSION_SECRET` and regenerates a random key on each restart. It also lets `/api/register` auto-verify accounts when SMTP is not configured, and it omits the `Secure` flag on session cookies, so they work over plain HTTP. In production, which is the default, the app marks session cookies `Secure`, so you must serve the app over HTTPS. Terminate TLS at the reverse proxy.

**Never set `APP_ENV=dev` on a public deployment.** In dev mode, anyone can register without owning the email address, and session cookies travel over unencrypted HTTP.

Other security-relevant settings:

```bash
RATE_LIMIT_AUTH_PER_MIN=10        # IP-budget for /api/login, /api/register, /api/verify-email
RATE_LIMIT_USER_PER_MIN=300       # per-user budget for all other API traffic
RATE_LIMIT_EXPENSIVE_PER_MIN=20   # budget for /api/translate, /api/change-password, LLM scene and sentence generation
RATE_LIMIT_GITHUB_ISSUE_PER_MIN=5 # budget for issue reports, enforced per user AND per IP
CSV_MAX_UPLOAD_MB=8               # max CSV upload body size; oversized uploads are rejected (413)
CSV_MAX_ROWS=5000                 # max data rows per CSV upload; over-cap uploads are rejected (400)
```

The CSV import endpoint (`POST /api/words/upload-csv`) also uses the expensive-request rate limiter. Its per-row text-to-speech generation runs in a bounded background worker pool, so a large import cannot exhaust resources.

The app has a built-in lockout for failed logins: five wrong passwords lock the account for 15 minutes. The next successful login clears the lockout.

The app caps JSON request bodies on `/api` routes, at 1 MiB by default, so an unbounded upload cannot exhaust memory. Oversized bodies get a `400` response.

```bash
MAX_BODY_BYTES=1048576   # max /api request body in bytes (default 1 MiB)
```

The HTTP server runs with read, write, and idle timeouts, so a slow or stalled client cannot tie up the single SQLite connection. It also shuts down gracefully on `SIGINT` and `SIGTERM`, draining in-flight requests for up to 30 seconds before it exits. This lets systemd auto-restarts (see [Self-hosting](self-hosting.md)) drop fewer in-flight requests.

The web client has its own timeouts. A request stops after 15 seconds. Slow calls (DeepL translation, LLM scene and sentence generation, CSV upload, subword backfill and issue reports) stop after 60 seconds, the server's write timeout. When a request stops, the page shows "The server did not answer. Try again." and you can try again. A button stays disabled while its request runs, so a second tap does not send the request again. When you change a filter or a search before the last result arrives, the page shows only the newest result.

The app sets a strict `Content-Security-Policy` header on every response, along with `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, and `Permissions-Policy: geolocation=(), microphone=(), camera=()`. Configure HTTPS at the reverse proxy — see `deploy/nginx.conf`, which also sets `Strict-Transport-Security`.

Always deploy behind a reverse proxy, for example nginx, that sets `X-Real-IP` to the real client address. Rate limiting reads the client IP from that header. Do not expose the binary directly to the internet. The app deliberately never trusts the spoofable `X-Forwarded-For` header, so a directly-exposed binary would rate-limit on `RemoteAddr` only.

## Auto-translate (free dictionary lookup + DeepL)

The **Translate** button in the Add/Edit Word form is always available — it detects direction automatically, based on which fields are filled:

- **Chinese filled, translation empty** → the app first tries a free, ungated exact-match lookup against the imported CC-CEDICT/HanDeDict data (see [Bilingual dictionary import](data-import.md#bilingual-dictionary-import-cc-cedict--handedict)); if nothing matches, and `DEEPL_API_KEY` is set (or the user has a personal DeepL key), it falls back to DeepL. The backend uses DeepL's `custom_instructions` to request up to 3 distinct meanings, and each meaning fills a separate translation field automatically. Either source generates pinyin.
- **Translation filled, Chinese empty** → DeepL only (no local reverse-lookup source), translates to Chinese and generates pinyin. Requires `DEEPL_API_KEY` (or a personal key).
- **Both filled** → the app generates pinyin only.

Set `DEEPL_API_KEY` in your `.env` file to enable the DeepL fallback:

```bash
DEEPL_API_KEY=your-deepl-api-key
DEEPL_TARGET_LANGUAGE=de   # any DeepL language code; default: en
DEEPL_API_BASE_URL=        # override the DeepL API base (tests); default https://api(-free).deepl.com/v2/translate
```

The app supports both free-tier (`:fx`) and pro API keys automatically. It generates pinyin server-side, using [go-pinyin](https://github.com/mozillazg/go-pinyin). The API key never reaches the browser. The backend proxies all DeepL calls.

## In-app issue reporting (GitHub)

When you configure this feature, **Report an issue** appears in the sidebar and a **Report** tab appears in the phone tab bar on every authenticated page. The report opens as a centred dialog on wide screens and as a bottom sheet on phones. The app hides the dialog while it takes the screenshot. When a user clicks the button, the app captures the current page: the URL, a screenshot, and non-sensitive client context (user agent, viewport, locale, timestamp). The user picks a type — **bug**, **idea**, **question**, or **misc** — and writes a title and description. On submit, the app creates a GitHub issue server-side.

If the app showed an error in the last 30 minutes, the dialog also shows **Add the last error**. This option is on by default. The dialog shows the error text, which the user cannot change: the message, the failed API call (method, path without the query string, status), the page and the time. For a script error, it shows the first lines of the stack. The browser keeps only the last error, in session storage for that tab. The issue shows the error as a folded *Last error* block.

```bash
GITHUB_TOKEN=github_pat_...      # fine-grained PAT; required to enable the feature
GITHUB_ISSUE_REPO=owner/repo     # target repository; required to enable the feature
GITHUB_ISSUE_LABELS=from-app     # comma-separated labels applied to created issues (default: from-app)
GITHUB_ASSETS_BRANCH=issue-assets # branch screenshots are uploaded to (default: issue-assets)
GITHUB_API_BASE_URL=             # override the GitHub API base (tests/GitHub Enterprise); default https://api.github.com
GITHUB_ISSUE_MAX_BODY_MB=6       # max request body for issue submission (screenshots are several MB)
```

This feature is optional. If `GITHUB_TOKEN` or `GITHUB_ISSUE_REPO` is unset, the report button stays hidden, and `POST /api/github/issues` returns `503`. Any authenticated user can submit a report. The app rate-limits submissions **per user and per IP**, using `RATE_LIMIT_GITHUB_ISSUE_PER_MIN`, with a default of 5 per minute.

The token must be a fine-grained PAT, scoped to the single target repository, with **Issues: write** and **Contents: write** permissions. The token never reaches the browser. GitHub's Issues API cannot attach images, so the app uploads screenshots through the Contents API to `GITHUB_ASSETS_BRANCH`, which it creates automatically from the default branch if the branch is missing. The app embeds these screenshots in the issue body by URL. They accumulate as blobs on that branch, never on the default branch, and you can prune them periodically. Each report carries a random UUID embedded in the issue body. The app records the UUID-to-user mapping only in the private audit log, so no email address or internal account ID appears in the, potentially public, issue.

## LLM scene and sentence generation

The Hanzi Movie Method scene builder can call an LLM to generate mnemonic scenes automatically. The same configuration enables **Generate sentences** in Vocabulary. Set one of the following provider configurations to enable the **Generate scene** button:

**Cloud providers** (one key is enough):

```bash
OPENAI_API_KEY=sk-...
# or
ANTHROPIC_API_KEY=sk-ant-...
# or
GEMINI_API_KEY=AI...
```

**Local / private model** (this takes priority over cloud keys when set):

```bash
LOCAL_LLM_URL=http://localhost:11434   # base URL of your local server
LOCAL_LLM_MODEL=llama3.1               # model name as known to that server
LOCAL_LLM_API_KEY=                     # optional bearer token (e.g. LM Studio)
```

You must set both `LOCAL_LLM_URL` and `LOCAL_LLM_MODEL` to activate the local provider. The server must expose an OpenAI-compatible chat completions endpoint at `POST /v1/chat/completions`. This works with [Ollama](https://ollama.com), [LM Studio](https://lmstudio.ai), [LocalAI](https://localai.io), vLLM, and any other OpenAI-compatible server.

When you configure a local model, it takes precedence over any cloud API keys that are also present. If the local server is unreachable, or returns an error when the app requests a scene, the app tries the next configured provider automatically. No restart is required.

## Text-to-speech (TTS)

The app generates audio with the Microsoft Edge neural TTS WebSocket API, using the `zh-CN-XiaoxiaoNeural` voice. This is implemented directly in Go, with no Python dependency or API key required. The app caches MP3 files in `AUDIO_DIR` (default: `data/audio/`) and serves them from the Go server.

TTS is always enabled. Set `AUDIO_DIR` to control where the app stores cached MP3 files:

```bash
AUDIO_DIR=/data/audio  # default when using Docker
```
