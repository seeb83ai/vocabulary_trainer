// Mismatches page logic

const MISMATCH_MODE_LABELS = {
  transl_to_zh: 'To Chinese',
  zh_to_transl: 'Chinese',
  zh_pinyin_to_transl: 'Chinese + Pinyin',
};

function getMismatchModeLabel(mode) {
  return t('mode.' + mode) || MISMATCH_MODE_LABELS[mode] || mode;
}

// kind is 'word' or 'component' (models.ConfusionKindWord/Component) — a
// component side plays via /api/audio/component/{char} instead of the
// per-word endpoint, since it has no word_id (issue #280).
function wordCell(text, pinyin, translations, kind, wordId, character) {
  const pinyinHtml = pinyin ? `<span class="text-gray-400 text-xs ml-1">${escHtml(pinyin)}</span>` : '';
  const allTexts = Object.values(translations || {}).flat();
  const transHtml = allTexts.length ? `<div class="text-gray-500 text-xs mt-0.5">${allTexts.map(escHtml).join(', ')}</div>` : '';
  let audioBtn = '';
  if (kind === 'component' && character) {
    audioBtn = `<button class="btn-word-play ml-1 text-gray-400 hover:text-blue-500 transition" data-kind="component" data-character="${escHtml(character)}" title="Read aloud">🔊</button>`;
  } else if (wordId) {
    audioBtn = `<button class="btn-word-play ml-1 text-gray-400 hover:text-blue-500 transition" data-kind="word" data-word-id="${wordId}" data-zh-text="${escHtml(text)}" title="Read aloud">🔊</button>`;
  }
  return `<div class="flex items-center gap-1 text-base font-medium text-gray-800">${escHtml(text)}${pinyinHtml}${audioBtn}</div>${transHtml}`;
}

function formatDate(iso) {
  const d = new Date(iso);
  const diffMs = Date.now() - d.getTime();
  const diffDays = Math.floor(diffMs / 86400000);
  if (diffDays === 0) return t('mismatches.today');
  if (diffDays === 1) return t('mismatches.yesterday');
  if (diffDays < 7) return t('mismatches.daysAgo', { n: diffDays });
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}

async function loadMismatches() {
  try {
    const items = await apiFetch('/api/mismatches');
    if (!items || items.length === 0) {
      show('empty-state');
      return;
    }
    show('table-wrap');
    const tbody = $('mismatches-tbody');
    tbody.innerHTML = '';
    for (const item of items) {
      const tr = document.createElement('tr');
      tr.className = 'border-b border-gray-200 hover:bg-gray-50';
      tr.innerHTML = `
        <td class="py-3 px-4">${wordCell(item.zh_text, item.zh_pinyin, item.zh_translations, item.zh_kind, item.zh_word_id, item.zh_component)}</td>
        <td class="py-3 px-4">${wordCell(item.confused_with_text, item.confused_with_pinyin, item.confused_with_translations, item.confused_with_kind, item.confused_with_id, item.confused_with_component)}</td>
        <td class="py-3 px-4 text-gray-500">${escHtml(getMismatchModeLabel(item.mode))}</td>
        <td class="py-3 px-4 font-semibold text-gray-700">${item.count}</td>
        <td class="py-3 px-4 text-gray-400">${formatDate(item.last_seen)}</td>`;
      tbody.appendChild(tr);
      tr.querySelectorAll('.btn-word-play').forEach(btn => {
        if (btn.dataset.kind === 'component') {
          btn.addEventListener('click', () => playComponentAudio(btn.dataset.character));
        } else {
          btn.addEventListener('click', () => playAudio(+btn.dataset.wordId, btn.dataset.zhText));
        }
      });
    }
  } catch (e) {
    alert('Failed to load mismatches: ' + e.message);
  }
}

document.addEventListener('DOMContentLoaded', loadMismatches);
