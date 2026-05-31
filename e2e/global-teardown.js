/**
 * Playwright globalTeardown — runs once after all E2E tests complete.
 *
 * Reads the server PID and temp DB path from e2e/.state/server.json (written by
 * globalSetup), kills the server process, and removes the temp SQLite file.
 */

import { readFileSync, unlinkSync, existsSync } from 'node:fs';
import { join } from 'node:path';

const STATE_FILE = join('e2e', '.state', 'server.json');

export default async function globalTeardown() {
  if (!existsSync(STATE_FILE)) {
    console.warn('[E2E] No server state file found — skipping teardown.');
    return;
  }

  let state;
  try {
    state = JSON.parse(readFileSync(STATE_FILE, 'utf8'));
  } catch (err) {
    console.error('[E2E] Failed to read server state:', err.message);
    return;
  }

  const { pid, dbPath } = state;

  // Kill the server process
  if (pid) {
    try {
      process.kill(pid, 'SIGTERM');
      console.log(`[E2E] Server (PID ${pid}) terminated.`);
    } catch (err) {
      // Process may have already exited
      if (err.code !== 'ESRCH') {
        console.error(`[E2E] Failed to kill server (PID ${pid}):`, err.message);
      }
    }
  }

  // Remove the temp SQLite file
  if (dbPath && existsSync(dbPath)) {
    try {
      unlinkSync(dbPath);
      console.log(`[E2E] Removed temp DB: ${dbPath}`);
    } catch (err) {
      console.error('[E2E] Failed to remove temp DB:', err.message);
    }
  }

  // Clean up WAL and SHM files if they exist
  for (const ext of ['-wal', '-shm']) {
    const walPath = dbPath + ext;
    if (walPath && existsSync(walPath)) {
      try { unlinkSync(walPath); } catch { /* ignore */ }
    }
  }

  console.log('[E2E] Global teardown complete ✓');
}
