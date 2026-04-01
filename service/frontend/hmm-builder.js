// HMM Scene Builder — reusable component for vocab edit and training result pages.
//
// Usage:
//   loadHMMBuilder('container-id', wordId)              — editable builder
//   loadHMMBuilder('container-id', wordId, {readOnly:true}) — read-only display
//   renderHMMSceneReadOnly('container-id', sceneText)  — static scene text display

// IDS operators with label keys (U+2FF0–U+2FFB)
const IDS_OPERATORS = [
  { op: '⿰', key: 'hmm.ids.lr',      arity: 2 },
  { op: '⿱', key: 'hmm.ids.tb',      arity: 2 },
  { op: '⿸', key: 'hmm.ids.ulFrame', arity: 2 },
  { op: '⿺', key: 'hmm.ids.llFrame', arity: 2 },
  { op: '⿹', key: 'hmm.ids.rFrame',  arity: 2 },
  { op: '⿴', key: 'hmm.ids.encl',    arity: 2 },
  { op: '⿵', key: 'hmm.ids.enclB',   arity: 2 },
  { op: '⿶', key: 'hmm.ids.enclT',   arity: 2 },
  { op: '⿷', key: 'hmm.ids.enclR',   arity: 2 },
  { op: '⿻', key: 'hmm.ids.over',    arity: 2 },
  { op: '⿲', key: 'hmm.ids.lrr',     arity: 3 },
  { op: '⿳', key: 'hmm.ids.tbb',     arity: 3 },
];

// Renders insert-only IDS operator buttons (no selection state).
// Clicking a button inserts the operator into the target input at the cursor.
function buildIDSInsertButtons(targetInputId) {
  const btnClass = 'hmm-ids-insert inline-flex flex-col items-center px-1.5 py-0.5 rounded text-xs border border-gray-200 bg-white text-gray-500 hover:border-purple-300 hover:text-purple-600 transition';
  const buttons = IDS_OPERATORS.map(({ op, key }) =>
    `<button type="button" class="${btnClass}" data-op="${escHtml(op)}" data-target="${escHtml(targetInputId)}" title="${escHtml(t(key))}">${escHtml(op)}<span class="text-gray-400 text-[9px] leading-tight">${escHtml(t(key))}</span></button>`
  ).join('');
  return `<div class="flex flex-wrap gap-1">${buttons}</div>`;
}

const HMM_CATEGORY_DOTS = {
  male:      'bg-blue-500',
  female:    'bg-pink-500',
  fictional: 'bg-green-500',
  wildcard:  'bg-purple-500',
};

function buildPromptLine(actor, location, room, props) {
  const parts = [];
  if (actor) parts.push(`<strong>${escHtml(actor)}</strong>`);
  else parts.push('<em class="text-gray-400">???</em>');
  parts.push(' is in ');
  if (location) parts.push(`<strong>${escHtml(location)}</strong>`);
  else parts.push('<em class="text-gray-400">???</em>');
  parts.push(', in the ');
  if (room) parts.push(`<strong>${escHtml(room)}</strong>`);
  else parts.push('<em class="text-gray-400">???</em>');
  if (props.length) {
    parts.push('. They see ');
    parts.push(props.map(p => `<strong>${escHtml(p)}</strong>`).join(', '));
  }
  parts.push('...');
  return parts.join('');
}

async function loadHMMBuilder(containerId, wordId, opts = {}) {
  const container = document.getElementById(containerId);
  if (!container) return;
  const readOnly = opts.readOnly || false;

  container.innerHTML = `<div class="text-sm text-gray-400">${t('hmm.loadingData')}</div>`;

  let ctx;
  try {
    ctx = await apiFetch(`/api/words/${wordId}/hmm/context`);
  } catch (e) {
    container.innerHTML = '';
    return;
  }

  // Build a global radical→prop_name lookup for autofill on new rows.
  const propsLookup = {};
  try {
    const allProps = await apiFetch('/api/hmm/props');
    for (const p of allProps) {
      if (p.radical && p.prop_name) propsLookup[p.radical] = p.prop_name;
    }
  } catch (e) { /* non-fatal */ }

  if (readOnly) {
    renderReadOnlyBuilder(container, ctx);
  } else {
    renderEditableBuilder(container, wordId, ctx, propsLookup);
  }
}

