# 词汇训练 · Vocabulary Trainer

A self-hosted Chinese–English vocabulary trainer with spaced repetition (SM-2).

## Features

- Add vocabulary with Chinese characters, pinyin, and one or more English translations
- N:N word relationships — the same English or Chinese word can be shared across entries
- **Four quiz modes** chosen at random or fixed by user: English → Chinese, Chinese → English, Chinese + Pinyin → English, and **Progressive** (auto-selects direction based on learning progress)
- [SM-2 spaced repetition](https://www.supermemo.com/en/blog/application-of-a-computer-to-improve-the-results-obtained-in-working-with-the-super-memo-method) — words you get wrong appear more often; correct answers are scheduled further into the future
- **Daily new-word cap** — limits how many brand-new words are introduced per day (default: 5, configurable via `MAX_NEW_WORDS`); once the cap is reached only already-seen cards are served for the rest of the day; the training page shows a "New today: X / Y" counter in the stats bar
- Flexible answer matching: parenthesised segments are optional (`(das) Essen` accepts `Essen`); slash-separated alternatives are each valid (`Essen / Gericht` accepts `Essen` or `Gericht`)
- On a wrong answer: see what you typed alongside the correct Chinese + pinyin + translations, and optionally add your answer as an accepted translation with one click
- **Training stats** — daily progress tracking: attempts, mistakes, accuracy, words known, new words learned, and best correct streak; view a Chart.js bar/line chart of the full history and a detailed table of the last 14 days on the `/stats` page
- **Word-level statistics** — real-time aggregate stats for all seen words on the `/stats` page: correctness milestones (1+/3+/5+/10+ correct), accuracy distribution (doughnut chart), avg/median/P95 of correct answers, attempts, accuracy, and ease factor; tables of the 5 hardest and 5 most-practiced words with translations; includes an info box explaining SM-2 ease factor and all metrics
- **Confusion tracking** — if your wrong answer is a valid translation of a *different* known word, it is recorded as a confusion pair (works in all quiz modes); a yellow hint box shows immediately on the result screen, and the full history is visible on the `/mismatches` page
- 🔊 Read-aloud button on every Chinese word — plays a cached MP3 (Microsoft Edge neural TTS, built into the binary), falls back silently to the browser's Web Speech API
- **Tags** — assign tags to vocabulary words (e.g. "HSK1", "food", "travel"); filter by tag on both the vocabulary list and training page (OR logic when multiple tags selected); tags are created on-the-fly via an autocomplete input and cleaned up automatically when no longer used
- **Auto-translate** — when a DeepL API key is configured, an auto-translate button appears in the Add/Edit Word form; enter Chinese to get the translation + pinyin filled in automatically, or enter the translation to get Chinese + pinyin back (pinyin generated locally via [go-pinyin](https://github.com/mozillazg/go-pinyin))
- Vocabulary management: add, edit, delete, search, paginate, sort by any column; SM-2 progress shown per word
- Due-date and correct-answer scheduling include a small random jitter to shuffle cards and avoid repetitive review patterns
- Bulk import from a structured text file (see `service/cmd/import`)
- HSK vocabulary import (HSK 1–6) fetched directly from mandarinbean.com, with automatic `hsk-N` tagging (see `service/cmd/import-hsk`)
- Optional single-user password protection (set `AUTH_USER` / `AUTH_PASSWORD` in `.env`)
- SQLite database stored on the host filesystem
- Runs in Docker or natively; static frontend is embedded in the Go binary — no Python or external tools required
- Deploy to Raspberry Pi with `make release` (cross-compiles for `linux/arm64`, rsyncs via SSH)

## Screenshots

Training — question
![Training question](images/chinese_train.png)

Training — answer
![Training answer](images/chinese_train_answer.png)

Vocabulary management |
![Vocabulary management](images/chinese_vocabulary.png)

Overview - Vocabulary Mismatches
![Training answer](images/chinese_mismatches.png)

## Quick start

**Requirements:** Docker and Docker Compose.

```bash
git clone <repo-url>
cd vocabulary_trainer
make run
```

Then open [http://localhost:8080](http://localhost:8080).

1. Go to **Vocabulary** (`/vocab`) and add some words.
2. Return to **Train** (`/`) to start a quiz session.
3. Check **Mismatches** (`/mismatches`) to review words you've confused with each other.

The SQLite database is stored in `./data/vocab.db` on your host.

## Authentication

Authentication is disabled by default. To enable it, set `AUTH_USER` and `AUTH_PASSWORD` in your `.env` file:

```bash
AUTH_USER=admin
AUTH_PASSWORD=yourpassword
```

When enabled, all pages and API endpoints require a valid session. Unauthenticated page requests are redirected to `/login`; unauthenticated API requests receive `401 Unauthorized`. Sessions expire after 24 hours. The session secret is generated randomly at startup, so all sessions are invalidated when the server restarts.

## Daily new-word cap

To avoid being overwhelmed when you have a large vocabulary list, the trainer limits how many brand-new words are introduced each day:

```bash
MAX_NEW_WORDS=5   # default; set to 0 to disable new words entirely
```

A *new word* is one that has never appeared as a quiz card before (tracked by a `first_seen_date` column in the database). Once the daily cap is reached, only cards you have already seen at least once will be served — reviews and retry cards are always available regardless of the cap. The counter resets at midnight (server-local date).

The training page stats bar shows **New today: X / Y** so you can see how many new words you have left for the day.

## Progressive mode

The **Progressive** quiz mode introduces new words gently and gradually increases difficulty based on your accuracy (correct answers ÷ total attempts):

| Condition | What happens |
|---|---|
| Brand new word (`total_attempts = 0`) | **Introduction** — shows Chinese, pinyin, and all English translations. No quiz. Choose "Got it" to start learning or "Skip" to defer 7 days. |
| **Learning phase** (`learning_new_word = true`) | Word is in the **New** bucket. Short retry intervals (minutes, not days) so you can drill it in one session. Get **3 correct in a row** to graduate. Wrong answers reset the streak. |
| `total_attempts < 3` | **EN → ZH** — not enough data yet; stay at the easiest direction |
| Accuracy < 50% | **EN → ZH** — still struggling; see English, type Chinese |
| Accuracy < 70% **or** `total_attempts < 10` | **ZH + Pinyin → EN** — making progress; see Chinese with pinyin hint, type English |
| Accuracy < 85% (and `total_attempts ≥ 10`) | **ZH → EN** — reliable; see Chinese only, type English |
| Accuracy ≥ 85% and `total_attempts ≥ 10` | **Random** — any of the three quiz directions |

**Learning phase ("New" bucket):**

When you acknowledge a new word ("Got it"), it enters the learning phase. During this phase:
- Short intervals (1–5 minutes) are used instead of day-scale SM-2 intervals
- You need **3 consecutive correct answers** to graduate
- Wrong answers reset the streak counter back to 0
- On graduation: SM-2 progress is reset to a clean baseline (accuracy starts at 100%, total_attempts = 3) and the word moves to the regular review queue with a 1-day interval

The training page and vocabulary list show a **New** tier badge for words still in the learning phase. You can filter by the "New" bucket to drill only recently introduced words.

**Accuracy tiers:**

| Tier | Criteria |
|---|---|
| **New** | `learning_new_word = true` (still in learning phase) |
| **Struggling** | `< 3 attempts` or `accuracy < 50%` |
| **Learning** | `≥ 3 attempts` and `50% ≤ accuracy < 70%` |
| **Practicing** | `≥ 10 attempts` and `70% ≤ accuracy < 85%` |
| **Mastered** | `≥ 10 attempts` and `accuracy ≥ 85%` |

**Skip vs Got it:**
- **Got it** marks the word as introduced, enters the learning phase, and makes it immediately available for quizzing (EN → ZH). Counts toward the daily new-word cap.
- **Skip** defers the word by 7 days. Does *not* count as seen — the word remains "new" and will be shown as an introduction again when it comes due.

## Auto-translate (DeepL)

Set `DEEPL_API_KEY` in your `.env` to enable the auto-translate button on the vocabulary page:

```bash
DEEPL_API_KEY=your-deepl-api-key
DEEPL_TARGET_LANGUAGE=de   # any DeepL language code; default: en
```

When enabled, an **Auto-translate** button appears in the Add/Edit Word form. It auto-detects direction based on which fields are filled:

- **Chinese filled, translation empty** → translates Chinese to the target language and generates pinyin. The backend uses DeepL's `custom_instructions` to request up to 3 distinct meanings; each meaning populates a separate translation field automatically.
- **Translation filled, Chinese empty** → translates to Chinese and generates pinyin
- **Both filled** → generates pinyin only

Both free-tier (`:fx` keys) and pro API keys are supported automatically. Pinyin is generated server-side using [go-pinyin](https://github.com/mozillazg/go-pinyin). The API key never reaches the browser — all DeepL calls are proxied through the backend.

## Makefile targets

| Target | Description |
|---|---|
| `make build` | Build the Docker image |
| `make run` | Start the app in the background |
| `make stop` | Stop the running container |
| `make logs` | Tail container logs |
| `make dev` | Run locally without Docker (requires Go 1.24+) |
| `make tidy` | Tidy Go module dependencies |
| `make import` | Import vocabulary from a text file (see below) |
| `make import-hsk` | Fetch and import HSK 1–6 vocabulary from mandarinbean.com (see below) |
| `make release` | Cross-compile for Raspberry Pi and rsync to `RSYNC_DEST` |
| `make test` | Run all Go and JS tests |
| `make clean` | Stop containers and remove build artifacts |

## Bulk import

Vocabulary can be imported from a plain-text file in the following format (3 lines per entry, blank lines ignored):

```
pinyin / 汉字
translation(s), comma-separated
rating string (ignored)
```

```bash
# Default: reads voc.txt, writes to data/vocab.db
make import

# Custom paths
make import FILE=my_vocab.txt DB=data/vocab.db

# Preview without writing
go run ./service/cmd/import -db data/vocab.db -file voc.txt -dry-run
```

Duplicate detection prevents re-inserting entries where both the Chinese text/pinyin and the English translation already exist.

## HSK vocabulary import

Fetches vocabulary directly from [mandarinbean.com](https://mandarinbean.com) and inserts it into the database. Each word is tagged `hsk-1` through `hsk-6`. If a word already exists its tag is still applied; if the exact Chinese+English pair already exists the row is skipped.

```bash
# Import all HSK levels (1-6)
make import-hsk

# Import only HSK 1 and 2
make import-hsk LEVELS=1,2

# Import with German translations (requires DEEPL_API_KEY)
DEEPL_API_KEY=your-key go run ./service/cmd/import-hsk -lang de

# Custom DB path
make import-hsk DB=/path/to/vocab.db

# Preview without writing
go run ./service/cmd/import-hsk -dry-run

# Single level, dry-run
go run ./service/cmd/import-hsk -levels 3 -dry-run
```

Flags:

| Flag | Default | Description |
|---|---|---|
| `-db` | `data/vocab.db` | Path to SQLite database |
| `-levels` | `1,2,3,4,5,6` | Comma-separated HSK levels to import |
| `-lang` | `en` | DeepL target language code (e.g. `de`, `fr`, `es`); requires `DEEPL_API_KEY` env var |
| `-dry-run` | false | Parse and check duplicates without writing |

When `-lang` is set to anything other than `en`, each English translation from the source
table is translated via the [DeepL API](https://www.deepl.com/en/products/api) before
being stored. Translations are always stored as `language='en'` rows so the existing quiz
logic works unchanged. If `DEEPL_API_KEY` is not set, the original English text is used
and a warning is printed.

## Deploy to Raspberry Pi

### Initial setup

locally copy `.env.example` to `.env` and set `RSYNC_DEST` to configure the deployment target:
(only `.env.example` will be synced with make release, not `.env`)
```bash
cp .env.example .env
# edit: RSYNC_DEST=pi@raspberrypi.local:/opt/vocab-trainer
```

run `make release` to copy all needed files

This cross-compiles for `linux/arm64` and rsyncs the binary plus `deploy/nginx.conf` and `deploy/vocab-trainer.service` to the Pi. Follow the printed instructions to install the systemd service (auto-restarts when the binary is updated) and the nginx reverse proxy.

> If your Pi runs a 32-bit OS, change `GOARCH=arm64` to `GOARCH=arm GOARM=7` in the Makefile.

Copy the .env.example file and adjust the settings

```
cp <deploy-dir>/.env.example <deploy-dir>/.env
```

Then cp or move the service files and eventually edit them to fix the path and port settings

```
sudo cp <deploy-dir>/vocab-trainer.service /etc/systemd/system/
sudo cp <deploy-dir>/vocab-trainer-watcher.service /etc/systemd/system/
sudo cp <deploy-dir>/vocab-trainer-watcher.path /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now vocab-trainer
sudo systemctl enable --now vocab-trainer-watcher.path vocab-trainer-watcher.service
sudo systemctl start --now vocab-trainer
sudo systemctl start --now vocab-trainer-watcher.path vocab-trainer-watcher.service
```

To install nginx config:

```
sudo cp <deploy-dir>/nginx.conf /etc/nginx/sites-available/vocab-trainer
sudo ln -sf /etc/nginx/sites-available/vocab-trainer /etc/nginx/sites-enabled/vocab-trainer
sudo nginx -t && sudo systemctl reload nginx
```

### Release changes

just running `make release` is good enough now to build the binary, deploy it and restart the service

## Text-to-speech (TTS)

Audio is generated using the Microsoft Edge neural TTS WebSocket API (`zh-CN-XiaoxiaoNeural` voice) — implemented directly in Go with no Python dependency or API key required. MP3 files are cached in `AUDIO_DIR` (default: `data/audio/`) and served by the Go server.

TTS is always enabled. Set `AUDIO_DIR` to control where cached MP3s are stored:

```bash
AUDIO_DIR=/data/audio  # default when using Docker
```

## Running without Docker

Requires Go 1.24 or later.

```bash
make dev
```

The server listens on `:8080` and stores the database at `data/vocab.db`.

## Project structure

```
vocabulary_trainer/
├── service/                 # All Go source and embedded frontend
│   ├── main.go              # Server entry point, router, embedded static files
│   ├── go.mod / go.sum
│   ├── db/
│   │   ├── migrate.go       # Version-based schema migrations
│   │   └── db.go            # Data access layer (Store)
│   ├── handlers/
│   │   ├── quiz.go          # GET /api/quiz/next, POST /api/quiz/answer, GET /api/quiz/stats
│   │   ├── words.go         # CRUD /api/words + POST /api/words/{id}/translations
│   │   ├── mismatches.go    # GET /api/mismatches
│   │   ├── translate.go     # POST /api/translate, GET /api/config — DeepL proxy + pinyin
│   │   └── audio.go         # GET /api/audio/{id} — serve/generate cached MP3
│   ├── models/models.go     # Shared structs and mode constants
│   ├── sm2/sm2.go           # SM-2 algorithm, answer checking, variant expansion
│   ├── tts/tts.go           # Microsoft Edge TTS WebSocket client
│   ├── cmd/import/main.go   # Standalone vocabulary import tool (text file)
│   ├── cmd/import-hsk/main.go # HSK vocabulary import from mandarinbean.com
│   └── frontend/
│       ├── index.html       # Training page
│       ├── vocab.html       # Vocabulary management page
│       ├── mismatches.html  # Confusion pairs page
│       ├── stats.html       # Training stats page
│       ├── app.js           # Shared fetch utilities and DOM helpers
│       ├── train.js         # Training page logic
│       ├── vocab.js         # Vocabulary management logic
│       ├── mismatches.js    # Confusion pairs page logic
│       └── stats.js         # Training stats page logic
├── deploy/
│   ├── nginx.conf           # Sample nginx reverse-proxy config
│   └── vocab-trainer.service # systemd unit (auto-restart on binary change)
└── Dockerfile / docker-compose.yml
```

## API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/quiz/next` | Get the next card to study (`mode`, `tags` query params) |
| `POST` | `/api/quiz/answer` | Submit an answer |
| `POST` | `/api/quiz/skip` | Skip a new word (defer due date by 7 days) |
| `POST` | `/api/quiz/acknowledge` | Mark a new word as introduced (ready for quizzing) |
| `GET` | `/api/quiz/stats` | Get due-today and total card counts (`tags` query param) |
| `GET` | `/api/quiz/daily-stats` | Get daily training stats history (attempts, mistakes, words known, new words, streak) |
| `GET` | `/api/quiz/word-stats` | Get per-word aggregate statistics: milestones, accuracy buckets, avg/median/P95, hardest & most-practiced words |
| `GET` | `/api/words` | List words (`q`, `page`, `per_page`, `sort`, `order`, `tags` query params) |
| `POST` | `/api/words` | Create a vocabulary entry |
| `GET` | `/api/words/{id}` | Get a single word with translations |
| `PUT` | `/api/words/{id}` | Update a word |
| `DELETE` | `/api/words/{id}` | Delete a word |
| `POST` | `/api/words/{id}/translations` | Add a single English translation to an existing word |
| `GET` | `/api/audio/{id}` | Serve cached MP3 for a Chinese word (generated on demand) |
| `GET` | `/api/tags` | List all tag names (alphabetically) |
| `GET` | `/api/config` | Frontend feature flags (`deepl_enabled`, etc.) |
| `POST` | `/api/translate` | Translate text via DeepL + generate pinyin (only available when `DEEPL_API_KEY` is set) |
| `GET` | `/api/mismatches` | List all recorded confusion pairs (wrong answers that matched a different known word) |

## License

[MIT](LICENSE)
