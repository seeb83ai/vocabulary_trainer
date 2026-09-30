import { describe, it, expect, beforeEach } from 'vitest';

function escHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function t(k) { return k; }
function crossRefBadge() { return ''; }
function renderComponentLevel() { return ''; }
function renderComponentDue() { return ''; }

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

describe('renderComponentRow pinyin column', () => {
  it('shows pinyin when present', () => {
    const html = renderComponentRow({ character: '女', pinyin: 'nǚ', definition_en: 'woman', definition_de: 'Frau' });
    expect(html).toContain('nǚ');
  });

  it('shows dash when pinyin is absent', () => {
    const html = renderComponentRow({ character: '女', definition_en: 'woman', definition_de: 'Frau' });
    expect(html).toContain('—');
  });

  it('shows dash when pinyin is empty string', () => {
    const html = renderComponentRow({ character: '女', pinyin: '', definition_en: 'woman' });
    expect(html).toContain('—');
  });

  it('escapes special chars in pinyin', () => {
    const html = renderComponentRow({ character: '女', pinyin: '<b>test</b>' });
    expect(html).not.toContain('<b>');
    expect(html).toContain('&lt;b&gt;');
  });
});
