// HIBP k-anonymity check
async function isPasswordPwned(password) {
  if (!crypto?.subtle) return false;
  try {
    const buf = await crypto.subtle.digest('SHA-1', new TextEncoder().encode(password));
    const hex = Array.from(new Uint8Array(buf))
      .map(b => b.toString(16).padStart(2, '0')).join('').toUpperCase();
    const prefix = hex.slice(0, 5), suffix = hex.slice(5);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 2000);
    let res;
    try {
      res = await fetch('https://api.pwnedpasswords.com/range/' + prefix,
        { headers: { 'Add-Padding': 'true' }, signal: controller.signal });
    } finally {
      clearTimeout(timer);
    }
    if (!res.ok) return false;
    const text = await res.text();
    return text.split('\r\n').some(l => l.split(':')[0] === suffix);
  } catch {
    return false;
  }
}

const MODE_OPTIONS = [
  { value: 'transl_to_zh',       label: 'Translation → Chinese' },
  { value: 'zh_to_transl',       label: 'Chinese → Translation' },
  { value: 'zh_pinyin_to_transl', label: 'Chinese + Pinyin → Translation' },
  { value: 'mask_pinyin',        label: 'Translation → Chinese (pinyin hint)' },
  { value: 'random',             label: 'Random' },
];

const CYCLE_STEP_OPTIONS = [
  { value: 'zh_pinyin_to_transl', label: 'Chinese + Pinyin → Translation' },
  { value: 'transl_to_zh',       label: 'Translation → Chinese' },
  { value: 'zh_to_transl',       label: 'Chinese → Translation' },
  { value: 'mask_pinyin',        label: 'Translation → Chinese (pinyin hint)' },
];

function populateCycleSelect(el, value) {
  el.innerHTML = '';
  for (const opt of CYCLE_STEP_OPTIONS) {
    const o = document.createElement('option');
    o.value = opt.value;
    o.textContent = opt.label;
    if (opt.value === value) o.selected = true;
    el.appendChild(o);
  }
}

function populateModeSelect(el, value) {
  el.innerHTML = '';
  for (const opt of MODE_OPTIONS) {
    const o = document.createElement('option');
    o.value = opt.value;
    o.textContent = opt.label;
    if (opt.value === value) o.selected = true;
    el.appendChild(o);
  }
}

function showMsg(id, text, isError) {
  const el = document.getElementById(id);
  if (!el) return;
  el.textContent = text || el.textContent;
  el.classList.remove('hidden');
  if (isError) {
    el.classList.add('text-red-600', 'bg-red-50', 'border-red-200');
    el.classList.remove('text-green-700', 'bg-green-50', 'border-green-200');
  } else {
    el.classList.add('text-green-700', 'bg-green-50', 'border-green-200');
    el.classList.remove('text-red-600', 'bg-red-50', 'border-red-200');
  }
}

function hideMsg(id) {
  const el = document.getElementById(id);
  if (el) el.classList.add('hidden');
}

// Load account info
fetch('/api/me').then(r => {
  if (r.status === 401) { window.location.replace('/'); return null; }
  return r.json();
}).then(data => {
  if (data) document.getElementById('account-email').textContent = data.email;
}).catch(() => {});

// ── Language preferences ───────────────────────────────────────────────────────

async function loadLanguages() {
  try {
    const res = await fetch('/api/quiz/langs');
    if (!res.ok) return;
    const langs = await res.json(); // e.g. ["en", "de"]
    const primaryEl = document.getElementById('primary-lang');
    const secondaryEl = document.getElementById('secondary-lang');
    const names = { en: 'English', de: 'German', zh: 'Chinese', fr: 'French', es: 'Spanish' };
    primaryEl.innerHTML = '';
    // Secondary starts with a "None" sentinel so the user can clear it
    secondaryEl.innerHTML = '<option value="">— None —</option>';
    for (const code of langs) {
      const label = names[code] || code;
      const o1 = document.createElement('option');
      o1.value = code; o1.textContent = label;
      primaryEl.appendChild(o1);
      const o2 = document.createElement('option');
      o2.value = code; o2.textContent = label;
      secondaryEl.appendChild(o2);
    }
  } catch { /* ignore */ }
}

