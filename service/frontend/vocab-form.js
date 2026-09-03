// vocab-form.js — add/edit word form

function applySecondaryLangVisibility() {
  const hasSecondary = secondaryLang !== '';

  // Word table: show/hide the secondary column header
  const colHeader = document.getElementById('col-secondary-lang');
  if (colHeader) colHeader.classList.toggle('hidden', !hasSecondary);

  // Edit form: show/hide the secondary-lang inputs block
  const deSection = document.getElementById('de-form-section');
  if (deSection) deSection.classList.toggle('hidden', !hasSecondary);

  // Missing-lang filter: show/hide the secondary option
  const deOption = document.getElementById('missing-lang-de-option');
  if (deOption) deOption.classList.toggle('hidden', !hasSecondary);
  // If the secondary is now hidden but was selected, reset the filter
  if (!hasSecondary && missingLangFilter === secondaryLang) {
    missingLangFilter = '';
    const sel = document.getElementById('missing-lang-select');
    if (sel) sel.value = '';
  }
}

async function loadLangSettings() {
  try {
    const res = await fetch('/api/settings');
    if (!res.ok) return;
    const st = await res.json();
    primaryLang = st.primary_lang || 'en';
    secondaryLang = st.secondary_lang ?? '';

    // Update the primary column header
    const primaryHeader = document.querySelector('[data-sort="en"], [data-sort="de"]');
    if (primaryHeader) {
      primaryHeader.querySelector('span[data-i18n]').textContent = LANG_NAMES[primaryLang] || primaryLang;
      primaryHeader.dataset.sort = primaryLang;
    }
    // Update the secondary column header (may be hidden below)
    const colHeader = document.getElementById('col-secondary-lang');
    if (colHeader && secondaryLang) {
      colHeader.querySelector('span[data-i18n]').textContent = LANG_NAMES[secondaryLang] || secondaryLang;
      colHeader.dataset.sort = secondaryLang;
    }

    // Update form labels to reflect actual primary/secondary language names
    const primaryLabel = document.getElementById('primary-lang-label');
    if (primaryLabel) {
      primaryLabel.textContent = `${LANG_NAMES[primaryLang] || primaryLang} Translation(s)`;
    }
    const secondaryLabel = document.getElementById('secondary-lang-label');
    if (secondaryLabel && secondaryLang) {
      secondaryLabel.textContent = `${LANG_NAMES[secondaryLang] || secondaryLang} Translation(s)`;
    }

    applySecondaryLangVisibility();
  } catch { /* ignore — use defaults */ }
}

function openEditForm(word) {
  editingWordId = word.id;
  setText('form-title', t('vocab.editWord'));
  $('form-zh').value = word.zh_text;
  $('form-pinyin').value = word.pinyin || '';
  const hanziwayLink = $('hanziway-link');
  hanziwayLink.href = 'https://hanziway.com/en/char?q=' + encodeURIComponent(word.zh_text);
  show('hanziway-link');
  show('form-cancel-btn');

  let notice = $('review-notice');
  if (word.needs_review) {
    if (!notice) {
      notice = document.createElement('p');
      notice.id = 'review-notice';
      notice.className = 'text-sm text-orange-600 bg-orange-50 border border-orange-200 rounded-lg px-3 py-2';
      notice.textContent = t('vocab.reviewNotice');
      $('word-form').prepend(notice);
    }
  } else if (notice) {
    notice.remove();
  }

  const enTexts = (word.translations || {})[primaryLang] || [];
  const container = $('en-inputs-container');
  container.innerHTML = '';
  for (const t of (enTexts.length ? enTexts : [''])) {
    addEnInput(t);
  }

  const deContainer = $('de-inputs-container');
  deContainer.innerHTML = '';
  if (secondaryLang) {
    const deTexts = (word.translations || {})[secondaryLang] || [];
    for (const t of (deTexts.length ? deTexts : [''])) {
      addDeInput(t);
    }
  }
  applySecondaryLangVisibility();

  formTags = [...(word.tags || [])];
  renderFormTags();

  if (word.total_attempts === 0) {
    show('start-training-row');
    $('form-start-training').checked = false;
    hide('form-reset-btn');
  } else {
    hide('start-training-row');
    show('form-reset-btn');
  }

  // HMM scene builder
  const hmmContainer = $('hmm-builder-container');
  if (word.id) {
    hmmContainer.classList.remove('hidden');
    loadHMMBuilder('hmm-builder-container', word.id, { zh: word.zh_text, en: (word.translations || {})['en'] || [] });
  } else {
    hmmContainer.classList.add('hidden');
    hmmContainer.innerHTML = '';
  }

  loadComponentsForEdit(word.zh_text);

  $('word-form-panel').scrollIntoView({ behavior: 'smooth' });
  $('form-zh').focus();
}

