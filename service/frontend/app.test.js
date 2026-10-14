import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ── escHtml ───────────────────────────────────────────────────────────────────
// escHtml is defined as a regular function in app.js, which we inline here
// to keep tests self-contained and independent of module bundling.

function escHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function shouldAutofocus(st) {
  const touch = window.matchMedia('(pointer: coarse)').matches;
  return touch ? !!st?.autofocus_mobile : st?.autofocus_desktop !== false;
}

describe('shouldAutofocus', () => {
  const mockPointer = coarse => {
    window.matchMedia = vi.fn().mockReturnValue({ matches: coarse });
  };

  it('uses the desktop setting on a fine pointer', () => {
    mockPointer(false);
    expect(shouldAutofocus({ autofocus_desktop: true, autofocus_mobile: false })).toBe(true);
    expect(shouldAutofocus({ autofocus_desktop: false, autofocus_mobile: true })).toBe(false);
  });

  it('uses the mobile setting on a coarse pointer', () => {
    mockPointer(true);
    expect(shouldAutofocus({ autofocus_desktop: true, autofocus_mobile: false })).toBe(false);
    expect(shouldAutofocus({ autofocus_desktop: false, autofocus_mobile: true })).toBe(true);
  });

  it('defaults to focus on desktop and none on mobile when settings are missing', () => {
    mockPointer(false);
    expect(shouldAutofocus(null)).toBe(true);
    mockPointer(true);
    expect(shouldAutofocus(null)).toBe(false);
  });
});

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

  it('escapes single quotes', () => {
    expect(escHtml("it's a 'test'")).toBe('it&#39;s a &#39;test&#39;');
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

// ── wordTier ─────────────────────────────────────────────────────────────────
// Inline from app.js

const TIERS = [
  { key: 'new',    label: 'New',        desc: 'Learning phase',   color: '#8b5cf6', pill: 'bg-violet-100 text-violet-700', icon: '🌰' },
  { key: '0-49',   label: 'Struggling', desc: 'EN → ZH',          color: '#ef4444', pill: 'bg-red-100 text-red-700',    icon: '🌱' },
  { key: '50-69',  label: 'Learning',   desc: 'ZH + Pinyin → EN', color: '#f59e0b', pill: 'bg-amber-100 text-amber-700', icon: '🌿' },
  { key: '70-84',  label: 'Practicing', desc: 'ZH → EN',          color: '#3b82f6', pill: 'bg-blue-100 text-blue-700',   icon: '🌳' },
  { key: '85-100', label: 'Mastered',   desc: 'All modes',        color: '#22c55e', pill: 'bg-green-100 text-green-700', icon: '🌸' },
];

function wordTier(totalCorrect, totalAttempts, learningNewWord, streakBonus) {
  if (totalAttempts === 0) return null;
  if (learningNewWord) return TIERS[0];
  const acc = (totalCorrect + (streakBonus || 0)) / totalAttempts;
  if (totalAttempts >= 10 && acc >= 0.85) return TIERS[4];
  if (totalAttempts >= 10 && acc >= 0.70) return TIERS[3];
  if (totalAttempts >= 3  && acc >= 0.50 && acc < 0.70) return TIERS[2];
  return TIERS[1];
}

describe('wordTier', () => {
  it('returns null for unseen words', () => {
    expect(wordTier(0, 0, false)).toBeNull();
  });

  it('returns New for learning words', () => {
    expect(wordTier(1, 2, true)).toEqual(TIERS[0]);
    expect(wordTier(1, 2, true).label).toBe('New');
  });

  it('returns Struggling for low accuracy graduated words', () => {
    const tier = wordTier(1, 3, false);
    expect(tier.label).toBe('Struggling');
  });

  it('returns Learning for mid accuracy graduated words', () => {
    const tier = wordTier(2, 3, false); // 67%
    expect(tier.label).toBe('Learning');
  });

  it('returns Practicing for high accuracy with enough attempts', () => {
    const tier = wordTier(8, 10, false); // 80%
    expect(tier.label).toBe('Practicing');
  });

  it('returns Mastered for very high accuracy with enough attempts', () => {
    const tier = wordTier(9, 10, false); // 90%
    expect(tier.label).toBe('Mastered');
  });

  it('returns Struggling for high accuracy but few attempts', () => {
    const tier = wordTier(3, 3, false);
    expect(tier.label).toBe('Struggling');
  });

  it('streak bonus boosts tier', () => {
    // Raw: 4/10 = 40% → Struggling. With bonus 3: 7/10 = 70% → Practicing
    const tier = wordTier(4, 10, false, 3);
    expect(tier.label).toBe('Practicing');
  });

  it('streak bonus defaults to 0', () => {
    // Same as without bonus
    const tier = wordTier(4, 10, false);
    expect(tier.label).toBe('Struggling');
  });
});

// ── tierIconHTML ──────────────────────────────────────────────────────────────
// Inline from app.js. Renders just the ONE icon for the word's current tier,
// for the compact inline result-screen indicator.

function tierIconHTML(tier, prevTier) {
  if (!tier) return '';
  const entry = TIERS.find(e => e.label === tier);
  if (!entry) return '';
  const changed = !!prevTier && prevTier !== tier;
  const cls = 'tier-icon' + (changed ? ' tier-icon-changed' : '');
  return `<span class="${cls}" title="${escHtml(tier)}">${entry.icon}</span>`;
}

describe('tierIconHTML', () => {
  it('returns empty string when there is no tier', () => {
    expect(tierIconHTML('', '')).toBe('');
    expect(tierIconHTML(undefined, undefined)).toBe('');
  });

  it('renders exactly one icon for the given tier', () => {
    const html = tierIconHTML('Practicing', '');
    const matches = html.match(/<span/g) || [];
    expect(matches.length).toBe(1);
    expect(html).toContain(TIERS.find(e => e.label === 'Practicing').icon);
  });

  it('does not render icons for other tiers', () => {
    const html = tierIconHTML('Practicing', '');
    expect(html).not.toContain(TIERS.find(e => e.label === 'Struggling').icon);
    expect(html).not.toContain(TIERS.find(e => e.label === 'Mastered').icon);
  });

  it('adds a changed class when prevTier differs from tier', () => {
    const html = tierIconHTML('Practicing', 'Learning');
    expect(html).toContain('tier-icon-changed');
  });

  it('does not add a changed class when prevTier equals tier', () => {
    const html = tierIconHTML('Practicing', 'Practicing');
    expect(html).not.toContain('tier-icon-changed');
  });

  it('does not add a changed class when prevTier is absent', () => {
    const html = tierIconHTML('Practicing', '');
    expect(html).not.toContain('tier-icon-changed');
  });

  it('escapes the tier label used in the title attribute', () => {
    const html = tierIconHTML('New', '');
    expect(html).toContain('title="New"');
  });
});

// ── apiFetch ──────────────────────────────────────────────────────────────────
// Re-implement apiFetch the same way app.js does, using the global fetch.

const t = key => key;
const API_TIMEOUT_MS = 15000;
const API_SLOW_TIMEOUT_MS = 60000;

async function apiFetch(path, options = {}) {
  const { timeoutMs = API_TIMEOUT_MS, signal, ...rest } = options;
  const ctrl = new AbortController();
  let timedOut = false;
  const timer = setTimeout(() => { timedOut = true; ctrl.abort(); }, timeoutMs);
  const cancel = () => ctrl.abort();
  if (signal?.aborted) ctrl.abort();
  else signal?.addEventListener('abort', cancel);
  try {
    const res = await fetch(path, {
      headers: { 'Content-Type': 'application/json', ...(rest.headers || {}) },
      ...rest,
      signal: ctrl.signal,
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
    return await res.json();
  } catch (err) {
    if (timedOut) {
      const timeoutErr = new Error(t('api.timeout'));
      timeoutErr.name = 'TimeoutError';
      throw timeoutErr;
    }
    throw err;
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener('abort', cancel);
  }
}

function isAbortError(err) {
  return err?.name === 'AbortError';
}

function isTimeoutError(err) {
  return err?.name === 'TimeoutError';
}

function latestRequest() {
  let ctrl = null;
  return () => {
    ctrl?.abort();
    ctrl = new AbortController();
    return ctrl.signal;
  };
}

async function withBusyButton(btn, fn) {
  if (btn.disabled) return;
  btn.disabled = true;
  try {
    return await fn();
  } finally {
    btn.disabled = false;
  }
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

// ── apiFetch timeouts and cancel ─────────────────────────────────────────────

// A fetch that never answers; it rejects only when its signal aborts, as a
// browser fetch does.
function hangingFetch() {
  return vi.fn((path, opts) => new Promise((resolve, reject) => {
    if (opts.signal.aborted) reject(new DOMException('aborted', 'AbortError'));
    opts.signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
  }));
}

describe('apiFetch timeout', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('fetch', hangingFetch());
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('rejects with a TimeoutError after the default 15 s', async () => {
    const p = apiFetch('/api/quiz/next');
    const check = expect(p).rejects.toMatchObject({ name: 'TimeoutError', message: 'api.timeout' });
    await vi.advanceTimersByTimeAsync(API_TIMEOUT_MS - 1);
    await vi.advanceTimersByTimeAsync(1);
    await check;
  });

  it('uses timeoutMs when the caller sets it', async () => {
    const p = apiFetch('/api/translate', { timeoutMs: API_SLOW_TIMEOUT_MS });
    let settled = false;
    p.catch(() => { settled = true; });
    await vi.advanceTimersByTimeAsync(API_TIMEOUT_MS);
    expect(settled).toBe(false);
    await vi.advanceTimersByTimeAsync(API_SLOW_TIMEOUT_MS - API_TIMEOUT_MS);
    await expect(p).rejects.toMatchObject({ name: 'TimeoutError' });
  });

  it('rejects with an AbortError when the caller cancels', async () => {
    const ctrl = new AbortController();
    const p = apiFetch('/api/words', { signal: ctrl.signal });
    ctrl.abort();
    const err = await p.catch(e => e);
    expect(isAbortError(err)).toBe(true);
    expect(isTimeoutError(err)).toBe(false);
  });

  it('rejects at once when the caller signal is already cancelled', async () => {
    const ctrl = new AbortController();
    ctrl.abort();
    const err = await apiFetch('/api/words', { signal: ctrl.signal }).catch(e => e);
    expect(isAbortError(err)).toBe(true);
  });

  it('clears the timer when the request answers', async () => {
    fetch.mockResolvedValue({ ok: true, status: 200, json: async () => ({}) });
    await apiFetch('/api/stats');
    expect(vi.getTimerCount()).toBe(0);
  });

  it('does not pass timeoutMs to fetch', async () => {
    fetch.mockResolvedValue({ ok: true, status: 204 });
    await apiFetch('/api/stats', { timeoutMs: 5 });
    expect(fetch.mock.calls[0][1]).not.toHaveProperty('timeoutMs');
  });
});

describe('isAbortError / isTimeoutError', () => {
  it('tells a cancel from a timeout and from other errors', () => {
    const abort = new DOMException('aborted', 'AbortError');
    const timeout = Object.assign(new Error('api.timeout'), { name: 'TimeoutError' });
    expect(isAbortError(abort)).toBe(true);
    expect(isAbortError(timeout)).toBe(false);
    expect(isAbortError(new Error('boom'))).toBe(false);
    expect(isAbortError(null)).toBe(false);
    expect(isTimeoutError(timeout)).toBe(true);
    expect(isTimeoutError(abort)).toBe(false);
    expect(isTimeoutError(undefined)).toBe(false);
  });
});

describe('latestRequest', () => {
  it('cancels the previous signal when a new one is taken', () => {
    const next = latestRequest();
    const first = next();
    const second = next();
    expect(first.aborted).toBe(true);
    expect(second.aborted).toBe(false);
  });

  it('keeps separate instances apart', () => {
    const a = latestRequest();
    const b = latestRequest();
    const sa = a();
    b();
    expect(sa.aborted).toBe(false);
  });
});

describe('withBusyButton', () => {
  it('disables the button while the action runs and enables it after', async () => {
    const btn = document.createElement('button');
    let during;
    await withBusyButton(btn, async () => { during = btn.disabled; });
    expect(during).toBe(true);
    expect(btn.disabled).toBe(false);
  });

  it('enables the button again when the action throws', async () => {
    const btn = document.createElement('button');
    await expect(withBusyButton(btn, async () => { throw new Error('x'); })).rejects.toThrow('x');
    expect(btn.disabled).toBe(false);
  });

  it('does not run the action a second time while the first runs', async () => {
    const btn = document.createElement('button');
    let release;
    const fn = vi.fn(() => new Promise(r => { release = r; }));
    const first = withBusyButton(btn, fn);
    await withBusyButton(btn, fn);
    expect(fn).toHaveBeenCalledTimes(1);
    release();
    await first;
  });

  it('returns the action result', async () => {
    const btn = document.createElement('button');
    expect(await withBusyButton(btn, async () => 42)).toBe(42);
  });
});

// ── UI language resolution (redesign) ───────────────────────────────────────
// Inlined from i18n.js (resolveUILang).

const UI_LANGS = ['en', 'de', 'zh'];
function resolveUILang(serverLang, localLang) {
  if (UI_LANGS.includes(serverLang)) return { lang: serverLang, seed: false };
  if (UI_LANGS.includes(localLang)) return { lang: localLang, seed: true };
  return { lang: 'en', seed: false };
}

describe('resolveUILang', () => {
  it('uses the server value when it is set', () => {
    expect(resolveUILang('de', 'en')).toEqual({ lang: 'de', seed: false });
    expect(resolveUILang('zh', null)).toEqual({ lang: 'zh', seed: false });
  });

  it('seeds the server from localStorage when the server has no value', () => {
    expect(resolveUILang('', 'de')).toEqual({ lang: 'de', seed: true });
  });

  it('falls back to English without seeding when nothing is known', () => {
    expect(resolveUILang('', null)).toEqual({ lang: 'en', seed: false });
    expect(resolveUILang(undefined, 'fr')).toEqual({ lang: 'en', seed: false });
  });

  it('ignores an unknown server value', () => {
    expect(resolveUILang('fr', 'de')).toEqual({ lang: 'de', seed: true });
  });
});

// ── tagGroupKey / groupTags ─────────────────────────────────────────────────
// Inlined from app.js. tagGroupKey mirrors tagGroupKey in service/db/words.go.

function tagGroupKey(name) {
  const s = String(name).toLowerCase();
  const i = s.indexOf('-');
  if (i < 0) return '';
  const prefix = s.slice(0, i);
  return prefix.startsWith('hsk') ? 'hsk' : prefix;
}

function groupTags(tags) {
  const groups = new Map();
  for (const tag of tags) {
    const key = tagGroupKey(tag);
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(tag);
  }
  return [...groups.entries()]
    .sort(([a], [b]) => (a === '') - (b === '') || a.localeCompare(b))
    .map(([key, list]) => ({ key, tags: list }));
}

describe('tagGroupKey', () => {
  it('uses the lowercased text before the first "-"', () => {
    expect(tagGroupKey('topic-food')).toBe('topic');
    expect(tagGroupKey('Topic-travel')).toBe('topic');
    expect(tagGroupKey('chapter-3-a')).toBe('chapter');
  });

  it('puts every hsk version into one group', () => {
    expect(tagGroupKey('hsk2-6')).toBe('hsk');
    expect(tagGroupKey('hsk3-1')).toBe('hsk');
    expect(tagGroupKey('hsk2-1_s')).toBe('hsk');
    expect(tagGroupKey('HSK3-2')).toBe('hsk');
  });

  it('puts tags without a prefix into the "" group', () => {
    expect(tagGroupKey('lesson1')).toBe('');
    expect(tagGroupKey('my words')).toBe('');
    expect(tagGroupKey('-odd')).toBe('');
  });
});

describe('groupTags', () => {
  it('sorts groups by key, keeps tag order and puts the "" group last', () => {
    expect(groupTags(['lesson1', 'topic-food', 'hsk2-2', 'hsk3-1', 'Topic-travel'])).toEqual([
      { key: 'hsk', tags: ['hsk2-2', 'hsk3-1'] },
      { key: 'topic', tags: ['topic-food', 'Topic-travel'] },
      { key: '', tags: ['lesson1'] },
    ]);
  });

  it('returns no groups for no tags', () => {
    expect(groupTags([])).toEqual([]);
  });
});

// ── timeoutSignal ────────────────────────────────────────────────────────────
// Inlined from app.js: the signal for direct fetch calls that skip apiFetch.

function timeoutSignal(ms = API_TIMEOUT_MS) {
  const ctrl = new AbortController();
  setTimeout(() => {
    const err = new Error(t('api.timeout'));
    err.name = 'TimeoutError';
    ctrl.abort(err);
  }, ms);
  return ctrl.signal;
}

describe('timeoutSignal', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('aborts with a TimeoutError after the default 15 s', () => {
    const signal = timeoutSignal();
    vi.advanceTimersByTime(API_TIMEOUT_MS - 1);
    expect(signal.aborted).toBe(false);
    vi.advanceTimersByTime(1);
    expect(signal.aborted).toBe(true);
    expect(isTimeoutError(signal.reason)).toBe(true);
    expect(signal.reason.message).toBe('api.timeout');
  });

  it('uses the given time', () => {
    const signal = timeoutSignal(5000);
    vi.advanceTimersByTime(5000);
    expect(signal.aborted).toBe(true);
  });
});
