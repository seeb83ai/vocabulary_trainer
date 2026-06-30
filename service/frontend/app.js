// Shared utilities used by both train.js and vocab.js

// Accuracy/attempt tier definitions — mirrors the progressive mode ladder.
const TIERS = [
  { key: 'new',    label: 'New',        i18nKey: 'tier.new',        desc: 'Learning phase',   color: '#8b5cf6', pill: 'bg-violet-100 text-violet-700' },
  { key: '0-49',   label: 'Struggling', i18nKey: 'tier.struggling', desc: 'EN → ZH',          color: '#ef4444', pill: 'bg-red-100 text-red-700'    },
  { key: '50-69',  label: 'Learning',   i18nKey: 'tier.learning',   desc: 'ZH + Pinyin → EN', color: '#f59e0b', pill: 'bg-amber-100 text-amber-700' },
  { key: '70-84',  label: 'Practicing', i18nKey: 'tier.practicing', desc: 'ZH → EN',          color: '#3b82f6', pill: 'bg-blue-100 text-blue-700'   },
  { key: '85-100', label: 'Mastered',   i18nKey: 'tier.mastered',   desc: 'All modes',        color: '#22c55e', pill: 'bg-green-100 text-green-700' },
];

// Returns the TIERS entry for a word, or null for brand-new words (0 attempts).
// Must stay in sync with tierFilter (db/db.go) and AccBuckets (GetWordStats).
//   New       : learning_new_word = true
//   Mastered  : ≥10 attempts AND acc ≥ 85 %
//   Practicing: ≥10 attempts AND 70 % ≤ acc < 85 %
//   Learning  : ≥3 attempts  AND acc ≥ 50 % (but not qualifying for Practicing/Mastered)
//   Struggling: everything else (< 3 attempts OR acc < 50 %)
function wordTier(totalCorrect, totalAttempts, learningNewWord, streakBonus) {
  if (totalAttempts === 0) return null;
  if (learningNewWord) return TIERS[0]; // "New"
  const acc = (totalCorrect + (streakBonus || 0)) / totalAttempts;
  if (totalAttempts >= 10 && acc >= 0.85) return TIERS[4];
  if (totalAttempts >= 10 && acc >= 0.70) return TIERS[3];
  if (totalAttempts >= 3  && acc >= 0.50) return TIERS[2];
  return TIERS[1];
}

function getModeLabel(mode) {
  return t('modeLabel.' + mode) || mode;
}

