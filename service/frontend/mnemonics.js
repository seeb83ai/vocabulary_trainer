// Mnemonics settings page — HMM library management

const CATEGORY_STYLES = {
  male:       { i18nKey: 'mnemonics.cat.male',      cls: 'is-male' },
  female:     { i18nKey: 'mnemonics.cat.female',    cls: 'is-female' },
  fictional:  { i18nKey: 'mnemonics.cat.fictional', cls: 'is-fictional' },
  wildcard:   { i18nKey: 'mnemonics.cat.wildcard',  cls: 'is-wildcard' },
};

// Tab → container holding that section's items.
const MN_SECTIONS = {
  actors: 'actors-container',
  locations: 'locations-container',
  rooms: 'tonerooms-container',
  props: 'props-container',
};

const TONE_LABEL_KEYS = {
  1: 'mnemonics.tone1',
  2: 'mnemonics.tone2',
  3: 'mnemonics.tone3',
  4: 'mnemonics.tone4',
  5: 'mnemonics.tone5',
};

// ── Auto-save helper ────────────────────────────────────────────────────

function flashSaved(el) {
  const indicator = el.parentElement.querySelector('.save-indicator');
  if (indicator) {
    indicator.textContent = t('mnemonics.saved');
    indicator.classList.remove('hidden');
    setTimeout(() => indicator.classList.add('hidden'), 1200);
  }
}

function autoSaveInput(input, saveFn) {
  let timer;
  input.addEventListener('blur', () => { clearTimeout(timer); saveFn(input); });
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); input.blur(); }
  });
}

// filledCount counts the non-blank values — the "filled" number on each tab.
function filledCount(values) {
  return (values || []).filter(v => (v || '').trim() !== '').length;
}

function updateTabCount(tab) {
  const inputs = [...$(MN_SECTIONS[tab]).querySelectorAll('.mn-item input')];
  const el = $('mn-tab-' + tab).querySelector('.mn-tab-count');
  el.textContent = inputs.length ? `${filledCount(inputs.map(i => i.value))}/${inputs.length}` : '';
}

// buildItem renders one library entry: a key tile (e.g. the initial "b" or a
// radical) with a small sub label, the auto-saving input and, for props, a
// delete button. The item is highlighted once it has a value.
function buildItem({ key, sub, value, placeholder, data, tab, hanzi, onSave, onDelete }) {
  const row = document.createElement('label');
  row.className = 'mn-item';
  row.innerHTML = `
    <span class="mn-key${hanzi ? ' hanzi' : ''}">${escHtml(key)}${sub ? `<span class="mn-key-sub">${escHtml(sub)}</span>` : ''}</span>
    <input type="text" value="${escHtml(value || '')}" placeholder="${escHtml(placeholder || '')}">
    <span class="save-indicator hidden"></span>
    ${onDelete ? `<button type="button" class="mn-delete" title="${escHtml(t('mnemonics.deleteLabel'))}" aria-label="${escHtml(t('mnemonics.deleteLabel'))}">&times;</button>` : ''}
  `;
  const input = row.querySelector('input');
  Object.assign(input.dataset, data);
  const syncFilled = () => row.classList.toggle('is-filled', input.value.trim() !== '');
  syncFilled();
  input.addEventListener('input', () => { syncFilled(); updateTabCount(tab); });
  autoSaveInput(input, onSave);
  if (onDelete) row.querySelector('.mn-delete').addEventListener('click', (e) => { e.preventDefault(); onDelete(row); });
  return row;
}

// ── Actors ──────────────────────────────────────────────────────────────

async function loadActors() {
  const actors = await apiFetch('/api/hmm/actors');
  const container = $('actors-container');
  container.innerHTML = '';

  // Group by category
  const groups = {};
  for (const a of actors) {
    (groups[a.category] = groups[a.category] || []).push(a);
  }

  for (const cat of ['male', 'female', 'fictional', 'wildcard']) {
    const items = groups[cat] || [];
    if (!items.length) continue;
    const style = CATEGORY_STYLES[cat];

    const section = document.createElement('div');
    section.className = `mn-group ${style.cls}`;
    section.innerHTML = `<div class="mn-group-title">${escHtml(t(style.i18nKey))} <span class="mn-group-count">${items.length}</span></div>`;

    const grid = document.createElement('div');
    grid.className = 'mn-grid';

    for (const actor of items) {
      grid.appendChild(buildItem({
        key: actor.initial === 'null' ? 'Ø' : actor.initial,
        value: actor.actor_name,
        placeholder: actor.hint,
        data: { initial: actor.initial },
        tab: 'actors',
        onSave: async (el) => {
          try {
            await apiFetch(`/api/hmm/actors/${encodeURIComponent(el.dataset.initial)}`, {
              method: 'PUT',
              body: JSON.stringify({ actor_name: el.value }),
            });
            flashSaved(el);
          } catch (e) { alert(t('mnemonics.saveFailed') + ': ' + e.message); }
        },
      }));
    }
    section.appendChild(grid);
    container.appendChild(section);
  }
  updateTabCount('actors');
}

