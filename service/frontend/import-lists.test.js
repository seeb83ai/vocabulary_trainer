import { describe, it, expect } from 'vitest';

// Mirrors the pure helpers in import-lists.js.

function hskVersions(tagNames) {
  const versions = new Set();
  for (const name of tagNames) {
    const m = /^hsk(\d+)-\d+$/.exec(name);
    if (m) versions.add(Number(m[1]));
  }
  return [...versions].sort((a, b) => b - a);
}

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

function buildListImportPayload(sourceTag, selected, applyTags, importEn, importDe, mode) {
  const payload = {
    tag: sourceTag,
    import_langs: [...(importEn ? ['en'] : []), ...(importDe ? ['de'] : [])],
    apply_tags: applyTags.filter(tg => tg === sourceTag || !selected.includes(tg)),
  };
  if (mode) payload.import_mode = mode;
  return payload;
}

function buildMatchAllPayload(selected, importEn, importDe, mode) {
  const payload = {
    tag: selected[0],
    and_tags: selected.slice(1),
    import_langs: [...(importEn ? ['en'] : []), ...(importDe ? ['de'] : [])],
    apply_tags: [...selected],
  };
  if (mode) payload.import_mode = mode;
  return payload;
}

describe('hskVersions', () => {
  it('lists the HSK versions in the library, newest first', () => {
    expect(hskVersions(['hsk2-1', 'food', 'hsk3-1', 'hsk3-2', 'hsk2-6'])).toEqual([3, 2]);
  });

  it('ignores tags that only look similar', () => {
    expect(hskVersions(['hsk1', 'HSK3-1', 's_hsk2-1', 'hsk3'])).toEqual([]);
  });

  it('handles an empty tag list', () => {
    expect(hskVersions([])).toEqual([]);
  });
});

describe('hskQuickStart', () => {
  const lib = ['hsk2-1', 'hsk2-2', 'hsk2-3', 'hsk3-1', 'hsk3-2', 'food'];

  it('maps the buttons to the lists of the chosen version', () => {
    expect(hskQuickStart(lib, 3)).toEqual({ beginner: ['hsk3-1'], basics: ['hsk3-2'] });
    expect(hskQuickStart(lib, 2)).toEqual({ beginner: ['hsk2-1'], basics: ['hsk2-2', 'hsk2-3'] });
  });

  it('offers nothing for a version without lists', () => {
    expect(hskQuickStart(['food'], 3)).toEqual({ beginner: [], basics: [] });
  });
});

describe('toggleListSelection', () => {
  it('adds a list that is not selected', () => {
    expect(toggleListSelection(['hsk3-1'], 'hsk2-2')).toEqual(['hsk3-1', 'hsk2-2']);
  });

  it('removes a list that is selected', () => {
    expect(toggleListSelection(['hsk3-1', 'hsk2-2'], 'hsk3-1')).toEqual(['hsk2-2']);
  });
});

describe('buildListImportPayload', () => {
  const selected = ['hsk3-1', 'hsk3-2'];

  it('gives each list only its own tag plus the extra tags', () => {
    const p = buildListImportPayload('hsk3-1', selected, ['hsk3-1', 'hsk3-2', 'mine'], true, false);
    expect(p).toEqual({ tag: 'hsk3-1', import_langs: ['en'], apply_tags: ['hsk3-1', 'mine'] });
  });

  it('leaves out a list tag the user removed', () => {
    const p = buildListImportPayload('hsk3-2', selected, ['hsk3-1'], true, true);
    expect(p).toEqual({ tag: 'hsk3-2', import_langs: ['en', 'de'], apply_tags: [] });
  });

  it('sends the chosen languages as import_langs', () => {
    expect(buildListImportPayload('hsk3-1', selected, [], false, true).import_langs).toEqual(['de']);
  });
});

describe('buildListImportPayload import mode', () => {
  it('leaves import_mode out when no mode is given', () => {
    expect(buildListImportPayload('hsk3-1', ['hsk3-1'], ['hsk3-1'], true, false)).not.toHaveProperty('import_mode');
  });

  it('passes the mode through', () => {
    const payload = buildListImportPayload('hsk3-1', ['hsk3-1'], ['hsk3-1'], true, true, 'known');
    expect(payload.import_mode).toBe('known');
    expect(payload.import_langs).toEqual(['en', 'de']);
  });
});

describe('buildMatchAllPayload', () => {
  it('imports the first tag narrowed by the others and applies all of them', () => {
    expect(buildMatchAllPayload(['hsk3-1', 'topic-food'], true, false)).toEqual({
      tag: 'hsk3-1', and_tags: ['topic-food'], import_langs: ['en'], apply_tags: ['hsk3-1', 'topic-food'],
    });
  });

  it('passes the languages and the import mode through', () => {
    const payload = buildMatchAllPayload(['a', 'b', 'c'], true, true, 'include');
    expect(payload.and_tags).toEqual(['b', 'c']);
    expect(payload.import_langs).toEqual(['en', 'de']);
    expect(payload.import_mode).toBe('include');
  });
});
