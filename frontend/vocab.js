// Vocabulary management page logic

let currentPage = 1;
const PER_PAGE = 20;
let searchQuery = '';
let editingWordId = null;
let searchTimer = null;

async function loadWords() {
  const params = new URLSearchParams({
    q: searchQuery,
    page: currentPage,
    per_page: PER_PAGE,
  });
  try {
    const data = await apiFetch(`/api/words?${params}`);
    renderTable(data.words);
    renderPagination(data.total, data.page, data.per_page);
  } catch (e) {
    alert('Failed to load words: ' + e.message);
  }
}

function renderTable(words) {
  const tbody = $('words-tbody');
  tbody.innerHTML = '';
  if (!words || words.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td colspan="4" class="text-center py-8 text-gray-500">No vocabulary entries found.</td>`;
    tbody.appendChild(tr);
    return;
  }
  for (const word of words) {
    const tr = document.createElement('tr');
    tr.className = 'border-b border-gray-200 hover:bg-gray-50';
    tr.innerHTML = `
      <td class="py-3 px-4 text-lg font-medium">${escHtml(word.zh_text)}</td>
      <td class="py-3 px-4 text-gray-600">${word.pinyin ? escHtml(word.pinyin) : '<span class="text-gray-400">—</span>'}</td>
      <td class="py-3 px-4">${word.en_texts.map(escHtml).join(', ')}</td>
      <td class="py-3 px-4 whitespace-nowrap text-xs">${renderProgress(word)}</td>
      <td class="py-3 px-4 whitespace-nowrap">
        <button class="btn-edit text-blue-600 hover:text-blue-800 mr-3 font-medium" data-id="${word.id}">Edit</button>
        <button class="btn-delete text-red-600 hover:text-red-800 font-medium" data-id="${word.id}">Delete</button>
      </td>`;
    tbody.appendChild(tr);
  }

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

  const container = $('en-inputs-container');
  container.innerHTML = '';
  for (const t of (word.en_texts.length ? word.en_texts : [''])) {
    addEnInput(t);
  }

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
    loadWords();
  } catch (e) {
    alert('Error: ' + e.message);
  }
}

async function deleteWord(id) {
  if (!confirm('Delete this word and all its translations? This cannot be undone.')) return;
  try {
    await apiFetch(`/api/words/${id}`, { method: 'DELETE' });
    loadWords();
  } catch (e) {
    alert('Failed to delete: ' + e.message);
  }
}

function renderProgress(word) {
  if (word.total_attempts === 0) {
    return '<span class="text-gray-400">New</span>';
  }
  const pct = word.total_attempts > 0
    ? Math.round((word.total_correct / word.total_attempts) * 100)
    : 0;
  const due = new Date(word.due_date);
  const now = new Date();
  const diffDays = Math.round((due - now) / 86400000);
  const dueStr = diffDays <= 0 ? '<span class="text-orange-500">Due</span>'
    : `in ${diffDays}d`;

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
      <div class="text-gray-400">${word.repetitions} reps · ${dueStr}</div>
    </div>`;
}

document.addEventListener('DOMContentLoaded', () => {
  resetForm();
  loadWords();

  $('word-form').addEventListener('submit', handleFormSubmit);

  $('add-en-btn').addEventListener('click', () => addEnInput(''));

  $('form-cancel-btn').addEventListener('click', () => {
    resetForm();
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
