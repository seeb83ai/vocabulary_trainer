// vocab-components.js — components table and component edit tab

async function loadComponents() {
  const params = new URLSearchParams({
    q: searchQuery,
    page: compPage,
    per_page: perPage,
  });
  if (compReviewFilterActive) params.set('review', '1');
  try {
    const data = await apiFetch(`/api/components?${params}`);
    renderComponentTable(data.components);
    renderPagination(data.total, data.page, data.per_page);
  } catch (e) {
    alert('Failed to load components: ' + e.message);
  }
}

function renderComponentTable(components) {
  const list = $('components-tbody');
  list.innerHTML = '';
  if (!components || components.length === 0) {
    list.innerHTML = `<div class="vb-empty">${escHtml(t('vocab.noEntries'))}</div>`;
    return;
  }
  for (const comp of components) {
    const row = document.createElement('div');
    row.className = 'vb-row btn-comp-edit';
    row.setAttribute('role', 'listitem');
    row.tabIndex = 0;
    row.dataset.char = comp.character;
    row.innerHTML = renderComponentRow(comp);
    const open = () => openComponentEdit(comp.character);
    row.addEventListener('click', open);
    row.addEventListener('keydown', e => { if (e.key === 'Enter') open(); });
    list.appendChild(row);
  }
}

// renderComponentRow renders the inner HTML of one component list row.
function renderComponentRow(comp) {
  const line = (lang, text) => `<span><span class="vb-lang">${lang}</span>${text ? escHtml(text) : '<span class="vb-row-py">—</span>'}</span>`;
  return `
    <div class="vb-row-main">
      <div class="vb-row-head">
        <span class="vb-row-zh font-hanzi">${escHtml(comp.character)}</span>
        <span class="vb-row-py">${comp.pinyin ? escHtml(comp.pinyin) : '—'}</span>
        ${crossRefBadge(comp.is_also_word, t('vocab.alsoWord'))}
      </div>
      <div class="vb-row-lines">${line('EN', comp.definition_en)}${line('DE', comp.definition_de)}</div>
    </div>
    <div class="vb-row-side">${renderComponentLevel(comp)}${renderComponentDue(comp)}</div>`;
}

function renderComponentLevel(comp) {
  if (!comp.first_seen_date) return tierChipFor(null);
  const tier = wordTier(comp.total_correct, comp.total_attempts, false, 0);
  if (!tier) return tierChipFor(null);
  const pct = comp.total_attempts > 0 ? Math.round(comp.total_correct / comp.total_attempts * 100) : 0;
  return tierChipFor(tier, pct);
}

function renderComponentDue(comp) {
  if (!comp.due_date) return '';
  const today = new Date().toISOString().slice(0, 10);
  if (comp.due_date <= today) return `<span class="vb-due is-today">${escHtml(t('vocab.dueTodayLabel'))}</span>`;
  const diffDays = Math.round((new Date(comp.due_date) - new Date(today)) / 86400000);
  return `<span class="vb-due">${escHtml(t(diffDays === 1 ? 'vocab.dueTomorrowLabel' : 'vocab.inDays', { n: diffDays }))}</span>`;
}

let editingCompChar = null;

function openComponentEdit(char) {
  editingCompChar = char;
  $('comp-edit-char').textContent = char;
  const hanziwayLink = $('comp-hanziway-link');
  hanziwayLink.href = 'https://hanziway.com/en/char?q=' + encodeURIComponent(char);
  show('comp-hanziway-link');
  $('tab-comp').classList.remove('is-disabled');
  $('comp-edit-form').innerHTML = `<span class="text-gray-400 text-sm">${escHtml(t('vocab.loading') || 'Loading…')}</span>`;
  openVocabSheet('comp');

  Promise.all([
    apiFetch(`/api/components/${encodeURIComponent(char)}/translations`),
    apiFetch(`/api/components/${encodeURIComponent(char)}/hmm/context`).catch(() => null),
  ]).then(([data, hmmCtx]) => {
    $('comp-edit-pinyin').textContent = hmmCtx?.pinyin || '';

    const langs = [primaryLang];
    if (secondaryLang) langs.push(secondaryLang);
    const form = $('comp-edit-form');
    form.innerHTML = '';
    for (const lang of langs) {
      const raw = data[lang] || data[lang.toUpperCase()] || '';
      const parts = raw ? raw.split(/[,;]+/).map(s => s.trim()).filter(Boolean) : [''];
      const langLabel = LANG_NAMES[lang] || lang.toUpperCase();
      const section = document.createElement('div');
      section.className = 'space-y-2';
      section.dataset.lang = lang;
      section.innerHTML = `
        <label class="block text-sm font-medium text-gray-700">${escHtml(langLabel)} Translation(s)</label>
        <div class="comp-trans-inputs space-y-1.5"></div>
        <button type="button" class="btn-add-comp-trans text-sm text-blue-600 hover:text-blue-800 font-medium">+ ${escHtml(t('vocab.addTranslation') || 'Add')}</button>`;
      const inputsDiv = section.querySelector('.comp-trans-inputs');
      for (const part of parts) inputsDiv.appendChild(makeCompTransInput(lang, part));
      section.querySelector('.btn-add-comp-trans').addEventListener('click', () =>
        inputsDiv.appendChild(makeCompTransInput(lang, ''))
      );
      form.appendChild(section);
    }

    const builderSection = document.createElement('div');
    builderSection.id = 'comp-hmm-builder';
    builderSection.className = 'pt-2 border-t border-gray-100';
    form.appendChild(builderSection);
    const enDef = data['en'] || data['EN'] || '';
    const deDef = data['de'] || data['DE'] || '';
    const translations = {};
    if (enDef) translations['en'] = enDef.split(/[,;]+/).map(s => s.trim()).filter(Boolean);
    if (deDef) translations['de'] = deDef.split(/[,;]+/).map(s => s.trim()).filter(Boolean);
    loadCompHMMBuilder('comp-hmm-builder', char, { preloadedCtx: hmmCtx, zh: char, translations });
  }).catch(e => {
    $('comp-edit-form').innerHTML = `<span class="text-red-500 text-sm">${escHtml(e.message)}</span>`;
  });
}

function makeCompTransInput(lang, value) {
  const div = document.createElement('div');
  div.className = 'flex gap-2';
  div.innerHTML = `
    <input type="text" class="comp-trans-input flex-1 border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
           data-lang="${escHtml(lang)}" value="${escHtml(value)}">
    <button type="button" class="btn-remove-comp-trans text-gray-400 hover:text-red-500 text-xl leading-none px-1" title="Remove">×</button>`;
  div.querySelector('.btn-remove-comp-trans').addEventListener('click', () => div.remove());
  return div;
}
