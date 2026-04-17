// Vocabulary management page logic

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

// Import tab state
let importSelectedTag = '';
let importApplyTags = [];
let importSourceTagsLoaded = false;

// Tags tab state
let tagsLoaded = false;

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
        ${(word.tags || []).map(tag => `<span class="inline-block bg-gray-200 text-gray-600 text-xs px-1.5 py-0.5 rounded-full ml-1 align-middle">${escHtml(tag)}</span>`).join('')}
      </td>
      <td class="py-3 px-4 text-gray-600">${word.pinyin ? escHtml(word.pinyin) : '<span class="text-gray-400">—</span>'}</td>
      <td class="py-3 px-4 text-gray-600">
        ${(word.en_texts && word.en_texts.length) ? word.en_texts.map(escHtml).join(', ') : '<span class="text-gray-400">—</span>'}
      </td>
      <td class="py-3 px-4 text-gray-600">
        ${(word.de_texts && word.de_texts.length) ? word.de_texts.map(escHtml).join(', ') : '<span class="text-gray-400">—</span>'}
      </td>
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

function openEditForm(word) {
  editingWordId = word.id;
  setText('form-title', t('vocab.editWord'));
  $('form-zh').value = word.zh_text;
  $('form-pinyin').value = word.pinyin || '';
  const hanziwayLink = $('hanziway-link');
  hanziwayLink.href = 'https://hanziway.com/en/char?q=' + encodeURIComponent(word.zh_text);
  show('hanziway-link');
  show('form-cancel-btn');

  let notice = $('review-notice');
  if (word.needs_review) {
    if (!notice) {
      notice = document.createElement('p');
      notice.id = 'review-notice';
      notice.className = 'text-sm text-orange-600 bg-orange-50 border border-orange-200 rounded-lg px-3 py-2';
      notice.textContent = t('vocab.reviewNotice');
      $('word-form').prepend(notice);
    }
  } else if (notice) {
    notice.remove();
  }

  const container = $('en-inputs-container');
  container.innerHTML = '';
  for (const t of (word.en_texts.length ? word.en_texts : [''])) {
    addEnInput(t);
  }

  const deContainer = $('de-inputs-container');
  deContainer.innerHTML = '';
  for (const t of ((word.de_texts && word.de_texts.length) ? word.de_texts : [''])) {
    addDeInput(t);
  }

  formTags = [...(word.tags || [])];
  renderFormTags();

  if (word.total_attempts === 0) {
    show('start-training-row');
    $('form-start-training').checked = false;
  } else {
    hide('start-training-row');
  }

  // HMM scene builder
  const hmmContainer = $('hmm-builder-container');
  if (word.id) {
    hmmContainer.classList.remove('hidden');
    loadHMMBuilder('hmm-builder-container', word.id, { zh: word.zh_text, en: word.en_texts || [] });
  } else {
    hmmContainer.classList.add('hidden');
    hmmContainer.innerHTML = '';
  }

  $('word-form-panel').scrollIntoView({ behavior: 'smooth' });
  $('form-zh').focus();
}

function resetForm() {
  editingWordId = null;
  setText('form-title', t('vocab.addWord'));
  $('form-zh').value = '';
  $('form-pinyin').value = '';
  hide('form-cancel-btn');
  hide('hanziway-link');
  $('en-inputs-container').innerHTML = '';
  addEnInput('');
  $('de-inputs-container').innerHTML = '';
  addDeInput('');
  formTags = [];
  renderFormTags();
  $('form-tag-input').value = '';
  const notice = $('review-notice');
  if (notice) notice.remove();
  show('start-training-row');
  $('form-start-training').checked = false;
  $('hmm-builder-container').classList.add('hidden');
  $('hmm-builder-container').innerHTML = '';
}

function addEnInput(value = '') {
  const container = $('en-inputs-container');
  const wrapper = document.createElement('div');
  wrapper.className = 'flex items-center gap-2 mb-2';
  wrapper.innerHTML = `
    <input type="text" class="en-input flex-1 border border-gray-300 rounded-lg px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
           placeholder="${escHtml(t('vocab.englishPlaceholder'))}" value="${escHtml(value)}">
    <button type="button" class="btn-remove-en text-gray-400 hover:text-red-500 text-xl leading-none" title="Remove">×</button>`;
  wrapper.querySelector('.btn-remove-en').addEventListener('click', () => {
    if (container.children.length > 1) wrapper.remove();
  });
  container.appendChild(wrapper);
}

