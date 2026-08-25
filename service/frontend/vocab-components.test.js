import { describe, it, expect, beforeEach } from 'vitest';

function escHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function renderComponentRow(comp) {
  return `
    <td class="py-3 px-4 text-lg font-medium">${escHtml(comp.character)}</td>
    <td class="py-3 px-4 text-gray-500 text-sm">${comp.pinyin ? escHtml(comp.pinyin) : '<span class="text-gray-400">—</span>'}</td>
    <td class="py-3 px-4 text-gray-600">${comp.definition_en ? escHtml(comp.definition_en) : '<span class="text-gray-400">—</span>'}</td>
    <td class="py-3 px-4 text-gray-600">${comp.definition_de ? escHtml(comp.definition_de) : '<span class="text-gray-400">—</span>'}</td>`;
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
