/**
 * Direct-SQLite test helper for scenarios the REST API can't drive — e.g.
 * seeding a *prior day's* daily_stats bucket snapshot so a test can assert
 * on today-vs-yesterday comparisons without waiting for real elapsed time.
 * Reads the DB path that global-setup.js wrote to e2e/.state/server.json.
 */
import { DatabaseSync } from 'node:sqlite';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

function getDbPath() {
  const state = JSON.parse(readFileSync(join('e2e', '.state', 'server.json'), 'utf8'));
  return state.dbPath;
}

/**
 * Insert (or overwrite) yesterday's daily_stats bucket snapshot for the user
 * with the given email. Buckets not passed default to 0.
 * @param {string} email
 * @param {{bucketNew?: number, bucketStruggling?: number, bucketLearning?: number, bucketPracticing?: number, bucketMastered?: number}} [buckets]
 */
export function seedYesterdayBucketSnapshot(email, buckets = {}) {
  const {
    bucketNew = 0,
    bucketStruggling = 0,
    bucketLearning = 0,
    bucketPracticing = 0,
    bucketMastered = 0,
  } = buckets;

  const db = new DatabaseSync(getDbPath());
  try {
    db.exec('PRAGMA busy_timeout = 5000');
    const userRow = db.prepare('SELECT id FROM users WHERE email = ?').get(email);
    if (!userRow) throw new Error(`seedYesterdayBucketSnapshot: no user found for email ${email}`);
    db.prepare(`
      INSERT INTO daily_stats (user_id, date, bucket_new, bucket_struggling, bucket_learning, bucket_practicing, bucket_mastered)
      VALUES (?, date('now', '-1 day'), ?, ?, ?, ?, ?)
      ON CONFLICT(user_id, date) DO UPDATE SET
        bucket_new        = excluded.bucket_new,
        bucket_struggling = excluded.bucket_struggling,
        bucket_learning   = excluded.bucket_learning,
        bucket_practicing = excluded.bucket_practicing,
        bucket_mastered   = excluded.bucket_mastered
    `).run(userRow.id, bucketNew, bucketStruggling, bucketLearning, bucketPracticing, bucketMastered);
  } finally {
    db.close();
  }
}

const WORD_ID_BY_EMAIL_AND_TEXT = `(SELECT w.id FROM words w JOIN users u ON u.id = w.user_id
  WHERE u.email = ? AND w.text = ? AND w.language = 'zh')`;

/**
 * Turn the zh word into a review word: it left the New bucket, is due now,
 * was last answered yesterday and failed its last `consecutiveLapses` reviews.
 * The next wrong answer in the main quiz is then lapse number consecutiveLapses+1.
 * @param {string} email
 * @param {string} zhText
 * @param {number} consecutiveLapses
 */
export function seedReviewWord(email, zhText, consecutiveLapses) {
  const db = new DatabaseSync(getDbPath());
  try {
    db.exec('PRAGMA busy_timeout = 5000');
    const res = db.prepare(`
      UPDATE sm2_progress SET learning_new_word = 0, repetitions = 2, interval_days = 3, total_attempts = 6,
        total_correct = 2, first_seen_at = datetime('now', '-10 days'), last_attempt_at = datetime('now', '-1 day'),
        due_date = datetime('now', '-1 hour'), lapses = ?, consecutive_lapses = ?
      WHERE word_id = ${WORD_ID_BY_EMAIL_AND_TEXT}
    `).run(consecutiveLapses, consecutiveLapses, email, zhText);
    if (res.changes !== 1) throw new Error(`seedReviewWord: no word ${zhText} for ${email}`);
  } finally {
    db.close();
  }
}

/**
 * Read the SM-2 progress row of the user's zh word.
 * @param {string} email
 * @param {string} zhText
 * @returns {{first_seen_at: string|null, lapses: number, consecutive_lapses: number}}
 */
export function getWordProgress(email, zhText) {
  const db = new DatabaseSync(getDbPath());
  try {
    db.exec('PRAGMA busy_timeout = 5000');
    return /** @type {any} */ (db.prepare(`
      SELECT first_seen_at, lapses, consecutive_lapses FROM sm2_progress
      WHERE word_id = ${WORD_ID_BY_EMAIL_AND_TEXT}
    `).get(email, zhText));
  } finally {
    db.close();
  }
}

