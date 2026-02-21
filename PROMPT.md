# Vocabulary Trainer — Implementation Prompt

## Project overview

Build a Chinese–English vocabulary trainer that runs locally via Docker. The backend is written in Go, the frontend in plain JS/HTML/CSS (no heavy JS framework), and data is stored in SQLite.

---

## Tech stack

| Layer | Choice |
|---|---|
| Backend | Go (net/http or chi router) |
| Frontend | Vanilla JS + HTML + CSS (Tailwind CDN or similar for modern look) |
| Database | SQLite (via `mattn/go-sqlite3`) |
| Container | Docker + docker-compose |

---

## Data model

### `words`
Stores individual vocabulary items (one row per unique word/phrase).

```sql
CREATE TABLE words (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  text      TEXT NOT NULL,
  language  TEXT NOT NULL CHECK(language IN ('en', 'zh')),
  pinyin    TEXT,           -- only for zh entries; NULL for en entries
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(text, language)
);
```

### `translations`
N:N relationship between English and Chinese words.

```sql
CREATE TABLE translations (
  en_word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  zh_word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  PRIMARY KEY (en_word_id, zh_word_id)
);
```

### `sm2_progress`
Spaced-repetition state per word (one row per word).

```sql
CREATE TABLE sm2_progress (
  word_id        INTEGER PRIMARY KEY REFERENCES words(id) ON DELETE CASCADE,
  repetitions    INTEGER NOT NULL DEFAULT 0,   -- number of consecutive correct answers
  easiness       REAL    NOT NULL DEFAULT 2.5, -- SM-2 easiness factor (EF)
  interval_days  INTEGER NOT NULL DEFAULT 1,   -- days until next review
  due_date       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  total_correct  INTEGER NOT NULL DEFAULT 0,
  total_attempts INTEGER NOT NULL DEFAULT 0
);
```

---

## Spaced repetition algorithm (SM-2)

Use the classic SM-2 algorithm. After each answer the user rates quality from 0–5 internally mapped from:
- Correct answer → quality = 4 (good) or 5 (perfect); since checking is exact-match, use 4 for correct.
- Wrong answer → quality = 0.

SM-2 update rules:
```
EF' = EF + (0.1 - (5 - q) * (0.08 + (5 - q) * 0.02))
EF' = max(1.3, EF')

if q < 3:
  repetitions = 0
  interval = 1
else:
  if repetitions == 0: interval = 1
  elif repetitions == 1: interval = 6
  else: interval = round(interval * EF')
  repetitions += 1

due_date = now + interval days
```

**Card selection:** Among all cards whose `due_date <= now`, pick the one with the lowest `due_date` (most overdue first). If no cards are due, pick the card with the nearest upcoming `due_date` so training can always continue.

---

## Answer checking

- Case-insensitive exact match (trim whitespace).
- For Chinese answers: compare the `text` field. Pinyin is not accepted as a substitute for Chinese characters.
- If a word has multiple accepted translations, any one correct match counts as correct.

---

## Quiz modes

When a card is shown, one of three modes is randomly selected with equal probability:
1. **English → Chinese**: Show English word, user types Chinese characters.
2. **Chinese → English**: Show Chinese characters (no pinyin), user types English.
3. **Chinese + Pinyin → English**: Show Chinese characters and pinyin, user types English.

The SM-2 progress is tracked **per source word** (the word being tested, not per translation pair).

---

## Pages and UI

The UI must be responsive, simple, and modern-looking (use Tailwind CSS via CDN).

### 1. Training page (`/`)
- Show the prompt (word + mode label).
- Text input for the user's answer.
- Submit button (also triggered by Enter key).
- After submission: show whether the answer was correct or wrong, display the correct answer(s), then a "Next" button to advance.
- Display a small stats bar: cards due today, total cards.

