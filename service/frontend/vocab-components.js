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
  const tbody = $('components-tbody');
  tbody.innerHTML = '';
  if (!components || components.length === 0) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td colspan="6" class="text-center py-8 text-gray-500">${escHtml(t('vocab.noEntries'))}</td>`;
    tbody.appendChild(tr);
    return;
  }
  for (const comp of components) {
    const tr = document.createElement('tr');
    tr.className = 'border-b border-gray-200 hover:bg-gray-50';
    tr.dataset.char = comp.character;
    tr.innerHTML = `
      <td class="py-3 px-4 text-lg font-medium">${escHtml(comp.character)}${crossRefBadge(comp.is_also_word, t('vocab.alsoWord'))}</td>
      <td class="py-3 px-4 text-gray-500 text-sm">${comp.pinyin ? escHtml(comp.pinyin) : '<span class="text-gray-400">—</span>'}</td>
      <td class="py-3 px-4 text-gray-600">${comp.definition_en ? escHtml(comp.definition_en) : '<span class="text-gray-400">—</span>'}</td>
      <td class="py-3 px-4 text-gray-600">${comp.definition_de ? escHtml(comp.definition_de) : '<span class="text-gray-400">—</span>'}</td>
      <td class="py-3 px-4 whitespace-nowrap">${renderComponentLevel(comp)}</td>
      <td class="py-3 px-4 whitespace-nowrap text-xs">${renderComponentDue(comp)}</td>
      <td class="py-3 px-4 whitespace-nowrap">
        <button class="btn-comp-edit text-blue-600 hover:text-blue-800 font-medium"
                data-char="${escHtml(comp.character)}"
                data-en="${escHtml(comp.definition_en || '')}"
                data-de="${escHtml(comp.definition_de || '')}">${escHtml(t('vocab.edit'))}</button>
      </td>`;
    tbody.appendChild(tr);
  }

  tbody.querySelectorAll('.btn-comp-edit').forEach(btn => {
    btn.addEventListener('click', () => openComponentEdit(btn.dataset.char));
  });
}

function renderComponentLevel(comp) {
  if (!comp.first_seen_date) {
    return `<span class="text-gray-400 text-xs">${escHtml(t('vocab.unseen'))}</span>`;
  }
  const tier = wordTier(comp.total_correct, comp.total_attempts, false, 0);
  if (!tier) return `<span class="text-gray-400 text-xs">${escHtml(t('vocab.unseen'))}</span>`;
  const pct = comp.total_attempts > 0 ? Math.round(comp.total_correct / comp.total_attempts * 100) : 0;
  return `<span class="inline-block px-2 py-0.5 rounded-full text-xs font-medium ${tier.pill}">${t(tier.i18nKey)}</span><span class="ml-1.5 text-xs text-gray-400">${pct}%</span>`;
}

function renderComponentDue(comp) {
  if (!comp.due_date) return '<span class="text-gray-400">—</span>';
  const today = new Date().toISOString().slice(0, 10);
  if (comp.due_date <= today) return `<span class="text-orange-500">${escHtml(t('vocab.dueLabel'))}</span>`;
  const diffDays = Math.round((new Date(comp.due_date) - new Date(today)) / 86400000);
  return `<span class="text-gray-500">${escHtml(t('vocab.inDays', { n: diffDays }))}</span>`;
}

let editingCompChar = null;

function openComponentEdit(char) {
  editingCompChar = char;
  $('comp-edit-char').textContent = char;
  const hanziwayLink = $('comp-hanziway-link');
  hanziwayLink.href = 'https://hanziway.com/en/char?q=' + encodeURIComponent(char);
  show('comp-hanziway-link');
  const tabComp = $('tab-comp');
  tabComp.classList.remove('opacity-40', 'cursor-not-allowed', 'pointer-events-none', 'text-gray-400');
  tabComp.classList.add('hover:text-gray-700', 'text-gray-500');
  $('comp-edit-form').innerHTML = `<span class="text-gray-400 text-sm">${escHtml(t('vocab.loading') || 'Loading…')}</span>`;
  switchTab('comp');
  $('word-form-panel').scrollIntoView({ behavior: 'smooth' });

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