async function apiFetch(path, options = {}) {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  if (res.status === 401) {
    window.location.href = '/login';
    return;
  }
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

async function logout() {
  await fetch('/api/logout', { method: 'POST' });
  window.location.href = '/login';
}

// Show the logout button only when auth is enabled.
// Initialize language selector and apply translations.
document.addEventListener('DOMContentLoaded', async () => {
  // Mobile hamburger menu toggle
  const navBtn = document.getElementById('nav-menu-btn');
  const navMenu = document.getElementById('nav-menu');
  if (navBtn && navMenu) {
    navBtn.addEventListener('click', () => {
      const isHidden = navMenu.classList.contains('hidden');
      if (isHidden) {
        navMenu.classList.remove('hidden');
        navMenu.classList.add('flex', 'flex-col', 'gap-3', 'w-full', 'pt-3', 'mt-1', 'border-t', 'border-gray-100');
      } else {
        navMenu.classList.add('hidden');
        navMenu.classList.remove('flex', 'flex-col', 'gap-3', 'w-full', 'pt-3', 'mt-1', 'border-t', 'border-gray-100');
      }
    });
  }

  // Language selector
  const langSelect = document.getElementById('lang-select');
  if (langSelect) {
    langSelect.value = getUILang();
    applyTranslations();
    langSelect.addEventListener('change', () => {
      setUILang(langSelect.value);
      applyTranslations();
      // Fire a custom event so page-specific JS can re-render dynamic content
      document.dispatchEvent(new Event('langchange'));
    });
  }

  try {
    const res = await fetch('/api/auth/status');
    if (res.ok) {
      const btn = document.getElementById('logout-btn');
      if (btn) {
        btn.classList.remove('hidden');
        btn.addEventListener('click', logout);
      }
    }
  } catch (_) {}
});

function $(id) {
  return document.getElementById(id);
}

function show(id) {
  const el = $(id);
  if (el) el.classList.remove('hidden');
}

function hide(id) {
  const el = $(id);
  if (el) el.classList.add('hidden');
}

function setText(id, text) {
  const el = $(id);
  if (el) el.textContent = text;
}

// playAudio plays the server-cached MP3 for wordId.
// Falls back silently to the Web Speech API if the MP3 is unavailable.
function playAudio(wordId, zhText) {
  const audio = new Audio(`/api/audio/${wordId}`);
  audio.play().catch(() => {
    if ('speechSynthesis' in window) {
      const u = new SpeechSynthesisUtterance(zhText);
      u.lang = 'zh-CN';
      speechSynthesis.speak(u);
    }
  });
}

function escHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// ── In-app GitHub issue reporting ───────────────────────────────────────────

// buildIssueMetadata collects non-sensitive client context for an issue report.
// Pure function (takes the window object) so it is unit-testable.
function buildIssueMetadata(win) {
  return {
    user_agent: (win.navigator && win.navigator.userAgent) || '',
    viewport: `${win.innerWidth}x${win.innerHeight}`,
    locale: (win.navigator && win.navigator.language) || '',
    timestamp: new Date().toISOString(),
  };
}

// validateIssueForm returns an i18n error key for the first problem found, or
// '' when the form is valid. Pure function for unit testing.
function validateIssueForm(form) {
  const valid = ['idea', 'bug', 'question', 'misc'];
  if (!valid.includes(form.category)) return 'issue.errCategory';
  if (!form.title || !form.title.trim()) return 'issue.errTitle';
  if (!form.description || !form.description.trim()) return 'issue.errDescription';
  return '';
}

// Lazy-load the vendored html2canvas only when a screenshot is needed.
let _html2canvasPromise = null;
function loadHtml2Canvas() {
  if (window.html2canvas) return Promise.resolve(window.html2canvas);
  if (_html2canvasPromise) return _html2canvasPromise;
  _html2canvasPromise = new Promise((resolve, reject) => {
    const s = document.createElement('script');
    s.src = '/html2canvas.min.js';
    s.onload = () => resolve(window.html2canvas);
    s.onerror = () => reject(new Error('failed to load html2canvas'));
    document.head.appendChild(s);
  });
  return _html2canvasPromise;
}

// captureScreenshot renders the page to a PNG data URL, hiding the report UI so
// it does not appear in the capture. Returns '' on failure (best-effort).
async function captureScreenshot() {
  const h2c = await loadHtml2Canvas();
  if (!h2c) return '';
  const btn = $('issue-report-btn');
  const modal = $('issue-modal');
  const btnWasHidden = btn && btn.classList.contains('hidden');
  const modalWasHidden = modal && modal.classList.contains('hidden');
  if (btn) btn.classList.add('hidden');
  if (modal) modal.classList.add('hidden');
  try {
    const canvas = await h2c(document.body, { logging: false, useCORS: true, scale: 1 });
    return canvas.toDataURL('image/png');
  } finally {
    if (btn && !btnWasHidden) btn.classList.remove('hidden');
    if (modal && !modalWasHidden) modal.classList.remove('hidden');
  }
}

async function initIssueReporter() {
  const btn = $('issue-report-btn');
  const modal = $('issue-modal');
  if (!btn || !modal) return;

  // Only enable when the server reports the feature is configured.
  let enabled = false;
  try {
    const res = await fetch('/api/github/config');
    if (res.ok) enabled = (await res.json()).enabled === true;
  } catch (_) { /* feature unavailable */ }
  if (!enabled) return;
  btn.classList.remove('hidden');

  let screenshotDataUrl = '';

  async function refreshScreenshot() {
    const preview = $('issue-screenshot-preview');
    const include = $('issue-include-screenshot');
    screenshotDataUrl = '';
    preview.classList.add('hidden');
    if (!include || !include.checked) return;
    try {
      screenshotDataUrl = await captureScreenshot();
      if (screenshotDataUrl) {
        preview.src = screenshotDataUrl;
        preview.classList.remove('hidden');
      }
    } catch (_) { /* screenshot is best-effort */ }
  }

  btn.addEventListener('click', async () => {
    setText('issue-status', '');
    await refreshScreenshot();
    show('issue-modal');
  });

  $('issue-cancel').addEventListener('click', () => hide('issue-modal'));
  modal.addEventListener('click', e => { if (e.target === modal) hide('issue-modal'); });
  $('issue-include-screenshot').addEventListener('change', refreshScreenshot);

  $('issue-submit').addEventListener('click', async () => {
    const form = {
      category: $('issue-category').value,
      title: $('issue-title').value,
      description: $('issue-description').value,
    };
    const errKey = validateIssueForm(form);
    if (errKey) { setText('issue-status', t(errKey)); return; }

    setText('issue-status', t('issue.submitting'));
    const payload = {
      ...form,
      page_url: location.pathname,
      meta: buildIssueMetadata(window),
    };
    const include = $('issue-include-screenshot');
    if (include && include.checked && screenshotDataUrl) {
      payload.screenshot_png_b64 = screenshotDataUrl;
    }
    try {
      const res = await apiFetch('/api/github/issues', { method: 'POST', body: JSON.stringify(payload) });
      const statusEl = $('issue-status');
      $('issue-title').value = '';
      $('issue-description').value = '';
      hide('issue-modal');
    } catch (err) {
      setText('issue-status', t('issue.error') + ' ' + err.message);
    }
  });
}

document.addEventListener('DOMContentLoaded', initIssueReporter);
