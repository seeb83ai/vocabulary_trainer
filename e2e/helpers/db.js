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
