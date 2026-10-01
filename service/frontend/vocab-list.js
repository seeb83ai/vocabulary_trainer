// vocab-list.js — word table, pagination, filters, view switching, init wiring

// Vocabulary management page logic

// Language settings — loaded once on init from /api/settings
let primaryLang = 'en';
let secondaryLang = '';  // empty means no secondary language

const LANG_NAMES = { en: 'English', de: 'German', zh: 'Chinese', fr: 'French', es: 'Spanish' };


let currentPage = 1;
let perPage = parseInt(localStorage.getItem('vocabPerPage')) || 20;
let searchQuery = '';
let sortBy = '';
let sortDir = 'desc';
let editingWordId = null;
let searchTimer = null;
let allTags = [];
let formTags = [];
let selectedFilterTags = [];
let reviewFilterActive = false;
let hideUnseenActive = true;
let selectedTierFilter = '';
let dueFilter = '';
let missingLangFilter = '';

let currentView = 'words'; // 'words' | 'components'
let compPage = 1;
let compSearchTimer = null;
let compReviewFilterActive = false;

// Import tab state
let importSelectedTags = [];
let importApplyTags = [];
let importSourceTagsLoaded = false;
let importAllTags = [];          // full tag list from server
let importFilterLangs = new Set(); // 'en' | 'de'
let importFilterMode = 'any';    // 'any' | 'all'

// Tags tab state
let tagsLoaded = false;

// CSV upload state
let csvUploadTags = [];
let csvUploadWordCount = 0;
let csvUploadFile = null;

async function loadWords() {
  const params = new URLSearchParams({
    q: searchQuery,
    page: currentPage,
    per_page: perPage,
  });
  if (sortBy) {
    params.set('sort', sortBy);
    params.set('order', sortDir);
  }
  if (selectedFilterTags.length) {
    params.set('tags', selectedFilterTags.join(','));
  }
  if (reviewFilterActive) {
    params.set('review', '1');
  }
  if (hideUnseenActive) {
    params.set('hide_unseen', '1');
  }
  if (selectedTierFilter) {
    params.set('bucket', selectedTierFilter);
  }
  if (dueFilter) {
    params.set('due', dueFilter);
  }
  if (missingLangFilter) {
    params.set('missing_lang', missingLangFilter);
  }
  try {
    const data = await apiFetch(`/api/words?${params}`);
    updateFilterChips();
    renderTable(data.words);
    renderPagination(data.total, data.page, data.per_page);
  } catch (e) {
    alert('Failed to load words: ' + e.message);
  }
}

function updateSortHeaders() {
  const sel = $('sort-select');
  if (sel) sel.value = sortBy;
  const dirBtn = $('sort-dir-btn');
  if (dirBtn) {
    dirBtn.textContent = sortDir === 'asc' ? '↑' : '↓';
    dirBtn.dataset.dir = sortDir;
    dirBtn.title = t(sortDir === 'asc' ? 'vocab.sortAsc' : 'vocab.sortDesc');
    dirBtn.disabled = !sortBy;
  }
}

// meaningLineHTML renders one "EN hello · hi" line of a word row; a missing
// language reads "missing" in red.
function meaningLineHTML(lang, texts) {
  const body = texts.length
    ? texts.map(escHtml).join(' · ')
    : `<span class="vb-missing">${escHtml(t('vocab.missing'))}</span>`;
  return `<span><span class="vb-lang">${escHtml(lang.toUpperCase())}</span>${body}</span>`;
}

