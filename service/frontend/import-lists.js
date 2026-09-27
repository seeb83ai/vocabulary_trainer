// Shared helpers for importing library word lists (tags), used by the
// onboarding on the training page and by Vocabulary → Import.

// hskVersions returns the HSK versions (e.g. [3, 2]) that have hsk<v>-<level>
// tags in the given library tag names, newest first.
function hskVersions(tagNames) {
  const versions = new Set();
  for (const name of tagNames) {
    const m = /^hsk(\d+)-\d+$/.exec(name);
    if (m) versions.add(Number(m[1]));
  }
  return [...versions].sort((a, b) => b - a);
}

// hskQuickStart returns the library tags behind the two one-click onboarding
// buttons for an HSK version.
function hskQuickStart(tagNames, version) {
  const has = n => tagNames.includes(n);
  return {
    beginner: [`hsk${version}-1`].filter(has),
    basics: [`hsk${version}-2`, `hsk${version}-3`].filter(has),
  };
}

function toggleListSelection(selected, name) {
  return selected.includes(name) ? selected.filter(n => n !== name) : [...selected, name];
}

// buildListImportPayload builds the /api/import body for one selected list.
// A selected list's own tag is applied only to that list's words; other tags
// in applyTags (typed by the user) go on every word.
function buildListImportPayload(sourceTag, selected, applyTags, importEn, importDe) {
  return {
    tag: sourceTag,
    import_langs: [...(importEn ? ['en'] : []), ...(importDe ? ['de'] : [])],
    apply_tags: applyTags.filter(tg => tg === sourceTag || !selected.includes(tg)),
  };
}

// importLists imports every selected list, one request per list, and returns
// the summed counts. A word in two lists is created by the first request and
// gets the second list's tag from the next one.
async function importLists(selected, applyTags, importEn, importDe) {
  const total = { imported: 0, tagged: 0, skipped: 0 };
  for (const tag of selected) {
    const result = await apiFetch('/api/import', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(buildListImportPayload(tag, selected, applyTags, importEn, importDe)),
    });
    total.imported += result.imported || 0;
    total.tagged += result.tagged || 0;
    total.skipped += result.skipped || 0;
  }
  return total;
}

function importResultText(result) {
  let text = `${t('vocab.importDone')} ${result.imported} ${t('vocab.importWords2')}`;
  if (result.tagged > 0) text += `, ${t('vocab.importTagged')} ${result.tagged} ${t('vocab.importTaggedOwned')}`;
  if (result.skipped > 0) text += `, ${t('vocab.importSkipped')} ${result.skipped}`;
  return text + '.';
}

function importPreviewURL(selected) {
  return '/api/import/preview?' + selected.map(tg => 'tag=' + encodeURIComponent(tg)).join('&');
}
