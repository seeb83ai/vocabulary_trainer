// ── Generate sentences tab ────────────────────────────────────────────────────
// The LLM writes example sentences from the user's known words; the user picks
// the good ones and adds them as words with a sentence tag (ends in "_s").

let sentTags = [];
let sentResults = [];
let sentLLMChecked = false;

// normalizeSentenceTag returns the tag name with the "_s" suffix that marks a
// sentence for fill-in-the-blank. Pure for unit testing.
function normalizeSentenceTag(name) {
  const tag = String(name || '').trim().replace(/\s+/g, '-');
  if (!tag) return '';
  return tag.endsWith('_s') ? tag : tag + '_s';
}

// sentenceWordPayload builds the POST /api/words body for one generated
// sentence. Pure for unit testing.
function sentenceWordPayload(sentence, tag) {
  const translations = {};
  for (const [lang, list] of Object.entries(sentence.translations || {})) {
    const clean = list.map(s => s.trim()).filter(Boolean);
    if (clean.length) translations[lang] = clean;
  }
  return {
    zh_text: sentence.zh_text.trim(),
    pinyin: (sentence.pinyin || '').trim(),
    translations,
    tags: [tag],
  };
}

function openSentencesPanel() {
  renderSentenceTags();
  renderSentenceTagOptions();
  if (sentLLMChecked) return;
  sentLLMChecked = true;
  apiFetch('/api/config').then(cfg => {
    const ok = !!(cfg && cfg.llm_available);
    $('sentences-no-llm').classList.toggle('hidden', ok);
    $('sentences-generate-btn').disabled = !ok;
  }).catch(() => {});
}

function renderSentenceTags() {
  const groups = $('sentences-tag-groups');
  groups.innerHTML = '';
  // Sentence tags hold no known words, so they are no source of focus words.
  const tags = allTags.filter(tag => !tag.endsWith('_s'));
  sentTags = sentTags.filter(tag => tags.includes(tag));
  renderTagGroups(groups, tags, (tag) => {
    const pill = document.createElement('button');
    pill.type = 'button';
    pill.className = 'ui-chip vb-chip';
    pill.setAttribute('aria-pressed', sentTags.includes(tag) ? 'true' : 'false');
    pill.textContent = tag;
    pill.addEventListener('click', () => {
      sentTags = sentTags.includes(tag) ? sentTags.filter(x => x !== tag) : [...sentTags, tag];
      renderSentenceTags();
    });
    return pill;
  });
  $('sentences-tags-hint').classList.toggle('hidden', sentTags.length > 0);
}

function renderSentenceTagOptions() {
  const list = $('sentences-tag-options');
  list.innerHTML = '';
  const sentenceTags = allTags.filter(tag => tag.endsWith('_s'));
  const others = allTags.filter(tag => !tag.endsWith('_s'));
  for (const tag of [...sentenceTags, ...others]) {
    const opt = document.createElement('option');
    opt.value = tag;
    list.appendChild(opt);
  }
}

function updateSentenceTagHint() {
  const tag = normalizeSentenceTag($('sentences-tag-input').value);
  const hint = $('sentences-tag-hint');
  if (!tag) {
    hint.textContent = t('sent.tagRequired');
  } else if (allTags.includes(tag)) {
    hint.textContent = t('sent.tagExisting', { tag });
  } else {
    hint.textContent = t('sent.tagNew', { tag });
  }
  updateSentenceAddBtn();
}

function updateSentenceAddBtn() {
  const n = $('sentences-results').querySelectorAll('.sentence-row input[type=checkbox]:checked').length;
  $('sentences-add-btn').textContent = t('sent.addN', { n });
  $('sentences-add-btn').disabled = n === 0 || !normalizeSentenceTag($('sentences-tag-input').value);
}

function setSentenceStatus(msg, isError) {
  const el = $('sentences-status');
  el.textContent = msg;
  el.className = 'text-sm mt-3 ' + (isError ? 'text-red-600' : 'text-green-700');
  el.classList.toggle('hidden', !msg);
}

