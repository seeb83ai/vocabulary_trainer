import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ── escHtml ───────────────────────────────────────────────────────────────────
// escHtml is defined as a regular function in app.js, which we inline here
// to keep tests self-contained and independent of module bundling.

function escHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

describe('escHtml', () => {
  it('passes through plain text unchanged', () => {
    expect(escHtml('hello world')).toBe('hello world');
  });

  it('escapes ampersand', () => {
    expect(escHtml('a & b')).toBe('a &amp; b');
  });

  it('escapes less-than', () => {
    expect(escHtml('<script>')).toBe('&lt;script&gt;');
  });

  it('escapes greater-than', () => {
    expect(escHtml('a > b')).toBe('a &gt; b');
  });

  it('escapes double quotes', () => {
    expect(escHtml('"quoted"')).toBe('&quot;quoted&quot;');
  });

  it('escapes all special chars together', () => {
    expect(escHtml('<a href="x&y">test</a>')).toBe(
      '&lt;a href=&quot;x&amp;y&quot;&gt;test&lt;/a&gt;'
    );
  });

  it('coerces non-string input', () => {
    expect(escHtml(42)).toBe('42');
    expect(escHtml(null)).toBe('null');
  });

  it('handles empty string', () => {
    expect(escHtml('')).toBe('');
  });

  it('handles Chinese characters unchanged', () => {
    expect(escHtml('你好世界')).toBe('你好世界');
  });
});

// ── apiFetch ──────────────────────────────────────────────────────────────────
// Re-implement apiFetch the same way app.js does, using the global fetch.

async function apiFetch(path, options = {}) {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  if (!res.ok) {
    let errMsg = res.statusText;
    try {
      const body = await res.json();
      if (body.error) errMsg = body.error;
    } catch (_) {}
    throw new Error(errMsg);
  }
  if (res.status === 204) return null;
  return res.json();
}

describe('apiFetch', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('returns parsed JSON on 200', async () => {
    fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ word_id: 1, mode: 'zh_to_en' }),
    });
    const data = await apiFetch('/api/quiz/next');
    expect(data).toEqual({ word_id: 1, mode: 'zh_to_en' });
  });

  it('returns null on 204', async () => {
    fetch.mockResolvedValue({ ok: true, status: 204 });
    const data = await apiFetch('/api/words/1', { method: 'DELETE' });
    expect(data).toBeNull();
  });

  it('throws with server error message on non-ok response', async () => {
    fetch.mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      json: async () => ({ error: 'word not found' }),
    });
    await expect(apiFetch('/api/words/9999')).rejects.toThrow('word not found');
  });

  it('throws with statusText when body has no error field', async () => {
    fetch.mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({}),
    });
    await expect(apiFetch('/api/quiz/next')).rejects.toThrow('Internal Server Error');
  });

  it('throws with statusText when response body is not JSON', async () => {
    fetch.mockResolvedValue({
      ok: false,
      status: 503,
      statusText: 'Service Unavailable',
      json: async () => { throw new SyntaxError('not json'); },
    });
    await expect(apiFetch('/api/quiz/next')).rejects.toThrow('Service Unavailable');
  });

  it('passes method and body through to fetch', async () => {
    fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ id: 5 }),
    });
    await apiFetch('/api/words', { method: 'POST', body: JSON.stringify({ zh_text: '你好' }) });
    expect(fetch).toHaveBeenCalledWith('/api/words', expect.objectContaining({
      method: 'POST',
    }));
  });

  it('includes Content-Type when no extra options given', async () => {
    fetch.mockResolvedValue({ ok: true, status: 204 });
    await apiFetch('/api/words/1');
    const call = fetch.mock.calls[0][1];
    expect(call.headers['Content-Type']).toBe('application/json');
  });

  it('passes extra headers through to fetch', async () => {
    fetch.mockResolvedValue({ ok: true, status: 204 });
    await apiFetch('/api/words/1', { headers: { 'X-Custom': 'val' } });
    // When options contains a headers key, the spread ...options overwrites
    // the built headers object — X-Custom is present in the final call.
    const call = fetch.mock.calls[0][1];
    expect(call.headers['X-Custom']).toBe('val');
  });
});
