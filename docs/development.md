# Development

The backend is Go (chi router, SQLite), the frontend is vanilla JS embedded in the binary with `//go:embed`. There is no frontend build step.

Contributors and coding agents follow the rules in [CLAUDE.md](../CLAUDE.md): plan first, test first, and the full file map.

## Setup

Requirements: Go 1.25+, Node.js (for the JS and E2E tests).

```bash
npm install
npx playwright install chromium   # once, for the E2E tests
make dev                           # http://localhost:8080, database in data/vocab.db
```

For local development, set `APP_ENV=dev` (see [Configuration](configuration.md#production-hardening)). New accounts are then verified without SMTP.

## Project layout

```
service/
├── main.go          # router, env configuration, embedded frontend
├── handlers/        # HTTP handlers, one file per domain
├── db/              # all SQL (Store methods), one file per domain
│   └── migrate/     # timestamp-versioned schema migrations
├── sm2/             # SM-2 algorithm, answer checking, pinyin helpers
├── models/          # shared structs and mode constants
├── tts/             # Google Translate TTS client, Microsoft Edge TTS fallback
├── cmd/             # CLI tools (imports, funnel, …)
└── frontend/        # HTML, JS and CSS pages
e2e/                 # Playwright tests
e2e-screenshots/     # README and landing-page screenshot specs
deploy/              # nginx, systemd units, backup script
docs/                # these docs and the ADRs
```

The [file map in CLAUDE.md](../CLAUDE.md#file-map) describes every file. Architecture decisions are in [docs/adr](adr/).

## Tests

| Command | What it runs |
|---|---|
| `make test-go` | Go unit and handler tests (in-memory SQLite) |
| `make test-js` | Vitest tests of the pure frontend functions |
| `make test-e2e` | Playwright browser tests against a fresh server on port 18080 |
| `make test` | Go + JS |
| `make test-all` | Go + JS + E2E |

## Screenshots

`make screenshots-readme` regenerates the images in `images/` (used by the README). It drives a real browser through Playwright against a seeded local server. The demo data (words, 190 days of history, confusions) comes from `e2e-screenshots/seed.js`.

`make screenshots-landing` regenerates the landing-page teaser images in `service/frontend/landing/teasers/`. It keeps the GitHub and terminal images, which it cannot capture. Run `make generate-landing` after you add or remove a teaser.

`make screenshots-pr FILE=e2e/<spec>.spec.js` captures screenshots for a pull request from the `captureForPR()` calls in an E2E spec.

## Make targets

| Target | Description |
|---|---|
| `make build` | Build the Docker image |
| `make run` / `make stop` / `make restart` | Start, stop or restart the container |
| `make logs` | Tail container logs |
| `make dev` | Run locally without Docker |
| `make tidy` | Tidy Go module dependencies |
| `make release` | Cross-compile for Raspberry Pi and rsync to `RSYNC_DEST` |
| `make backup` / `make restore FROM=…` | Back up or restore the database |
| `make funnel` | Print the signup → activation → retention funnel |
| `make import`, `make import-hsk`, `make import-cedict`, `make import-handedict`, `make import-hanzi`, `make import-pinyin`, `make import-frequency`, `make import-topics`, `make classify-topics`, `make refresh-library`, `make fill-translations` | Data import tools, see [Data import](data-import.md) |
| `make test`, `make test-go`, `make test-js`, `make test-e2e`, `make test-all` | Tests, see above |
| `make screenshots-readme`, `make screenshots-landing`, `make screenshots-pr` | Screenshots, see above |
| `make generate-landing` | Regenerate the landing-page teaser grid |
| `make clean` | Stop containers and remove build artifacts |

## Schema changes

Add a migration file `service/db/migrate/v<YYYYMMDDHHMMSS>_<slug>.go`. Migrations run in version order at startup. The details are in [CLAUDE.md](../CLAUDE.md#schema-changes).

## Answer buckets

The app counts attempts and errors per word and per *answer bucket*. A bucket tells what help the user had on the card before the answer:

| Bucket | When |
|---|---|
| `zh_only` | Hanzi only: no pinyin seen, no sound played |
| `pinyin` | The pinyin was visible, or blurred and tapped |
| `voice` | The sound played (play button or autoplay), with the hanzi visible |
| `voice_only` | Voice-only card (`voice_to_transl`), unless *voice unavailable* is set |
| `to_zh` | Translation → Chinese card |

*Voice → Chinese* answers (`voice_to_zh`) are not counted in a bucket.

An answer with pinyin and sound counts in `pinyin` and in `voice`.

Four *help buckets* tell how often the user chose a help themselves:

| Bucket | When |
|---|---|
| `pinyin_reveal_offered` | The pinyin hint was blurred, so the user could tap it |
| `pinyin_revealed` | The user tapped the blurred pinyin hint |
| `play_offered` | The question screen showed the play button (not on voice-only cards, where the sound is the question) |
| `play_pressed` | The user pressed play, also as a replay after autoplay. Autoplay alone does not count |

In all buckets, `attempts` counts the answers in that situation and `errors` counts the wrong answers in that situation. The use rate of a help is `pinyin_revealed.attempts / pinyin_reveal_offered.attempts` (the same for play). The buckets overlap, so never add all buckets together.

The client sends `pinyin_seen`, `audio_played` and `play_pressed`. The server ignores a flag that the card mode makes impossible. If a flag is missing, the server counts the help as used when the card offered it, and it records no help buckets.

The counts go into two tables: `answer_stats_daily` (one row per day, kept for 30 days) and `answer_stats_monthly` (one row per month, kept). Each answer deletes the user's daily rows that are older than 30 days. Drill answers and retypes after a wrong answer do not count. *Accept as correct* removes the error again. The app does not show these stats yet.
