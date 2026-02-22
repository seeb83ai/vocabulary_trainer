# 词汇训练 · Vocabulary Trainer

A self-hosted Chinese–English vocabulary trainer with spaced repetition (SM-2).

## Screenshots

Training — question 
![Training question](images/chinese_train.png)

Training — answer
![Training answer](images/chinese_train_answer.png)

Vocabulary management |
![Vocabulary management](images/chinese_vocabulary.png)

## Features

- Add vocabulary with Chinese characters, pinyin, and one or more English translations
- N:N word relationships — the same English or Chinese word can be shared across entries
- Three quiz modes chosen at random: English → Chinese, Chinese → English, Chinese + Pinyin → English
- [SM-2 spaced repetition](https://www.supermemo.com/en/blog/application-of-a-computer-to-improve-the-results-obtained-in-working-with-the-super-memo-method) — words you get wrong appear more often; correct answers are scheduled further into the future
- Flexible answer matching: parenthesised segments are optional (`(das) Essen` accepts `Essen`); slash-separated alternatives are each valid (`Essen / Gericht` accepts `Essen` or `Gericht`)
- On a wrong answer: see what you typed alongside the correct Chinese + pinyin + translations, and optionally add your answer as an accepted translation with one click
- 🔊 Read-aloud button on every Chinese word — plays a cached MP3 (Microsoft Edge neural TTS via `edge-tts`), falls back silently to the browser's Web Speech API
- Vocabulary management: add, edit, delete, search, paginate; SM-2 progress shown per word
- Bulk import from a structured text file (see `cmd/import`)
- Single-user, no login required
- SQLite database stored on the host filesystem
- Runs in Docker or natively; static frontend is embedded in the Go binary
- Deploy to Raspberry Pi with `make release` (cross-compiles for `linux/arm64`, rsyncs via SSH)

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

The SQLite database is stored in `./data/vocab.db` on your host.

## Makefile targets

| Target | Description |
|---|---|
| Target | Description |
|---|---|
| `make build` | Build the Docker image |
| `make run` | Start the app in the background |
| `make stop` | Stop the running container |
| `make logs` | Tail container logs |
| `make dev` | Run locally without Docker (requires Go 1.24+) |
| `make tidy` | Tidy Go module dependencies |
| `make import` | Import vocabulary from a text file (see below) |
| `make tts-setup` | Create Python venv and install `edge-tts` (run once, host only) |
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
go run ./cmd/import -db data/vocab.db -file voc.txt -dry-run
```

Duplicate detection prevents re-inserting entries where both the Chinese text/pinyin and the English translation already exist.

## Deploy to Raspberry Pi

Copy `.env.example` to `.env` and set `RSYNC_DEST`:

```bash
cp .env.example .env
# edit: RSYNC_DEST=pi@raspberrypi.local:/opt/vocab-trainer

make release
```

This cross-compiles for `linux/arm64` and rsyncs the binary plus `deploy/nginx.conf` and `deploy/vocab-trainer.service` to the Pi. Follow the printed instructions to install the systemd service (auto-restarts when the binary is updated) and the nginx reverse proxy.

> If your Pi runs a 32-bit OS, change `GOARCH=arm64` to `GOARCH=arm GOARM=7` in the Makefile.

## Text-to-speech (TTS)

Audio is generated using [edge-tts](https://github.com/rany2/edge-tts) (Microsoft Edge's free neural TTS, no API key required) with the `zh-CN-XiaoxiaoNeural` voice. MP3 files are cached in `data/audio/` and served by the Go server.

**Docker** (`make run`): TTS is included in the image automatically — no setup needed.

**Native / Raspberry Pi**: run once to create the venv:

```bash
make tts-setup
```

Then set the env vars before starting the server (or add them to `.env`):

```bash
export TTS_SCRIPT=$(pwd)/cmd/tts/generate.py
export VENV_PYTHON=$(pwd)/tts-venv/bin/python3
make dev
```

If `TTS_SCRIPT` is not set, TTS is disabled and the play button falls back to the browser's Web Speech API.

## Running without Docker

Requires Go 1.24 or later.

```bash
make dev
```

The server listens on `:8080` and stores the database at `data/vocab.db`.

## Project structure

```
vocabulary_trainer/
├── main.go                  # Server entry point, router, embedded static files
├── db/
│   ├── schema.sql           # SQLite schema (auto-applied on startup)
│   └── db.go                # Data access layer (Store)
├── handlers/
│   ├── quiz.go              # GET /api/quiz/next, POST /api/quiz/answer, GET /api/quiz/stats
│   ├── words.go             # CRUD /api/words + POST /api/words/{id}/translations
│   └── audio.go             # GET /api/audio/{id} — serve/generate cached MP3
├── models/models.go         # Shared structs and mode constants
├── sm2/sm2.go               # SM-2 algorithm, answer checking, variant expansion
├── cmd/import/main.go       # Standalone vocabulary import tool
├── cmd/tts/generate.py      # edge-tts wrapper — called by the server to generate MP3s
├── cmd/tts/requirements.txt # Python dependencies
├── deploy/
│   ├── nginx.conf           # Sample nginx reverse-proxy config
│   └── vocab-trainer.service # systemd unit (auto-restart on binary change)
└── frontend/
    ├── index.html           # Training page
    ├── vocab.html           # Vocabulary management page
    ├── app.js               # Shared fetch utilities and DOM helpers
    ├── train.js             # Training page logic
    └── vocab.js             # Vocabulary management logic
```

## API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/quiz/next` | Get the next card to study |
| `POST` | `/api/quiz/answer` | Submit an answer |
| `GET` | `/api/quiz/stats` | Get due-today and total card counts |
| `GET` | `/api/words` | List words (`q`, `page`, `per_page` query params) |
| `POST` | `/api/words` | Create a vocabulary entry |
| `GET` | `/api/words/{id}` | Get a single word with translations |
| `PUT` | `/api/words/{id}` | Update a word |
| `DELETE` | `/api/words/{id}` | Delete a word |
| `POST` | `/api/words/{id}/translations` | Add a single English translation to an existing word |
| `GET` | `/api/audio/{id}` | Serve cached MP3 for a Chinese word (generated on demand) |

## License

[MIT](LICENSE)
