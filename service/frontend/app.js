// Shared utilities used by both train.js and vocab.js

// Accuracy/attempt tier definitions — mirrors the progressive mode ladder.
const TIERS = [
  { key: 'new',    label: 'New',        i18nKey: 'tier.new',        desc: 'Learning phase',   color: '#8b5cf6', pill: 'bg-violet-100 text-violet-700', icon: '🌰' },
  { key: '0-49',   label: 'Struggling', i18nKey: 'tier.struggling', desc: 'EN → ZH',          color: '#ef4444', pill: 'bg-red-100 text-red-700',    icon: '🌱' },
  { key: '50-69',  label: 'Learning',   i18nKey: 'tier.learning',   desc: 'ZH + Pinyin → EN', color: '#f59e0b', pill: 'bg-amber-100 text-amber-700', icon: '🌿' },
  { key: '70-84',  label: 'Practicing', i18nKey: 'tier.practicing', desc: 'ZH → EN',          color: '#3b82f6', pill: 'bg-blue-100 text-blue-700',   icon: '🌳' },
  { key: '85-100', label: 'Mastered',   i18nKey: 'tier.mastered',   desc: 'All modes',        color: '#22c55e', pill: 'bg-green-100 text-green-700', icon: '🌸' },
];

// Builds a single icon for the word's current tier — the compact inline
// indicator shown on every result screen (vocab word, HMM, component).
// Pure — testable in isolation. The celebration screen builds its own
// old/new icon pair directly (via TIERS) since it crossfades between them.
function tierIconHTML(tier, prevTier) {
  if (!tier) return '';
  const entry = TIERS.find(e => e.label === tier);
  if (!entry) return '';
  const changed = !!prevTier && prevTier !== tier;
  const cls = 'tier-icon' + (changed ? ' tier-icon-changed' : '');
  return `<span class="${cls}" title="${escHtml(tier)}">${entry.icon}</span>`;
}

// Renders the single current-tier icon into a container element. Caller is
// responsible for show()/hide()'ing the element based on whether a tier is
// present. Shared by train.js's vocab/HMM/component result screens so the
// bucket indicator isn't reimplemented per card type.
function renderTierIcon(el, tier, prevTier) {
  if (!el) return;
  el.innerHTML = tierIconHTML(tier, prevTier);
}

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

// changeUILang switches the UI language, re-renders translated text and stores
// the choice on the server (Settings → Languages → App language).
function changeUILang(lang) {
  if (!UI_LANGS.includes(lang)) return Promise.resolve();
  setUILang(lang);
  applyTranslations();
  document.dispatchEvent(new Event('langchange'));
  return apiFetch('/api/settings/ui-lang', { method: 'PUT', body: JSON.stringify({ ui_lang: lang }) });
}

// syncUILang applies the server-stored UI language (it wins over the
// localStorage cache) and seeds the server from the cache for users who
// picked a language before it was stored server-side.
async function syncUILang() {
  let st;
  try {
    const res = await fetch('/api/settings');
    if (!res.ok) return;
    st = await res.json();
  } catch (_) { return; }
  const { lang, seed } = resolveUILang(st.ui_lang, localStorage.getItem('uiLang'));
  if (lang !== getUILang()) {
    setUILang(lang);
    applyTranslations();
    document.dispatchEvent(new Event('langchange'));
  }
  if (seed) {
    fetch('/api/settings/ui-lang', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ui_lang: lang }),
    }).catch(() => {});
  }
}

function openMoreSheet() {
  show('more-sheet');
  document.body.style.overflow = 'hidden';
}

function closeMoreSheet() {
  hide('more-sheet');
  document.body.style.overflow = '';
}

document.addEventListener('DOMContentLoaded', async () => {
  applyTranslations();

  const moreBtn = $('tab-more');
  if (moreBtn) moreBtn.addEventListener('click', openMoreSheet);
  const moreBackdrop = $('more-sheet-backdrop');
  if (moreBackdrop) moreBackdrop.addEventListener('click', closeMoreSheet);
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && $('more-sheet') && !$('more-sheet').classList.contains('hidden')) closeMoreSheet();
  });

  if ($('app-sidebar')) syncUILang();

  try {
    const res = await fetch('/api/auth/status');
    if (res.ok) {
      for (const id of ['logout-btn', 'more-logout-btn']) {
        const btn = $(id);
        if (!btn) continue;
        btn.classList.remove('hidden');
        btn.addEventListener('click', logout);
      }
    }
  } catch (_) {}

  initFullscreenToggle();
});

function fullscreenAvailable() {
  return !!document.documentElement.requestFullscreen && !window.matchMedia('(display-mode: standalone)').matches;
}

function toggleFullscreen() {
  if (document.fullscreenElement) {
    document.exitFullscreen();
  } else {
    document.documentElement.requestFullscreen().catch(() => {});
  }
}