function addDeInput(value = '') {
  const container = $('de-inputs-container');
  const wrapper = document.createElement('div');
  wrapper.className = 'flex items-center gap-2 mb-2';
  wrapper.innerHTML = `
    <input type="text" class="de-input flex-1 border border-gray-300 rounded-lg px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
           placeholder="${escHtml(t('vocab.germanPlaceholder'))}" value="${escHtml(value)}">
    <button type="button" class="btn-remove-de text-gray-400 hover:text-red-500 text-xl leading-none" title="Remove">×</button>`;
  wrapper.querySelector('.btn-remove-de').addEventListener('click', () => {
    if (container.children.length > 1) wrapper.remove();
  });
  container.appendChild(wrapper);
}

function buildFormPayload() {
  const pinyin = $('form-pinyin').value.trim();
  return {
    zh_text: $('form-zh').value.trim(),
    pinyin: pinyin,
    en_texts: Array.from(document.querySelectorAll('.en-input'))
      .map(i => i.value.trim())
      .filter(Boolean),
    de_texts: Array.from(document.querySelectorAll('.de-input'))
      .map(i => i.value.trim())
      .filter(Boolean),
    tags: [...formTags],
    start_training: $('form-start-training').checked,
  };
}

async function handleFormSubmit(e) {
  e.preventDefault();
  const payload = buildFormPayload();
  if (!payload.zh_text) { alert(t('vocab.zhRequired')); return; }
  if (!payload.en_texts.length) { alert(t('vocab.enRequired')); return; }

  try {
    if (editingWordId) {
      await apiFetch(`/api/words/${editingWordId}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      });
    } else {
      await apiFetch('/api/words', {
        method: 'POST',
        body: JSON.stringify(payload),
      });
    }
    resetForm();
    loadTags();
    loadWords();
  } catch (e) {
    alert('Error: ' + e.message);
  }
}

async function deleteWord(id) {
  if (!confirm(t('vocab.confirmDelete'))) return;
  try {
    await apiFetch(`/api/words/${id}`, { method: 'DELETE' });
    loadTags();
    loadWords();
  } catch (e) {
    alert('Failed to delete: ' + e.message);
  }
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

async function loadTags() {
  try {
    allTags = await apiFetch('/api/tags');
  } catch (_) {
    allTags = [];
  }
  // Invalidate tags panel so it re-fetches with fresh details on next visit.
  tagsLoaded = false;
  renderFilterTags();
}

function renderFormTags() {
  const container = $('form-tags');
  container.innerHTML = '';
  for (const tag of formTags) {
    const pill = document.createElement('span');
    pill.className = 'inline-flex items-center bg-gray-200 text-gray-700 text-sm px-2 py-0.5 rounded-full';
    pill.innerHTML = `${escHtml(tag)} <button type="button" class="ml-1 text-gray-400 hover:text-red-500 leading-none">&times;</button>`;
    pill.querySelector('button').addEventListener('click', () => {
      formTags = formTags.filter(t => t !== tag);
      renderFormTags();
    });
    container.appendChild(pill);
  }
}

function showTagAutocomplete(query) {
  const dropdown = $('tag-autocomplete');
  const q = query.toLowerCase();
  const matches = allTags.filter(t => t.toLowerCase().includes(q) && !formTags.includes(t));
  if (q && !allTags.includes(query) && !formTags.includes(query)) {
    matches.unshift(query);
  }
  if (matches.length === 0) {
    dropdown.classList.add('hidden');
    return;
  }
  dropdown.innerHTML = '';
  for (const m of matches.slice(0, 10)) {
    const item = document.createElement('div');
    item.className = 'px-3 py-1.5 text-sm hover:bg-blue-50 cursor-pointer';
    item.textContent = m === query && !allTags.includes(query) ? t('vocab.createTag', { tag: m }) : m;
    item.addEventListener('mousedown', (e) => {
      e.preventDefault();
      addFormTag(m);
    });
    dropdown.appendChild(item);
  }
  dropdown.classList.remove('hidden');
}

function addFormTag(tag) {
  tag = tag.trim();
  if (!tag || formTags.includes(tag)) return;
  formTags.push(tag);
  renderFormTags();
  $('form-tag-input').value = '';
  $('tag-autocomplete').classList.add('hidden');
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

async function applyPinyin(newPinyin) {
  if (!newPinyin) return;
  const field = $('form-pinyin');
  const current = field.value.trim();
  if (!current) {
    field.value = newPinyin;
  } else if (current !== newPinyin) {
    if (confirm(t('vocab.replacePinyin', { old: current, new: newPinyin }))) {
      field.value = newPinyin;
    }
  }
}

let pinyinTimer = null;

async function fetchAndFillPinyin(zh) {
  if (!zh) return;
  try {
    const result = await apiFetch('/api/pinyin', {
      method: 'POST',
      body: JSON.stringify({ zh_text: zh }),
    });
    await applyPinyin(result.pinyin);
  } catch (_) {}
}

async function initTranslateButton() {
  try {
    const cfg = await apiFetch('/api/config');
    if (cfg && cfg.deepl_enabled) {
      show('translate-btn');
    }
  } catch (_) {}
}

async function handleTranslate() {
  const btn = $('translate-btn');
  const zh = $('form-zh').value.trim();
  const enInputs = document.querySelectorAll('.en-input');
  const deInputs = document.querySelectorAll('.de-input');
  const en = enInputs.length > 0 ? enInputs[0].value.trim() : '';
  const de = deInputs.length > 0 ? deInputs[0].value.trim() : '';

  if (!zh && !en && !de) {
    alert(t('vocab.enterTextFirst'));
    return;
  }

  const origText = btn.textContent;
  btn.textContent = t('vocab.translating');
  btn.disabled = true;

  try {
    // Translate zh → EN if en field is empty
    const enPromise = (zh && !en)
      ? apiFetch('/api/translate', {
          method: 'POST',
          body: JSON.stringify({ zh_text: zh, target_lang: 'EN' }),
        }).catch(() => null)
      : Promise.resolve(null);

    // Translate zh → DE if de field is empty
    const dePromise = (zh && !de)
      ? apiFetch('/api/translate', {
          method: 'POST',
          body: JSON.stringify({ zh_text: zh, target_lang: 'DE' }),
        }).catch(() => null)
      : Promise.resolve(null);

    // Translate en/de → zh if zh field is empty
    const zhPromise = (!zh && (en || de))
      ? apiFetch('/api/translate', {
          method: 'POST',
          body: JSON.stringify({ en_text: en || de }),
        }).catch(() => null)
      : Promise.resolve(null);

    const [enResult, deResult, zhResult] = await Promise.all([enPromise, dePromise, zhPromise]);

    if (zhResult) {
      if (zhResult.zh_text) $('form-zh').value = zhResult.zh_text;
      await applyPinyin(zhResult.pinyin);
    } else if (enResult) {
      await applyPinyin(enResult.pinyin);
    } else if (deResult) {
      await applyPinyin(deResult.pinyin);
    }

    if (enResult) {
      const translations = enResult.en_texts || (enResult.en_text ? [enResult.en_text] : []);
      if (translations.length > 0) {
        const container = $('en-inputs-container');
        container.innerHTML = '';
        for (const tr of translations) addEnInput(tr);
      }
    }

    if (deResult) {
      const translations = deResult.en_texts || (deResult.en_text ? [deResult.en_text] : []);
      if (translations.length > 0) {
        const container = $('de-inputs-container');
        container.innerHTML = '';
        for (const tr of translations) addDeInput(tr);
      }
    }
  } catch (e) {
    alert('Translation failed: ' + e.message);
  } finally {
    btn.textContent = origText;
    btn.disabled = false;
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

function openDownloadModal() {
  show('download-modal');
}

async function executeDownload() {
  const cols = {
    zh:       $('dl-col-zh').checked,
    pinyin:   $('dl-col-pinyin').checked,
    en:       $('dl-col-en').checked,
    tags:     $('dl-col-tags').checked,
    tier:     $('dl-col-tier').checked,
    accuracy: $('dl-col-accuracy').checked,
    attempts: $('dl-col-attempts').checked,
    due:      $('dl-col-due').checked,
  };
  const format = document.querySelector('input[name="dl-format"]:checked').value;

  const params = new URLSearchParams({ q: searchQuery });
  if (sortBy) { params.set('sort', sortBy); params.set('order', sortDir); }
  if (selectedFilterTags.length) params.set('tags', selectedFilterTags.join(','));
  if (reviewFilterActive) params.set('review', '1');
  if (hideUnseenActive) params.set('hide_unseen', '1');
  if (selectedTierFilter) params.set('bucket', selectedTierFilter);
  if (dueFilter) params.set('due', dueFilter);

  const btn = $('dl-confirm-btn');
  btn.textContent = t('download.downloading');
  btn.disabled = true;
  try {
    const words = await apiFetch(`/api/words/export?${params}`);
    const { content, mime, ext } = buildDownload(words, cols, format);
    const blob = new Blob(['\uFEFF' + content], { type: mime + ';charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `vocabulary.${ext}`;
    a.click();
    URL.revokeObjectURL(url);
    hide('download-modal');
  } catch (e) {
    alert('Download failed: ' + e.message);
  } finally {
    btn.textContent = t('download.download');
    btn.disabled = false;
  }
}

function buildDownload(words, cols, format) {
  const headers = [];
  if (cols.zh)       headers.push('Chinese');
  if (cols.pinyin)   headers.push('Pinyin');
  if (cols.en)       headers.push('Translations');
  if (cols.tags)     headers.push('Tags');
  if (cols.tier)     headers.push('Level');
  if (cols.accuracy) headers.push('Accuracy');
  if (cols.attempts) headers.push('Attempts');
  if (cols.due)      headers.push('Due Date');

  function rowValues(word) {
    const vals = [];
    if (cols.zh)     vals.push(word.zh_text || '');
    if (cols.pinyin) vals.push(word.pinyin || '');
    if (cols.en)     vals.push((word.en_texts || []).join('; '));
    if (cols.tags)   vals.push((word.tags || []).join('; '));
    if (cols.tier) {
      const t = wordTier(word.total_correct, word.total_attempts, word.learning_new_word, word.streak_bonus);
      vals.push(t ? t.label : '');
    }
    if (cols.accuracy) {
      const effCorrect = word.total_correct + (word.streak_bonus || 0);
      vals.push(word.total_attempts > 0
        ? Math.round(effCorrect / word.total_attempts * 100) + '%'
        : '');
    }
    if (cols.attempts) vals.push(String(word.total_attempts));
    if (cols.due) {
      vals.push(word.total_attempts > 0 ? word.due_date.substring(0, 10) : '');
    }
    return vals;
  }

  if (format === 'json') {
    const out = words.map(word => {
      const obj = {};
      if (cols.zh)       obj.chinese      = word.zh_text || '';
      if (cols.pinyin)   obj.pinyin       = word.pinyin || '';
      if (cols.en)       obj.translations = word.en_texts || [];
      if (cols.tags)     obj.tags         = word.tags || [];
      if (cols.tier) {
        const t = wordTier(word.total_correct, word.total_attempts, word.learning_new_word, word.streak_bonus);
        obj.level = t ? t.label : '';
      }
      if (cols.accuracy) {
        const effCorrect = word.total_correct + (word.streak_bonus || 0);
        obj.accuracy = word.total_attempts > 0
          ? Math.round(effCorrect / word.total_attempts * 100)
          : null;
      }
      if (cols.attempts) obj.attempts = word.total_attempts;
      if (cols.due)      obj.due_date = word.total_attempts > 0 ? word.due_date.substring(0, 10) : null;
      return obj;
    });
    return { content: JSON.stringify(out, null, 2), mime: 'application/json', ext: 'json' };
  }

  if (format === 'tsv') {
    const lines = [headers.join('\t')];
    for (const word of words) {
      lines.push(rowValues(word).map(v => v.replace(/[\t\n\r]/g, ' ')).join('\t'));
    }
    return { content: lines.join('\n'), mime: 'text/tab-separated-values', ext: 'tsv' };
  }

  if (format === 'txt') {
    const lines = words.map(word => rowValues(word).join(' | '));
    return { content: lines.join('\n'), mime: 'text/plain', ext: 'txt' };
  }

  // CSV (default)
  function csvField(v) {
    if (/[",\n\r]/.test(v)) return '"' + v.replace(/"/g, '""') + '"';
    return v;
  }
  const lines = [headers.map(csvField).join(',')];
  for (const word of words) {
    lines.push(rowValues(word).map(csvField).join(','));
  }
  return { content: lines.join('\n'), mime: 'text/csv', ext: 'csv' };
}

// ── Import tab ────────────────────────────────────────────────────────────────

function switchTab(name) {
  const tabs = ['add', 'import', 'tags'];
  tabs.forEach(tab => {
    const active = tab === name;
    $('panel-' + tab).classList.toggle('hidden', !active);
    $('tab-' + tab).classList.toggle('border-blue-600', active);
    $('tab-' + tab).classList.toggle('text-blue-600', active);
    $('tab-' + tab).classList.toggle('border-transparent', !active);
    $('tab-' + tab).classList.toggle('text-gray-500', !active);
  });

  if (name === 'import' && !importSourceTagsLoaded) {
    loadImportSourceTags();
  }
  if (name === 'tags' && !tagsLoaded) {
    loadTagDetails();
  }
}

async function loadImportSourceTags() {
  importSourceTagsLoaded = true;
  const list = $('import-tag-list');
  try {
    const tags = await apiFetch('/api/import/source-tags');
    list.innerHTML = '';
    if (!tags || tags.length === 0) {
      list.innerHTML = `<span class="text-sm text-gray-400">${escHtml(t('vocab.importNoTags'))}</span>`;
      return;
    }
    for (const tag of tags) {
      const pill = document.createElement('button');
      pill.type = 'button';
      pill.className = 'import-source-tag px-3 py-1 rounded-full text-sm font-medium border border-gray-300 text-gray-600 hover:bg-blue-50 hover:border-blue-400 hover:text-blue-600 transition';
      pill.textContent = tag.name;
      if (tag.description) pill.title = tag.description;
      pill.addEventListener('click', () => selectImportSourceTag(tag, pill));
      list.appendChild(pill);
    }
  } catch (e) {
    list.innerHTML = `<span class="text-sm text-red-500">${escHtml(e.message)}</span>`;
  }
}

async function selectImportSourceTag(tag, pill) {
  importSelectedTag = tag.name;
  $('import-next-btn').disabled = true;
  hide('import-preview');

  // Highlight selected pill
  document.querySelectorAll('.import-source-tag').forEach(p => {
    p.classList.remove('bg-blue-600', 'text-white', 'border-blue-600');
    p.classList.add('border-gray-300', 'text-gray-600');
  });
  pill.classList.add('bg-blue-600', 'text-white', 'border-blue-600');
  pill.classList.remove('border-gray-300', 'text-gray-600');

  await loadImportPreview(tag.name, tag.description);
}

async function loadImportPreview(tagName, tagDescription) {
  try {
    const data = await apiFetch('/api/import/preview?tag=' + encodeURIComponent(tagName));
    const examples = (data.examples || []).join('、');
    const descLine = tagDescription ? `${tagDescription}` : '';
    const countLine = data.total === 0
      ? t('vocab.importPreviewEmpty')
      : `${descLine} (${data.total} ${t('vocab.importPreviewWords')}) — ${examples}`;
    $('import-preview-text').textContent = countLine;
    show('import-preview');
    $('import-next-btn').disabled = false;
  } catch (e) {
    $('import-preview-text').textContent = e.message;
    show('import-preview');
  }
}

function showImportStep(n) {
  [1, 2, 3].forEach(i => {
    const el = $('import-step' + i);
    if (el) el.classList.toggle('hidden', i !== n);
  });
}

function renderImportApplyTags() {
  const container = $('import-apply-tags');
  container.innerHTML = '';
  for (const tag of importApplyTags) {
    const pill = document.createElement('span');
    pill.className = 'inline-flex items-center bg-gray-200 text-gray-700 text-sm px-2 py-0.5 rounded-full';
    pill.innerHTML = `${escHtml(tag)} <button type="button" class="ml-1 text-gray-400 hover:text-red-500 leading-none">&times;</button>`;
    pill.querySelector('button').addEventListener('click', () => {
      importApplyTags = importApplyTags.filter(t => t !== tag);
      renderImportApplyTags();
    });
    container.appendChild(pill);
  }
}

function addImportTag(tag) {
  tag = tag.trim();
  if (!tag || importApplyTags.includes(tag)) return;
  importApplyTags.push(tag);
  renderImportApplyTags();
  $('import-tag-input').value = '';
  $('import-tag-autocomplete').classList.add('hidden');
}

function showImportTagAutocomplete(query) {
  const dropdown = $('import-tag-autocomplete');
  const q = query.toLowerCase();
  const matches = allTags.filter(tag => tag.toLowerCase().includes(q) && !importApplyTags.includes(tag));
  if (query && !allTags.includes(query) && !importApplyTags.includes(query)) {
    matches.push(query);
  }
  if (!matches.length) { dropdown.classList.add('hidden'); return; }
  dropdown.innerHTML = '';
  dropdown.classList.remove('hidden');
  for (const m of matches) {
    const item = document.createElement('div');
    item.className = 'px-3 py-1.5 text-sm hover:bg-blue-50 cursor-pointer';
    item.textContent = m === query && !allTags.includes(query) ? t('vocab.createTag', { tag: m }) : m;
    item.addEventListener('mousedown', (e) => {
      e.preventDefault();
      addImportTag(m);
    });
    dropdown.appendChild(item);
  }
}

async function executeImport() {
  const btn = $('import-submit-btn');
  const statusEl = $('import-status');
  btn.disabled = true;
  btn.textContent = t('vocab.importing');
  statusEl.className = 'mt-3 text-sm text-gray-500';
  statusEl.textContent = '';
  show('import-status');

  try {
    const result = await apiFetch('/api/import', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        tag: importSelectedTag,
        import_en: $('import-en').checked,
        import_de: $('import-de').checked,
        apply_tags: [...importApplyTags],
      }),
    });
    const skippedNote = result.skipped > 0
      ? `, ${t('vocab.importSkipped')} ${result.skipped} ${t('vocab.importAlreadyOwned')}`
      : '';
    statusEl.className = 'mt-3 text-sm text-green-600';
    statusEl.textContent = `${t('vocab.importDone')} ${result.imported} ${t('vocab.importWords2')}${skippedNote}.`;
    loadTags();
    loadWords();
  } catch (e) {
    statusEl.className = 'mt-3 text-sm text-red-500';
    statusEl.textContent = e.message;
  } finally {
    btn.disabled = false;
    btn.textContent = t('vocab.import');
  }
}

function resetImportPanel() {
  importSelectedTag = '';
  importApplyTags = [];
  showImportStep(1);
  hide('import-preview');
  $('import-next-btn').disabled = true;
  hide('import-status');
  if ($('import-en')) $('import-en').checked = true;
  if ($('import-de')) $('import-de').checked = false;
}

// ── Tags tab ───────────────────────────────────────────────────────────────────

async function loadTagDetails() {
  tagsLoaded = true;
  const container = $('tags-list');
  try {
    const tags = await apiFetch('/api/tags/details');
    container.innerHTML = '';
    if (!tags || tags.length === 0) {
      container.innerHTML = `<span class="text-sm text-gray-400">${escHtml(t('vocab.tagsEmpty'))}</span>`;
      return;
    }
    for (const tag of tags) {
      container.appendChild(buildTagRow(tag));
    }
  } catch (e) {
    container.innerHTML = `<span class="text-sm text-red-500">${escHtml(e.message)}</span>`;
  }
}

function buildTagRow(tag) {
  const row = document.createElement('div');
  row.className = 'flex flex-col sm:flex-row sm:items-center gap-2 py-2 border-b border-gray-100 last:border-0';
  row.dataset.tagName = tag.name;

  const nameSpan = document.createElement('span');
  nameSpan.className = 'text-sm font-medium text-gray-700 w-32 flex-none';
  nameSpan.textContent = tag.name;

  const descInput = document.createElement('input');
  descInput.type = 'text';
  descInput.className = 'flex-1 border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500';
  descInput.placeholder = t('vocab.tagsDescPlaceholder');
  descInput.value = tag.description || '';
  descInput.maxLength = 200;

  const toggleLabel = document.createElement('label');
  toggleLabel.className = 'flex items-center gap-1.5 text-sm text-gray-600 cursor-pointer flex-none';

  const toggleInput = document.createElement('input');
  toggleInput.type = 'checkbox';
  toggleInput.className = 'w-4 h-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500';
  toggleInput.checked = tag.importable;

  const toggleText = document.createElement('span');
  toggleText.dataset.i18n = 'vocab.tagsImportable';
  toggleText.textContent = t('vocab.tagsImportable');

  toggleLabel.append(toggleInput, toggleText);

  const saveBtn = document.createElement('button');
  saveBtn.type = 'button';
  saveBtn.className = 'text-sm text-blue-600 hover:text-blue-800 font-medium px-3 py-1.5 rounded-lg border border-blue-200 hover:border-blue-400 transition flex-none';
  saveBtn.textContent = t('vocab.save');

  const statusSpan = document.createElement('span');
  statusSpan.className = 'text-xs text-green-600 hidden flex-none';
  statusSpan.textContent = t('vocab.tagsSaved');

  saveBtn.addEventListener('click', async () => {
    saveBtn.disabled = true;
    try {
      await saveTagMeta(tag.name, descInput.value.trim(), toggleInput.checked);
      statusSpan.classList.remove('hidden');
      setTimeout(() => statusSpan.classList.add('hidden'), 2000);
    } catch (e) {
      statusSpan.className = 'text-xs text-red-500 flex-none';
      statusSpan.textContent = e.message;
      statusSpan.classList.remove('hidden');
    } finally {
      saveBtn.disabled = false;
    }
  });

  row.append(nameSpan, descInput, toggleLabel, saveBtn, statusSpan);
  return row;
}

async function saveTagMeta(name, description, importable) {
  await apiFetch('/api/tags/' + encodeURIComponent(name), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ description, importable }),
  });
}

// ── End tags tab ───────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
  resetForm();
  loadTags();
  loadWords();
  renderTierFilter();
  initTranslateButton();

  // Handle ?edit=<wordId> for deep-linking to edit form (e.g. from training page)
  const editParam = new URLSearchParams(window.location.search).get('edit');
  if (editParam) {
    apiFetch(`/api/words/${editParam}`).then(word => {
      if (word) openEditForm(word);
    }).catch(() => {});
  }

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
    loadWords();
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
    pinyinTimer = setTimeout(() => fetchAndFillPinyin(zh), 500);
    const hanziwayLink = $('hanziway-link');
    if (zh) {
      hanziwayLink.href = 'https://hanziway.com/en/char?q=' + encodeURIComponent(zh);
      show('hanziway-link');
    } else {
      hide('hanziway-link');
    }
  });

  $('add-en-btn').addEventListener('click', () => addEnInput(''));
  $('add-de-btn').addEventListener('click', () => addDeInput(''));
  $('translate-btn').addEventListener('click', handleTranslate);

  $('form-cancel-btn').addEventListener('click', () => {
    resetForm();
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

  $('search-input').addEventListener('input', () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      searchQuery = $('search-input').value.trim();
      currentPage = 1;
      loadWords();
    }, 300);
  });

  $('prev-btn').addEventListener('click', () => {
    if (currentPage > 1) { currentPage--; loadWords(); }
  });

  $('next-page-btn').addEventListener('click', () => {
    currentPage++;
    loadWords();
  });

  $('download-btn').addEventListener('click', openDownloadModal);
  $('dl-cancel-btn').addEventListener('click', () => hide('download-modal'));
  $('dl-confirm-btn').addEventListener('click', executeDownload);
  $('download-modal').addEventListener('click', e => {
    if (e.target === $('download-modal')) hide('download-modal');
  });

  // Import tab
  $('tab-add').addEventListener('click', () => switchTab('add'));
  $('tab-import').addEventListener('click', () => switchTab('import'));
  $('tab-tags').addEventListener('click', () => switchTab('tags'));
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
    loadWords();
  });
});