function renderTable(words) {
  updateSortHeaders();
  const list = $('words-tbody');
  list.innerHTML = '';
  if (!words || words.length === 0) {
    list.innerHTML = `<div class="vb-empty">${escHtml(t('vocab.noEntries'))}</div>`;
    return;
  }
  for (const word of words) {
    const row = document.createElement('div');
    row.className = 'vb-row btn-edit';
    row.setAttribute('role', 'listitem');
    row.tabIndex = 0;
    row.dataset.id = word.id;
    const tr = word.translations || {};
    const lines = [meaningLineHTML(primaryLang, tr[primaryLang] || [])];
    if (secondaryLang) lines.push(meaningLineHTML(secondaryLang, tr[secondaryLang] || []));
    row.innerHTML = `
      <div class="vb-row-main">
        <div class="vb-row-head">
          <span class="vb-row-zh font-hanzi">${escHtml(word.zh_text)}</span>
          ${word.pinyin ? `<span class="vb-row-py">${escHtml(word.pinyin)}</span>` : ''}
          <button type="button" class="btn-play vb-play" data-id="${word.id}" data-zh="${escHtml(word.zh_text)}" title="${escHtml(t('card.readAloud'))}" aria-label="${escHtml(t('card.readAloud'))}"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M11 5 6 9H3v6h3l5 4zM15.5 8.5a5 5 0 0 1 0 7"/></svg></button>
          ${word.needs_review ? `<span class="vb-badge vb-badge-review">${escHtml(t('vocab.needsReview'))}</span>` : ''}
          ${word.known ? `<span class="vb-badge vb-badge-known">${escHtml(t('vocab.known'))}</span>` : ''}
          ${crossRefBadge(word.is_also_component, t('vocab.alsoComponent'))}
          ${(word.tags || []).map(tag => `<span class="vb-badge vb-badge-tag">${escHtml(tag)}</span>`).join('')}
        </div>
        <div class="vb-row-lines">${lines.join('')}</div>
      </div>
      <div class="vb-row-side">${renderTierBadge(word)}${renderDue(word)}</div>`;
    list.appendChild(row);
  }

  list.querySelectorAll('.btn-play').forEach(btn => {
    btn.addEventListener('click', e => {
      e.stopPropagation();
      playAudio(parseInt(btn.dataset.id), btn.dataset.zh);
    });
  });
  list.querySelectorAll('.vb-row').forEach(row => {
    const open = () => openEditForm(words.find(w => w.id == row.dataset.id));
    row.addEventListener('click', open);
    row.addEventListener('keydown', e => { if (e.key === 'Enter') open(); });
  });
}

function renderPagination(total, page, ppSize) {
  const totalPages = Math.max(1, Math.ceil(total / ppSize));
  $('prev-btn').disabled = page <= 1;
  $('next-page-btn').disabled = page >= totalPages;

  // Page number links
  const pageNums = $('page-numbers');
  pageNums.innerHTML = '';
  const maxVisible = window.innerWidth < 640 ? 3 : 7;
  let start = Math.max(1, page - Math.floor(maxVisible / 2));
  let end = Math.min(totalPages, start + maxVisible - 1);
  if (end - start < maxVisible - 1) start = Math.max(1, end - maxVisible + 1);

  if (start > 1) {
    pageNums.appendChild(makePageBtn(1, page));
    if (start > 2) {
      const dots = document.createElement('span');
      dots.className = 'tr-muted-sm';
      dots.textContent = '…';
      pageNums.appendChild(dots);
    }
  }
  for (let i = start; i <= end; i++) {
    pageNums.appendChild(makePageBtn(i, page));
  }
  if (end < totalPages) {
    if (end < totalPages - 1) {
      const dots = document.createElement('span');
      dots.className = 'tr-muted-sm';
      dots.textContent = '…';
      pageNums.appendChild(dots);
    }
    pageNums.appendChild(makePageBtn(totalPages, page));
  }

  // Total count
  setText('page-total', t('vocab.entries', { n: total }));

  // Per-page dropdown
  $('per-page-select').value = ppSize;
}

function makePageBtn(pageNum, activePage) {
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.textContent = pageNum;
  btn.className = 'vb-page-num' + (pageNum === activePage ? ' is-active' : '');
  btn.addEventListener('click', () => {
    currentPage = pageNum;
    loadWords();
  });
  return btn;
}

// Cross-reference badge: shown on the Words tab when a word's character is
// also tracked as a component, and on the Components tab when a component's
// character is also stored as a word.
function crossRefBadge(show, label) {
  return show
    ? `<span class="vb-badge vb-badge-cross">${escHtml(label)}</span>`
    : '';
}