// Fullscreen can't be forced on page load — browsers only grant it from a
// user gesture — so the Train page shows a toggle button and every page gets
// a "Fullscreen" row in the phone More sheet. Both stay hidden when the
// Fullscreen API isn't available (e.g. already running installed/standalone,
// where there's no browser chrome left to hide).
function initFullscreenToggle() {
  if (!fullscreenAvailable()) return;
  const btn = $('fullscreen-toggle-btn');
  const row = $('more-fullscreen-btn');
  if (btn) btn.classList.remove('hidden');
  if (row) row.classList.remove('hidden');
  const updateLabel = () => {
    const active = !!document.fullscreenElement;
    const key = active ? 'fullscreen.exitTitle' : 'fullscreen.enterTitle';
    for (const el of [btn, row]) {
      if (!el) continue;
      el.setAttribute('aria-pressed', String(active));
      el.title = t(key);
      el.setAttribute('data-i18n-title', key);
    }
  };
  if (btn) btn.addEventListener('click', toggleFullscreen);
  if (row) row.addEventListener('click', () => { closeMoreSheet(); toggleFullscreen(); });
  document.addEventListener('fullscreenchange', updateLabel);
  updateLabel();
}

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
// Returns the Audio element so callers can track/stop it (e.g. auto-play).
function playAudio(wordId, zhText) {
  const audio = new Audio(`/api/audio/${wordId}`);
  audio.play().catch(() => {
    if ('speechSynthesis' in window) {
      const u = new SpeechSynthesisUtterance(zhText);
      u.lang = 'zh-CN';
      speechSynthesis.speak(u);
    }
  });
  return audio;
}