async function loadSettings() {
  try {
    const res = await fetch('/api/settings');
    if (!res.ok) return;
    const st = await res.json();

    // Language prefs
    const primaryEl = document.getElementById('primary-lang');
    const secondaryEl = document.getElementById('secondary-lang');
    if (primaryEl) primaryEl.value = st.primary_lang || 'en';
    if (secondaryEl) secondaryEl.value = st.secondary_lang ?? '';

    // Progressive tier selects
    populateModeSelect(document.getElementById('mode-prog-new'),        st.prog_new            || 'transl_to_zh');
    populateModeSelect(document.getElementById('mode-prog-struggling'), st.prog_tier_struggling || 'transl_to_zh');
    populateModeSelect(document.getElementById('mode-prog-learning'),   st.prog_tier_learning   || 'zh_pinyin_to_transl');
    populateModeSelect(document.getElementById('mode-prog-practicing'), st.prog_tier_practicing || 'zh_to_transl');
    populateModeSelect(document.getElementById('mode-prog-mastered'),   st.prog_tier_mastered   || 'random');

    // New-word step selects
    populateModeSelect(document.getElementById('mode-new-0'), st.new_word_mode_0 || 'transl_to_zh');
    populateModeSelect(document.getElementById('mode-new-1'), st.new_word_mode_1 || 'transl_to_zh');
    populateModeSelect(document.getElementById('mode-new-2'), st.new_word_mode_2 || 'zh_to_transl');

    // Cycle step selects
    const defaultSeq = 'zh_pinyin_to_transl,transl_to_zh,zh_to_transl';
    const cycleSteps = (st.cycle_sequence || defaultSeq).split(',');
    populateCycleSelect(document.getElementById('cycle-step-0'), cycleSteps[0] || 'zh_pinyin_to_transl');
    populateCycleSelect(document.getElementById('cycle-step-1'), cycleSteps[1] || 'transl_to_zh');
    populateCycleSelect(document.getElementById('cycle-step-2'), cycleSteps[2] || 'zh_to_transl');

    // API key status
    if (st.deepl_key_masked) {
      const el = document.getElementById('deepl-key-status');
      if (el) { el.textContent = 'Current: ' + st.deepl_key_masked; el.classList.remove('hidden'); }
    }
    if (st.llm_key_masked) {
      const el = document.getElementById('llm-key-status');
      if (el) { el.textContent = 'Current: ' + st.llm_key_masked; el.classList.remove('hidden'); }
    }
    const providerEl = document.getElementById('llm-provider');
    if (providerEl && st.llm_provider) {
      providerEl.value = st.llm_provider;
      toggleLocalURLRow(st.llm_provider);
    }
    const localURLEl = document.getElementById('llm-local-url');
    if (localURLEl && st.llm_local_url) localURLEl.value = st.llm_local_url;
  } catch { /* ignore */ }
}

// Populate mode selects before loading settings (so options exist)
for (const id of ['mode-prog-new','mode-prog-struggling','mode-prog-learning','mode-prog-practicing','mode-prog-mastered',
                   'mode-new-0','mode-new-1','mode-new-2']) {
  const el = document.getElementById(id);
  if (el) populateModeSelect(el, '');
}
for (const id of ['cycle-step-0','cycle-step-1','cycle-step-2']) {
  const el = document.getElementById(id);
  if (el) populateCycleSelect(el, '');
}

loadLanguages().then(() => loadSettings());