// tierChipFor renders a tier chip (icon + label) for a TIERS entry, or the
// grey "Unseen" chip.
function tierChipFor(tier, pct) {
  if (!tier) return `<span class="tier-chip"><span class="tier-icon" aria-hidden="true">○</span>${escHtml(t('vocab.unseen'))}</span>`;
  const pctHtml = pct === undefined ? '' : `<span class="vb-pct">${pct}%</span>`;
  return `<span class="tier-chip tier-chip-${tier.label.toLowerCase()}"><span class="tier-icon" aria-hidden="true">${tier.icon}</span>${escHtml(t(tier.i18nKey))}</span>${pctHtml}`;
}

function renderTierBadge(word) {
  const tier = wordTier(word.total_correct, word.total_attempts, word.learning_new_word, word.streak_bonus);
  if (!tier || word.learning_new_word) return tierChipFor(tier);
  const pct = Math.round((word.total_correct + (word.streak_bonus || 0)) / word.total_attempts * 100);
  return tierChipFor(tier, pct);
}

// dueLabel returns the row's due text as an i18n key (+ params), or null for
// unseen words and missing dates. Pure for unit testing.
function dueLabel(word, now) {
  if (!word.total_attempts || !word.due_date) return null;
  const due = new Date(word.due_date);
  if (isNaN(due.getTime())) return null;
  const diffDays = Math.round((due - now) / 86400000);
  if (diffDays <= 0) return { key: 'vocab.dueTodayLabel', today: true };
  if (diffDays === 1) return { key: 'vocab.dueTomorrowLabel', today: false };
  return { key: 'vocab.inDays', n: diffDays, today: false };
}

function renderDue(word) {
  const d = dueLabel(word, new Date());
  if (!d) return '';
  return `<span class="vb-due${d.today ? ' is-today' : ''}">${escHtml(t(d.key, { n: d.n }))}</span>`;
}

function renderFilterTags() {
  const bar = $('filter-tags-bar');
  const pills = bar.querySelectorAll('.filter-tag-pill');
  pills.forEach(p => p.remove());
  if (allTags.length === 0) {
    bar.classList.add('hidden');
    return;
  }
  bar.classList.remove('hidden');
  for (const tag of allTags) {
    const pill = document.createElement('button');
    const active = selectedFilterTags.includes(tag);
    pill.type = 'button';
    pill.className = 'filter-tag-pill ui-chip vb-chip';
    pill.setAttribute('aria-pressed', active ? 'true' : 'false');
    pill.textContent = tag;
    pill.addEventListener('click', () => {
      if (selectedFilterTags.includes(tag)) {
        selectedFilterTags = selectedFilterTags.filter(t => t !== tag);
      } else {
        selectedFilterTags.push(tag);
      }
      currentPage = 1;
      renderFilterTags();
      loadWords();
    });
    bar.appendChild(pill);
  }
}

function renderTierFilter() {
  const bar = $('filter-tier-bar');
  bar.querySelectorAll('.tier-filter-pill').forEach(p => p.remove());
  for (const tier of TIERS) {
    const pill = document.createElement('button');
    const active = selectedTierFilter === tier.key;
    pill.type = 'button';
    pill.className = 'tier-filter-pill ui-chip vb-chip';
    pill.setAttribute('aria-pressed', active ? 'true' : 'false');
    pill.innerHTML = `<span aria-hidden="true">${tier.icon}</span>${escHtml(t(tier.i18nKey))}`;
    pill.addEventListener('click', () => {
      selectedTierFilter = selectedTierFilter === tier.key ? '' : tier.key;
      currentPage = 1;
      renderTierFilter();
      loadWords();
    });
    bar.appendChild(pill);
  }
}

function setChip(id, on) {
  const el = $(id);
  if (el) el.setAttribute('aria-pressed', on ? 'true' : 'false');
}

// hasWordFilters reports whether any narrowing filter (other than the
// hide-unseen toggle) is active — "All" is pressed when none is.
function hasWordFilters() {
  return !!(dueFilter || reviewFilterActive || missingLangFilter || selectedTierFilter || selectedFilterTags.length);
}

