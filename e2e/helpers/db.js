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
