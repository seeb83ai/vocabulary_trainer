// vocab-download.js — export modal (CSV/TSV/JSON/TXT)

function openDownloadModal() {
  show('download-modal');
}

async function executeDownload() {
  const cols = {
    zh:       $('dl-col-zh').checked,
    pinyin:   $('dl-col-pinyin').checked,
    en:       $('dl-col-en').checked,
    tags:     $('dl-col-tags').checked,
    tier:     $('dl-col-tier').checked,
    accuracy: $('dl-col-accuracy').checked,
    attempts: $('dl-col-attempts').checked,
    due:      $('dl-col-due').checked,
  };
  const format = document.querySelector('input[name="dl-format"]:checked').value;

  const params = new URLSearchParams({ q: searchQuery });
  if (sortBy) { params.set('sort', sortBy); params.set('order', sortDir); }
  if (selectedFilterTags.length) params.set('tags', selectedFilterTags.join(','));
  if (reviewFilterActive) params.set('review', '1');
  if (hideUnseenActive) params.set('hide_unseen', '1');
  if (selectedTierFilter) params.set('bucket', selectedTierFilter);
  if (dueFilter) params.set('due', dueFilter);

  const btn = $('dl-confirm-btn');
  btn.textContent = t('download.downloading');
  btn.disabled = true;
  try {
    const words = await apiFetch(`/api/words/export?${params}`);
    const { content, mime, ext } = buildDownload(words, cols, format);
    const blob = new Blob(['\uFEFF' + content], { type: mime + ';charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `vocabulary.${ext}`;
    a.click();
    URL.revokeObjectURL(url);
    hide('download-modal');
  } catch (e) {
    alert('Download failed: ' + e.message);
  } finally {
    btn.textContent = t('download.download');
    btn.disabled = false;
  }
}

function buildDownload(words, cols, format) {
  const headers = [];
  if (cols.zh)       headers.push('Chinese');
  if (cols.pinyin)   headers.push('Pinyin');
  if (cols.en)       headers.push('Translations');
  if (cols.tags)     headers.push('Tags');
  if (cols.tier)     headers.push('Level');
  if (cols.accuracy) headers.push('Accuracy');
  if (cols.attempts) headers.push('Attempts');
  if (cols.due)      headers.push('Due Date');

  function rowValues(word) {
    const vals = [];
    if (cols.zh)     vals.push(word.zh_text || '');
    if (cols.pinyin) vals.push(word.pinyin || '');
    if (cols.en)     vals.push(Object.values(word.translations || {}).flat().join('; '));
    if (cols.tags)   vals.push((word.tags || []).join('; '));
    if (cols.tier) {
      const t = wordTier(word.total_correct, word.total_attempts, word.learning_new_word, word.streak_bonus);
      vals.push(t ? t.label : '');
    }
    if (cols.accuracy) {
      const effCorrect = word.total_correct + (word.streak_bonus || 0);
      vals.push(word.total_attempts > 0
        ? Math.round(effCorrect / word.total_attempts * 100) + '%'
        : '');
    }
    if (cols.attempts) vals.push(String(word.total_attempts));
    if (cols.due) {
      vals.push(word.total_attempts > 0 ? word.due_date.substring(0, 10) : '');
    }
    return vals;
  }

  if (format === 'json') {
    const out = words.map(word => {
      const obj = {};
      if (cols.zh)       obj.chinese      = word.zh_text || '';
      if (cols.pinyin)   obj.pinyin       = word.pinyin || '';
      if (cols.en)       obj.translations = Object.values(word.translations || {}).flat();
      if (cols.tags)     obj.tags         = word.tags || [];
      if (cols.tier) {
        const t = wordTier(word.total_correct, word.total_attempts, word.learning_new_word, word.streak_bonus);
        obj.level = t ? t.label : '';
      }
      if (cols.accuracy) {
        const effCorrect = word.total_correct + (word.streak_bonus || 0);
        obj.accuracy = word.total_attempts > 0
          ? Math.round(effCorrect / word.total_attempts * 100)
          : null;
      }
      if (cols.attempts) obj.attempts = word.total_attempts;
      if (cols.due)      obj.due_date = word.total_attempts > 0 ? word.due_date.substring(0, 10) : null;
      return obj;
    });
    return { content: JSON.stringify(out, null, 2), mime: 'application/json', ext: 'json' };
  }

  if (format === 'tsv') {
    const lines = [headers.join('\t')];
    for (const word of words) {
      lines.push(rowValues(word).map(v => v.replace(/[\t\n\r]/g, ' ')).join('\t'));
    }
    return { content: lines.join('\n'), mime: 'text/tab-separated-values', ext: 'tsv' };
  }

  if (format === 'txt') {
    const lines = words.map(word => rowValues(word).join(' | '));
    return { content: lines.join('\n'), mime: 'text/plain', ext: 'txt' };
  }

  // CSV (default)
  function csvField(v) {
    if (/[",\n\r]/.test(v)) return '"' + v.replace(/"/g, '""') + '"';
    return v;
  }
  const lines = [headers.map(csvField).join(',')];
  for (const word of words) {
    lines.push(rowValues(word).map(csvField).join(','));
  }
  return { content: lines.join('\n'), mime: 'text/csv', ext: 'csv' };
}
