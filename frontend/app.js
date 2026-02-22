// Shared utilities used by both train.js and vocab.js

const MODE_LABELS = {
  'en_to_zh': 'English → Chinese',
  'zh_to_en': 'Chinese → English',
  'zh_pinyin_to_en': 'Chinese + Pinyin → English',
};

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
document.addEventListener('DOMContentLoaded', async () => {
  try {
    const res = await fetch('/api/auth/status');
    if (res.ok) {
      const btn = document.getElementById('logout-btn');
      if (btn) btn.classList.remove('hidden');
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
    .replace(/"/g, '&quot;');
}
