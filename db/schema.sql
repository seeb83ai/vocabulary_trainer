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
  word_id          INTEGER PRIMARY KEY REFERENCES words(id) ON DELETE CASCADE,
  repetitions      INTEGER NOT NULL DEFAULT 0,
  easiness         REAL    NOT NULL DEFAULT 2.5,
  interval_days    INTEGER NOT NULL DEFAULT 1,
  due_date         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  total_correct    INTEGER NOT NULL DEFAULT 0,
  total_attempts   INTEGER NOT NULL DEFAULT 0,
  -- Calendar date (YYYY-MM-DD) when this card was first returned as a quiz prompt.
  -- NULL means the word has never been presented. Used to enforce the daily new-word cap.
  first_seen_date  TEXT DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_sm2_due ON sm2_progress(due_date);
CREATE INDEX IF NOT EXISTS idx_words_text_lang ON words(text, language);
CREATE INDEX IF NOT EXISTS idx_translations_zh ON translations(zh_word_id);

-- Tracks wrong answers that matched a different known vocabulary word.
-- zh_word_id: the word being quizzed (always a zh word)
-- confused_with_id: the zh word whose translation the user typed instead
-- mode: quiz mode (en_to_zh | zh_to_en | zh_pinyin_to_en)
-- count: how many times this exact confusion occurred
-- last_seen: timestamp of most recent occurrence
CREATE TABLE IF NOT EXISTS confusion_pairs (
  zh_word_id       INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  confused_with_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  mode             TEXT    NOT NULL,
  count            INTEGER NOT NULL DEFAULT 1,
  last_seen        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (zh_word_id, confused_with_id, mode)
);

-- Tags for categorising vocabulary words (e.g. "food", "travel", "HSK1")
CREATE TABLE IF NOT EXISTS tags (
  id   INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE
);

-- Many-to-many link between zh words and tags
CREATE TABLE IF NOT EXISTS word_tags (
  word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (word_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_word_tags_word ON word_tags(word_id);