// Language save
document.getElementById('lang-save-btn')?.addEventListener('click', async () => {
  hideMsg('lang-success'); hideMsg('lang-error');
  const primary = document.getElementById('primary-lang').value;
  const secondary = document.getElementById('secondary-lang').value;
  if (secondary !== '' && primary === secondary) {
    showMsg('lang-error', 'Primary and secondary languages must differ.', true);
    return;
  }
  // Collect current mode values to avoid overwriting them
  const modePayload = buildModePayload();
  const payload = { primary_lang: primary, secondary_lang: secondary, ...modePayload };
  try {
    const res = await fetch('/api/settings', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const d = await res.json();
      showMsg('lang-error', d.error || 'Failed to save.', true);
    } else {
      showMsg('lang-success', 'Saved.', false);
    }
  } catch {
    showMsg('lang-error', 'Network error.', true);
  }
});

// ── Training mode ──────────────────────────────────────────────────────────────

function buildCycleSequence() {
  const steps = [
    document.getElementById('cycle-step-0')?.value || 'zh_pinyin_to_transl',
    document.getElementById('cycle-step-1')?.value || 'transl_to_zh',
    document.getElementById('cycle-step-2')?.value || 'zh_to_transl',
  ];
  return steps.join(',');
}

function buildModePayload() {
  return {
    prog_new:             document.getElementById('mode-prog-new')?.value        || 'transl_to_zh',
    prog_tier_struggling: document.getElementById('mode-prog-struggling')?.value || 'transl_to_zh',
    prog_tier_learning:   document.getElementById('mode-prog-learning')?.value   || 'zh_pinyin_to_transl',
    prog_tier_practicing: document.getElementById('mode-prog-practicing')?.value || 'zh_to_transl',
    prog_tier_mastered:   document.getElementById('mode-prog-mastered')?.value   || 'random',
    new_word_mode_0:      document.getElementById('mode-new-0')?.value           || 'transl_to_zh',
    new_word_mode_1:      document.getElementById('mode-new-1')?.value           || 'transl_to_zh',
    new_word_mode_2:      document.getElementById('mode-new-2')?.value           || 'zh_to_transl',
    cycle_sequence:       buildCycleSequence(),
  };
}

document.getElementById('mode-save-btn')?.addEventListener('click', async () => {
  hideMsg('mode-success'); hideMsg('mode-error');
  const payload = {
    primary_lang:   document.getElementById('primary-lang')?.value   || 'en',
    secondary_lang: document.getElementById('secondary-lang')?.value || 'de',
    ...buildModePayload(),
  };
  try {
    const res = await fetch('/api/settings', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const d = await res.json();
      showMsg('mode-error', d.error || 'Failed to save.', true);
    } else {
      showMsg('mode-success', 'Saved.', false);
    }
  } catch {
    showMsg('mode-error', 'Network error.', true);
  }
});

// ── Cycle mode ────────────────────────────────────────────────────────────────

document.getElementById('cycle-save-btn')?.addEventListener('click', async () => {
  hideMsg('cycle-success'); hideMsg('cycle-error');
  const payload = {
    primary_lang:   document.getElementById('primary-lang')?.value   || 'en',
    secondary_lang: document.getElementById('secondary-lang')?.value || '',
    ...buildModePayload(),
  };
  try {
    const res = await fetch('/api/settings', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const d = await res.json();
      showMsg('cycle-error', d.error || 'Failed to save.', true);
    } else {
      showMsg('cycle-success', 'Saved.', false);
    }
  } catch {
    showMsg('cycle-error', 'Network error.', true);
  }
});

// ── API keys ───────────────────────────────────────────────────────────────────

function toggleLocalURLRow(provider) {
  const row = document.getElementById('llm-local-url-row');
  if (row) row.classList.toggle('hidden', provider !== 'local');
}

document.getElementById('llm-provider')?.addEventListener('change', e => {
  toggleLocalURLRow(e.target.value);
});

