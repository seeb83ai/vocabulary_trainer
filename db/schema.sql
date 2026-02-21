CREATE TABLE IF NOT EXISTS words (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  text       TEXT    NOT NULL,
  language   TEXT    NOT NULL CHECK(language IN ('en', 'zh')),
  pinyin     TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(text, language)
);

CREATE TABLE IF NOT EXISTS translations (
  en_word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  zh_word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  PRIMARY KEY (en_word_id, zh_word_id)
);

CREATE TABLE IF NOT EXISTS sm2_progress (
  word_id        INTEGER PRIMARY KEY REFERENCES words(id) ON DELETE CASCADE,
  repetitions    INTEGER NOT NULL DEFAULT 0,
  easiness       REAL    NOT NULL DEFAULT 2.5,
  interval_days  INTEGER NOT NULL DEFAULT 1,
  due_date       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  total_correct  INTEGER NOT NULL DEFAULT 0,
  total_attempts INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_sm2_due ON sm2_progress(due_date);
CREATE INDEX IF NOT EXISTS idx_words_text_lang ON words(text, language);
