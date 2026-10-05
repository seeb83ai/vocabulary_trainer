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

function buildListImportPayload(sourceTag, selected, applyTags, mode) {
  const payload = {
    tag: sourceTag,
    apply_tags: applyTags.filter(tg => tg === sourceTag || !selected.includes(tg)),
  };
  if (mode) payload.import_mode = mode;
  return payload;
}

function buildMatchAllPayload(selected, mode) {
  const payload = {
    tag: selected[0],
    and_tags: selected.slice(1),
    apply_tags: [...selected],
  };
  if (mode) payload.import_mode = mode;
  return payload;
}

function nativeLangsLabel(primary, secondary) {
  return [primary, secondary].filter(Boolean).map(l => l.toUpperCase()).join(' + ');
}

function summarizeImportJobs(jobs) {
  const sum = { total: 0, done: 0, imported: 0, tagged: 0, skipped: 0, failed: false, error: '' };
  for (const job of jobs) {
    sum.total += job.total || 0;
    sum.done += job.done || 0;
    sum.imported += job.imported || 0;
    sum.tagged += job.tagged || 0;
    sum.skipped += job.skipped || 0;
    if (job.status === 'failed') {
      sum.failed = true;
      sum.error = sum.error || job.error || '';
    }
  }
  sum.finished = jobs.length > 0 && jobs.every(job => job.status === 'done' || job.status === 'failed');
  sum.canStart = sum.finished || jobs.some(job => job.import_mode !== 'known' && (job.imported || 0) > 0);
  return sum;
}

const t = (key, vars = {}) => key === 'import.progress'
  ? `Importing… ${vars.done} of ${vars.total} words`
  : 'Importing…';

function importProgressText(summary) {
  if (!summary.total) return t('vocab.importing');
  return t('import.progress', { done: summary.done, total: summary.total });
}

describe('summarizeImportJobs', () => {
  it('adds up the counters of all jobs', () => {
    const sum = summarizeImportJobs([
      { status: 'done', total: 10, done: 10, imported: 8, tagged: 1, skipped: 1 },
      { status: 'running', total: 20, done: 5, imported: 5, tagged: 0, skipped: 0 },
    ]);
    expect(sum).toMatchObject({ total: 30, done: 15, imported: 13, tagged: 1, skipped: 1 });
  });

  it('is finished only when every job is done or failed', () => {
    expect(summarizeImportJobs([{ status: 'done' }, { status: 'running' }]).finished).toBe(false);
    expect(summarizeImportJobs([{ status: 'done' }, { status: 'queued' }]).finished).toBe(false);
    expect(summarizeImportJobs([{ status: 'done' }, { status: 'done' }]).finished).toBe(true);
  });

  it('is never finished without jobs', () => {
    expect(summarizeImportJobs([]).finished).toBe(false);
  });

  it('lets the learner start once the first words are imported', () => {
    expect(summarizeImportJobs([{ status: 'queued' }]).canStart).toBe(false);
    expect(summarizeImportJobs([{ status: 'running', total: 500, done: 0, imported: 0 }]).canStart).toBe(false);
    expect(summarizeImportJobs([{ status: 'running', total: 500, done: 200, imported: 200 }]).canStart).toBe(true);
  });

  it('does not count words imported as known as words to train', () => {
    const known = { status: 'done', total: 50, done: 50, imported: 50, import_mode: 'known' };
    expect(summarizeImportJobs([known, { status: 'running', import_mode: 'include', imported: 0 }]).canStart).toBe(false);
    expect(summarizeImportJobs([known, { status: 'running', import_mode: 'include', imported: 200 }]).canStart).toBe(true);
    expect(summarizeImportJobs([known, { status: 'running', import_mode: 'review', imported: 200 }]).canStart).toBe(true);
  });

  it('lets the learner start when the import finished without new words', () => {
    expect(summarizeImportJobs([{ status: 'done', total: 5, done: 5, imported: 0, tagged: 5 }]).canStart).toBe(true);
  });

  it('reports the first error of a failed job', () => {
    const sum = summarizeImportJobs([
      { status: 'failed', error: 'boom' },
      { status: 'failed', error: 'later' },
    ]);
    expect(sum.failed).toBe(true);
    expect(sum.error).toBe('boom');
  });
});

describe('importProgressText', () => {
  it('shows the counts once the total is known', () => {
    expect(importProgressText({ done: 200, total: 500 })).toBe('Importing… 200 of 500 words');
  });

  it('shows a plain message while the job has not started counting', () => {
    expect(importProgressText({ done: 0, total: 0 })).toBe('Importing…');
  });
});

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
    const p = buildListImportPayload('hsk3-1', selected, ['hsk3-1', 'hsk3-2', 'mine']);
    expect(p).toEqual({ tag: 'hsk3-1', apply_tags: ['hsk3-1', 'mine'] });
  });

  it('leaves out a list tag the user removed', () => {
    const p = buildListImportPayload('hsk3-2', selected, ['hsk3-1']);
    expect(p).toEqual({ tag: 'hsk3-2', apply_tags: [] });
  });

  it('sends no languages: the learner settings decide which translations show', () => {
    expect(buildListImportPayload('hsk3-1', selected, [])).not.toHaveProperty('import_langs');
  });
});

describe('buildListImportPayload import mode', () => {
  it('leaves import_mode out when no mode is given', () => {
    expect(buildListImportPayload('hsk3-1', ['hsk3-1'], ['hsk3-1'])).not.toHaveProperty('import_mode');
  });

  it('passes the mode through', () => {
    expect(buildListImportPayload('hsk3-1', ['hsk3-1'], ['hsk3-1'], 'known').import_mode).toBe('known');
  });
});

describe('buildMatchAllPayload', () => {
  it('imports the first tag narrowed by the others and applies all of them', () => {
    expect(buildMatchAllPayload(['hsk3-1', 'topic-food'])).toEqual({
      tag: 'hsk3-1', and_tags: ['topic-food'], apply_tags: ['hsk3-1', 'topic-food'],
    });
  });

  it('passes the import mode through', () => {
    const payload = buildMatchAllPayload(['a', 'b', 'c'], 'include');
    expect(payload.and_tags).toEqual(['b', 'c']);
    expect(payload.import_mode).toBe('include');
  });
});

describe('nativeLangsLabel', () => {
  it('joins primary and secondary language', () => {
    expect(nativeLangsLabel('en', 'de')).toBe('EN + DE');
  });

  it('shows only the primary language without a secondary one', () => {
    expect(nativeLangsLabel('de', '')).toBe('DE');
  });
});

function buildListSyncPayload(list, includeRemoved) {
  const payload = { tag: list.tag, and_tags: [...(list.and_tags || [])], apply_tags: [...(list.apply_tags || [])] };
  if (includeRemoved) payload.include_removed = true;
  return payload;
}

describe('buildListSyncPayload', () => {
  const list = { tag: 'hsk3-1', and_tags: ['topic-food'], apply_tags: ['hsk3-1', 'mine'], new: 2, removed: 1 };

  it('imports the same list with the tags of the last import', () => {
    expect(buildListSyncPayload(list, false)).toEqual({ tag: 'hsk3-1', and_tags: ['topic-food'], apply_tags: ['hsk3-1', 'mine'] });
  });

  it('adds the removed words again only when asked', () => {
    expect(buildListSyncPayload(list, true).include_removed).toBe(true);
  });
});
