// vocab-tags.js — word/form tag pills and tags-tab management

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