async function generateSentences() {
  const btn = $('sentences-generate-btn');
  btn.disabled = true;
  setSentenceStatus('');
  show('sentences-loading');
  try {
    const data = await apiFetch('/api/sentences/generate', {
      method: 'POST',
      body: JSON.stringify({ tags: sentTags, count: Number($('sentences-count').value) }),
      timeoutMs: API_SLOW_TIMEOUT_MS,
    });
    if (data && data.error) throw new Error(data.error);
    sentResults = data.sentences || [];
    renderSentenceResults();
    if (sentResults.length === 0) setSentenceStatus(t('sent.none'), true);
  } catch (e) {
    setSentenceStatus(e.message, true);
  } finally {
    hide('sentences-loading');
    btn.disabled = false;
  }
}

function renderSentenceResults() {
  const list = $('sentences-results');
  list.innerHTML = '';
  sentResults.forEach((s, i) => {
    const row = document.createElement('label');
    row.className = 'sentence-row';
    const transl = Object.entries(s.translations || {}).map(([lang, texts]) =>
      `<span class="sentence-transl"><span class="sentence-lang">${escHtml(lang.toUpperCase())}</span>` +
      `<input type="text" class="sentence-input" data-index="${i}" data-lang="${escHtml(lang)}" value="${escHtml(texts.join(', '))}"></span>`,
    ).join('');
    row.innerHTML =
      `<input type="checkbox" class="sentence-check" data-index="${i}" checked>` +
      `<span class="sentence-body">` +
      `<span class="sentence-zh" lang="zh">${escHtml(s.zh_text)}</span>` +
      `<input type="text" class="sentence-input sentence-pinyin" data-index="${i}" data-field="pinyin" value="${escHtml(s.pinyin || '')}" aria-label="Pinyin">` +
      transl +
      `</span>`;
    list.appendChild(row);
  });
  list.querySelectorAll('.sentence-check').forEach(cb => cb.addEventListener('change', updateSentenceAddBtn));
  list.querySelectorAll('.sentence-input').forEach(input => input.addEventListener('input', () => {
    const s = sentResults[Number(input.dataset.index)];
    if (input.dataset.field === 'pinyin') s.pinyin = input.value;
    else s.translations[input.dataset.lang] = [input.value];
  }));
  $('sentences-import').classList.toggle('hidden', sentResults.length === 0);
  updateSentenceTagHint();
}

async function addSelectedSentences() {
  const tag = normalizeSentenceTag($('sentences-tag-input').value);
  if (!tag) return;
  const btn = $('sentences-add-btn');
  btn.disabled = true;
  const checked = [...$('sentences-results').querySelectorAll('.sentence-check:checked')];
  const addedIdx = new Set();
  const failed = [];
  for (const cb of checked) {
    const i = Number(cb.dataset.index);
    const s = sentResults[i];
    try {
      await apiFetch('/api/words', { method: 'POST', body: JSON.stringify(sentenceWordPayload(s, tag)) });
      addedIdx.add(i);
    } catch (e) {
      failed.push(`${s.zh_text}: ${e.message}`);
    }
  }
  const added = addedIdx.size;
  sentResults = sentResults.filter((_, i) => !addedIdx.has(i));
  renderSentenceResults();
  setSentenceStatus(failed.length
    ? t('sent.addedSome', { n: added, tag }) + ' ' + failed.join('; ')
    : t('sent.added', { n: added, tag }), failed.length > 0);
  loadWords();
  await loadTags();
  renderSentenceTags();
  renderSentenceTagOptions();
  updateSentenceTagHint();
}

document.addEventListener('DOMContentLoaded', () => {
  $('sentences-generate-btn').addEventListener('click', generateSentences);
  $('sentences-tag-input').addEventListener('input', updateSentenceTagHint);
  $('sentences-add-btn').addEventListener('click', addSelectedSentences);
});