// playComponentAudio plays TTS audio for a single component character.
// Uses a dedicated endpoint (/api/audio/component/{char}) whose cached files
// are named c_{hex}.mp3, distinct from word audio files ({word_id}.mp3).
// Returns the Audio element so callers can track/stop it (e.g. auto-play).
function playComponentAudio(char) {
  const audio = new Audio(`/api/audio/component/${encodeURIComponent(char)}`);
  audio.play().catch(() => {
    if ('speechSynthesis' in window) {
      const u = new SpeechSynthesisUtterance(char);
      u.lang = 'zh-CN';
      speechSynthesis.speak(u);
    }
  });
  return audio;
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

// screenshotOptions returns html2canvas options for full-page or visible-only capture.
function screenshotOptions(fullPage, win) {
  const base = { logging: false, useCORS: true, scale: 1 };
  if (fullPage) return base;
  return { ...base, height: win.innerHeight, y: win.scrollY };
}

// captureScreenshot renders the page to a PNG data URL, hiding the report UI so
// it does not appear in the capture. Returns '' on failure (best-effort).
async function captureScreenshot(fullPage) {
  const h2c = await loadHtml2Canvas();
  if (!h2c) return '';
  const modal = $('issue-modal');
  const modalWasHidden = modal && modal.classList.contains('hidden');
  if (modal) modal.classList.add('hidden');
  try {
    const canvas = await h2c(document.body, screenshotOptions(fullPage, window));
    return canvas.toDataURL('image/png');
  } finally {
    if (modal && !modalWasHidden) modal.classList.remove('hidden');
  }
}

async function initIssueReporter() {
  const modal = $('issue-modal');
  if (!modal) return;

  // Only enable when the server reports the feature is configured.
  let enabled = false;
  try {
    const res = await fetch('/api/github/config');
    if (res.ok) enabled = (await res.json()).enabled === true;
  } catch (_) { /* feature unavailable */ }
  if (!enabled) return;
  show('nav-report');
  show('tab-report');

  let screenshotDataUrl = '';
  let submitting = false;

  const titleEl = $('issue-title');
  const descEl = $('issue-description');
  const includeEl = $('issue-include-screenshot');
  const fullPageEl = $('issue-fullpage-screenshot');

  function setCategory(cat) {
    $('issue-category').value = cat;
    modal.querySelectorAll('[data-issue-cat]').forEach(b => {
      b.setAttribute('aria-pressed', String(b.dataset.issueCat === cat));
    });
    descEl.placeholder = t('issue.descPh.' + cat);
  }

  function updateCounters() {
    setText('issue-title-count', `${titleEl.value.length}/120`);
    setText('issue-description-count', `${descEl.value.length}/4000`);
  }

  function setFieldError(field, on) {
    const input = $('issue-' + field);
    const err = $('issue-' + field + '-err');
    if (input) input.classList.toggle('rp-invalid', on);
    if (err) err.classList.toggle('hidden', !on);
  }

  function clearErrors() {
    setFieldError('title', false);
    setFieldError('description', false);
    hide('issue-status');
  }

  function setSubmitState(state) {
    const btn = $('issue-submit');
    const busy = state === 'busy';
    btn.disabled = busy;
    btn.classList.toggle('is-busy', busy);
    $('issue-submit-spinner').classList.toggle('hidden', !busy);
    const key = busy ? 'issue.submitting' : state === 'retry' ? 'issue.retry' : 'issue.submit';
    const label = $('issue-submit-label');
    label.setAttribute('data-i18n', key);
    label.textContent = t(key);
  }

  function syncShotControls() {
    const on = includeEl.checked;
    $('issue-shot-body').classList.toggle('hidden', !on);
    $('issue-shot-visible').setAttribute('aria-pressed', String(!fullPageEl.checked));
    $('issue-shot-full').setAttribute('aria-pressed', String(fullPageEl.checked));
    $('issue-preview-wrap').classList.toggle('rp-full', fullPageEl.checked);
  }

  async function refreshScreenshot() {
    const preview = $('issue-screenshot-preview');
    screenshotDataUrl = '';
    preview.classList.add('hidden');
    syncShotControls();
    if (!includeEl.checked) return;
    show('issue-preview-wrap');
    show('issue-capturing');
    try {
      screenshotDataUrl = await captureScreenshot(fullPageEl.checked);
      if (screenshotDataUrl) {
        preview.src = screenshotDataUrl;
        preview.classList.remove('hidden');
      } else {
        hide('issue-preview-wrap');
      }
    } catch (_) {
      hide('issue-preview-wrap'); /* screenshot is best-effort */
    } finally {
      hide('issue-capturing');
    }
  }

  function closeReport() {
    if (submitting) return;
    hide('issue-modal');
  }

  async function openReport() {
    closeMoreSheet();
    clearErrors();
    setSubmitState('idle');
    show('issue-form-view');
    hide('issue-success');
    setCategory($('issue-category').value || 'bug');
    updateCounters();
    await refreshScreenshot();
    show('issue-modal');
  }

  for (const id of ['nav-report', 'tab-report']) {
    const el = $(id);
    if (el) el.addEventListener('click', openReport);
  }
  modal.querySelectorAll('[data-issue-cat]').forEach(b => {
    b.addEventListener('click', () => setCategory(b.dataset.issueCat));
  });
  titleEl.addEventListener('input', () => { updateCounters(); setFieldError('title', false); });
  descEl.addEventListener('input', () => { updateCounters(); setFieldError('description', false); });
  $('issue-close').addEventListener('click', closeReport);
  $('issue-cancel').addEventListener('click', closeReport);
  $('issue-done').addEventListener('click', closeReport);
  modal.addEventListener('click', e => { if (e.target === modal) closeReport(); });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && !modal.classList.contains('hidden')) closeReport();
  });
  includeEl.addEventListener('change', refreshScreenshot);
  $('issue-shot-visible').addEventListener('click', () => {
    if (!fullPageEl.checked) return;
    fullPageEl.checked = false;
    refreshScreenshot();
  });
  $('issue-shot-full').addEventListener('click', () => {
    if (fullPageEl.checked) return;
    fullPageEl.checked = true;
    refreshScreenshot();
  });
  document.addEventListener('langchange', () => setCategory($('issue-category').value || 'bug'));

  $('issue-submit').addEventListener('click', async () => {
    const form = {
      category: $('issue-category').value,
      title: titleEl.value,
      description: descEl.value,
    };
    clearErrors();
    const errKey = validateIssueForm(form);
    if (errKey === 'issue.errTitle') { setFieldError('title', true); return; }
    if (errKey === 'issue.errDescription') { setFieldError('description', true); return; }
    if (errKey) { setText('issue-status', t(errKey)); show('issue-status'); return; }

    const payload = {
      ...form,
      page_url: location.pathname,
      meta: buildIssueMetadata(window),
    };
    if (includeEl.checked && screenshotDataUrl) {
      payload.screenshot_png_b64 = screenshotDataUrl;
    }
    submitting = true;
    setSubmitState('busy');
    try {
      const res = await apiFetch('/api/github/issues', { method: 'POST', body: JSON.stringify(payload) });
      setText('issue-success-num', res && res.number ? `#${res.number}` : '');
      setText('issue-success-title', form.title.trim());
      const link = $('issue-success-link');
      if (res && res.issue_url) {
        link.href = res.issue_url;
        link.classList.remove('hidden');
      } else {
        link.classList.add('hidden');
      }
      titleEl.value = '';
      descEl.value = '';
      updateCounters();
      hide('issue-form-view');
      show('issue-success');
      setSubmitState('idle');
    } catch (err) {
      setText('issue-status', t('issue.error') + ' ' + err.message);
      show('issue-status');
      setSubmitState('retry');
    } finally {
      submitting = false;
    }
  });
}

document.addEventListener('DOMContentLoaded', initIssueReporter);
