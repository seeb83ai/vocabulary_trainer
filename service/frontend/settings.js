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
  { value: 'zh_to_transl_no_sound', label: 'Chinese (no sound) → Translation' },
  { value: 'zh_pinyin_to_transl', label: 'Chinese + Pinyin → Translation' },
  { value: 'voice_to_transl',    label: 'Voice → Translation' },
  { value: 'mask_pinyin',        label: 'Translation → Chinese (pinyin hint)' },
  { value: 'random',             label: 'Random' },
];

const CYCLE_STEP_OPTIONS = [
  { value: 'zh_pinyin_to_transl', label: 'Chinese + Pinyin → Translation' },
  { value: 'transl_to_zh',       label: 'Translation → Chinese' },
  { value: 'zh_to_transl',       label: 'Chinese → Translation' },
  { value: 'zh_to_transl_no_sound', label: 'Chinese (no sound) → Translation' },
  { value: 'voice_to_transl',    label: 'Voice → Translation' },
  { value: 'mask_pinyin',        label: 'Translation → Chinese (pinyin hint)' },
];

function populateCycleSelect(el, value) {
  el.innerHTML = '';
  const empty = document.createElement('option');
  empty.value = '';
  empty.textContent = '— (disabled)';
  if (value === '') empty.selected = true;
  el.appendChild(empty);
  for (const opt of CYCLE_STEP_OPTIONS) {
    const o = document.createElement('option');
    o.value = opt.value;
    o.textContent = opt.label;
    if (opt.value === value) o.selected = true;
    el.appendChild(o);
  }
}

// Learning buckets, in increasing-difficulty order — mirrors TIERS in app.js
// and the sm2 package's bucketOrder.
const BUCKETS = [
  { key: 'new',    label: 'New' },
  { key: '0-49',   label: 'Struggling' },
  { key: '50-69',  label: 'Learning' },
  { key: '70-84',  label: 'Practicing' },
  { key: '85-100', label: 'Mastered' },
];

// The 5 candidate modes governed by the random/cycle per-bucket setting, with
// the field name UserSettings uses and the built-in default range (mirrors
// sm2.DefaultRandomModeConfig).
const RANDOM_MODES = [
  { key: 'transl_to_zh',          field: 'random_mode_range_transl_to_zh',          def: 'new,50-69' },
  { key: 'zh_pinyin_to_transl',   field: 'random_mode_range_zh_pinyin_to_transl',   def: 'new,70-84' },
  { key: 'zh_to_transl',          field: 'random_mode_range_zh_to_transl',          def: '0-49,85-100' },
  { key: 'zh_to_transl_no_sound', field: 'random_mode_range_zh_to_transl_no_sound', def: '50-69,85-100' },
  { key: 'voice_to_transl',       field: 'random_mode_range_voice_to_transl',       def: '70-84,85-100' },
];

// Parses a random-mode-range setting value ("" | "off" | "<from>,<to>") into
// UI state, falling back to defaultValue when value is "" (unset).
function parseModeRangeForUI(value, defaultValue) {
  if (value === 'off') return { off: true, from: '', to: '' };
  const [from, to] = (value || defaultValue).split(',');
  return { off: false, from, to };
}

// Builds the random-mode-range setting value ("off" | "<from>,<to>") from UI state.
function modeRangeValue(off, from, to) {
  return off ? 'off' : `${from},${to}`;
}