### 2. Vocabulary management page (`/vocab`)
- **List view**: paginated table of all entries showing Chinese, Pinyin, English translation(s), and action buttons.
- **Add entry form**: fields for Chinese text, Pinyin, and one or more English translations. On submit, create/find the `zh` word and each `en` word and link them via `translations`. If a word already exists (same text + language), reuse it.
- **Edit entry**: allow editing Chinese text, pinyin, and the set of linked English translations.
- **Delete entry**: deletes the word and cascades to translations and SM-2 progress.
- **Search / filter**: simple text search across Chinese and English words.

---

## REST API

All endpoints return JSON. Prefix: `/api`.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/quiz/next` | Return the next card to study |
| `POST` | `/api/quiz/answer` | Submit an answer, get result + updated stats |
| `GET` | `/api/words` | List words (query params: `q`, `page`, `per_page`) |
| `POST` | `/api/words` | Create a new vocabulary entry (zh + en + pinyin) |
| `GET` | `/api/words/:id` | Get a single word with its translations |
| `PUT` | `/api/words/:id` | Update a word |
| `DELETE` | `/api/words/:id` | Delete a word |

### `GET /api/quiz/next` response
```json
{
  "word_id": 42,
  "mode": "en_to_zh",          // "en_to_zh" | "zh_to_en" | "zh_pinyin_to_en"
  "prompt": "Hello",
  "pinyin": null,               // populated only in zh_pinyin_to_en mode
  "due_date": "2026-02-21T10:00:00Z",
  "interval_days": 3
}
```

### `POST /api/quiz/answer` request
```json
{
  "word_id": 42,
  "mode": "en_to_zh",
  "answer": "你好"
}
```

### `POST /api/quiz/answer` response
```json
{
  "correct": true,
  "correct_answers": ["你好"],
  "next_due": "2026-02-25T10:00:00Z",
  "interval_days": 4,
  "total_correct": 10,
  "total_attempts": 12
}
```

### `POST /api/words` request
```json
{
  "zh_text": "你好",
  "pinyin": "nǐ hǎo",
  "en_texts": ["Hello", "Hi"]
}
```

---

## Docker setup

### `Dockerfile`
- Multi-stage: build Go binary in a builder stage, copy to a minimal runtime image (e.g., `debian:bookworm-slim` — needed for CGO/SQLite).
- Expose port `8080`.
- Store SQLite database at `/data/vocab.db`; mount `/data` as a volume.

### `docker-compose.yml`
```yaml
services:
  app:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - vocab_data:/data
volumes:
  vocab_data:
```

---

## Project structure

```
vocabulary_trainer/
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── go.sum
├── main.go
├── db/
│   ├── schema.sql
│   └── db.go          # open DB, run migrations
├── handlers/
│   ├── quiz.go        # /api/quiz/*
│   └── words.go       # /api/words/*
├── models/
│   └── models.go      # Go structs matching DB tables
├── sm2/
│   └── sm2.go         # SM-2 algorithm
└── frontend/
    ├── index.html     # Training page
    ├── vocab.html     # Vocabulary management page
    ├── app.js         # Shared JS utilities + routing
    ├── train.js       # Training page logic
    └── vocab.js       # Vocab management logic
```

The Go server serves `frontend/` as static files and mounts the API under `/api`.

---

## Implementation notes

1. Use database transactions when creating a vocabulary entry (insert zh word, en words, translation links) to keep it atomic.
2. Initialize `sm2_progress` rows with `due_date = CURRENT_TIMESTAMP` when a new word is first created so it is immediately due.
3. Use `modernc.org/sqlite` (pure Go, no CGO) if you want a simpler Docker setup without CGO; otherwise use `mattn/go-sqlite3` with CGO enabled.
4. The frontend communicates with the backend exclusively via the REST API (fetch).
5. No authentication is required — single user, no login.
6. All static assets (HTML/JS/CSS) are embedded into the Go binary using `//go:embed frontend/*` for a single-binary deployment.
