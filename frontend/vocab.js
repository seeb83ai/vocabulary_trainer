// Vocabulary management page logic

let currentPage = 1;
const PER_PAGE = 20;
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

async function loadWords() {
  const params = new URLSearchParams({
    q: searchQuery,
    page: currentPage,
    per_page: PER_PAGE,
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
    tr.innerHTML = `<td colspan="6" class="text-center py-8 text-gray-500">No vocabulary entries found.</td>`;
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
        ${word.needs_review ? '<span class="inline-block bg-orange-100 text-orange-600 text-xs px-1.5 py-0.5 rounded-full ml-1 align-middle">review</span>' : ''}
      </td>
      <td class="py-3 px-4 text-gray-600">${word.pinyin ? escHtml(word.pinyin) : '<span class="text-gray-400">—</span>'}</td>
      <td class="py-3 px-4">
        ${word.en_texts.map(escHtml).join(', ')}
        ${(word.tags || []).map(t => `<span class="inline-block bg-gray-200 text-gray-600 text-xs px-1.5 py-0.5 rounded-full ml-1">${escHtml(t)}</span>`).join('')}
      </td>
      <td class="py-3 px-4 whitespace-nowrap text-xs">${renderRepetitions(word)}</td>
      <td class="py-3 px-4 whitespace-nowrap text-xs">${renderDue(word)}</td>
      <td class="py-3 px-4 whitespace-nowrap">
        <button class="btn-edit text-blue-600 hover:text-blue-800 mr-3 font-medium" data-id="${word.id}">Edit</button>
        <button class="btn-delete text-red-600 hover:text-red-800 font-medium" data-id="${word.id}">Delete</button>
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

function renderPagination(total, page, perPage) {
  const totalPages = Math.max(1, Math.ceil(total / perPage));
  setText('page-info', `Page ${page} of ${totalPages} (${total} entries)`);
  $('prev-btn').disabled = page <= 1;
  $('next-page-btn').disabled = page >= totalPages;
}

function openEditForm(word) {
  editingWordId = word.id;
  setText('form-title', 'Edit Word');
  $('form-zh').value = word.zh_text;
  $('form-pinyin').value = word.pinyin || '';
  show('form-cancel-btn');

  let notice = $('review-notice');
  if (word.needs_review) {
    if (!notice) {
      notice = document.createElement('p');
      notice.id = 'review-notice';
      notice.className = 'text-sm text-orange-600 bg-orange-50 border border-orange-200 rounded-lg px-3 py-2';
      notice.textContent = 'This word is flagged for review — the flag will be cleared when you save.';
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

  formTags = [...(word.tags || [])];
  renderFormTags();

  $('word-form-panel').scrollIntoView({ behavior: 'smooth' });
  $('form-zh').focus();
}

function resetForm() {
  editingWordId = null;
  setText('form-title', 'Add Word');
  $('form-zh').value = '';
  $('form-pinyin').value = '';
  hide('form-cancel-btn');
  $('en-inputs-container').innerHTML = '';
  addEnInput('');
  formTags = [];
  renderFormTags();
  $('form-tag-input').value = '';
  const notice = $('review-notice');
  if (notice) notice.remove();
}

function addEnInput(value = '') {
  const container = $('en-inputs-container');
  const wrapper = document.createElement('div');
  wrapper.className = 'flex items-center gap-2 mb-2';
  wrapper.innerHTML = `
    <input type="text" class="en-input flex-1 border border-gray-300 rounded-lg px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
           placeholder="English translation" value="${escHtml(value)}">
    <button type="button" class="btn-remove-en text-gray-400 hover:text-red-500 text-xl leading-none" title="Remove">×</button>`;
  wrapper.querySelector('.btn-remove-en').addEventListener('click', () => {
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
    tags: [...formTags],
  };
}

async function handleFormSubmit(e) {
  e.preventDefault();
  const payload = buildFormPayload();
  if (!payload.zh_text) { alert('Chinese text is required.'); return; }
  if (!payload.en_texts.length) { alert('At least one English translation is required.'); return; }

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
  if (!confirm('Delete this word and all its translations? This cannot be undone.')) return;
  try {
    await apiFetch(`/api/words/${id}`, { method: 'DELETE' });
    loadTags();
    loadWords();
  } catch (e) {
    alert('Failed to delete: ' + e.message);
  }
}

function renderRepetitions(word) {
  if (word.total_attempts === 0) {
    return '<span class="text-gray-400">New</span>';
  }
  const pct = Math.round((word.total_correct / word.total_attempts) * 100);
  let barColor = 'bg-red-400';
  if (pct >= 80) barColor = 'bg-green-400';
  else if (pct >= 50) barColor = 'bg-yellow-400';
  return `
    <div class="flex flex-col gap-0.5 min-w-[90px]">
      <div class="flex items-center gap-1">
        <div class="w-16 h-1.5 bg-gray-200 rounded-full overflow-hidden">
          <div class="${barColor} h-full rounded-full" style="width:${pct}%"></div>
        </div>
        <span class="text-gray-500">${pct}%</span>
      </div>
      <div class="text-gray-400">${word.repetitions} reps</div>
    </div>`;
}

function renderDue(word) {
  if (word.total_attempts === 0) {
    return '<span class="text-gray-400">—</span>';
  }
  const due = new Date(word.due_date);
  const diffDays = Math.round((due - new Date()) / 86400000);
  if (diffDays <= 0) return '<span class="text-orange-500">Due</span>';
  return `<span class="text-gray-500">in ${diffDays}d</span>`;
}

async function loadTags() {
  try {
    allTags = await apiFetch('/api/tags');
  } catch (_) {
    allTags = [];
  }
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
    item.textContent = m === query && !allTags.includes(query) ? `Create "${m}"` : m;
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
  const en = enInputs.length > 0 ? enInputs[0].value.trim() : '';

  if (!zh && !en) {
    alert('Enter Chinese or English text first.');
    return;
  }

  const origText = btn.textContent;
  btn.textContent = 'Translating…';
  btn.disabled = true;

  try {
    const result = await apiFetch('/api/translate', {
      method: 'POST',
      body: JSON.stringify({ zh_text: zh, en_text: en }),
    });

    if (result.zh_text && !zh) {
      $('form-zh').value = result.zh_text;
    }
    if (result.pinyin && !$('form-pinyin').value.trim()) {
      $('form-pinyin').value = result.pinyin;
    }
    if (result.en_text && !en) {
      if (enInputs.length > 0) {
        enInputs[0].value = result.en_text;
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

document.addEventListener('DOMContentLoaded', () => {
  resetForm();
  loadTags();
  loadWords();
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

  $('add-en-btn').addEventListener('click', () => addEnInput(''));
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
});