function populateBucketSelect(el, value) {
  el.innerHTML = '';
  for (const b of BUCKETS) {
    const o = document.createElement('option');
    o.value = b.key;
    o.textContent = b.label;
    if (b.key === value) o.selected = true;
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
    for (let i = 0; i < 5; i++) {
      const el = document.getElementById('cycle-step-' + i);
      if (el) populateCycleSelect(el, cycleSteps[i] || '');
    }
    const advanceEl = document.getElementById('cycle-advance-on-success-only');
    if (advanceEl) advanceEl.checked = !!st.cycle_advance_on_success_only;

    // Random/cycle mode-by-bucket rows
    for (const m of RANDOM_MODES) {
      const fromEl = document.getElementById('random-mode-' + m.key + '-from');
      const toEl = document.getElementById('random-mode-' + m.key + '-to');
      const offEl = document.getElementById('random-mode-' + m.key + '-off');
      if (!fromEl || !toEl || !offEl) continue;
      const state = parseModeRangeForUI(st[m.field] || '', m.def);
      populateBucketSelect(fromEl, state.from || BUCKETS[0].key);
      populateBucketSelect(toEl, state.to || BUCKETS[BUCKETS.length - 1].key);
      offEl.checked = state.off;
      fromEl.disabled = state.off;
      toEl.disabled = state.off;
    }

    // Accept-as-correct mode
    const acmValue = st.accept_correct_mode || 'typo';
    document.querySelectorAll('input[name="accept-correct-mode"]').forEach(el => {
      el.checked = el.value === acmValue;
    });

    // New word confirmation checkboxes (default true when field absent)
    const requireZhEl = document.getElementById('require-zh');
    if (requireZhEl) requireZhEl.checked = st.new_word_require_zh !== false;
    const requireTransEl = document.getElementById('require-trans');
    if (requireTransEl) requireTransEl.checked = st.new_word_require_trans !== false;
    const retypeOnWrongEl = document.getElementById('retype-on-wrong');
    if (retypeOnWrongEl) retypeOnWrongEl.checked = !!st.retype_on_wrong;
    const blurPinyinEl = document.getElementById('blur-pinyin');
    if (blurPinyinEl) blurPinyinEl.checked = !!st.blur_pinyin;
    const noAutoVoiceOnBlurEl = document.getElementById('no-auto-voice-on-blur');
    if (noAutoVoiceOnBlurEl) noAutoVoiceOnBlurEl.checked = !!st.no_auto_voice_on_blur;
    const celebrateBucketChangeEl = document.getElementById('celebrate-bucket-change');
    if (celebrateBucketChangeEl) celebrateBucketChangeEl.checked = !!st.celebrate_bucket_change;
    const sentenceBlankEnabledEl = document.getElementById('sentence-blank-enabled');
    if (sentenceBlankEnabledEl) sentenceBlankEnabledEl.checked = !!st.sentence_blank_enabled;
    const sentenceBlankRatioEl = document.getElementById('sentence-blank-ratio');
    if (sentenceBlankRatioEl) sentenceBlankRatioEl.value = st.sentence_blank_ratio ?? 20;

    // Daily learning
    const maxNewEl = document.getElementById('max-new-words');
    if (maxNewEl) maxNewEl.value = st.max_new_words_per_day ?? 5;
    const cooldownEl = document.getElementById('new-word-cooldown');
    if (cooldownEl) cooldownEl.value = st.new_word_cooldown_minutes ?? 1;
    const skipVisEl = document.getElementById('skip-new-visible');
    if (skipVisEl) skipVisEl.checked = st.skip_new_words_visible !== false;
    const extendSessionEl = document.getElementById('extend-session-extra-words');
    if (extendSessionEl) extendSessionEl.checked = st.extend_session_with_extra_words !== false;
    setBaselineRow('baseline-due-today', st.baseline_due_today_enabled, st.baseline_due_today_value ?? 20);
    setBaselineRow('baseline-struggling', st.baseline_struggling_enabled, st.baseline_struggling_value ?? 10);
    setBaselineRow('baseline-learning', st.baseline_learning_enabled, st.baseline_learning_value ?? 20);
    setBaselineRow('baseline-new-bucket', st.baseline_new_bucket_enabled, st.baseline_new_bucket_value ?? 10);

    const gamEnabledEl = document.getElementById('gamification-enabled');
    if (gamEnabledEl) gamEnabledEl.checked = !!st.gamification_enabled;
    const gamFreqEl = document.getElementById('gamification-frequency');
    if (gamFreqEl) gamFreqEl.value = st.gamification_frequency ?? 5;
    const gameModeMismatchEl = document.getElementById('game-mode-mismatch');
    if (gameModeMismatchEl) gameModeMismatchEl.checked = st.game_mode_mismatch !== false;
    const gameModeNewestEl = document.getElementById('game-mode-newest');
    if (gameModeNewestEl) gameModeNewestEl.checked = st.game_mode_newest !== false;
    const gameModeHardestEl = document.getElementById('game-mode-hardest');
    if (gameModeHardestEl) gameModeHardestEl.checked = st.game_mode_hardest !== false;
    const gameModeLastMistakesEl = document.getElementById('game-mode-last-mistakes');
    if (gameModeLastMistakesEl) gameModeLastMistakesEl.checked = st.game_mode_last_mistakes !== false;

    const compThresholdEl = document.getElementById('component-coverage-threshold');
    if (compThresholdEl) compThresholdEl.value = st.component_coverage_threshold ?? 0;
    updateComponentCoverageSummary();

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
for (const id of ['cycle-step-0','cycle-step-1','cycle-step-2','cycle-step-3','cycle-step-4']) {
  const el = document.getElementById(id);
  if (el) populateCycleSelect(el, '');
}
for (const m of RANDOM_MODES) {
  const fromEl = document.getElementById('random-mode-' + m.key + '-from');
  const toEl = document.getElementById('random-mode-' + m.key + '-to');
  if (fromEl) populateBucketSelect(fromEl, '');
  if (toEl) populateBucketSelect(toEl, '');
}

// Wire each random-mode "Off" checkbox to enable/disable its bucket selects.
for (const m of RANDOM_MODES) {
  document.getElementById('random-mode-' + m.key + '-off')?.addEventListener('change', e => {
    const fromEl = document.getElementById('random-mode-' + m.key + '-from');
    const toEl = document.getElementById('random-mode-' + m.key + '-to');
    if (fromEl) fromEl.disabled = e.target.checked;
    if (toEl) toEl.disabled = e.target.checked;
  });
}

loadLanguages().then(() => loadSettings());
loadComponentCoverage();

// ── Training mode ──────────────────────────────────────────────────────────────

function buildCycleSequence() {
  const steps = [];
  for (let i = 0; i < 5; i++) {
    const v = document.getElementById('cycle-step-' + i)?.value;
    if (v) steps.push(v);
  }
  return steps.join(',');
}

function buildRandomModePayload() {
  const payload = {};
  for (const m of RANDOM_MODES) {
    const fromEl = document.getElementById('random-mode-' + m.key + '-from');
    const toEl = document.getElementById('random-mode-' + m.key + '-to');
    const offEl = document.getElementById('random-mode-' + m.key + '-off');
    if (!fromEl || !toEl || !offEl) continue;
    payload[m.field] = modeRangeValue(offEl.checked, fromEl.value, toEl.value);
  }
  return payload;
}

function buildModePayload() {
  return {
    prog_new:               document.getElementById('mode-prog-new')?.value        || 'transl_to_zh',
    prog_tier_struggling:   document.getElementById('mode-prog-struggling')?.value || 'transl_to_zh',
    prog_tier_learning:     document.getElementById('mode-prog-learning')?.value   || 'zh_pinyin_to_transl',
    prog_tier_practicing:   document.getElementById('mode-prog-practicing')?.value || 'zh_to_transl',
    prog_tier_mastered:     document.getElementById('mode-prog-mastered')?.value   || 'random',
    new_word_mode_0:        document.getElementById('mode-new-0')?.value           || 'transl_to_zh',
    new_word_mode_1:        document.getElementById('mode-new-1')?.value           || 'transl_to_zh',
    new_word_mode_2:        document.getElementById('mode-new-2')?.value           || 'zh_to_transl',
    cycle_sequence:                 buildCycleSequence(),
    cycle_advance_on_success_only:  !!(document.getElementById('cycle-advance-on-success-only')?.checked),
    new_word_require_zh:            !!(document.getElementById('require-zh')?.checked),
    new_word_require_trans: !!(document.getElementById('require-trans')?.checked),
    retype_on_wrong:        !!(document.getElementById('retype-on-wrong')?.checked),
    blur_pinyin:            !!(document.getElementById('blur-pinyin')?.checked),
    no_auto_voice_on_blur:  !!(document.getElementById('no-auto-voice-on-blur')?.checked),
    celebrate_bucket_change: !!(document.getElementById('celebrate-bucket-change')?.checked),
    ...buildRandomModePayload(),
    sentence_blank_enabled: !!(document.getElementById('sentence-blank-enabled')?.checked),
    sentence_blank_ratio:   parseInt(document.getElementById('sentence-blank-ratio')?.value || '20', 10),
  };
}

function buildDailyPayload() {
  return {
    max_new_words_per_day:         parseInt(document.getElementById('max-new-words')?.value || '5', 10),
    new_word_cooldown_minutes:     parseInt(document.getElementById('new-word-cooldown')?.value || '1', 10),
    skip_new_words_visible:        !!(document.getElementById('skip-new-visible')?.checked),
    extend_session_with_extra_words: !!(document.getElementById('extend-session-extra-words')?.checked),
    baseline_due_today_enabled:    !!(document.getElementById('baseline-due-today-enabled')?.checked),
    baseline_due_today_value:      parseInt(document.getElementById('baseline-due-today-value')?.value || '20', 10),
    baseline_struggling_enabled:   !!(document.getElementById('baseline-struggling-enabled')?.checked),
    baseline_struggling_value:     parseInt(document.getElementById('baseline-struggling-value')?.value || '10', 10),
    baseline_learning_enabled:     !!(document.getElementById('baseline-learning-enabled')?.checked),
    baseline_learning_value:       parseInt(document.getElementById('baseline-learning-value')?.value || '20', 10),
    baseline_new_bucket_enabled:   !!(document.getElementById('baseline-new-bucket-enabled')?.checked),
    baseline_new_bucket_value:     parseInt(document.getElementById('baseline-new-bucket-value')?.value || '10', 10),
  };
}

function setBaselineRow(prefix, enabled, value) {
  const cbEl = document.getElementById(prefix + '-enabled');
  const valEl = document.getElementById(prefix + '-value');
  if (cbEl) cbEl.checked = !!enabled;
  if (valEl) {
    valEl.value = value;
    valEl.disabled = !enabled;
  }
}

// Wire each baseline checkbox to enable/disable its threshold input.
for (const prefix of ['baseline-due-today', 'baseline-struggling', 'baseline-learning', 'baseline-new-bucket']) {
  document.getElementById(prefix + '-enabled')?.addEventListener('change', e => {
    const valEl = document.getElementById(prefix + '-value');
    if (valEl) valEl.disabled = !e.target.checked;
  });
}

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

// ── Component training threshold ────────────────────────────────────────────

let componentCoverageData = [];
let componentCoverageTotalWords = 0;
let componentCoverageWordCounts = {}; // word_id (string) -> number of trainable components

// computeCoverageSelection is pure: given the fetched per-component word-ID
// sets, the per-word trainable-component counts, and a candidate coverage-target
// percentage, greedily picks components — always the one that immediately fully
// covers the most additional words next (a word is fully covered when all its
// trainable components are selected; words with 0 components are auto-covered).
// Mirrors selectComponentsForCoverage in service/db/components.go.
function computeCoverageSelection(components, wordComponentCounts, totalWords, targetPct) {
  const totalComponents = components.length;
  if (totalWords <= 0 || targetPct <= 0) {
    return { selectedCount: 0, totalComponents };
  }
  const target = Math.ceil((targetPct / 100) * totalWords);

  // remaining[wid] = unselected component count still needed for that word.
  const remaining = {};
  let coveredCount = 0;
  for (const [wid, cnt] of Object.entries(wordComponentCounts)) {
    if (cnt === 0) {
      coveredCount++;
    } else {
      remaining[wid] = cnt;
    }
  }
  if (coveredCount >= target) return { selectedCount: 0, totalComponents };

  const candidates = components
    .map(c => ({ character: c.character, wordIds: c.word_ids || [] }))
    .sort((a, b) => (a.character < b.character ? -1 : a.character > b.character ? 1 : 0));

  let selectedCount = 0;
  while (coveredCount < target && candidates.length > 0) {
    let bestIdx = -1, bestGain = 0;
    for (let i = 0; i < candidates.length; i++) {
      let gain = 0;
      for (const wid of candidates[i].wordIds) {
        if (remaining[wid] === 1) gain++;
      }
      if (gain > bestGain) { bestGain = gain; bestIdx = i; }
    }
    if (bestIdx === -1) break;
    for (const wid of candidates[bestIdx].wordIds) {
      if (remaining[wid] !== undefined) {
        if (remaining[wid] === 1) {
          delete remaining[wid];
          coveredCount++;
        } else {
          remaining[wid]--;
        }
      }
    }
    candidates.splice(bestIdx, 1);
    selectedCount++;
  }
  return { selectedCount, totalComponents };
}

function updateComponentCoverageSummary() {
  const raw = parseFloat(document.getElementById('component-coverage-threshold')?.value || '0');
  const targetPct = isNaN(raw) ? 0 : raw;
  const { selectedCount, totalComponents } = computeCoverageSelection(componentCoverageData, componentCoverageWordCounts, componentCoverageTotalWords, targetPct);
  const summaryEl = document.getElementById('component-coverage-summary');
  if (!summaryEl) return;
  if (totalComponents === 0) {
    summaryEl.textContent = 'No components found in your vocabulary yet.';
  } else if (targetPct <= 0) {
    summaryEl.textContent = 'All ' + totalComponents + ' components would be added to training (no coverage target set).';
  } else {
    const pct = Math.round((selectedCount / totalComponents) * 100);
    summaryEl.textContent = selectedCount + ' of ' + totalComponents + ' components (' + pct +
      '%) would be added to training to cover ' + targetPct + '% of your ' + componentCoverageTotalWords + ' Chinese words.';
  }
}

async function loadComponentCoverage() {
  try {
    const res = await fetch('/api/components/coverage');
    if (!res.ok) return;
    const data = await res.json();
    componentCoverageData = data.components || [];
    componentCoverageWordCounts = data.word_component_counts || {};
    componentCoverageTotalWords = data.total_words || 0;
    updateComponentCoverageSummary();
  } catch { /* ignore */ }
}

document.getElementById('component-coverage-threshold')?.addEventListener('input', updateComponentCoverageSummary);

// ── Auto-save ────────────────────────────────────────────────────────────────
// Every settings card except Change Password and API Keys (both security-
// sensitive, explicit-submit flows) saves automatically as the user edits it.
// Whatever field changed, the full settings payload is sent — the PATCH
// handler treats most fields as plain (non-pointer) values that get zeroed
// out if omitted, so a partial payload would silently wipe unrelated settings.

function buildFullSettingsPayload() {
  return {
    primary_lang:        document.getElementById('primary-lang')?.value   || 'en',
    secondary_lang:      document.getElementById('secondary-lang')?.value || '',
    accept_correct_mode: (document.querySelector('input[name="accept-correct-mode"]:checked') || {}).value || 'typo',
    ...buildModePayload(),
    ...buildDailyPayload(),
    gamification_enabled:   !!(document.getElementById('gamification-enabled')?.checked),
    gamification_frequency: parseInt(document.getElementById('gamification-frequency')?.value || '5', 10),
    game_mode_mismatch:      !!(document.getElementById('game-mode-mismatch')?.checked),
    game_mode_newest:        !!(document.getElementById('game-mode-newest')?.checked),
    game_mode_hardest:       !!(document.getElementById('game-mode-hardest')?.checked),
    game_mode_last_mistakes: !!(document.getElementById('game-mode-last-mistakes')?.checked),
    component_coverage_threshold: parseFloat(document.getElementById('component-coverage-threshold')?.value || '0'),
  };
}

// Pure: mirrors the per-card validation the old individual Save buttons used
// to run before submitting. Only checks the concern owned by `group` — a
// card with a stale invalid value elsewhere on the page still saves its own
// change, exactly as the old per-button handlers behaved.
function localValidationError(group, payload) {
  if (group === 'lang' && payload.secondary_lang !== '' && payload.primary_lang === payload.secondary_lang) {
    return 'Primary and secondary languages must differ.';
  }
  if (group === 'daily' && (!payload.max_new_words_per_day || payload.max_new_words_per_day < 1)) {
    return 'New words per day must be at least 1.';
  }
  if (group === 'gamification' && (!payload.gamification_frequency || payload.gamification_frequency < 1 || payload.gamification_frequency > 1440)) {
    return 'Frequency must be between 1 and 1440 minutes.';
  }
  if (group === 'component-threshold' && (isNaN(payload.component_coverage_threshold) || payload.component_coverage_threshold < 0 || payload.component_coverage_threshold > 100)) {
    return 'Threshold must be between 0 and 100.';
  }
  return null;
}

const autoSaveTimers = {};

function scheduleAutoSave(group) {
  clearTimeout(autoSaveTimers[group]);
  autoSaveTimers[group] = setTimeout(() => autoSaveSettings(group), 400);
}

async function autoSaveSettings(group) {
  hideMsg(group + '-success'); hideMsg(group + '-error');
  const payload = buildFullSettingsPayload();

  const localErr = localValidationError(group, payload);
  if (localErr) {
    showMsg(group + '-error', localErr, true);
    return;
  }

  try {
    const res = await fetch('/api/settings', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const d = await res.json().catch(() => ({}));
      showMsg(group + '-error', d.error || 'Failed to save.', true);
      return;
    }
    showMsg(group + '-success', 'Saved.', false);
    setTimeout(() => hideMsg(group + '-success'), 2500);
  } catch {
    showMsg(group + '-error', 'Network error.', true);
  }
}

document.querySelectorAll('[data-settings-autosave] input, [data-settings-autosave] select').forEach(el => {
  const group = el.closest('[data-settings-autosave]')?.dataset.settingsAutosave;
  if (!group) return;
  const evt = (el.tagName === 'SELECT' || el.type === 'checkbox' || el.type === 'radio') ? 'change' : 'input';
  el.addEventListener(evt, () => scheduleAutoSave(group));
});
