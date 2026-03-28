// HMM Scene Builder — reusable component for vocab edit and training result pages.
//
// Usage:
//   loadHMMBuilder('container-id', wordId)              — editable builder
//   loadHMMBuilder('container-id', wordId, {readOnly:true}) — read-only display
//   renderHMMSceneReadOnly('container-id', sceneText)  — static scene text display

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

  container.innerHTML = '<div class="text-sm text-gray-400">Loading mnemonic data...</div>';

  let ctx;
  try {
    ctx = await apiFetch(`/api/words/${wordId}/hmm/context`);
  } catch (e) {
    container.innerHTML = '';
    return;
  }

  if (readOnly) {
    renderReadOnlyBuilder(container, ctx);
  } else {
    renderEditableBuilder(container, wordId, ctx);
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
  const propNames = (ctx.props || []).filter(p => p.prop_name).map(p => p.prop_name);

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
      <div class="text-xs text-purple-500 uppercase tracking-wide mb-1">Mnemonic Scene</div>
      ${breakdownHtml}
      <div class="text-sm text-gray-700 whitespace-pre-wrap">${escHtml(scene.scene_text)}</div>
    </div>
  `;
}

function renderEditableBuilder(container, wordId, ctx) {
  const actorName = ctx.actor?.actor_name || '';
  const actorHint = ctx.actor?.hint || '';
  const actorCat = ctx.actor?.category || 'male';
  const locName = ctx.location?.location_name || '';
  const roomName = ctx.tone_room?.room_name || '';
  const sceneText = ctx.scene?.scene_text || '';
  const dotClass = HMM_CATEGORY_DOTS[actorCat] || 'bg-gray-400';

  // Build props rows
  const allRadicals = ctx.radicals || [];
  const propMap = {};
  for (const p of (ctx.props || [])) propMap[p.radical] = p.prop_name;

  let propsHtml = '';
  for (const rad of allRadicals) {
    const pName = propMap[rad] || '';
    propsHtml += `
      <div class="flex items-center gap-2">
        <span class="w-8 text-center text-lg shrink-0">${escHtml(rad)}</span>
        <input type="text" value="${escHtml(pName)}" placeholder="3D object..."
          class="hmm-prop-input flex-1 border border-gray-200 rounded px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-purple-400"
          data-radical="${escHtml(rad)}">
      </div>`;
  }

  const initialDisplay = ctx.initial === 'null' ? 'Ø' : (ctx.initial || '?');
  const finalDisplay = ctx.final === 'null' ? 'Ø' : (ctx.final || '?');

  container.innerHTML = `
    <div class="border border-purple-200 rounded-xl p-4 bg-purple-50/50 space-y-3">
      <div class="text-sm font-semibold text-purple-700">Mnemonic Scene Builder</div>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <div class="flex items-center gap-2">
          <span class="w-3 h-3 rounded-full ${dotClass} shrink-0"></span>
          <span class="text-xs text-gray-500 w-8 shrink-0">${escHtml(initialDisplay)}</span>
          <input id="hmm-actor" type="text" value="${escHtml(actorName)}" placeholder="${escHtml(actorHint || 'Actor...')}"
            class="flex-1 border border-gray-200 rounded px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-purple-400">
          <span class="text-xs text-gray-400 shrink-0">Actor</span>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-xs text-gray-500 w-11 shrink-0 text-right">${escHtml(finalDisplay)}</span>
          <input id="hmm-location" type="text" value="${escHtml(locName)}" placeholder="Location..."
            class="flex-1 border border-gray-200 rounded px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-purple-400">
          <span class="text-xs text-gray-400 shrink-0">Location</span>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-xs text-gray-500 w-11 shrink-0 text-right">T${ctx.tone || '?'}</span>
          <input id="hmm-room" type="text" value="${escHtml(roomName)}" placeholder="Room..."
            class="flex-1 border border-gray-200 rounded px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-purple-400">
          <span class="text-xs text-gray-400 shrink-0">Room</span>
        </div>
      </div>

      ${propsHtml ? `<div class="space-y-1"><div class="text-xs text-gray-500">Props (radicals)</div>${propsHtml}</div>` : ''}

      <div id="hmm-prompt-line" class="text-xs text-gray-500 italic"></div>

      <textarea id="hmm-scene-text" rows="3" placeholder="Write your mnemonic scene here..."
        class="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-purple-400 resize-y">${escHtml(sceneText)}</textarea>

      <div class="flex items-center gap-3">
        <button id="hmm-save-btn" class="px-4 py-1.5 bg-purple-600 hover:bg-purple-700 text-white text-sm font-medium rounded-lg transition">Save Scene</button>
        <span id="hmm-save-status" class="hidden text-xs text-green-500">Saved!</span>
      </div>
    </div>
  `;

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

  // Save handler
  document.getElementById('hmm-save-btn').addEventListener('click', async () => {
    const btn = document.getElementById('hmm-save-btn');
    btn.disabled = true;
    try {
      const props = [...container.querySelectorAll('.hmm-prop-input')].map(el => ({
        radical: el.dataset.radical,
        prop_name: el.value.trim(),
      }));
      await apiFetch(`/api/words/${wordId}/hmm`, {
        method: 'PUT',
        body: JSON.stringify({
          scene_text: document.getElementById('hmm-scene-text').value,
          actor_name: document.getElementById('hmm-actor').value.trim(),
          location_name: document.getElementById('hmm-location').value.trim(),
          room_name: document.getElementById('hmm-room').value.trim(),
          props,
        }),
      });
      const status = document.getElementById('hmm-save-status');
      status.classList.remove('hidden');
      setTimeout(() => status.classList.add('hidden'), 2000);
    } catch (e) {
      alert('Failed to save scene: ' + e.message);
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
      <div class="text-xs text-purple-500 uppercase tracking-wide mb-1">Mnemonic Scene</div>
      <div class="text-sm text-gray-700 whitespace-pre-wrap">${escHtml(sceneText)}</div>
    </div>
  `;
}