function updateFilterChips() {
  setChip('filter-all-btn', !hasWordFilters());
  setChip('hide-unseen-btn', hideUnseenActive);
  setChip('review-filter-btn', reviewFilterActive);
  ['today', 'tomorrow', 'known'].forEach(key => setChip('due-' + key + '-btn', dueFilter === key));
  setChip('missing-de-chip', !!secondaryLang && missingLangFilter === secondaryLang);
  const missingChip = $('missing-de-chip');
  if (missingChip) {
    missingChip.classList.toggle('hidden', !secondaryLang);
    if (secondaryLang) missingChip.textContent = t('vocab.missingLangChip', { lang: secondaryLang.toUpperCase() });
  }
  if ($('missing-lang-select')) $('missing-lang-select').value = missingLangFilter;
}

function updateHideUnseenBtn() { updateFilterChips(); }
function updateReviewFilterBtn() { updateFilterChips(); }
function updateDueFilterBtns() { updateFilterChips(); }

function updateCompReviewFilterBtn() {
  setChip('comp-review-filter-btn', compReviewFilterActive);
}

// ── Sheet (Add / Edit / Import / Tags / Component) and header menu ────────────

function openVocabSheet(tab) {
  if (tab) switchTab(tab);
  show('vocab-sheet');
  document.body.style.overflow = 'hidden';
}

function closeVocabSheet() {
  hide('vocab-sheet');
  document.body.style.overflow = '';
}

function toggleVocabMenu(open) {
  const menu = $('vocab-menu');
  const btn = $('vocab-menu-btn');
  const willOpen = open === undefined ? menu.classList.contains('hidden') : open;
  menu.classList.toggle('hidden', !willOpen);
  btn.setAttribute('aria-expanded', String(willOpen));
}

// loadVocabSummary fills the header's "N words · M due today".
async function loadVocabSummary() {
  try {
    const st = await apiFetch('/api/quiz/stats');
    setText('vocab-summary', t(st.total === 1 ? 'vocab.summaryOne' : 'vocab.summary', { n: st.total, due: st.due_today }));
  } catch (_) { /* keep placeholder */ }
}

// ── Component view ────────────────────────────────────────────────────────────

function switchView(view) {
  currentView = view;
  const isWords = view === 'words';
  $('words-table-wrap').classList.toggle('hidden', !isWords);
  $('components-table-wrap').classList.toggle('hidden', isWords);
  $('word-filters-row').classList.toggle('hidden', !isWords);
  $('vocab-more-filters-toggle').classList.toggle('hidden', !isWords);
  if (!isWords) $('vocab-more-filters').classList.add('hidden');
  $('component-filters-row').classList.toggle('hidden', isWords);
  if (isWords) renderFilterTags(); else $('filter-tags-bar').classList.add('hidden');

  setChip('view-words-btn', isWords);
  setChip('view-components-btn', !isWords);

  currentPage = 1;
  compPage = 1;
  if (isWords) loadWords(); else loadComponents();
}