function renderReadOnlyBuilder(container, ctx) {
  const scene = ctx.scene;
  if (!scene || !scene.scene_text) {
    container.innerHTML = '';
    return;
  }

  const actorName = ctx.actor?.actor_name || '';
  const locName = ctx.location?.location_name || '';
  const roomName = ctx.tone_room?.room_name || '';

  let breakdownHtml = '';
  const parts = [];
  if (actorName) parts.push(`<span class="font-medium">${escHtml(actorName)}</span> <span class="text-gray-400">(${escHtml(ctx.initial)})</span>`);
  if (locName) parts.push(`<span class="font-medium">${escHtml(locName)}</span> <span class="text-gray-400">(${escHtml(ctx.final)})</span>`);
  if (roomName) parts.push(`<span class="font-medium">${escHtml(roomName)}</span> <span class="text-gray-400">(tone ${ctx.tone})</span>`);
  if (parts.length) {
    breakdownHtml = `<div class="text-xs text-gray-400 mb-1">${parts.join(' · ')}</div>`;
  }

  container.innerHTML = `
    <div class="p-3 bg-purple-50 border border-purple-200 rounded-xl">
      <div class="text-xs text-purple-500 uppercase tracking-wide mb-1">${t('hmm.mnemonicScene')}</div>
      ${breakdownHtml}
      <div class="text-sm text-gray-700 whitespace-pre-wrap">${escHtml(scene.scene_text)}</div>
    </div>
  `;
}

