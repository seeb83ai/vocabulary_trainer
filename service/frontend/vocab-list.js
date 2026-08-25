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
let importSelectedTag = '';
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
    renderTable(data.words);
    renderPagination(data.total, data.page, data.per_page);
  } catch (e) {
    alert('Failed to load words: ' + e.message);
  }
}

function updateSortHeaders() {
  document.querySelectorAll('th[data-sort]').forEach(th => {
    const indicator = th.querySelector('.sort-indicator');
    if (th.dataset.sort === sortBy) {
      indicator.textContent = sortDir === 'asc' ? ' ▲' : ' ▼';
    } else {
      indicator.textContent = '';
    }
  });
}

function renderTable(words) {
  updateSortHeaders();
  const tbody = $('words-tbody');
  tbody.innerHTML = '';
  if (!words || words.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td colspan="7" class="text-center py-8 text-gray-500">${escHtml(t('vocab.noEntries'))}</td>`;
    tbody.appendChild(tr);
    return;
  }
  for (const word of words) {
    const tr = document.createElement('tr');
    tr.className = 'border-b border-gray-200 hover:bg-gray-50';
    tr.innerHTML = `
      <td class="py-3 px-4 text-lg font-medium">
        <span class="mr-1">${escHtml(word.zh_text)}</span>
        <button class="btn-play text-base text-gray-400 hover:text-blue-500 transition leading-none align-middle" data-id="${word.id}" data-zh="${escHtml(word.zh_text)}" title="Read aloud">🔊</button>
        ${word.needs_review ? `<span class="inline-block bg-orange-100 text-orange-600 text-xs px-1.5 py-0.5 rounded-full ml-1 align-middle">${escHtml(t('vocab.review'))}</span>` : ''}
        ${crossRefBadge(word.is_also_component, t('vocab.alsoComponent'))}
        ${(word.tags || []).map(tag => `<span class="inline-block bg-gray-200 text-gray-600 text-xs px-1.5 py-0.5 rounded-full ml-1 align-middle">${escHtml(tag)}</span>`).join('')}
      </td>
      <td class="py-3 px-4 text-gray-600">${word.pinyin ? escHtml(word.pinyin) : '<span class="text-gray-400">—</span>'}</td>
      <td class="py-3 px-4 text-gray-600">
        ${((word.translations || {})[primaryLang] || []).length ? (word.translations[primaryLang]).map(escHtml).join(', ') : '<span class="text-gray-400">—</span>'}
      </td>
      ${secondaryLang ? `<td class="py-3 px-4 text-gray-600">
        ${((word.translations || {})[secondaryLang] || []).length ? (word.translations[secondaryLang]).map(escHtml).join(', ') : '<span class="text-gray-400">—</span>'}
      </td>` : ''}
      <td class="py-3 px-4 whitespace-nowrap">${renderTierBadge(word)}</td>
      <td class="py-3 px-4 whitespace-nowrap text-xs">${renderDue(word)}</td>
      <td class="py-3 px-4 whitespace-nowrap">
        <button class="btn-edit text-blue-600 hover:text-blue-800 mr-3 font-medium" data-id="${word.id}">${escHtml(t('vocab.edit'))}</button>
        <button class="btn-delete text-red-600 hover:text-red-800 font-medium" data-id="${word.id}">${escHtml(t('vocab.delete'))}</button>
      </td>`;
    tbody.appendChild(tr);
  }

  tbody.querySelectorAll('.btn-play').forEach(btn => {
    btn.addEventListener('click', () => playAudio(parseInt(btn.dataset.id), btn.dataset.zh));
  });
  tbody.querySelectorAll('.btn-edit').forEach(btn => {
    btn.addEventListener('click', () => openEditForm(words.find(w => w.id == btn.dataset.id)));
  });
  tbody.querySelectorAll('.btn-delete').forEach(btn => {
    btn.addEventListener('click', () => deleteWord(parseInt(btn.dataset.id)));
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
      dots.className = 'px-1 text-gray-400';
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
      dots.className = 'px-1 text-gray-400';
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
  btn.textContent = pageNum;
  btn.className = pageNum === activePage
    ? 'px-2.5 py-1 rounded text-sm font-medium bg-blue-600 text-white'
    : 'px-2.5 py-1 rounded text-sm font-medium text-gray-600 hover:bg-gray-100';
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
    ? `<span class="inline-block bg-purple-100 text-purple-600 text-xs px-1.5 py-0.5 rounded-full ml-1 align-middle">${escHtml(label)}</span>`
    : '';
}

function renderTierBadge(word) {
  const tier = wordTier(word.total_correct, word.total_attempts, word.learning_new_word, word.streak_bonus);
  if (!tier) return `<span class="text-gray-400 text-xs">${escHtml(t('vocab.unseen'))}</span>`;
  if (word.learning_new_word) return `<span class="inline-block px-2 py-0.5 rounded-full text-xs font-medium ${tier.pill}">${t(tier.i18nKey)}</span>`;
  const pct = Math.round((word.total_correct + (word.streak_bonus || 0)) / word.total_attempts * 100);
  return `<span class="inline-block px-2 py-0.5 rounded-full text-xs font-medium ${tier.pill}">${t(tier.i18nKey)}</span><span class="ml-1.5 text-xs text-gray-400">${pct}%</span>`;
}

function renderDue(word) {
  if (word.total_attempts === 0) {
    return '<span class="text-gray-400">—</span>';
  }
  if (!word.due_date) {
    return '<span class="text-gray-400">—</span>';
  }
  const due = new Date(word.due_date);
  if (isNaN(due.getTime())) {
    return '<span class="text-gray-400">—</span>';
  }
  const diffDays = Math.round((due - new Date()) / 86400000);
  if (diffDays <= 0) return `<span class="text-orange-500">${escHtml(t('vocab.dueLabel'))}</span>`;
  return `<span class="text-gray-500">${escHtml(t('vocab.inDays', { n: diffDays }))}</span>`;
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
    pill.className = `filter-tag-pill px-2.5 py-0.5 rounded-full text-xs font-medium transition ${active ? 'bg-blue-600 text-white' : 'bg-gray-200 text-gray-600 hover:bg-gray-300'}`;
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
    pill.className = `tier-filter-pill px-2.5 py-0.5 rounded-full text-xs font-medium transition ${active ? 'text-white' : 'bg-gray-200 text-gray-600 hover:bg-gray-300'}`;
    if (active) pill.style.backgroundColor = tier.color;
    pill.textContent = t(tier.i18nKey);
    pill.addEventListener('click', () => {
      selectedTierFilter = selectedTierFilter === tier.key ? '' : tier.key;
      currentPage = 1;
      renderTierFilter();
      loadWords();
    });
    bar.appendChild(pill);
  }
}

function updateHideUnseenBtn() {
  const btn = $('hide-unseen-btn');
  if (hideUnseenActive) {
    btn.className = 'px-3 py-1.5 rounded-lg border text-sm font-medium transition border-blue-400 bg-blue-50 text-blue-600';
  } else {
    btn.className = 'px-3 py-1.5 rounded-lg border text-sm font-medium transition border-gray-300 text-gray-600 hover:bg-gray-100';
  }
}

function updateReviewFilterBtn() {
  const btn = $('review-filter-btn');
  if (reviewFilterActive) {
    btn.className = 'px-3 py-1.5 rounded-lg border text-sm font-medium transition border-orange-400 bg-orange-50 text-orange-600';
  } else {
    btn.className = 'px-3 py-1.5 rounded-lg border text-sm font-medium transition border-gray-300 text-gray-600 hover:bg-gray-100';
  }
}

function updateCompReviewFilterBtn() {
  const btn = $('comp-review-filter-btn');
  if (!btn) return;
  if (compReviewFilterActive) {
    btn.className = 'px-3 py-1.5 rounded-lg border text-sm font-medium transition border-orange-400 bg-orange-50 text-orange-600';
  } else {
    btn.className = 'px-3 py-1.5 rounded-lg border text-sm font-medium transition border-gray-300 text-gray-600 hover:bg-gray-100';
  }
}

function updateDueFilterBtns() {
  ['today', 'tomorrow'].forEach(key => {
    const btn = $('due-' + key + '-btn');
    if (dueFilter === key) {
      btn.className = 'px-3 py-1.5 rounded-lg border text-sm font-medium transition border-blue-400 bg-blue-50 text-blue-600';
    } else {
      btn.className = 'px-3 py-1.5 rounded-lg border text-sm font-medium transition border-gray-300 text-gray-600 hover:bg-gray-100';
    }
  });
}

// ── Component view ────────────────────────────────────────────────────────────

function switchView(view) {
  currentView = view;
  const isWords = view === 'words';
  $('words-table-wrap').classList.toggle('hidden', !isWords);
  $('components-table-wrap').classList.toggle('hidden', isWords);
  $('word-filters-row').classList.toggle('hidden', !isWords);
  $('filter-tier-bar').classList.toggle('hidden', !isWords);
  $('component-filters-row').classList.toggle('hidden', isWords);
  if (isWords) renderFilterTags(); else $('filter-tags-bar').classList.add('hidden');

  $('view-words-btn').className = `px-3 py-1.5 transition ${isWords ? 'bg-blue-600 text-white' : 'bg-white text-gray-600 hover:bg-gray-100'}`;
  $('view-components-btn').className = `px-3 py-1.5 transition ${!isWords ? 'bg-blue-600 text-white' : 'bg-white text-gray-600 hover:bg-gray-100'}`;

  currentPage = 1;
  compPage = 1;
  if (isWords) loadWords(); else loadComponents();
}

document.addEventListener('DOMContentLoaded', () => {
  loadLangSettings().then(() => {
    resetForm();
    loadWords();

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

  ['today', 'tomorrow'].forEach(key => {
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
  });

  $('form-reset-btn').addEventListener('click', () => {
    resetWordProgress();
  });

  document.querySelectorAll('th[data-sort]').forEach(th => {
    th.addEventListener('click', () => {
      const col = th.dataset.sort;
      if (sortBy === col) {
        sortDir = sortDir === 'asc' ? 'desc' : 'asc';
      } else {
        sortBy = col;
        sortDir = 'asc';
      }
      currentPage = 1;
      loadWords();
    });
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

  $('download-btn').addEventListener('click', openDownloadModal);
  $('dl-cancel-btn').addEventListener('click', () => hide('download-modal'));
  $('dl-confirm-btn').addEventListener('click', executeDownload);
  $('download-modal').addEventListener('click', e => {
    if (e.target === $('download-modal')) hide('download-modal');
  });

  // CSV upload modal
  $('csv-upload-btn').addEventListener('click', openCsvUploadModal);
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
    tabComp.classList.add('opacity-40', 'cursor-not-allowed', 'pointer-events-none', 'text-gray-400');
    tabComp.classList.remove('hover:text-gray-700', 'text-gray-500');
    switchTab('add');
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
    importApplyTags = importSelectedTag ? [importSelectedTag] : [];
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
    if (currentView === 'words') loadWords(); else loadComponents();
  });
});