document.addEventListener('DOMContentLoaded', () => {
  loadLangSettings().then(() => {
    resetForm();
    updateFilterChips();
    loadWords();
    loadVocabSummary();

    // Handle ?edit=<wordId> for deep-linking to edit form (e.g. from training page)
    const editParam = new URLSearchParams(window.location.search).get('edit');
    if (editParam) {
      apiFetch(`/api/words/${editParam}`).then(word => {
        if (word) openEditForm(word);
      }).catch(() => {});
    }

    // Handle ?editComp=<char> for deep-linking to component edit tab
    const editCompParam = new URLSearchParams(window.location.search).get('editComp');
    if (editCompParam) openComponentEdit(editCompParam);
  });
  loadTags();
  renderTierFilter();
  initTranslateButton();

  $('hide-unseen-btn').addEventListener('click', () => {
    hideUnseenActive = !hideUnseenActive;
    updateHideUnseenBtn();
    currentPage = 1;
    loadWords();
  });

  $('review-filter-btn').addEventListener('click', () => {
    reviewFilterActive = !reviewFilterActive;
    updateReviewFilterBtn();
    currentPage = 1;
    loadWords();
  });

  $('comp-review-filter-btn').addEventListener('click', () => {
    compReviewFilterActive = !compReviewFilterActive;
    updateCompReviewFilterBtn();
    compPage = 1;
    loadComponents();
  });

  ['today', 'tomorrow', 'known'].forEach(key => {
    $('due-' + key + '-btn').addEventListener('click', () => {
      dueFilter = dueFilter === key ? '' : key;
      updateDueFilterBtns();
      currentPage = 1;
      loadWords();
    });
  });

  $('per-page-select').addEventListener('change', (e) => {
    perPage = parseInt(e.target.value);
    localStorage.setItem('vocabPerPage', perPage);
    currentPage = 1;
    compPage = 1;
    if (currentView === 'words') loadWords(); else loadComponents();
  });

  $('missing-lang-select').addEventListener('change', (e) => {
    missingLangFilter = e.target.value;
    currentPage = 1;
    loadWords();
  });

  $('missing-de-chip').addEventListener('click', () => {
    missingLangFilter = missingLangFilter === secondaryLang ? '' : secondaryLang;
    currentPage = 1;
    loadWords();
  });

  $('filter-all-btn').addEventListener('click', () => {
    dueFilter = '';
    reviewFilterActive = false;
    missingLangFilter = '';
    selectedTierFilter = '';
    selectedFilterTags = [];
    currentPage = 1;
    renderTierFilter();
    renderFilterTags();
    loadWords();
  });

  $('vocab-more-filters-toggle').addEventListener('click', () => {
    const body = $('vocab-more-filters');
    const open = body.classList.contains('hidden');
    body.classList.toggle('hidden', !open);
    const btn = $('vocab-more-filters-toggle');
    btn.setAttribute('aria-expanded', String(open));
    btn.firstChild.textContent = open ? '▾ ' : '▸ ';
  });

  $('sort-select').addEventListener('change', e => {
    sortBy = e.target.value;
    if (sortBy && !$('sort-dir-btn').dataset.touched) sortDir = 'asc';
    currentPage = 1;
    loadWords();
  });
  $('sort-dir-btn').addEventListener('click', () => {
    sortDir = sortDir === 'asc' ? 'desc' : 'asc';
    $('sort-dir-btn').dataset.touched = '1';
    currentPage = 1;
    loadWords();
  });

  // Header: Add word / Import / ⋯ menu, and the sheet
  $('open-add-btn').addEventListener('click', () => { resetForm(); openVocabSheet('add'); $('form-zh').focus(); });
  $('open-import-btn').addEventListener('click', () => openVocabSheet('import'));
  $('vocab-menu-btn').addEventListener('click', e => { e.stopPropagation(); toggleVocabMenu(); });
  document.addEventListener('click', e => {
    if (!$('vocab-menu').classList.contains('hidden') && !e.target.closest('.vb-menu-wrap')) toggleVocabMenu(false);
  });
  $('open-tags-btn').addEventListener('click', () => { toggleVocabMenu(false); openVocabSheet('tags'); });
  for (const id of ['hmm-builder-toggle', 'components-edit-toggle']) {
    $(id).addEventListener('click', () => setEditExtraOpen(id, $(id).getAttribute('aria-expanded') !== 'true'));
  }
  $('vocab-sheet-close').addEventListener('click', closeVocabSheet);
  $('vocab-sheet-backdrop').addEventListener('click', closeVocabSheet);
  document.addEventListener('keydown', e => {
    if (e.key !== 'Escape') return;
    if (!$('vocab-menu').classList.contains('hidden')) toggleVocabMenu(false);
    else if (!$('vocab-sheet').classList.contains('hidden')) closeVocabSheet();
  });

  $('word-form').addEventListener('submit', handleFormSubmit);

  $('form-tag-input').addEventListener('input', () => {
    const v = $('form-tag-input').value.trim();
    if (v) {
      showTagAutocomplete(v);
    } else {
      $('tag-autocomplete').classList.add('hidden');
    }
  });
  $('form-tag-input').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      const v = $('form-tag-input').value.trim();
      if (v) addFormTag(v);
    }
  });
  $('form-tag-input').addEventListener('blur', () => {
    setTimeout(() => $('tag-autocomplete').classList.add('hidden'), 150);
  });

  $('form-zh').addEventListener('input', () => {
    clearTimeout(pinyinTimer);
    const zh = $('form-zh').value.trim();
    // Pinyin is already set: recalculating on every keystroke would repeatedly
    // prompt to overwrite it (applyPinyin's confirm dialog), so wait for a
    // longer idle period instead. An empty field still fills in quickly.
    const delay = $('form-pinyin').value.trim() ? 3000 : 500;
    pinyinTimer = setTimeout(() => fetchAndFillPinyin(zh), delay);
    const hanziwayLink = $('hanziway-link');
    if (zh) {
      hanziwayLink.href = 'https://hanziway.com/en/char?q=' + encodeURIComponent(zh);
      show('hanziway-link');
    } else {
      hide('hanziway-link');
    }
  });
  $('form-zh').addEventListener('blur', () => {
    clearTimeout(pinyinTimer);
    fetchAndFillPinyin($('form-zh').value.trim());
  });

  $('add-en-btn').addEventListener('click', () => addEnInput(''));
  $('add-de-btn').addEventListener('click', () => addDeInput(''));
  $('translate-btn').addEventListener('click', handleTranslate);

  $('form-cancel-btn').addEventListener('click', () => {
    resetForm();
    closeVocabSheet();
  });

  $('form-delete-btn').addEventListener('click', async () => {
    if (!editingWordId) return;
    if (await deleteWord(editingWordId)) {
      resetForm();
      closeVocabSheet();
    }
  });

  $('form-reset-btn').addEventListener('click', () => {
    resetWordProgress();
  });

  $('form-known-btn').addEventListener('click', () => {
    toggleWordKnown();
  });

  $('view-words-btn').addEventListener('click', () => { if (currentView !== 'words') switchView('words'); });
  $('view-components-btn').addEventListener('click', () => { if (currentView !== 'components') switchView('components'); });

  $('search-input').addEventListener('input', () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      searchQuery = $('search-input').value.trim();
      currentPage = 1;
      compPage = 1;
      if (currentView === 'words') loadWords(); else loadComponents();
    }, 300);
  });

  $('prev-btn').addEventListener('click', () => {
    if (currentView === 'words') {
      if (currentPage > 1) { currentPage--; loadWords(); }
    } else {
      if (compPage > 1) { compPage--; loadComponents(); }
    }
  });

  $('next-page-btn').addEventListener('click', () => {
    if (currentView === 'words') { currentPage++; loadWords(); }
    else { compPage++; loadComponents(); }
  });

  $('download-btn').addEventListener('click', () => { toggleVocabMenu(false); openDownloadModal(); });
  $('dl-cancel-btn').addEventListener('click', () => hide('download-modal'));
  $('dl-confirm-btn').addEventListener('click', executeDownload);
  $('download-modal').addEventListener('click', e => {
    if (e.target === $('download-modal')) hide('download-modal');
  });

  // CSV upload modal
  $('csv-upload-btn').addEventListener('click', () => { toggleVocabMenu(false); openCsvUploadModal(); });
  $('csv-upload-cancel-btn').addEventListener('click', closeCsvUploadModal);
  $('csv-upload-modal').addEventListener('click', e => {
    if (e.target === $('csv-upload-modal')) closeCsvUploadModal();
  });
  $('csv-upload-file').addEventListener('change', e => {
    onCsvFileChange(e.target.files[0] || null);
  });
  $('csv-upload-slider').addEventListener('input', () => {
    $('csv-upload-slider-val').textContent = $('csv-upload-slider').value;
  });
  $('csv-upload-tag-input').addEventListener('input', e => {
    showCsvUploadTagAutocomplete(e.target.value.trim());
  });
  $('csv-upload-tag-input').addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      const v = $('csv-upload-tag-input').value.trim();
      if (v) addCsvUploadTag(v);
    }
  });
  $('csv-upload-tag-input').addEventListener('blur', () => {
    setTimeout(() => $('csv-upload-tag-autocomplete').classList.add('hidden'), 150);
  });
  $('csv-upload-submit-btn').addEventListener('click', executeCsvUpload);

  // Import tab
  $('tab-add').addEventListener('click', () => switchTab('add'));
  $('tab-import').addEventListener('click', () => switchTab('import'));
  $('tab-tags').addEventListener('click', () => switchTab('tags'));
  $('tab-comp').addEventListener('click', () => { if (editingCompChar) switchTab('comp'); });

  $('comp-edit-save-btn').addEventListener('click', async () => {
    if (!editingCompChar) return;
    const btn = $('comp-edit-save-btn');
    btn.disabled = true;
    const byLang = {};
    $('comp-edit-form').querySelectorAll('.comp-trans-input').forEach(input => {
      const lang = input.dataset.lang;
      const val = input.value.trim();
      if (!byLang[lang]) byLang[lang] = [];
      if (val) byLang[lang].push(val);
    });
    try {
      for (const [lang, parts] of Object.entries(byLang)) {
        await apiFetch(`/api/components/${encodeURIComponent(editingCompChar)}/translation`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ lang, definition: parts.join(', ') }),
        });
      }
      if (currentView === 'components') loadComponents();
    } catch (e) {
      alert('Failed to save: ' + e.message);
    } finally {
      btn.disabled = false;
    }
  });

  $('comp-edit-cancel-btn').addEventListener('click', () => {
    editingCompChar = null;
    hide('comp-hanziway-link');
    const tabComp = $('tab-comp');
    tabComp.classList.add('is-disabled');
    switchTab('add');
    closeVocabSheet();
  });

  // Import language filter toggle buttons
  ['en', 'de'].forEach(lang => {
    $('import-filter-' + lang).addEventListener('click', () => {
      if (importFilterLangs.has(lang)) {
        importFilterLangs.delete(lang);
      } else {
        importFilterLangs.add(lang);
      }
      const btn = $('import-filter-' + lang);
      const active = importFilterLangs.has(lang);
      btn.classList.toggle('bg-blue-600', active);
      btn.classList.toggle('text-white', active);
      btn.classList.toggle('border-blue-600', active);
      btn.classList.toggle('border-gray-300', !active);
      btn.classList.toggle('text-gray-500', !active);
      if (importSourceTagsLoaded) renderImportTagPills();
    });
  });
  document.querySelectorAll('input[name="import-filter-mode"]').forEach(radio => {
    radio.addEventListener('change', () => {
      importFilterMode = radio.value;
      if (importSourceTagsLoaded) renderImportTagPills();
    });
  });

  $('import-next-btn').addEventListener('click', () => showImportStep(2));
  $('import-back1-btn').addEventListener('click', () => showImportStep(1));
  $('import-next2-btn').addEventListener('click', () => {
    importApplyTags = [...importSelectedTags];
    renderImportApplyTags();
    showImportStep(3);
  });
  $('import-back2-btn').addEventListener('click', () => showImportStep(2));
  $('import-submit-btn').addEventListener('click', executeImport);

  $('import-tag-input').addEventListener('input', () => {
    const v = $('import-tag-input').value.trim();
    if (v) {
      showImportTagAutocomplete(v);
    } else {
      $('import-tag-autocomplete').classList.add('hidden');
    }
  });
  $('import-tag-input').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      const v = $('import-tag-input').value.trim();
      if (v) addImportTag(v);
    }
  });
  $('import-tag-input').addEventListener('blur', () => {
    setTimeout(() => $('import-tag-autocomplete').classList.add('hidden'), 150);
  });

  // Re-render dynamic text when UI language changes
  document.addEventListener('langchange', () => {
    renderTierFilter();
    updateFilterChips();
    loadVocabSummary();
    if (currentView === 'words') loadWords(); else loadComponents();
  });
});
