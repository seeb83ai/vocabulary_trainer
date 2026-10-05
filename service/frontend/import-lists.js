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
// It sends no languages: imported words are library references, and the
// learner's primary/secondary language decide which translations show.
// A selected list's own tag is applied only to that list's words; other tags
// in applyTags (typed by the user) go on every word.
function buildListImportPayload(sourceTag, selected, applyTags, mode) {
  const payload = {
    tag: sourceTag,
    apply_tags: applyTags.filter(tg => tg === sourceTag || !selected.includes(tg)),
  };
  if (mode) payload.import_mode = mode;
  return payload;
}

const IMPORT_POLL_MS = 1000;

// summarizeImportJobs adds up the progress of several import jobs. finished
// means every job is done or failed. canStart means there are words to train:
// a job that adds trainable words (not import_mode "known") has imported its
// first chunk, or nothing is left to wait for.
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

function importProgressText(summary) {
  if (!summary.total) return t('vocab.importing');
  return t('import.progress', { done: summary.done, total: summary.total });
}

async function queueImportJob(payload) {
  return apiFetch('/api/import', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

// startListImports queues one background import job per selected list and
// returns the jobs. A word in two lists is created by the first job and gets
// the second list's tag from the next one (jobs run one after the other).
async function startListImports(selected, applyTags, mode) {
  const jobs = [];
  for (const tag of selected) {
    jobs.push(await queueImportJob(buildListImportPayload(tag, selected, applyTags, mode)));
  }
  return jobs;
}

// waitForImport polls the jobs until until(summary) is true and returns that
// summary. It throws when a job failed.
async function waitForImport(jobs, until, onProgress) {
  for (;;) {
    const summary = summarizeImportJobs(await Promise.all(jobs.map(job => apiFetch(`/api/import/jobs/${job.id}`))));
    if (summary.failed) throw new Error(summary.error || t('empty.qsFailed'));
    if (onProgress) onProgress(summary);
    if (until(summary)) return summary;
    await new Promise(resolve => setTimeout(resolve, IMPORT_POLL_MS));
  }
}

// importLists imports every selected list and resolves with the summed counts
// once all jobs are finished.
async function importLists(selected, applyTags, mode, onProgress) {
  const jobs = await startListImports(selected, applyTags, mode);
  return waitForImport(jobs, summary => summary.finished, onProgress);
}

// buildMatchAllPayload builds the /api/import body that imports only words
// carrying every selected tag (e.g. HSK 1 + Food). All selected tags are
// applied to the imported words.
function buildMatchAllPayload(selected, mode) {
  const payload = {
    tag: selected[0],
    and_tags: selected.slice(1),
    apply_tags: [...selected],
  };
  if (mode) payload.import_mode = mode;
  return payload;
}

// nativeLangsLabel names the languages a learner sees library translations in
// (primary + secondary language), e.g. "EN + DE".
function nativeLangsLabel(primary, secondary) {
  return [primary, secondary].filter(Boolean).map(l => l.toUpperCase()).join(' + ');
}

// startMatchAllImport queues the single "all tags" import job.
async function startMatchAllImport(selected, mode) {
  return [await queueImportJob(buildMatchAllPayload(selected, mode))];
}

let activeImportWatcher = false;

// watchActiveImports polls the user's queued and running import jobs and
// reports their summary, then null once none are left. Only one watcher runs
// at a time.
async function watchActiveImports(onUpdate) {
  if (activeImportWatcher) return;
  activeImportWatcher = true;
  try {
    for (;;) {
      const jobs = await apiFetch('/api/import/jobs');
      if (!jobs || !jobs.length) {
        onUpdate(null);
        return;
      }
      onUpdate(summarizeImportJobs(jobs));
      await new Promise(resolve => setTimeout(resolve, IMPORT_POLL_MS));
    }
  } catch (e) {
    onUpdate(null);
  } finally {
    activeImportWatcher = false;
  }
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

// buildListSyncPayload builds the /api/import body that brings an imported
// list up to date: the same list and tags as the last import. Words the
// learner deleted are added again only with includeRemoved.
function buildListSyncPayload(list, includeRemoved) {
  const payload = { tag: list.tag, and_tags: [...(list.and_tags || [])], apply_tags: [...(list.apply_tags || [])] };
  if (includeRemoved) payload.include_removed = true;
  return payload;
}
