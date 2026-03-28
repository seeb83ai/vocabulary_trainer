// Mnemonics settings page — HMM library management

const CATEGORY_STYLES = {
  male:       { label: 'Male',       border: 'border-l-blue-500',   bg: 'bg-blue-50',   text: 'text-blue-700'   },
  female:     { label: 'Female',     border: 'border-l-pink-500',   bg: 'bg-pink-50',   text: 'text-pink-700'   },
  fictional:  { label: 'Fictional',  border: 'border-l-green-500',  bg: 'bg-green-50',  text: 'text-green-700'  },
  wildcard:   { label: 'Wildcard',   border: 'border-l-purple-500', bg: 'bg-purple-50', text: 'text-purple-700' },
};

const TONE_LABELS = {
  1: '1st tone (ā) — high level',
  2: '2nd tone (á) — rising',
  3: '3rd tone (ǎ) — dipping',
  4: '4th tone (à) — falling',
  5: '5th tone · neutral',
};

// ── Auto-save helper ────────────────────────────────────────────────────

function flashSaved(el) {
  const indicator = el.parentElement.querySelector('.save-indicator');
  if (indicator) {
    indicator.textContent = 'Saved';
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
    section.className = `border-l-4 ${style.border} pl-4 mb-4`;
    section.innerHTML = `<div class="text-sm font-semibold ${style.text} mb-2">${style.label} <span class="font-normal text-gray-400">(${items.length})</span></div>`;

    const grid = document.createElement('div');
    grid.className = 'grid grid-cols-1 sm:grid-cols-2 gap-2';

    for (const actor of items) {
      const row = document.createElement('div');
      row.className = 'flex items-center gap-2';
      row.innerHTML = `
        <span class="w-10 text-sm font-mono font-bold text-gray-600 text-right shrink-0">${escHtml(actor.initial === 'null' ? 'Ø' : actor.initial)}</span>
        <input type="text" value="${escHtml(actor.actor_name)}" placeholder="${escHtml(actor.hint)}"
          class="flex-1 border border-gray-200 rounded-lg px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          data-initial="${escHtml(actor.initial)}">
        <span class="save-indicator hidden text-xs text-green-500 w-10 shrink-0">Saved</span>
      `;
      const input = row.querySelector('input');
      autoSaveInput(input, async (el) => {
        try {
          await apiFetch(`/api/hmm/actors/${encodeURIComponent(el.dataset.initial)}`, {
            method: 'PUT',
            body: JSON.stringify({ actor_name: el.value }),
          });
          flashSaved(el);
        } catch (e) { alert('Save failed: ' + e.message); }
      });
      grid.appendChild(row);
    }
    section.appendChild(grid);
    container.appendChild(section);
  }
}

// ── Locations ───────────────────────────────────────────────────────────

async function loadLocations() {
  const locs = await apiFetch('/api/hmm/locations');
  const container = $('locations-container');
  container.innerHTML = '';

  for (const loc of locs) {
    const row = document.createElement('div');
    row.className = 'flex items-center gap-2';
    const label = loc.final_key === 'null' ? 'Ø (null)' : loc.final_key;
    const placeholder = loc.final_key === 'null' ? 'Your childhood home' : 'A familiar place...';
    row.innerHTML = `
      <span class="w-14 text-sm font-mono font-bold text-gray-600 text-right shrink-0">${escHtml(label)}</span>
      <input type="text" value="${escHtml(loc.location_name)}" placeholder="${escHtml(placeholder)}"
        class="flex-1 border border-gray-200 rounded-lg px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        data-final="${escHtml(loc.final_key)}">
      <span class="save-indicator hidden text-xs text-green-500 w-10 shrink-0">Saved</span>
    `;
    const input = row.querySelector('input');
    autoSaveInput(input, async (el) => {
      try {
        await apiFetch(`/api/hmm/locations/${encodeURIComponent(el.dataset.final)}`, {
          method: 'PUT',
          body: JSON.stringify({ location_name: el.value }),
        });
        flashSaved(el);
      } catch (e) { alert('Save failed: ' + e.message); }
    });
    container.appendChild(row);
  }
}

// ── Tone Rooms ──────────────────────────────────────────────────────────

async function loadToneRooms() {
  const rooms = await apiFetch('/api/hmm/tone-rooms');
  const container = $('tonerooms-container');
  container.innerHTML = '';

  for (const room of rooms) {
    const row = document.createElement('div');
    row.className = 'flex items-center gap-2';
    row.innerHTML = `
      <span class="w-48 text-sm text-gray-600 shrink-0">${escHtml(TONE_LABELS[room.tone] || 'Tone ' + room.tone)}</span>
      <input type="text" value="${escHtml(room.room_name)}" placeholder="Room or area..."
        class="flex-1 border border-gray-200 rounded-lg px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        data-tone="${room.tone}">
      <span class="save-indicator hidden text-xs text-green-500 w-10 shrink-0">Saved</span>
    `;
    const input = row.querySelector('input');
    autoSaveInput(input, async (el) => {
      try {
        await apiFetch(`/api/hmm/tone-rooms/${el.dataset.tone}`, {
          method: 'PUT',
          body: JSON.stringify({ room_name: el.value }),
        });
        flashSaved(el);
      } catch (e) { alert('Save failed: ' + e.message); }
    });
    container.appendChild(row);
  }
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
    const row = document.createElement('div');
    row.className = 'flex items-center gap-2';
    row.innerHTML = `
      <span class="w-10 text-lg text-center shrink-0">${escHtml(prop.radical)}</span>
      <input type="text" value="${escHtml(prop.prop_name)}" placeholder="3D object..."
        class="flex-1 border border-gray-200 rounded-lg px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        data-radical="${escHtml(prop.radical)}">
      <span class="save-indicator hidden text-xs text-green-500 w-10 shrink-0">Saved</span>
      <button class="text-gray-300 hover:text-red-500 text-sm transition shrink-0" data-radical="${escHtml(prop.radical)}" title="Delete">&times;</button>
    `;
    const input = row.querySelector('input');
    autoSaveInput(input, async (el) => {
      try {
        await apiFetch('/api/hmm/props', {
          method: 'PUT',
          body: JSON.stringify({ radical: el.dataset.radical, prop_name: el.value }),
        });
        flashSaved(el);
      } catch (e) { alert('Save failed: ' + e.message); }
    });
    row.querySelector('button').addEventListener('click', async (e) => {
      const radical = e.target.dataset.radical;
      if (!confirm(`Delete prop for "${radical}"?`)) return;
      try {
        await apiFetch(`/api/hmm/props/${encodeURIComponent(radical)}`, { method: 'DELETE' });
        row.remove();
      } catch (err) { alert('Delete failed: ' + err.message); }
    });
    container.appendChild(row);
  }
}

function setupAddProp() {
  $('add-prop-btn').addEventListener('click', async () => {
    const radical = $('new-prop-radical').value.trim();
    const name = $('new-prop-name').value.trim();
    if (!radical) { alert('Radical is required'); return; }
    try {
      await apiFetch('/api/hmm/props', {
        method: 'PUT',
        body: JSON.stringify({ radical, prop_name: name }),
      });
      $('new-prop-radical').value = '';
      $('new-prop-name').value = '';
      await loadProps();
    } catch (e) { alert('Failed to add prop: ' + e.message); }
  });
}

// ── Init ────────────────────────────────────────────────────────────────

async function init() {
  try {
    await Promise.all([loadActors(), loadLocations(), loadToneRooms(), loadProps()]);
    setupAddProp();
  } catch (e) {
    console.error('Failed to load HMM library:', e);
  }
}

init();
