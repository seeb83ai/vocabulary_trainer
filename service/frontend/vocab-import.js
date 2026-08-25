// vocab-import.js — library/CSV import tab and modal

// ── CSV Upload ────────────────────────────────────────────────────────────────

function openCsvUploadModal() {
  csvUploadTags = [];
  csvUploadWordCount = 0;
  csvUploadFile = null;
  $('csv-upload-file').value = '';
  $('csv-upload-tag-input').value = '';
  $('csv-upload-preview-info').classList.add('hidden');
  $('csv-upload-slider-row').classList.add('hidden');
  $('csv-upload-slider').max = 0;
  $('csv-upload-slider').value = 0;
  $('csv-upload-slider-val').textContent = '0';
  $('csv-upload-slider-max').textContent = '0';
  $('csv-upload-status').classList.add('hidden');
  $('csv-upload-status').textContent = '';
  renderCsvUploadTags();
  updateCsvUploadSubmitState();
  show('csv-upload-modal');
}

function closeCsvUploadModal() {
  hide('csv-upload-modal');
}

function renderCsvUploadTags() {
  const container = $('csv-upload-tags');
  container.innerHTML = '';
  for (const tag of csvUploadTags) {
    const pill = document.createElement('span');
    pill.className = 'inline-flex items-center bg-gray-200 text-gray-700 text-sm px-2 py-0.5 rounded-full';
    pill.innerHTML = `${escHtml(tag)} <button type="button" class="ml-1 text-gray-400 hover:text-red-500 leading-none">&times;</button>`;
    pill.querySelector('button').addEventListener('click', () => {
      csvUploadTags = csvUploadTags.filter(t => t !== tag);
      renderCsvUploadTags();
      updateCsvUploadSubmitState();
    });
    container.appendChild(pill);
  }
}

function showCsvUploadTagAutocomplete(query) {
  const dropdown = $('csv-upload-tag-autocomplete');
  const q = query.toLowerCase();
  const matches = allTags.filter(t => t.toLowerCase().includes(q) && !csvUploadTags.includes(t));
  if (q && !allTags.includes(query) && !csvUploadTags.includes(query)) {
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
    item.textContent = m;
    item.addEventListener('mousedown', (e) => {
      e.preventDefault();
      addCsvUploadTag(m);
    });
    dropdown.appendChild(item);
  }
  dropdown.classList.remove('hidden');
}

function addCsvUploadTag(tag) {
  tag = tag.trim();
  if (!tag || csvUploadTags.includes(tag)) return;
  csvUploadTags.push(tag);
  renderCsvUploadTags();
  $('csv-upload-tag-input').value = '';
  $('csv-upload-tag-autocomplete').classList.add('hidden');
  updateCsvUploadSubmitState();
}

function onCsvFileChange(file) {
  csvUploadFile = file || null;
  if (!file) {
    $('csv-upload-preview-info').classList.add('hidden');
    $('csv-upload-slider-row').classList.add('hidden');
    updateCsvUploadSubmitState();
    return;
  }
  const reader = new FileReader();
  reader.onload = (e) => {
    const text = e.target.result;
    // Count non-empty lines after the header (rough count for slider max only)
    const lines = text.split('\n').filter((l, i) => i > 0 && l.trim() !== '');
    csvUploadWordCount = lines.length;
    $('csv-upload-slider').max = csvUploadWordCount;
    $('csv-upload-slider').value = 0;
    $('csv-upload-slider-val').textContent = '0';
    $('csv-upload-slider-max').textContent = String(csvUploadWordCount);
    $('csv-upload-preview-info').textContent = `${csvUploadWordCount} word row${csvUploadWordCount !== 1 ? 's' : ''} detected`;
    $('csv-upload-preview-info').classList.remove('hidden');
    $('csv-upload-slider-row').classList.remove('hidden');
    updateCsvUploadSubmitState();
  };
  reader.readAsText(file);
}

function updateCsvUploadSubmitState() {
  const ready = csvUploadFile !== null && csvUploadTags.length > 0;
  $('csv-upload-submit-btn').disabled = !ready;
}

async function executeCsvUpload() {
  const btn = $('csv-upload-submit-btn');
  btn.disabled = true;
  const status = $('csv-upload-status');
  status.classList.remove('hidden');
  status.textContent = 'Uploading…';

  const formData = new FormData();
  formData.append('file', csvUploadFile);
  formData.append('tags', csvUploadTags.join(','));
  formData.append('start_training_count', $('csv-upload-slider').value);

  try {
    const resp = await fetch('/api/words/upload-csv', { method: 'POST', body: formData });
    if (resp.status === 401) { window.location.href = '/login'; return; }
    const data = await resp.json();
    if (!resp.ok) {
      status.textContent = `Error: ${data.error || resp.statusText}`;
      status.className = 'text-sm text-red-600';
      btn.disabled = false;
      return;
    }
    status.textContent = `Done — imported: ${data.imported}, updated: ${data.updated}, skipped: ${data.skipped}`;
    status.className = 'text-sm text-green-600';
    loadWords();
    loadTags();
    setTimeout(closeCsvUploadModal, 2000);
  } catch (err) {
    status.textContent = `Network error: ${err.message}`;
    status.className = 'text-sm text-red-600';
    btn.disabled = false;
  }
}

// ── Import tab ────────────────────────────────────────────────────────────────

function switchTab(name) {
  const tabs = ['add', 'import', 'tags', 'comp'];
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
    importAllTags = await apiFetch('/api/import/source-tags');
    renderImportTagPills();
  } catch (e) {
    list.innerHTML = `<span class="text-sm text-red-500">${escHtml(e.message)}</span>`;
  }
}