async function saveAPIKeys(clearAll) {
  hideMsg('apikey-success'); hideMsg('apikey-error');
  const payload = clearAll ? {
    deepl_key: '', llm_provider: '', llm_key: '', llm_local_url: '',
  } : {
    deepl_key:    document.getElementById('deepl-key')?.value     || '',
    llm_provider: document.getElementById('llm-provider')?.value  || '',
    llm_key:      document.getElementById('llm-key')?.value       || '',
    llm_local_url: document.getElementById('llm-local-url')?.value || '',
  };
  try {
    const res = await fetch('/api/settings/api-keys', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const d = await res.json().catch(() => ({}));
      showMsg('apikey-error', d.error || 'Failed to save API keys.', true);
      return;
    }
    const st = await res.json();
    showMsg('apikey-success', clearAll ? 'API keys cleared.' : 'API keys saved.', false);

    // Update masked status
    const deeplStatusEl = document.getElementById('deepl-key-status');
    if (deeplStatusEl) {
      if (st.deepl_key_masked) {
        deeplStatusEl.textContent = 'Current: ' + st.deepl_key_masked;
        deeplStatusEl.classList.remove('hidden');
      } else {
        deeplStatusEl.classList.add('hidden');
      }
    }
    const llmStatusEl = document.getElementById('llm-key-status');
    if (llmStatusEl) {
      if (st.llm_key_masked) {
        llmStatusEl.textContent = 'Current: ' + st.llm_key_masked;
        llmStatusEl.classList.remove('hidden');
      } else {
        llmStatusEl.classList.add('hidden');
      }
    }
    // Clear inputs
    ['deepl-key', 'llm-key'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.value = '';
    });
    if (clearAll) {
      const providerEl = document.getElementById('llm-provider');
      if (providerEl) { providerEl.value = ''; toggleLocalURLRow(''); }
      const localEl = document.getElementById('llm-local-url');
      if (localEl) localEl.value = '';
    }
  } catch {
    showMsg('apikey-error', 'Network error.', true);
  }
}

document.getElementById('apikey-save-btn')?.addEventListener('click', () => saveAPIKeys(false));
document.getElementById('apikey-clear-btn')?.addEventListener('click', () => saveAPIKeys(true));

// ── Change password ────────────────────────────────────────────────────────────

document.getElementById('pw-form').addEventListener('submit', async e => {
  e.preventDefault();
  const errEl = document.getElementById('pw-error');
  const okEl = document.getElementById('pw-success');
  errEl.classList.add('hidden');
  okEl.classList.add('hidden');

  const btn = document.getElementById('pw-btn');
  const currentPw = document.getElementById('pw-current').value;
  const newPw = document.getElementById('pw-new').value;
  const confirmPw = document.getElementById('pw-confirm').value;

  if (newPw !== confirmPw) {
    errEl.textContent = 'New passwords do not match.';
    errEl.classList.remove('hidden');
    return;
  }
  if (newPw.length < 8) {
    errEl.textContent = 'New password must be at least 8 characters.';
    errEl.classList.remove('hidden');
    return;
  }

  btn.disabled = true;
  btn.textContent = 'Checking password…';

  const pwned = await isPasswordPwned(newPw);
  if (pwned) {
    errEl.textContent = 'This password has appeared in a data breach. Please choose a different password.';
    errEl.classList.remove('hidden');
    btn.disabled = false;
    btn.textContent = 'Update Password';
    return;
  }

  btn.textContent = 'Updating…';

  try {
    const res = await fetch('/api/change-password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ current_password: currentPw, new_password: newPw })
    });
    const data = await res.json();

    if (!res.ok) {
      errEl.textContent = data.error || 'Failed to update password.';
      errEl.classList.remove('hidden');
    } else {
      okEl.classList.remove('hidden');
      document.getElementById('pw-form').reset();
    }
  } catch {
    errEl.textContent = 'Network error. Please try again.';
    errEl.classList.remove('hidden');
  }

  btn.disabled = false;
  btn.textContent = 'Update Password';
});