async function loadComponentsForEdit(zhText) {
  const section = $('components-edit-section');
  section.innerHTML = '';
  section.classList.add('hidden');
  if (!zhText) return;

  const langs = [primaryLang];
  if (secondaryLang) langs.push(secondaryLang);
  const langParam = langs.join(',');

  let data;
  try {
    data = await apiFetch(`/api/hanzi/decompose?chars=${encodeURIComponent(zhText)}&langs=${encodeURIComponent(langParam)}`);
  } catch (_) {
    return;
  }

  const compSet = new Map();
  for (const entry of (data || [])) {
    for (const comp of (entry.components || [])) {
      if (!compSet.has(comp.character)) compSet.set(comp.character, comp.definitions || {});
    }
  }
  if (compSet.size === 0) return;

  const langName = l => ({ en: 'EN', de: 'DE', fr: 'FR', es: 'ES', zh: 'ZH' }[l] || l.toUpperCase());
  let html = `<div class="border border-gray-200 rounded-xl p-4">
    <div class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-3">${escHtml(t('vocab.components') || 'Components')}</div>
    <div class="space-y-2">`;
  for (const [char, defs] of compSet) {
    html += `<div class="flex items-start gap-3">
      <span class="text-xl font-bold text-gray-800 w-8 shrink-0 pt-1">${escHtml(char)}</span>
      <div class="flex flex-wrap gap-2 flex-1">`;
    for (const lang of langs) {
      const val = (defs[lang] || defs[lang.toUpperCase()] || '');
      html += `<div class="flex items-center gap-1">
        <span class="text-xs font-semibold text-gray-400 w-6">${escHtml(langName(lang))}</span>
        <input type="text" class="comp-def-input border border-gray-300 rounded-lg px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 w-48"
               data-char="${escHtml(char)}" data-lang="${escHtml(lang)}" value="${escHtml(val)}" placeholder="${escHtml(langName(lang))} definition">
      </div>`;
    }
    html += `</div></div>`;
  }
  html += `</div></div>`;
  section.innerHTML = html;
  section.classList.remove('hidden');

  section.querySelectorAll('.comp-def-input').forEach(input => {
    input.addEventListener('blur', async () => {
      const char = input.dataset.char;
      const lang = input.dataset.lang;
      const definition = input.value.trim();
      try {
        await apiFetch(`/api/components/${encodeURIComponent(char)}/translation`, {
          method: 'PUT',
          body: JSON.stringify({ lang, definition }),
        });
      } catch (err) {
        console.error('Failed to save component translation:', err);
      }
    });
  });
}

