# 词汇训练 · Vocabulary Trainer

A self-hosted Chinese–English vocabulary trainer with spaced repetition (SM-2).

## Features

- Add vocabulary with Chinese characters, pinyin, and one or more English translations
- N:N word relationships — the same English or Chinese word can be shared across entries
- Three quiz modes chosen at random: English → Chinese, Chinese → English, Chinese + Pinyin → English
- [SM-2 spaced repetition](https://www.supermemo.com/en/blog/application-of-a-computer-to-improve-the-results-obtained-in-working-with-the-super-memo-method) — words you get wrong appear more often; correct answers are scheduled further into the future
- Full vocabulary management: add, edit, delete, search, paginate
- Single-user, no login required
- SQLite database stored on the host filesystem
- Runs entirely in Docker; static frontend embedded in the Go binary

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
| `make build` | Build the Docker image |
| `make run` | Start the app in the background |
| `make stop` | Stop the running container |
| `make logs` | Tail container logs |
| `make dev` | Run locally without Docker (requires Go 1.22+) |
| `make tidy` | Tidy Go module dependencies |
| `make clean` | Stop containers and remove build artifacts |

## Running without Docker

Requires Go 1.22 or later.

```bash
make dev
```

The server listens on `:8080` and stores the database at `data/vocab.db`.

## Project structure

```
vocabulary_trainer/
├── main.go              # Server entry point, router, embedded static files
├── db/
│   ├── schema.sql       # SQLite schema (auto-applied on startup)
│   └── db.go            # Data access layer (Store)
├── handlers/
│   ├── quiz.go          # GET /api/quiz/next, POST /api/quiz/answer
│   └── words.go         # CRUD /api/words
├── models/models.go     # Shared structs and mode constants
├── sm2/sm2.go           # SM-2 algorithm and answer checking
└── frontend/
    ├── index.html       # Training page
    ├── vocab.html       # Vocabulary management page
    ├── app.js           # Shared fetch utilities
    ├── train.js         # Training page logic
    └── vocab.js         # Vocabulary management logic
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

## License

[MIT](LICENSE)
