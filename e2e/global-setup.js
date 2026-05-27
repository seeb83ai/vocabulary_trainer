/**
 * Playwright globalSetup — runs once before all E2E tests.
 *
 * 1. Builds the Go binary (bin/e2e-server).
 * 2. Starts the server on a dedicated port (18080) with a fresh temp SQLite DB.
 * 3. Registers a test user (auto-verified because no SMTP is configured).
 * 4. Seeds vocabulary words via the REST API.
 * 5. Saves the session cookie as a Playwright storage-state file so tests start
 *    already authenticated.
 * 6. Saves the server PID + DB path to e2e/.state/server.json for globalTeardown.
 */

import { execSync, spawn } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { parseSetCookieHeaders, seedWord } from './helpers/api.js';

export const E2E_PORT = 18080;
export const BASE_URL = `http://localhost:${E2E_PORT}`;
export const TEST_EMAIL = 'e2e@test.local';
export const TEST_PASSWORD = 'E2eTestPassword123!';

// A fixed 32-byte hex key for deterministic session tokens across server restarts.
// This is only used for the isolated E2E test server — not production.
const SESSION_SECRET = '00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff';

const STATE_DIR = 'e2e/.state';
const AUTH_DIR = 'e2e/.auth';

/** Poll a URL until it returns HTTP 200 or the timeout elapses. */
async function waitForServer(url, timeoutMs = 20_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url, { signal: AbortSignal.timeout(1_000) });
      if (res.ok || res.status === 302 || res.status === 200) return;
    } catch {
      // server not up yet — keep waiting
    }
    await new Promise(r => setTimeout(r, 300));
  }
  throw new Error(`Server at ${url} did not become ready within ${timeoutMs}ms`);
}

export default async function globalSetup() {
  // ── 1. Build Go binary ──────────────────────────────────────────────────────
  console.log('[E2E] Building Go server binary…');
  execSync('cd service && go build -o ../bin/e2e-server .', {
    stdio: 'inherit',
    cwd: process.cwd(),
  });

  // ── 2. Create a fresh temp SQLite DB ────────────────────────────────────────
  const dbPath = join(tmpdir(), `vocab-e2e-${Date.now()}.db`);
  console.log(`[E2E] Using DB: ${dbPath}`);

  // ── 3. Spawn the server ─────────────────────────────────────────────────────
  const server = spawn('./bin/e2e-server', [], {
    env: {
      ...process.env,
      PORT: String(E2E_PORT),
      DB_PATH: dbPath,
      SESSION_SECRET,
      // Skip interactive first-run setup prompts (migration v20)
      ADMIN_EMAIL: 'admin@e2e.local',
      ADMIN_PASSWORD: 'AdminE2ePassword1!',
      USER_EMAIL: 'seed@e2e.local',
      USER_PASSWORD: 'SeedE2ePassword1!',
      // Use minimum bcrypt cost for fast test user creation
      BCRYPT_COST: 'min',
      // Explicitly unset SMTP so the server auto-verifies registrations
      SMTP_HOST: '',
      SMTP_PORT: '',
      SMTP_USER: '',
      SMTP_PASS: '',
    },
    stdio: 'pipe',
    detached: false,
  });

  server.stderr.on('data', d => process.stderr.write(`[server] ${d}`));
  server.stdout.on('data', d => process.stdout.write(`[server] ${d}`));

  // Persist PID + DB path for teardown
  mkdirSync(STATE_DIR, { recursive: true });
  writeFileSync(
    join(STATE_DIR, 'server.json'),
    JSON.stringify({ pid: server.pid, dbPath }),
  );

  // ── 4. Wait for the server to be ready ──────────────────────────────────────
  console.log(`[E2E] Waiting for server on port ${E2E_PORT}…`);
  await waitForServer(`${BASE_URL}/`);
  console.log('[E2E] Server is up.');

  // ── 5. Register test user (auto-verified — no SMTP configured) ──────────────
  const regRes = await fetch(`${BASE_URL}/api/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: TEST_EMAIL, password: TEST_PASSWORD }),
  });

  if (!regRes.ok) {
    const body = await regRes.text();
    throw new Error(`Registration failed (${regRes.status}): ${body}`);
  }

  const regData = await regRes.json();
  if (!regData.auto_login) {
    throw new Error('Expected auto_login=true (no SMTP should be configured in E2E env)');
  }

  // Extract session cookies from the registration response
  const setCookieHeaders = regRes.headers.getSetCookie?.() ?? [];
  if (setCookieHeaders.length === 0) {
    throw new Error('No Set-Cookie header in registration response');
  }
  const cookies = parseSetCookieHeaders(setCookieHeaders);
  const cookieHeader = cookies.map(c => `${c.name}=${c.value}`).join('; ');

  // ── 6. Seed vocabulary words ─────────────────────────────────────────────────
  const words = [
    { zh: '你好', pinyin: 'nǐ hǎo', en: ['hello', 'hi'] },
    { zh: '谢谢', pinyin: 'xiè xiè', en: ['thank you', 'thanks'] },
    { zh: '再见', pinyin: 'zài jiàn', en: ['goodbye', 'bye'] },
  ];

  for (const word of words) {
    await seedWord(BASE_URL, cookieHeader, word);
    console.log(`[E2E] Seeded word: ${word.zh}`);
  }

  // ── 7. Save Playwright storage state ────────────────────────────────────────
  mkdirSync(AUTH_DIR, { recursive: true });
  const storageState = { cookies, origins: [] };
  writeFileSync(join(AUTH_DIR, 'user.json'), JSON.stringify(storageState, null, 2));
  console.log('[E2E] Auth state saved to e2e/.auth/user.json');

  console.log('[E2E] Global setup complete ✓');
}