function importTagMatchesFilter(tag) {
  if (importFilterLangs.size === 0) return true;
  const langs = tag.available_langs || [];
  if (importFilterMode === 'all') {
    for (const lang of importFilterLangs) {
      if (!langs.includes(lang)) return false;
    }
    return true;
  }
  // any
  for (const lang of importFilterLangs) {
    if (langs.includes(lang)) return true;
  }
  return false;
}

function renderImportTagPills() {
  const list = $('import-tag-list');
  list.innerHTML = '';
  const visible = importAllTags.filter(importTagMatchesFilter);
  if (visible.length === 0) {
    list.innerHTML = `<span class="text-sm text-gray-400">${escHtml(importAllTags.length === 0 ? t('vocab.importNoTags') : t('vocab.importNoTagsMatch'))}</span>`;
    // If selected tag is now hidden, clear selection and preview.
    if (importSelectedTag) {
      importSelectedTag = '';
      $('import-next-btn').disabled = true;
      hide('import-preview');
    }
    return;
  }
  let selectedStillVisible = false;
  for (const tag of visible) {
    const pill = document.createElement('button');
    pill.type = 'button';
    pill.dataset.tagName = tag.name;
    const isSelected = tag.name === importSelectedTag;
    if (isSelected) selectedStillVisible = true;
    pill.className = 'import-source-tag px-3 py-1 rounded-full text-sm font-medium border transition ' +
      (isSelected
        ? 'bg-blue-600 text-white border-blue-600'
        : 'border-gray-300 text-gray-600 hover:bg-blue-50 hover:border-blue-400 hover:text-blue-600');
    pill.textContent = tag.name;
    if (tag.description) pill.title = tag.description;
    pill.addEventListener('click', () => selectImportSourceTag(tag, pill));
    list.appendChild(pill);
  }
  if (!selectedStillVisible && importSelectedTag) {
    importSelectedTag = '';
    $('import-next-btn').disabled = true;
    hide('import-preview');
  }
}

async function selectImportSourceTag(tag) {
  importSelectedTag = tag.name;
  $('import-next-btn').disabled = true;
  hide('import-preview');
  renderImportTagPills();
  await loadImportPreview(tag.name, tag.description);
}

async function loadImportPreview(tagName, tagDescription) {
  const descEl = $('import-preview-desc');
  const statsEl = $('import-preview-stats');
  const tableWrap = $('import-preview-table-wrap');
  const tbody = $('import-preview-tbody');

  statsEl.textContent = t('vocab.importLoading');
  descEl.classList.add('hidden');
  tableWrap.classList.add('hidden');
  tbody.innerHTML = '';
  show('import-preview');

  try {
    const data = await apiFetch('/api/import/preview?tag=' + encodeURIComponent(tagName));

    // Description line
    if (tagDescription) {
      descEl.textContent = tagDescription;
      descEl.classList.remove('hidden');
    } else {
      descEl.classList.add('hidden');
    }

    if (data.total === 0) {
      statsEl.textContent = t('vocab.importPreviewEmpty');
      $('import-next-btn').disabled = true;
      return;
    }

    // Stats line: "123 words · 120 EN · 45 DE · ..."
    const availLangs = data.available_langs || {};
    const parts = [`${data.total} ${t('vocab.importPreviewWords')}`];
    for (const [lang, count] of Object.entries(availLangs).sort()) {
      if (count > 0) parts.push(`${count} ${lang.toUpperCase()}`);
    }
    statsEl.textContent = parts.join(' · ');

    // Example table (up to 50 rows)
    const hasDe = (data.examples || []).some(e => (e.translations || {})['de']?.length > 0);
    for (const ex of (data.examples || [])) {
      const tr = document.createElement('tr');
      tr.className = 'border-b border-gray-100 last:border-0';
      const exTransl = ex.translations || {};
      const en = (exTransl['en'] || []).map(escHtml).join(', ') || '<span class="text-gray-300">—</span>';
      const de = (exTransl['de'] || []).map(escHtml).join(', ') || '<span class="text-gray-300">—</span>';
      tr.innerHTML = `
        <td class="py-1 px-2 font-medium">${escHtml(ex.zh_text)}</td>
        <td class="py-1 px-2 text-gray-500">${escHtml(ex.pinyin)}</td>
        <td class="py-1 px-2 text-gray-700">${en}</td>
        <td class="py-1 px-2 text-gray-500">${hasDe ? de : ''}</td>`;
      tbody.appendChild(tr);
    }
    tableWrap.classList.remove('hidden');
    $('import-next-btn').disabled = false;
  } catch (e) {
    statsEl.textContent = e.message;
    $('import-next-btn').disabled = true;
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
        import_langs: [
          ...($('import-en').checked ? ['en'] : []),
          ...($('import-de').checked ? ['de'] : []),
        ],
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
  importFilterLangs = new Set();
  importFilterMode = 'any';
  // Reset filter button styles
  ['en', 'de'].forEach(lang => {
    const btn = $('import-filter-' + lang);
    if (btn) {
      btn.classList.remove('bg-blue-600', 'text-white', 'border-blue-600');
      btn.classList.add('border-gray-300', 'text-gray-500');
    }
  });
  const anyRadio = document.querySelector('input[name="import-filter-mode"][value="any"]');
  if (anyRadio) anyRadio.checked = true;
  showImportStep(1);
  hide('import-preview');
  $('import-next-btn').disabled = true;
  hide('import-status');
  if ($('import-en')) $('import-en').checked = true;
  if ($('import-de')) $('import-de').checked = false;
}
