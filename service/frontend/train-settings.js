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

function applyModeButtons() {
  document.querySelectorAll('.mode-btn').forEach(btn => {
    const active = btn.dataset.mode === selectedMode;
    btn.className = active
      ? 'mode-btn px-3 py-1 rounded-full text-sm font-medium transition bg-blue-600 text-white'
      : 'mode-btn px-3 py-1 rounded-full text-sm font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  });
  // Mobile: update the single visible label
  const mobileLabel = document.getElementById('mode-mobile-label');
  if (mobileLabel) mobileLabel.textContent = t('mode.' + selectedMode);
  // Overlay: apply same active/inactive styling
  document.querySelectorAll('.overlay-mode-btn').forEach(btn => {
    const active = btn.dataset.mode === selectedMode;
    btn.className = active
      ? 'overlay-mode-btn px-4 py-2 rounded-full text-sm font-medium transition bg-blue-600 text-white'
      : 'overlay-mode-btn px-4 py-2 rounded-full text-sm font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  });
}

function applyTierPills() {
  document.querySelectorAll('.tier-pill, .overlay-tier-btn').forEach(btn => {
    const active = btn.dataset.bucket === selectedBucket;
    const isMini = btn.classList.contains('tier-pill');
    btn.className = (isMini
      ? `tier-pill px-2.5 py-0.5 rounded-full text-xs font-medium transition `
      : `overlay-tier-btn px-3 py-1.5 rounded-full text-sm font-medium transition `) +
      (active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200');
  });
  // Mobile: update the single level chip next to the mode chip
  const levelLabel = document.getElementById('level-mobile-label');
  if (levelLabel) {
    const tier = TIERS.find(t => t.key === selectedBucket);
    if (tier) {
      levelLabel.textContent = t('tier.' + tier.label.toLowerCase());
      levelLabel.className = 'px-3 py-1 rounded-full text-sm font-medium bg-blue-600 text-white';
    } else {
      levelLabel.textContent = t('tier.all');
      levelLabel.className = 'px-3 py-1 rounded-full text-sm font-medium bg-gray-100 text-gray-600';
    }
  }
}

// quickStartPlan decides which one-click onboarding buttons to offer for a
// given list of importable library tag names.
function quickStartPlan(tagNames) {
  const has = n => tagNames.includes(n);
  return { hsk1: has('hsk1'), hsk23: ['hsk2', 'hsk3'].filter(has) };
}

function applyMnemonicPill() {
  const active = includeMnemonics;
  const cls = active
    ? 'px-2.5 py-0.5 rounded-full text-xs font-medium transition bg-blue-600 text-white'
    : 'px-2.5 py-0.5 rounded-full text-xs font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  const overlayCls = active
    ? 'px-4 py-2 rounded-full text-sm font-medium transition bg-blue-600 text-white'
    : 'px-4 py-2 rounded-full text-sm font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  const label = t('filter.mnemonicsOn');
  const pill = $('mnemonics-pill');
  if (pill) { pill.className = cls; pill.textContent = label; }
  const overlayPill = $('overlay-mnemonics-pill');
  if (overlayPill) { overlayPill.className = overlayCls; overlayPill.textContent = label; }
}

function applyComponentPill() {
  const active = includeComponents;
  const cls = active
    ? 'px-2.5 py-0.5 rounded-full text-xs font-medium transition bg-blue-600 text-white'
    : 'px-2.5 py-0.5 rounded-full text-xs font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  const overlayCls = active
    ? 'px-4 py-2 rounded-full text-sm font-medium transition bg-blue-600 text-white'
    : 'px-4 py-2 rounded-full text-sm font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  const label = t('filter.componentsOn');
  const pill = $('components-pill');
  if (pill) { pill.className = cls; pill.textContent = label; }
  const overlayPill = $('overlay-components-pill');
  if (overlayPill) { overlayPill.className = overlayCls; overlayPill.textContent = label; }
}

async function loadTrainSettings() {
  await _settingsPromise;
  try {
    const st = await apiFetch('/api/settings');
    requireNewWordZh    = st.new_word_require_zh    !== false;
    requireNewWordTrans = st.new_word_require_trans !== false;
    retypeOnWrong        = !!st.retype_on_wrong;
  } catch (_) { /* keep defaults */ }
  // Re-apply filter UI in case _settingsPromise updated state after DOMContentLoaded.
  applyModeButtons();
  applyTierPills();
  applyMnemonicPill();
  applyComponentPill();
}

function applyLangChips(allLangs) {
  const desktopContainer = $('lang-chips-desktop');
  desktopContainer.querySelectorAll('.lang-pill').forEach(p => p.remove());
  const overlayContainer = $('overlay-lang-chips');
  overlayContainer.innerHTML = '';

  $('overlay-langs-section').classList.toggle('hidden', allLangs.length < 2);

  for (const lang of allLangs) {
    const active = selectedLangs.includes(lang);

    // Desktop chip — insert before the separator (third child: label, sep, mnemonics-pill)
    const sep = desktopContainer.querySelector('span.text-gray-300');
    const pill = document.createElement('button');
    pill.className = `lang-pill px-2.5 py-0.5 rounded-full text-xs font-medium transition ${active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}`;
    pill.textContent = lang.toUpperCase();
    pill.addEventListener('click', () => toggleLang(lang, allLangs));
    desktopContainer.insertBefore(pill, sep);

    // Overlay chip
    const overlayPill = document.createElement('button');
    overlayPill.className = `overlay-lang-btn px-3 py-1.5 rounded-full text-sm font-medium transition ${active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}`;
    overlayPill.textContent = lang.toUpperCase();
    overlayPill.addEventListener('click', () => toggleLang(lang, allLangs));
    overlayContainer.appendChild(overlayPill);
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

  // Desktop: render tag pills into the tag bar
  const tagBar = $('tag-filter-bar');
  const desktopContainer = $('tag-chips-desktop');
  desktopContainer.querySelectorAll('.tag-pill').forEach(p => p.remove());
  if (allTags.length === 0) {
    tagBar.classList.remove('sm:block');
    $('overlay-tags-section').classList.add('hidden');
    return;
  }
  // Keep the base "hidden" class so the bar stays hidden on mobile; only
  // toggle "sm:block" to show/hide it on desktop.
  tagBar.classList.add('sm:block');
  for (const tag of allTags) {
    const pill = document.createElement('button');
    const active = selectedTags.includes(tag);
    pill.className = `tag-pill px-2.5 py-0.5 rounded-full text-xs font-medium transition ${active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}`;
    pill.textContent = tag;
    pill.addEventListener('click', () => {
      if (selectedTags.includes(tag)) {
        selectedTags = selectedTags.filter(t => t !== tag);
      } else {
        selectedTags.push(tag);
      }
      localStorage.setItem('quizTags', JSON.stringify(selectedTags));
      scheduleFilterSave();
      loadTrainTags();
      loadNextCard();
    });
    desktopContainer.appendChild(pill);
  }

  // Overlay: render all tag chips with toggle behaviour
  const overlayTagChips = $('overlay-tag-chips');
  overlayTagChips.innerHTML = '';
  for (const tag of allTags) {
    const pill = document.createElement('button');
    const active = selectedTags.includes(tag);
    pill.className = `overlay-tag-btn px-3 py-1.5 rounded-full text-sm font-medium transition ${active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}`;
    pill.textContent = tag;
    pill.addEventListener('click', () => {
      if (selectedTags.includes(tag)) {
        selectedTags = selectedTags.filter(t => t !== tag);
      } else {
        selectedTags.push(tag);
      }
      localStorage.setItem('quizTags', JSON.stringify(selectedTags));
      scheduleFilterSave();
      loadTrainTags();
    });
    overlayTagChips.appendChild(pill);
  }
  $('overlay-tags-section').classList.remove('hidden');
}