/**
 * Read today's answer-bucket counters of the user's zh word.
 * @param {string} email
 * @param {string} zhText
 * @returns {Record<string, {attempts: number, errors: number}>}
 */
export function getTodayAnswerBuckets(email, zhText) {
  const db = new DatabaseSync(getDbPath());
  try {
    db.exec('PRAGMA busy_timeout = 5000');
    const rows = db.prepare(`
      SELECT bucket, attempts, errors FROM answer_stats_daily
      WHERE day = date('now') AND word_id = ${WORD_ID_BY_EMAIL_AND_TEXT}
    `).all(email, zhText);
    return Object.fromEntries(rows.map(r => [r.bucket, { attempts: Number(r.attempts), errors: Number(r.errors) }]));
  } finally {
    db.close();
  }
}

/**
 * Make the user's zh word due now, so it comes back as the next card without
 * waiting for the wrong-answer retry delay.
 * @param {string} email
 * @param {string} zhText
 */
export function makeWordDueNow(email, zhText) {
  const db = new DatabaseSync(getDbPath());
  try {
    db.exec('PRAGMA busy_timeout = 5000');
    const res = db.prepare(`
      UPDATE sm2_progress SET due_date = datetime('now', '-1 minute')
      WHERE word_id = ${WORD_ID_BY_EMAIL_AND_TEXT}
    `).run(email, zhText);
    if (res.changes !== 1) throw new Error(`makeWordDueNow: no word ${zhText} for ${email}`);
  } finally {
    db.close();
  }
}

/**
 * Read the due date of the user's zh word, as SQLite stores it (UTC).
 * @param {string} email
 * @param {string} zhText
 * @returns {string}
 */
export function getWordDueDate(email, zhText) {
  const db = new DatabaseSync(getDbPath());
  try {
    db.exec('PRAGMA busy_timeout = 5000');
    const row = /** @type {any} */ (db.prepare(`
      SELECT due_date FROM sm2_progress WHERE word_id = ${WORD_ID_BY_EMAIL_AND_TEXT}
    `).get(email, zhText));
    return row.due_date;
  } finally {
    db.close();
  }
}

/**
 * Give the user a component that is due now, with an English definition, so
 * the Train page can serve and check it. hanzi_decomposition is shared by all
 * users; the rows are only inserted when missing.
 * @param {string} email
 * @param {string} character
 * @param {string} definition
 */
export function seedDueComponent(email, character, definition) {
  const db = new DatabaseSync(getDbPath());
  try {
    db.exec('PRAGMA busy_timeout = 5000');
    const userRow = db.prepare('SELECT id FROM users WHERE email = ?').get(email);
    if (!userRow) throw new Error(`seedDueComponent: no user found for email ${email}`);
    db.prepare('INSERT OR IGNORE INTO hanzi_decomposition (character, definition) VALUES (?, ?)').run(character, definition);
    db.prepare(`INSERT INTO hanzi_decomposition_translation (character, lang, definition)
      SELECT ?, 'EN', ? WHERE NOT EXISTS (
        SELECT 1 FROM hanzi_decomposition_translation WHERE character = ? AND lang = 'EN' AND user_id IS NULL)`)
      .run(character, definition, character);
    db.prepare(`INSERT OR REPLACE INTO component_progress (user_id, character, due_date)
      VALUES (?, ?, datetime('now', '-1 minute'))`).run(userRow.id, character);
  } finally {
    db.close();
  }
}

/**
 * Add a dictionary entry (cedict_entries), as import-cedict would.
 * @param {string} simplified
 * @param {string} lang 'en' or 'de'
 * @param {string} pinyin
 * @param {string} definition
 */
export function seedCedictEntry(simplified, lang, pinyin, definition) {
  const db = new DatabaseSync(getDbPath());
  try {
    db.exec('PRAGMA busy_timeout = 5000');
    db.prepare('INSERT INTO cedict_entries (simplified, lang, pinyin, definition) VALUES (?, ?, ?, ?)')
      .run(simplified, lang, pinyin, definition);
  } finally {
    db.close();
  }
}