// ── Locations ───────────────────────────────────────────────────────────

async function loadLocations() {
  const locs = await apiFetch('/api/hmm/locations');
  const container = $('locations-container');
  container.innerHTML = '';

  for (const loc of locs) {
    const placeholder = loc.final_key === 'null' ? 'Your childhood home' : 'A familiar place...';
    container.appendChild(buildItem({
      key: loc.final_key === 'null' ? 'Ø' : loc.final_key,
      value: loc.location_name,
      placeholder,
      data: { final: loc.final_key },
      tab: 'locations',
      onSave: async (el) => {
        try {
          await apiFetch(`/api/hmm/locations/${encodeURIComponent(el.dataset.final)}`, {
            method: 'PUT',
            body: JSON.stringify({ location_name: el.value }),
          });
          flashSaved(el);
        } catch (e) { alert(t('mnemonics.saveFailed') + ': ' + e.message); }
      },
    }));
  }
  updateTabCount('locations');
}

// ── Tone Rooms ──────────────────────────────────────────────────────────

async function loadToneRooms() {
  const rooms = await apiFetch('/api/hmm/tone-rooms');
  const container = $('tonerooms-container');
  container.innerHTML = '';

  for (const room of rooms) {
    const item = buildItem({
      key: String(room.tone),
      sub: t('mnemonics.toneShort'),
      value: room.room_name,
      placeholder: 'Room or area...',
      data: { tone: String(room.tone) },
      tab: 'rooms',
      onSave: async (el) => {
        try {
          await apiFetch(`/api/hmm/tone-rooms/${el.dataset.tone}`, {
            method: 'PUT',
            body: JSON.stringify({ room_name: el.value }),
          });
          flashSaved(el);
        } catch (e) { alert(t('mnemonics.saveFailed') + ': ' + e.message); }
      },
    });
    item.title = TONE_LABEL_KEYS[room.tone] ? t(TONE_LABEL_KEYS[room.tone]) : t('hmm.tone', { n: room.tone });
    container.appendChild(item);
  }
  updateTabCount('rooms');
}

// ── Props ───────────────────────────────────────────────────────────────

async function loadProps() {
  const props = await apiFetch('/api/hmm/props');
  renderProps(props);
}

function renderProps(props) {
  const container = $('props-container');
  container.innerHTML = '';

  for (const prop of props) {
    container.appendChild(buildItem({
      key: prop.radical,
      value: prop.prop_name,
      placeholder: '3D object...',
      data: { radical: prop.radical },
      tab: 'props',
      hanzi: true,
      onSave: async (el) => {
        try {
          await apiFetch('/api/hmm/props', {
            method: 'PUT',
            body: JSON.stringify({ radical: el.dataset.radical, prop_name: el.value }),
          });
          flashSaved(el);
        } catch (e) { alert(t('mnemonics.saveFailed') + ': ' + e.message); }
      },
      onDelete: async (row) => {
        const radical = prop.radical;
        if (!confirm(t('mnemonics.deleteProp', { radical }))) return;
        try {
          await apiFetch(`/api/hmm/props/${encodeURIComponent(radical)}`, { method: 'DELETE' });
          row.remove();
          updateTabCount('props');
        } catch (err) { alert(t('mnemonics.deleteFailed') + ': ' + err.message); }
      },
    }));
  }
  updateTabCount('props');
}

function setupAddProp() {
  $('add-prop-btn').addEventListener('click', async () => {
    const radical = $('new-prop-radical').value.trim();
    const name = $('new-prop-name').value.trim();
    if (!radical) { alert(t('mnemonics.propRequired')); return; }
    try {
      await apiFetch('/api/hmm/props', {
        method: 'PUT',
        body: JSON.stringify({ radical, prop_name: name }),
      });
      $('new-prop-radical').value = '';
      $('new-prop-name').value = '';
      await loadProps();
    } catch (e) { alert(t('mnemonics.addFailed') + ': ' + e.message); }
  });
}

// ── Tabs ────────────────────────────────────────────────────────────────

function setupTabs() {
  document.querySelectorAll('.mn-tabs [data-tab]').forEach(btn => {
    btn.addEventListener('click', () => {
      for (const tab of Object.keys(MN_SECTIONS)) {
        const active = tab === btn.dataset.tab;
        $('mn-tab-' + tab).setAttribute('aria-pressed', String(active));
        $('mn-panel-' + tab).classList.toggle('hidden', !active);
      }
    });
  });
}

// ── Init ────────────────────────────────────────────────────────────────

async function init() {
  try {
    await Promise.all([loadActors(), loadLocations(), loadToneRooms(), loadProps()]);
  } catch (e) {
    console.error('Failed to load HMM library:', e);
  }
}

setupTabs();
// Wire the Add button once; init() runs again on every language change.
setupAddProp();
init();

// Re-render when UI language changes
document.addEventListener('langchange', () => init());
