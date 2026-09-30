// train-settings.js — filters / settings / tags / langs

let _saveFiltersTimer = null;
function scheduleFilterSave() {
  clearTimeout(_saveFiltersTimer);
  _saveFiltersTimer = setTimeout(saveTrainFilters, 500);
}
async function saveTrainFilters() {
  try {
    await apiFetch('/api/training-filters', {
      method: 'PATCH',
      body: JSON.stringify({
        mode: selectedMode,
        bucket: selectedBucket,
        langs: selectedLangs,
        mnemonics: includeMnemonics,
        components: includeComponents,
        tags: selectedTags,
      }),
    });
  } catch (_) {}
}

// updateSessionChip writes "Mode · Level" (plus the tag count) on the
// session bar chip that opens the session sheet.
function updateSessionChip() {
  const el = document.getElementById('session-chip-label');
  if (!el) return;
  const tier = TIERS.find(x => x.key === selectedBucket);
  const level = tier ? t(tier.i18nKey) : t('tier.allLevels');
  let label = `${t('mode.' + selectedMode)} · ${level}`;
  if (selectedTags.length) label += ` · ${t('session.tagCount', { n: selectedTags.length })}`;
  el.textContent = label;
}

function setPressed(el, on) {
  if (el) el.setAttribute('aria-pressed', on ? 'true' : 'false');
}

function applyModeButtons() {
  document.querySelectorAll('.overlay-mode-btn').forEach(btn => setPressed(btn, btn.dataset.mode === selectedMode));
  updateSessionChip();
}

function applyTierPills() {
  document.querySelectorAll('.overlay-tier-btn').forEach(btn => setPressed(btn, btn.dataset.bucket === selectedBucket));
  updateSessionChip();
}

function applyMnemonicPill() {
  setPressed($('overlay-mnemonics-pill'), includeMnemonics);
}

function applyComponentPill() {
  setPressed($('overlay-components-pill'), includeComponents);
}

async function loadTrainSettings() {
  await _settingsPromise;
  try {
    const st = await apiFetch('/api/settings');
    requireNewWordZh    = st.new_word_require_zh    !== false;
    requireNewWordTrans = st.new_word_require_trans !== false;
    wrongAnswerRetryMode = st.wrong_answer_retry_mode || 'off';
  } catch (_) { /* keep defaults */ }
  // Re-apply filter UI in case _settingsPromise updated state after DOMContentLoaded.
  applyModeButtons();
  applyTierPills();
  applyMnemonicPill();
  applyComponentPill();
}

function applyLangChips(allLangs) {
  const overlayContainer = $('overlay-lang-chips');
  overlayContainer.innerHTML = '';
  $('overlay-langs-section').classList.toggle('hidden', allLangs.length < 2);
  const names = { en: 'English', de: 'Deutsch' };
  for (const lang of allLangs) {
    const chip = document.createElement('button');
    chip.type = 'button';
    chip.className = 'overlay-lang-btn ui-chip';
    chip.dataset.lang = lang;
    setPressed(chip, selectedLangs.includes(lang));
    chip.textContent = names[lang] || lang.toUpperCase();
    chip.addEventListener('click', () => toggleLang(lang, allLangs));
    overlayContainer.appendChild(chip);
  }
}

function toggleLang(lang, allLangs) {
  if (selectedLangs.includes(lang)) {
    // Don't allow deselecting the last lang
    if (selectedLangs.length <= 1) return;
    selectedLangs = selectedLangs.filter(l => l !== lang);
  } else {
    selectedLangs.push(lang);
  }
  localStorage.setItem('quizLangs', JSON.stringify(selectedLangs));
  scheduleFilterSave();
  applyLangChips(allLangs);
  loadNextCard();
}

async function loadLangs() {
  let availableLangs = [];
  try {
    availableLangs = await apiFetch('/api/quiz/langs');
  } catch (_) {}
  await _settingsPromise;
  // Only show langs the user has configured, in primary-first order
  const userLangs = [userPrimaryLang, userSecondaryLang].filter(l => l && availableLangs.includes(l));
  const allLangs = userLangs.length > 0 ? userLangs : availableLangs;
  // Prune stale selections
  selectedLangs = selectedLangs.filter(l => allLangs.includes(l));
  if (selectedLangs.length === 0) {
    selectedLangs = allLangs.length > 0 ? [allLangs[0]] : [userPrimaryLang];
  }
  localStorage.setItem('quizLangs', JSON.stringify(selectedLangs));
  scheduleFilterSave();
  applyLangChips(allLangs);
}

async function loadTrainTags() {
  let allTags = [];
  try {
    allTags = await apiFetch('/api/tags');
  } catch (_) {}

  // Remove stale tags from selection
  selectedTags = selectedTags.filter(t => allTags.includes(t));
  localStorage.setItem('quizTags', JSON.stringify(selectedTags));
  updateSessionChip();

  const overlayTagChips = $('overlay-tag-chips');
  overlayTagChips.innerHTML = '';
  if (allTags.length === 0) {
    $('overlay-tags-section').classList.add('hidden');
    return;
  }
  // Tag changes apply when the session sheet closes (Done / backdrop).
  for (const tag of allTags) {
    const chip = document.createElement('button');
    chip.type = 'button';
    chip.className = 'overlay-tag-btn ui-chip';
    setPressed(chip, selectedTags.includes(tag));
    chip.textContent = tag;
    chip.addEventListener('click', () => {
      if (selectedTags.includes(tag)) {
        selectedTags = selectedTags.filter(t => t !== tag);
      } else {
        selectedTags.push(tag);
      }
      localStorage.setItem('quizTags', JSON.stringify(selectedTags));
      scheduleFilterSave();
      loadTrainTags();
    });
    overlayTagChips.appendChild(chip);
  }
  $('overlay-tags-section').classList.remove('hidden');
}