function renderEditableBuilder(container, wordId, ctx, propsLookup = {}) {
  const actorName = ctx.actor?.actor_name || '';
  const actorHint = ctx.actor?.hint || '';
  const actorCat = ctx.actor?.category || 'male';
  const locName = ctx.location?.location_name || '';
  const roomName = ctx.tone_room?.room_name || '';
  const sceneText = ctx.scene?.scene_text || '';
  const dotClass = HMM_CATEGORY_DOTS[actorCat] || 'bg-gray-400';
  const decomposition = ctx.decomposition || '';

  const catLabel = t('hmm.cat.' + actorCat) || actorCat;

  // Build props rows
  const allRadicals = ctx.radicals || [];
  const propMap = {};
  for (const p of (ctx.props || [])) propMap[p.radical] = p.prop_name;

  // Track which props existed on load so we can DELETE removed ones
  const originalPropRadicals = new Set((ctx.props || []).map(p => p.radical));
  const deletedRadicals = new Set();

  const propRowClass = 'flex items-center gap-2 hmm-prop-row';
  const propInputClass = 'hmm-prop-input flex-1 border border-gray-200 rounded px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-purple-400';
  const removeBtnClass = 'hmm-remove-prop text-gray-300 hover:text-red-400 text-xl leading-none px-1 transition shrink-0';

  let propsRowsHtml = '';
  for (const rad of allRadicals) {
    const pName = propMap[rad] || '';
    propsRowsHtml += `
      <div class="${propRowClass}" data-radical="${escHtml(rad)}">
        <span class="w-8 text-center text-lg shrink-0">${escHtml(rad)}</span>
        <input type="text" value="${escHtml(pName)}" placeholder="${escHtml(t('hmm.propPlaceholder', {rad}))}"
          class="${propInputClass}">
        <button class="${removeBtnClass}" title="${escHtml(t('hmm.removeProp'))}">×</button>
      </div>`;
  }

  const initialDisplay = ctx.initial === 'null' ? 'Ø' : (ctx.initial || '?');
  const finalDisplay = ctx.final === 'null' ? 'Ø' : (ctx.final || '?');

  const actorPlaceholder = actorHint
    ? t('hmm.actorPlaceholderHint', { cat: catLabel, hint: actorHint })
    : t('hmm.actorPlaceholder', { cat: catLabel });

  const locPlaceholder = ctx.final === 'null'
    ? t('hmm.locPlaceholderNull')
    : t('hmm.locPlaceholder', { final: finalDisplay });

  const roomPlaceholder = roomName
    ? ''
    : t('hmm.roomPlaceholder', { tone: ctx.tone || '?' });

  container.innerHTML = `
    <div class="border border-purple-200 rounded-xl p-4 bg-purple-50/50 space-y-3">
      <div class="flex items-center justify-between">
        <div class="text-sm font-semibold text-purple-700">${t('hmm.sceneBuilder')}</div>
        <button id="hmm-help-toggle" class="text-xs text-purple-400 hover:text-purple-600 transition">${t('hmm.howItWorks')}</button>
      </div>

      <div id="hmm-help-box" class="hidden text-xs text-gray-600 bg-white border border-purple-100 rounded-lg p-3 space-y-1">
        <p><strong>${escHtml(t('hmm.helpIntro'))}</strong></p>
        <p><span class="inline-block w-2 h-2 rounded-full ${dotClass}"></span> ${escHtml(t('hmm.helpActor', { cat: catLabel.toLowerCase(), initial: initialDisplay }))}</p>
        <p>${escHtml(t('hmm.helpLocation', { final: finalDisplay }))}</p>
        <p>${escHtml(t('hmm.helpRoom', { tone: ctx.tone || '?' }))}</p>
        <p>${escHtml(t('hmm.helpProps'))}</p>
        <p class="text-gray-400 mt-1">${escHtml(t('hmm.helpTip'))}</p>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <div>
          <div class="flex items-center gap-1 mb-0.5">
            <span class="w-2.5 h-2.5 rounded-full ${dotClass} shrink-0"></span>
            <span class="text-xs text-gray-400">${escHtml(catLabel)} ${t('hmm.actor')}</span>
            <span class="text-xs text-gray-300">(${escHtml(initialDisplay)})</span>
          </div>
          <input id="hmm-actor" type="text" value="${escHtml(actorName)}" placeholder="${escHtml(actorPlaceholder)}"
            class="w-full border border-gray-200 rounded px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-purple-400">
        </div>
        <div>
          <div class="flex items-center gap-1 mb-0.5">
            <span class="text-xs text-gray-400">${t('hmm.location')}</span>
            <span class="text-xs text-gray-300">(final: ${escHtml(finalDisplay)})</span>
          </div>
          <input id="hmm-location" type="text" value="${escHtml(locName)}" placeholder="${escHtml(locPlaceholder)}"
            class="w-full border border-gray-200 rounded px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-purple-400">
        </div>
        <div>
          <div class="flex items-center gap-1 mb-0.5">
            <span class="text-xs text-gray-400">${t('hmm.room')}</span>
            <span class="text-xs text-gray-300">(${t('hmm.tone', {n: ctx.tone || '?'})})</span>
          </div>
          <input id="hmm-room" type="text" value="${escHtml(roomName)}" placeholder="${escHtml(roomPlaceholder)}"
            class="w-full border border-gray-200 rounded px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-purple-400">
        </div>
      </div>

      <div class="space-y-1.5">
        <div class="text-xs text-gray-400">${t('hmm.decompLabel')} <span class="text-gray-300">(${t('hmm.decompDesc')})</span></div>
        <input id="hmm-decomp" type="text" value="${escHtml(decomposition)}"
          placeholder="${escHtml(t('hmm.decompPlaceholder'))}"
          class="w-full border border-gray-200 rounded px-2 py-1 text-sm font-mono focus:outline-none focus:ring-1 focus:ring-purple-400">
        ${buildIDSInsertButtons('hmm-decomp')}
      </div>

      <div class="space-y-1">
        <div class="flex items-center justify-between">
          <div class="text-xs text-gray-400">${t('hmm.props')} <span class="text-gray-300">(${t('hmm.propsDesc')})</span></div>
          <button id="hmm-add-prop" class="text-xs text-purple-500 hover:text-purple-700 transition">${t('hmm.addProp')}</button>
        </div>
        <div id="hmm-props-list" class="space-y-1">${propsRowsHtml}</div>
      </div>

      <div id="hmm-prompt-line" class="text-xs text-gray-500 italic"></div>

      <textarea id="hmm-scene-text" rows="3" placeholder="${escHtml(t('hmm.scenePlaceholder'))}"
        class="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-purple-400 resize-y">${escHtml(sceneText)}</textarea>

      <div class="flex items-center gap-3">
        <button id="hmm-save-btn" class="px-4 py-1.5 bg-purple-600 hover:bg-purple-700 text-white text-sm font-medium rounded-lg transition">${t('hmm.saveScene')}</button>
        <span id="hmm-save-status" class="hidden text-xs text-green-500">${t('hmm.savedBang')}</span>
      </div>
    </div>
  `;

  // Toggle help box
  document.getElementById('hmm-help-toggle').addEventListener('click', () => {
    document.getElementById('hmm-help-box').classList.toggle('hidden');
  });

  // IDS insert buttons — insert operator at cursor position in the target input
  container.querySelectorAll('.hmm-ids-insert').forEach(btn => {
    btn.addEventListener('click', () => {
      const input = document.getElementById(btn.dataset.target);
      if (!input) return;
      const start = input.selectionStart ?? input.value.length;
      const end = input.selectionEnd ?? input.value.length;
      input.value = input.value.slice(0, start) + btn.dataset.op + input.value.slice(end);
      const pos = start + btn.dataset.op.length;
      input.setSelectionRange(pos, pos);
      input.focus();
    });
  });

  // Update prompt line dynamically
  function updatePrompt() {
    const actor = document.getElementById('hmm-actor').value.trim();
    const loc = document.getElementById('hmm-location').value.trim();
    const room = document.getElementById('hmm-room').value.trim();
    const propNames = [...container.querySelectorAll('.hmm-prop-input')]
      .map(el => el.value.trim()).filter(Boolean);
    document.getElementById('hmm-prompt-line').innerHTML = buildPromptLine(actor, loc, room, propNames);
  }
  container.querySelectorAll('input').forEach(el => el.addEventListener('input', updatePrompt));
  updatePrompt();

  // Remove prop row (delegated)
  container.querySelector('#hmm-props-list').addEventListener('click', e => {
    if (!e.target.classList.contains('hmm-remove-prop')) return;
    const row = e.target.closest('.hmm-prop-row');
    if (!row) return;
    const rad = row.dataset.radical;
    if (rad && originalPropRadicals.has(rad)) deletedRadicals.add(rad);
    row.remove();
    updatePrompt();
  });

  // Add new prop row (radical + name only — autofills name from global library)
  document.getElementById('hmm-add-prop').addEventListener('click', () => {
    const list = document.getElementById('hmm-props-list');
    const row = document.createElement('div');
    row.className = propRowClass;
    row.innerHTML = `
      <input type="text" placeholder="${escHtml(t('hmm.propRadicalPlaceholder'))}"
        class="hmm-prop-radical w-12 text-center text-lg border border-gray-200 rounded px-1 py-1 focus:outline-none focus:ring-1 focus:ring-purple-400 shrink-0">
      <input type="text" placeholder="${escHtml(t('hmm.propPlaceholderNew'))}"
        class="${propInputClass}">
      <button class="${removeBtnClass}" title="${escHtml(t('hmm.removeProp'))}">×</button>
    `;
    list.appendChild(row);

    const radInput  = row.querySelector('.hmm-prop-radical');
    const nameInput = row.querySelector('.hmm-prop-input');

    // lastAutofill: the value we last wrote via autofill (null = user has taken over).
    // We only overwrite the name input if its current value still matches lastAutofill.
    let lastAutofill = null;

    radInput.addEventListener('input', () => {
      // Trim to the first Unicode character so pasting e.g. "丿stroke" just gives "丿".
      const runes = [...radInput.value.trim()];
      if (runes.length > 1) radInput.value = runes[0];
      const rad = radInput.value.trim();
      const canOverwrite = nameInput.value === (lastAutofill ?? '');
      if (rad.length === 1 && propsLookup[rad]) {
        if (canOverwrite) {
          lastAutofill = propsLookup[rad];
          nameInput.value = lastAutofill;
          nameInput.classList.add('text-gray-400');
        }
      } else if (canOverwrite && lastAutofill !== null) {
        lastAutofill = '';
        nameInput.value = '';
        nameInput.classList.remove('text-gray-400');
      }
      updatePrompt();
    });

    nameInput.addEventListener('input', () => {
      // User is editing manually — stop autofilling.
      lastAutofill = null;
      nameInput.classList.remove('text-gray-400');
      updatePrompt();
    });

    radInput.focus();
  });

  // Save handler
  document.getElementById('hmm-save-btn').addEventListener('click', async () => {
    const btn = document.getElementById('hmm-save-btn');
    btn.disabled = true;
    try {
      // Delete removed props
      for (const rad of deletedRadicals) {
        await apiFetch(`/api/hmm/props/${encodeURIComponent(rad)}`, { method: 'DELETE' });
      }
      deletedRadicals.clear();

      // Collect remaining props (decomposition rows use data-radical; new rows use the radical input)
      const props = [...container.querySelectorAll('.hmm-prop-row')].map(row => {
        const radInput = row.querySelector('.hmm-prop-radical');
        const radical = radInput ? radInput.value.trim() : row.dataset.radical;
        const prop_name = row.querySelector('.hmm-prop-input')?.value.trim() || '';
        return { radical, prop_name };
      }).filter(p => p.radical);

      await apiFetch(`/api/words/${wordId}/hmm`, {
        method: 'PUT',
        body: JSON.stringify({
          scene_text: document.getElementById('hmm-scene-text').value,
          actor_name: document.getElementById('hmm-actor').value.trim(),
          location_name: document.getElementById('hmm-location').value.trim(),
          room_name: document.getElementById('hmm-room').value.trim(),
          props,
          decomposition: document.getElementById('hmm-decomp').value.trim(),
        }),
      });
      const status = document.getElementById('hmm-save-status');
      status.classList.remove('hidden');
      setTimeout(() => status.classList.add('hidden'), 2000);
    } catch (e) {
      alert(t('hmm.saveFailed') + ': ' + e.message);
    } finally {
      btn.disabled = false;
    }
  });
}

function renderHMMSceneReadOnly(containerId, sceneText) {
  const container = document.getElementById(containerId);
  if (!container || !sceneText) { if (container) container.innerHTML = ''; return; }
  container.innerHTML = `
    <div class="p-3 bg-purple-50 border border-purple-200 rounded-xl">
      <div class="text-xs text-purple-500 uppercase tracking-wide mb-1">${t('hmm.mnemonicScene')}</div>
      <div class="text-sm text-gray-700 whitespace-pre-wrap">${escHtml(sceneText)}</div>
    </div>
  `;
}
