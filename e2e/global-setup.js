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
import { createServer } from 'node:http';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { parseSetCookieHeaders, seedWord } from './helpers/api.js';

export const E2E_PORT = 18080;
export const BASE_URL = `http://localhost:${E2E_PORT}`;
// Mock GitHub API for the in-app issue-reporting feature. The Go server calls
// it (server→server) instead of api.github.com, so a real HTTP listener is
// required — Playwright page.route only intercepts browser requests.
export const MOCK_GITHUB_PORT = 18081;
export const MOCK_GITHUB_URL = `http://localhost:${MOCK_GITHUB_PORT}`;

/**
 * Start a minimal mock of the GitHub REST endpoints the issue handler uses:
 * repo lookup, branch ref read/create, contents upload, and issue creation.
 * Stored on globalThis so globalTeardown can close it.
 */
function startMockGitHub() {
  const server = createServer((req, res) => {
    let body = '';
    req.on('data', c => (body += c));
    req.on('end', () => {
      const { method, url } = req;
      const send = (status, obj) => {
        res.writeHead(status, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(obj));
      };
      if (method === 'POST' && url.endsWith('/issues')) {
        return send(201, { number: 101, html_url: 'https://github.com/owner/repo/issues/101' });
      }
      if (method === 'PUT' && url.includes('/contents/')) {
        return send(201, { content: { download_url: `${MOCK_GITHUB_URL}/raw/screenshot.png` } });
      }
      if (method === 'POST' && url.endsWith('/git/refs')) {
        return send(201, { ref: 'refs/heads/issue-assets' });
      }
      if (method === 'GET' && url.includes('/git/ref/')) {
        return send(200, { object: { sha: 'deadbeef' } });
      }
      if (method === 'GET' && /\/repos\/[^/]+\/[^/]+$/.test(url)) {
        return send(200, { default_branch: 'main' });
      }
      return send(404, { message: 'not found' });
    });
  });
  server.listen(MOCK_GITHUB_PORT);
  server.unref();
  globalThis.__mockGitHubServer = server;
  console.log(`[E2E] Mock GitHub API listening on ${MOCK_GITHUB_URL}`);
}
export const TEST_EMAIL = 'e2e@test.local';
export const TEST_PASSWORD = 'E2eTestPassword123!';
// Second test user — has a single unseen word (start_training: false).
// Used by quiz.spec.js to test the new-word introduction flow.
export const TEST_NEWWORD_EMAIL = 'e2e-newword@test.local';
export const TEST_NEWWORD_PASSWORD = 'E2eNewWordPassword123!';

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
  // ── 0. Start the mock GitHub API ────────────────────────────────────────────
  startMockGitHub();

  // ── 1. Build Go binary ──────────────────────────────────────────────────────
  console.log('[E2E] Building Go server binary…');
  execSync('cd service && go build -o ../bin/e2e-server .', {
    stdio: 'inherit',
    cwd: process.cwd(),
  });

  // ── 2. Create a fresh temp SQLite DB ────────────────────────────────────────
  const dbPath = join(tmpdir(), `vocab-e2e-${Date.now()}.db`);
  console.log(`[E2E] Using DB: ${dbPath}`);

  // ── 2b. Seed cedict_entries with a tiny fixture dictionary ─────────────────
  // Mirrors the real deployment order (run the import tool once, then start
  // the server): this creates + migrates the DB and populates just enough
  // CC-CEDICT-format data for the sub-word auto-creation and free
  // dictionary-lookup E2E tests to exercise real segmentation, not a mock.
  console.log('[E2E] Importing cedict fixture data…');
  execSync(
    `cd service && go run ./cmd/import-cedict -db ${dbPath} -file ../e2e/fixtures/cedict-sample.u8 -lang en`,
    {
      stdio: 'inherit',
      cwd: process.cwd(),
      env: {
        ...process.env,
        // Skip interactive first-run setup prompts (migration v20) — same
        // credentials the server itself uses below, since Migrate() creates
        // these accounts on first run regardless of which binary calls it.
        ADMIN_EMAIL: 'admin@e2e.local',
        ADMIN_PASSWORD: 'AdminE2ePassword1!',
        USER_EMAIL: 'seed@e2e.local',
        USER_PASSWORD: 'SeedE2ePassword1!',
        BCRYPT_COST: 'min',
      },
    },
  );

  // ── 3. Spawn the server ─────────────────────────────────────────────────────
  const server = spawn('./bin/e2e-server', [], {
    env: {
      ...process.env,
      PORT: String(E2E_PORT),
      DB_PATH: dbPath,
      SESSION_SECRET,
      // e2e runs over plain HTTP on localhost; dev mode omits the cookie
      // Secure flag so the browser stores the session cookie.
      APP_ENV: 'dev',
      // Skip interactive first-run setup prompts (migration v20)
      ADMIN_EMAIL: 'admin@e2e.local',
      ADMIN_PASSWORD: 'AdminE2ePassword1!',
      USER_EMAIL: 'seed@e2e.local',
      USER_PASSWORD: 'SeedE2ePassword1!',
      // Use minimum bcrypt cost for fast test user creation
      BCRYPT_COST: 'min',
      // The default per-minute rate limits are tuned for real user traffic
      // (~5 rps). A single E2E worker fires far more requests than that in
      // rapid page-navigation bursts (every static asset + API call shares
      // one bucket), which trips 429s and makes tests flaky. Raise the caps
      // for the isolated E2E server only.
      RATE_LIMIT_USER_PER_MIN: '100000',
      RATE_LIMIT_EXPENSIVE_PER_MIN: '100000',
      // Several specs register a fresh user per test (e.g. the ambiguous-answer
      // suite in quiz.spec.js), which can exceed the default 10/min auth-IP
      // budget under CI's tighter timing and cause flaky 429s on /api/register.
      RATE_LIMIT_AUTH_PER_MIN: '100000',
      // Explicitly unset SMTP so the server auto-verifies registrations
      SMTP_HOST: '',
      SMTP_PORT: '',
      SMTP_USER: '',
      SMTP_PASS: '',
      // Enable in-app GitHub issue reporting against the mock GitHub API.
      GITHUB_TOKEN: 'e2e-test-token',
      GITHUB_ISSUE_REPO: 'owner/repo',
      GITHUB_API_BASE_URL: MOCK_GITHUB_URL,
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

  // ── 7. Save Playwright storage state for main user ──────────────────────────
  mkdirSync(AUTH_DIR, { recursive: true });
  const storageState = { cookies, origins: [] };
  writeFileSync(join(AUTH_DIR, 'user.json'), JSON.stringify(storageState, null, 2));
  console.log('[E2E] Auth state saved to e2e/.auth/user.json');

  // ── 8. Register the new-word test user ───────────────────────────────────────
  // This user has one unseen word (start_training: false) so the quiz shows the
  // new-word introduction screen instead of a regular card.
  const nwRegRes = await fetch(`${BASE_URL}/api/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: TEST_NEWWORD_EMAIL, password: TEST_NEWWORD_PASSWORD }),
  });
  if (!nwRegRes.ok) {
    const body = await nwRegRes.text();
    throw new Error(`New-word user registration failed (${nwRegRes.status}): ${body}`);
  }
  const nwRegData = await nwRegRes.json();
  if (!nwRegData.auto_login) {
    throw new Error('Expected auto_login=true for new-word user');
  }
  const nwSetCookieHeaders = nwRegRes.headers.getSetCookie?.() ?? [];
  if (nwSetCookieHeaders.length === 0) {
    throw new Error('No Set-Cookie header in new-word user registration response');
  }
  const nwCookies = parseSetCookieHeaders(nwSetCookieHeaders);
  const nwCookieHeader = nwCookies.map(c => `${c.name}=${c.value}`).join('; ');

  // Seed one unseen word (水/shuǐ/water) with start_training: false.
  // total_attempts=0 → quiz returns mode='new_word' for this user.
  await seedWord(BASE_URL, nwCookieHeader, { zh: '水', pinyin: 'shuǐ', en: ['water'] }, false);
  console.log('[E2E] Seeded unseen word: 水 (for new-word user)');

  // Save auth state for the new-word user
  const nwStorageState = { cookies: nwCookies, origins: [] };
  writeFileSync(join(AUTH_DIR, 'new-word-user.json'), JSON.stringify(nwStorageState, null, 2));
  console.log('[E2E] Auth state saved to e2e/.auth/new-word-user.json');

  console.log('[E2E] Global setup complete ✓');
}
