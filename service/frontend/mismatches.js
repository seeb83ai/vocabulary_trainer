// Mismatches page logic

const MISMATCH_MODE_LABELS = {
  en_to_zh: 'EN → ZH',
  zh_to_en: 'ZH → EN',
  zh_pinyin_to_en: 'ZH + Pinyin → EN',
};

function wordCell(text, pinyin, enTexts) {
  const pinyinHtml = pinyin ? `<span class="text-gray-400 text-xs ml-1">${escHtml(pinyin)}</span>` : '';
  const enHtml = enTexts.length ? `<div class="text-gray-500 text-xs mt-0.5">${enTexts.map(escHtml).join(', ')}</div>` : '';
  return `<div class="text-base font-medium text-gray-800">${escHtml(text)}${pinyinHtml}</div>${enHtml}`;
}

function formatDate(iso) {
  const d = new Date(iso);
  const diffMs = Date.now() - d.getTime();
  const diffDays = Math.floor(diffMs / 86400000);
  if (diffDays === 0) return 'Today';
  if (diffDays === 1) return 'Yesterday';
  if (diffDays < 7) return `${diffDays}d ago`;
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
        <td class="py-3 px-4">${wordCell(item.zh_text, item.zh_pinyin, item.zh_en_texts)}</td>
        <td class="py-3 px-4">${wordCell(item.confused_with_text, item.confused_with_pinyin, item.confused_with_en_texts)}</td>
        <td class="py-3 px-4 text-gray-500">${escHtml(MISMATCH_MODE_LABELS[item.mode] || item.mode)}</td>
        <td class="py-3 px-4 font-semibold text-gray-700">${item.count}</td>
        <td class="py-3 px-4 text-gray-400">${formatDate(item.last_seen)}</td>`;
      tbody.appendChild(tr);
    }
  } catch (e) {
    alert('Failed to load mismatches: ' + e.message);
  }
}

document.addEventListener('DOMContentLoaded', loadMismatches);