function resetForm() {
  editingWordId = null;
  setText('form-title', t('vocab.addWord'));
  $('form-zh').value = '';
  $('form-pinyin').value = '';
  hide('form-cancel-btn');
  hide('form-reset-btn');
  hide('hanziway-link');
  const compSection = $('components-edit-section');
  if (compSection) { compSection.innerHTML = ''; compSection.classList.add('hidden'); }
  $('en-inputs-container').innerHTML = '';
  addEnInput('');
  $('de-inputs-container').innerHTML = '';
  if (secondaryLang) addDeInput('');
  applySecondaryLangVisibility();
  formTags = [];
  renderFormTags();
  $('form-tag-input').value = '';
  const notice = $('review-notice');
  if (notice) notice.remove();
  show('start-training-row');
  $('form-start-training').checked = false;
  $('hmm-builder-container').classList.add('hidden');
  $('hmm-builder-container').innerHTML = '';
}

function addEnInput(value = '') {
  const container = $('en-inputs-container');
  const wrapper = document.createElement('div');
  wrapper.className = 'flex items-center gap-2 mb-2';
  const placeholder = escHtml(LANG_NAMES[primaryLang] || primaryLang);
  wrapper.innerHTML = `
    <input type="text" class="en-input flex-1 border border-gray-300 rounded-lg px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
           placeholder="${placeholder}" value="${escHtml(value)}">
    <button type="button" class="btn-remove-en text-gray-400 hover:text-red-500 text-xl leading-none" title="Remove">×</button>`;
  wrapper.querySelector('.btn-remove-en').addEventListener('click', () => {
    if (container.children.length > 1) wrapper.remove();
  });
  container.appendChild(wrapper);
}

function addDeInput(value = '') {
  const container = $('de-inputs-container');
  const wrapper = document.createElement('div');
  wrapper.className = 'flex items-center gap-2 mb-2';
  const placeholder = escHtml(LANG_NAMES[secondaryLang] || secondaryLang);
  wrapper.innerHTML = `
    <input type="text" class="de-input flex-1 border border-gray-300 rounded-lg px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
           placeholder="${placeholder}" value="${escHtml(value)}">
    <button type="button" class="btn-remove-de text-gray-400 hover:text-red-500 text-xl leading-none" title="Remove">×</button>`;
  wrapper.querySelector('.btn-remove-de').addEventListener('click', () => {
    if (container.children.length > 1) wrapper.remove();
  });
  container.appendChild(wrapper);
}

function buildFormPayload() {
  const pinyin = $('form-pinyin').value.trim();
  const translations = {
    [primaryLang]: Array.from(document.querySelectorAll('.en-input')).map(i => i.value.trim()).filter(Boolean),
  };
  if (secondaryLang) {
    translations[secondaryLang] = Array.from(document.querySelectorAll('.de-input')).map(i => i.value.trim()).filter(Boolean);
  }
  return {
    zh_text: $('form-zh').value.trim(),
    pinyin: pinyin,
    translations,
    tags: [...formTags],
    start_training: $('form-start-training').checked,
  };
}

