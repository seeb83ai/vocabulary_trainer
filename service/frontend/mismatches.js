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
function wordCell(text, pinyin, translations, kind, wordId, character, other) {
  const pinyinHtml = pinyin ? `<span class="mm-py">${escHtml(pinyin)}</span>` : '';
  const allTexts = Object.values(translations || {}).flat();
  const transHtml = allTexts.length ? `<span class="mm-en">${allTexts.map(escHtml).join(', ')}</span>` : '';
  let audioBtn = '';
  if (kind === 'component' && character) {
    audioBtn = `<button type="button" class="btn-word-play mm-play" data-kind="component" data-character="${escHtml(character)}" title="Read aloud" aria-label="Read aloud">🔊</button>`;
  } else if (wordId) {
    audioBtn = `<button type="button" class="btn-word-play mm-play" data-kind="word" data-word-id="${wordId}" data-zh-text="${escHtml(text)}" title="Read aloud" aria-label="Read aloud">🔊</button>`;
  }
  return `<div class="mm-side${other ? ' is-other' : ''}"><span class="mm-word"><span class="hanzi mm-zh">${escHtml(text)}</span>${pinyinHtml}${audioBtn}</span>${transHtml}</div>`;
}

// sortMismatches returns a sorted copy: 'often' = highest count first,
// 'recent' = latest last_seen first. Ties fall back to the other key.
function sortMismatches(items, by) {
  const list = [...(items || [])];
  const time = x => new Date(x.last_seen).getTime() || 0;
  if (by === 'recent') {
    list.sort((a, b) => time(b) - time(a) || b.count - a.count);
  } else {
    list.sort((a, b) => b.count - a.count || time(b) - time(a));
  }
  return list;
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

const MIXUP_ICON = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 7h14l-3-3M20 17H6l3 3"/></svg>';

let mismatchItems = [];
let mismatchSort = 'often';

function renderMismatches() {
  const list = $('mismatches-list');
  list.innerHTML = '';
  for (const item of sortMismatches(mismatchItems, mismatchSort)) {
    const row = document.createElement('div');
    row.className = 'mm-row';
    row.innerHTML = `
      <div class="mm-pair">
        ${wordCell(item.zh_text, item.zh_pinyin, item.zh_translations, item.zh_kind, item.zh_word_id, item.zh_component)}
        <span class="mm-swap">${MIXUP_ICON}</span>
        ${wordCell(item.confused_with_text, item.confused_with_pinyin, item.confused_with_translations, item.confused_with_kind, item.confused_with_id, item.confused_with_component, true)}
      </div>
      <div class="mm-meta">
        <span class="mm-count${item.count >= 5 ? ' is-high' : ''}">${escHtml(t('mismatches.timesConfused', { n: item.count }))}</span>
        <span class="mm-last">${escHtml(formatDate(item.last_seen))}</span>
      </div>
      <div class="mm-foot"><span class="mm-mode">${escHtml(getMismatchModeLabel(item.mode))}</span></div>`;
    list.appendChild(row);
    row.querySelectorAll('.btn-word-play').forEach(btn => {
      if (btn.dataset.kind === 'component') {
        btn.addEventListener('click', () => playComponentAudio(btn.dataset.character));
      } else {
        btn.addEventListener('click', () => playAudio(+btn.dataset.wordId, btn.dataset.zhText));
      }
    });
  }
}

function setMismatchSort(by) {
  mismatchSort = by;
  $('mm-sort-often').setAttribute('aria-pressed', String(by === 'often'));
  $('mm-sort-recent').setAttribute('aria-pressed', String(by === 'recent'));
  renderMismatches();
}

async function loadMismatches() {
  try {
    mismatchItems = (await apiFetch('/api/mismatches')) || [];
  } catch (e) {
    setText('error-msg', t('mismatches.loadFailed', { error: e.message }));
    show('error-state');
    return;
  }
  if (mismatchItems.length === 0) {
    show('empty-state');
    return;
  }
  show('mm-sort');
  show('mismatches-list');
  renderMismatches();
}

document.addEventListener('DOMContentLoaded', () => {
  $('mm-sort-often').addEventListener('click', () => setMismatchSort('often'));
  $('mm-sort-recent').addEventListener('click', () => setMismatchSort('recent'));
  loadMismatches();
});
