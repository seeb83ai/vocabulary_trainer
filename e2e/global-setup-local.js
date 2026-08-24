/**
 * Playwright globalSetup for local-server PR screenshots.
 *
 * Does NOT build a binary or spin up a temp DB. Instead it logs in to an
 * already-running local server as the user identified by LOCAL_USER_EMAIL /
 * LOCAL_USER_PASSWORD and saves the session cookie so spec files start
 * authenticated.
 *
 * Required env vars:
 *   LOCAL_SERVER_URL   — e.g. http://localhost:8080 (default)
 *   LOCAL_USER_EMAIL   — email of the account to screenshot
 *   LOCAL_USER_PASSWORD — password for that account
 */

import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { parseSetCookieHeaders } from './helpers/api.js';

export const LOCAL_PORT_DEFAULT = 8080;

const AUTH_DIR = 'e2e/.auth';

export default async function globalSetup() {
  const baseURL = process.env.LOCAL_SERVER_URL || `http://localhost:${LOCAL_PORT_DEFAULT}`;
  const email = process.env.LOCAL_USER_EMAIL;
  const password = process.env.LOCAL_USER_PASSWORD;

  if (!email || !password) {
    throw new Error(
      'LOCAL_USER_EMAIL and LOCAL_USER_PASSWORD must be set when USE_LOCAL_SERVER=1',
    );
  }

  console.log(`[E2E-local] Logging in to ${baseURL} as ${email}…`);

  const res = await fetch(`${baseURL}/api/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });

  if (!res.ok) {
    const body = await res.text();
    throw new Error(`Login failed (${res.status}): ${body}`);
  }

  const setCookieHeaders = res.headers.getSetCookie?.() ?? [];
  if (setCookieHeaders.length === 0) {
    throw new Error('No Set-Cookie header in login response');
  }

  const cookies = parseSetCookieHeaders(setCookieHeaders);
  mkdirSync(AUTH_DIR, { recursive: true });
  writeFileSync(
    join(AUTH_DIR, 'user.json'),
    JSON.stringify({ cookies, origins: [] }, null, 2),
  );

  // ponytail: write baseURL so the config can pick it up without re-parsing env
  writeFileSync(join(AUTH_DIR, 'local-base-url.txt'), baseURL);

  console.log(`[E2E-local] Auth state saved for ${email} ✓`);
}