async function handleFormSubmit(e) {
  e.preventDefault();
  const payload = buildFormPayload();
  if (!payload.zh_text) { alert(t('vocab.zhRequired')); return; }
  const totalTranslations = Object.values(payload.translations).reduce((s, a) => s + a.length, 0);
  if (!totalTranslations) { alert(t('vocab.translationRequired')); return; }

  try {
    if (editingWordId) {
      await apiFetch(`/api/words/${editingWordId}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      });
    } else {
      await apiFetch('/api/words', {
        method: 'POST',
        body: JSON.stringify(payload),
      });
    }
    resetForm();
    loadTags();
    loadWords();
  } catch (e) {
    alert('Error: ' + e.message);
  }
}

async function deleteWord(id) {
  if (!confirm(t('vocab.confirmDelete'))) return;
  try {
    await apiFetch(`/api/words/${id}`, { method: 'DELETE' });
    loadTags();
    loadWords();
  } catch (e) {
    alert('Failed to delete: ' + e.message);
  }
}

async function resetWordProgress() {
  if (!editingWordId) return;
  if (!confirm(t('vocab.resetWordConfirm'))) return;
  try {
    const word = await apiFetch(`/api/words/${editingWordId}/reset`, { method: 'POST' });
    openEditForm(word);
    loadWords();
  } catch (e) {
    alert('Failed to reset: ' + e.message);
  }
}

async function applyPinyin(newPinyin) {
  if (!newPinyin) return;
  const field = $('form-pinyin');
  const current = field.value.trim();
  if (!current) {
    field.value = newPinyin;
  } else if (current !== newPinyin) {
    if (confirm(t('vocab.replacePinyin', { old: current, new: newPinyin }))) {
      field.value = newPinyin;
    }
  }
}

let pinyinTimer = null;

async function fetchAndFillPinyin(zh) {
  if (!zh) return;
  try {
    const result = await apiFetch('/api/pinyin', {
      method: 'POST',
      body: JSON.stringify({ zh_text: zh }),
    });
    await applyPinyin(result.pinyin);
  } catch (_) {}
}

function initTranslateButton() {
  show('translate-btn');
}

async function handleTranslate() {
  const btn = $('translate-btn');
  const zh = $('form-zh').value.trim();
  const enInputs = document.querySelectorAll('.en-input');
  const deInputs = document.querySelectorAll('.de-input');
  const en = enInputs.length > 0 ? enInputs[0].value.trim() : '';
  const de = deInputs.length > 0 ? deInputs[0].value.trim() : '';

  if (!zh && !en && !de) {
    alert(t('vocab.enterTextFirst'));
    return;
  }

  const origText = btn.textContent;
  btn.textContent = t('vocab.translating');
  btn.disabled = true;

  try {
    // Translate zh → primary language (en-inputs-container) if that field is empty.
    // Uses the user's actual primary_lang setting, not a hardcoded 'EN' — the
    // primary/secondary language pair is user-configurable and can be swapped
    // (e.g. German as primary), so the target language must follow the field
    // it's about to fill, not the field's fixed DOM id (issue #342).
    const enPromise = (zh && !en)
      ? apiFetch('/api/translate', {
          method: 'POST',
          body: JSON.stringify({ zh_text: zh, target_lang: primaryLang }),
        }).catch(() => null)
      : Promise.resolve(null);

    // Translate zh → secondary language (de-inputs-container) if that field
    // is empty and a secondary language is configured.
    const dePromise = (zh && !de && secondaryLang)
      ? apiFetch('/api/translate', {
          method: 'POST',
          body: JSON.stringify({ zh_text: zh, target_lang: secondaryLang }),
        }).catch(() => null)
      : Promise.resolve(null);

    // Translate en/de → zh if zh field is empty
    const zhPromise = (!zh && (en || de))
      ? apiFetch('/api/translate', {
          method: 'POST',
          body: JSON.stringify({ source_text: en || de }),
        }).catch(() => null)
      : Promise.resolve(null);

    const [enResult, deResult, zhResult] = await Promise.all([enPromise, dePromise, zhPromise]);

    if (zhResult) {
      if (zhResult.zh_text) $('form-zh').value = zhResult.zh_text;
      await applyPinyin(zhResult.pinyin);
    } else if (enResult) {
      await applyPinyin(enResult.pinyin);
    } else if (deResult) {
      await applyPinyin(deResult.pinyin);
    }

    if (enResult) {
      const translTexts = enResult.translations || (enResult.source_text ? [enResult.source_text] : []);
      if (translTexts.length > 0) {
        const container = $('en-inputs-container');
        container.innerHTML = '';
        for (const tr of translTexts) addEnInput(tr);
      }
    }

    if (deResult) {
      const translTexts = deResult.translations || (deResult.source_text ? [deResult.source_text] : []);
      if (translTexts.length > 0) {
        const container = $('de-inputs-container');
        container.innerHTML = '';
        for (const tr of translTexts) addDeInput(tr);
      }
    }
  } catch (e) {
    alert('Translation failed: ' + e.message);
  } finally {
    btn.textContent = origText;
    btn.disabled = false;
  }
}
