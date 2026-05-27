/**
 * E2E API helpers — used in globalSetup and optionally in tests.
 * These bypass the browser and call the server REST API directly.
 */

/**
 * Parse Set-Cookie response headers into Playwright storage-state format.
 * @param {string[]} setCookieHeaders - Array of Set-Cookie header values
 * @returns {object[]} Playwright cookie objects
 */
export function parseSetCookieHeaders(setCookieHeaders) {
  return setCookieHeaders.map(header => {
    const parts = header.split(';').map(p => p.trim());
    const eqIdx = parts[0].indexOf('=');
    const name = parts[0].slice(0, eqIdx);
    const value = parts[0].slice(eqIdx + 1);

    const attrs = {};
    for (const part of parts.slice(1)) {
      const eqPos = part.indexOf('=');
      const k = eqPos === -1 ? part.toLowerCase() : part.slice(0, eqPos).toLowerCase().trim();
      const v = eqPos === -1 ? true : part.slice(eqPos + 1).trim();
      attrs[k] = v;
    }

    let expires = -1;
    if (attrs['max-age']) {
      expires = Math.floor(Date.now() / 1000) + parseInt(attrs['max-age'], 10);
    }

    return {
      name,
      value,
      domain: 'localhost',
      path: attrs['path'] || '/',
      expires,
      httpOnly: Boolean(attrs['httponly']),
      secure: Boolean(attrs['secure']),
      sameSite: (() => {
        const s = (attrs['samesite'] || '').toLowerCase();
        if (s === 'strict') return 'Strict';
        if (s === 'none') return 'None';
        return 'Lax'; // default
      })(),
    };
  });
}

/**
 * Seed a vocabulary word via the REST API.
 * @param {string} baseURL
 * @param {string} cookieHeader - Value of the Cookie: header (e.g. "vocab_session=...")
 * @param {{ zh: string, pinyin?: string, en: string[] }} word
 */
export async function seedWord(baseURL, cookieHeader, { zh, pinyin, en }) {
  const res = await fetch(`${baseURL}/api/words`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Cookie: cookieHeader,
    },
    body: JSON.stringify({
      zh_text: zh,
      pinyin: pinyin || '',
      translations: { en },
      tags: [],
      start_training: true,
    }),
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`seedWord(${zh}) failed ${res.status}: ${body}`);
  }
  return res.json();
}
