// onboarding-wizard.js — "First vocabulary setup" wizard shown on the Train
// page while the user has no words. Needs import-lists.js (importLists) and
// app.js (apiFetch, escHtml).

// Levels per band and HSK version. HSK 3.0 level 7 is the combined list 7–9.
const WZ_BANDS = {
  3: { beg: [1, 2, 3], int: [4, 5, 6], adv: [7] },
  2: { beg: [1, 2], int: [3, 4], adv: [5, 6] },
};
const WZ_BAND_IDS = ['beg', 'int', 'adv'];
const WZ_PACES = [5, 10, 20, 30];
const WZ_BAR_COUNT = { 3: 9, 2: 6 };

function wizardLevelLabel(version, level) {
  return version === 3 && level === 7 ? '7–9' : String(level);
}

// wizardLevels returns the levels of an HSK version that exist in the library
// tags ({name, word_count}), lowest first.
function wizardLevels(tags, version) {
  const levels = [];
  for (const tg of tags) {
    const m = /^hsk(\d+)-(\d+)$/.exec(tg.name);
    if (!m || Number(m[1]) !== version) continue;
    levels.push({ level: Number(m[2]), tag: tg.name, words: tg.word_count || 0 });
  }
  return levels.sort((a, b) => a.level - b.level);
}

// wizardBands returns the bands of a version that have at least one level,
// each with its levels.
function wizardBands(version, levels) {
  return WZ_BAND_IDS
    .map(id => ({ id, levels: levels.filter(l => WZ_BANDS[version][id].includes(l.level)) }))
    .filter(b => b.levels.length > 0);
}

// wizardPlan works out what to import. The levels from `start` to the end of
// the band are learned; lower levels of the version follow `below`
// ('known' | 'review' | 'include'). Learn levels come first so a word that
// sits in both stays a new word.
function wizardPlan(levels, bandLevels, start, below) {
  const learn = bandLevels.filter(l => l.level >= start);
  const lower = levels.filter(l => l.level < start);
  const sum = list => list.reduce((n, l) => n + l.words, 0);
  const lowerWords = sum(lower);
  return {
    learn,
    lower,
    lowerWords,
    words: sum(learn) + (below === 'include' ? lowerWords : 0),
  };
}

function wizardEstimate(pace, words) {
  return { minutes: Math.round(pace * 1.5), days: Math.ceil(words / pace) };
}

// wizardToggleLang flips a language but never leaves the list empty.
function wizardToggleLang(langs, lang) {
  if (!langs.includes(lang)) return [...langs, lang];
  return langs.length === 1 ? langs : langs.filter(l => l !== lang);
}

function wizardSettingsPatch(current, choices) {
  return {
    ...current,
    max_new_words_per_day: choices.pace,
    gamification_enabled: choices.game,
    autoplay_always: choices.audio,
  };
}

function wizardFormat(n) {
  return n.toLocaleString(typeof _uiLang === 'string' ? _uiLang : 'en');
}

const wizard = {
  root: null,
  tags: [],
  onDone: null,
  state: null,
  levels: [],
  imported: 0,
};

function wizardInitialState(tags) {
  const versions = hskVersions(tags.map(tg => tg.name));
  const version = versions.includes(3) ? 3 : versions[0];
  return { step: 0, version, band: null, start: null, below: 'known', pace: 10, langs: ['en'], game: true, audio: true, done: false, busy: false, error: '' };
}

// wizardView derives everything the screens show from the state.
function wizardView(state, tags) {
  const levels = wizardLevels(tags, state.version);
  const bands = wizardBands(state.version, levels);
  const band = bands.find(b => b.id === state.band) || bands[0];
  const start = band.levels.some(l => l.level === state.start) ? state.start : band.levels[0].level;
  const plan = wizardPlan(levels, band.levels, start, state.below);
  return { levels, bands, band, start, plan };
}

function wizardStart(root, tags, onDone) {
  const versions = hskVersions(tags.map(tg => tg.name));
  if (versions.length === 0) return false;
  wizard.root = root;
  wizard.tags = tags;
  wizard.onDone = onDone;
  wizard.state = wizardInitialState(tags);
  root.addEventListener('click', wizardClick);
  wizardRender();
  return true;
}

function wizardSet(patch) {
  Object.assign(wizard.state, patch);
  wizardRender();
}

function wizardClick(e) {
  const el = e.target.closest('[data-wz]');
  if (!el || !wizard.root.contains(el) || wizard.state.busy) return;
  const s = wizard.state;
  const [action, value] = el.dataset.wz.split(':');
  switch (action) {
    case 'version': return wizardSet({ version: Number(value), band: null, start: null });
    case 'band': return wizardSet({ band: value, start: null });
    case 'start': return wizardSet({ start: Number(value) });
    case 'below': return wizardSet({ below: value });
    case 'pace': return wizardSet({ pace: Number(value) });
    case 'lang': return wizardSet({ langs: wizardToggleLang(s.langs, value) });
    case 'game': return wizardSet({ game: value === 'on' });
    case 'audio': return wizardSet({ audio: value === 'on' });
    case 'step': return wizardSet({ step: Number(value), error: '' });
    case 'next': return s.step < 2 ? wizardSet({ step: s.step + 1 }) : wizardImport();
    case 'back': return wizardSet({ step: s.step - 1, error: '' });
    case 'adjust': return wizardSet({ done: false, step: 0 });
    case 'train': return wizard.onDone && wizard.onDone({ langs: s.langs, audio: s.audio, game: s.game });
    case 'custom': return document.dispatchEvent(new CustomEvent('ob:custom'));
  }
}

async function wizardImport() {
  const s = wizard.state;
  const view = wizardView(s, wizard.tags);
  wizardSet({ busy: true, error: '' });
  try {
    const en = s.langs.includes('en');
    const de = s.langs.includes('de');
    const learnTags = view.plan.learn.map(l => l.tag);
    const lowerTags = view.plan.lower.map(l => l.tag);
    await importLists(learnTags, learnTags, en, de, 'include');
    if (lowerTags.length) await importLists(lowerTags, lowerTags, en, de, s.below);

    const current = await apiFetch('/api/settings');
    await apiFetch('/api/settings', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(wizardSettingsPatch(current, s)),
    });
    // A debounced filter save from page load may still be pending; make it
    // carry the chosen languages so it cannot overwrite them.
    selectedLangs = [...s.langs];
    localStorage.setItem('quizLangs', JSON.stringify(selectedLangs));
    await apiFetch('/api/training-filters', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        mode: current.train_mode || 'progressive',
        bucket: current.train_bucket || '',
        langs: s.langs,
        mnemonics: current.train_mnemonics !== false,
        components: current.train_components !== false,
        tags: current.train_tags || [],
      }),
    });
    wizard.imported = view.plan.words;
    wizardSet({ busy: false, done: true });
  } catch (err) {
    wizardSet({ busy: false, error: t('wz.failed') });
  }
}

function wizardRender() {
  const s = wizard.state;
  const view = wizardView(s, wizard.tags);
  const head = s.done ? '' : wizardHead(s);
  let body = '';
  if (s.done) body = wizardDoneHTML(s, view);
  else if (s.step === 0) body = wizardStep0HTML(s);
  else if (s.step === 1) body = wizardStep1HTML(s, view);
  else body = wizardStep2HTML(s, view);
  const foot = s.done ? '' : wizardFootHTML(s, view);
  wizard.root.innerHTML = `<div class="wz-card">${head}${body}${foot}</div>`;
}

function wizardHead(s) {
  const segs = [0, 1, 2].map(i => `<span class="${i <= s.step ? 'on' : ''}"></span>`).join('');
  return `<div class="wz-pad wz-head">
    <div class="wz-head-row">
      <span class="wz-eyebrow">${escHtml(t('wz.eyebrow'))}</span>
      <span class="wz-stepno" id="wz-step">${escHtml(t('wz.stepOf', { n: s.step + 1 }))}</span>
    </div>
    <div class="wz-progress">${segs}</div>
  </div>`;
}

function wizardPressed(on) { return `aria-pressed="${on ? 'true' : 'false'}"`; }

function wizardStep0HTML(s) {
  const versions = hskVersions(wizard.tags.map(tg => tg.name));
  const cards = versions.map(v => {
    const levels = wizardLevels(wizard.tags, v);
    const total = levels.reduce((n, l) => n + l.words, 0);
    const count = WZ_BAR_COUNT[v] || levels.length;
    const bars = Array.from({ length: count }, (_, i) => {
      const lv = i + 1;
      const inLib = v === 3 && lv >= 7 ? 7 : lv;
      const bandId = WZ_BAND_IDS.find(id => WZ_BANDS[v][id].includes(inLib)) || 'adv';
      return `<span class="${bandId}" style="height:${22 + (i / (count - 1)) * 78}%"></span>`;
    }).join('');
    const bandCount = wizardBands(v, levels).length;
    const levelsText = v === 3 ? t('wz.levels3', { n: levels.length, bands: bandCount }) : t('wz.levels2', { n: levels.length });
    return `<button type="button" id="wz-version-${v}" class="wz-version" data-wz="version:${v}" ${wizardPressed(s.version === v)}>
      <div class="wz-version-top">
        <div><div class="wz-version-hsk">HSK</div><div class="wz-version-num">${v}.0</div></div>
        <span class="wz-tag">${escHtml(t(v === 3 ? 'wz.tagNew' : 'wz.tagClassic'))}</span>
      </div>
      <div class="wz-bars">${bars}</div>
      <div>
        <span class="wz-version-levels">${escHtml(levelsText)}</span>
        <span class="wz-version-words">${escHtml(t('wz.wordsSince', { n: wizardFormat(total), year: v === 3 ? 2021 : 2012 }))}</span>
      </div>
    </button>`;
  }).join('');
  return `<div class="wz-pad wz-body">
    <h1 class="wz-h1">${escHtml(t('wz.v.title'))}</h1>
    <p class="wz-sub">${escHtml(t('wz.v.sub'))}</p>
    <div class="wz-versions">${cards}</div>
    <div class="wz-legend">
      <span><i style="background:#93c5fd"></i>${escHtml(t('wz.band.beg'))}</span>
      <span><i style="background:#3b82f6"></i>${escHtml(t('wz.band.int'))}</span>
      <span><i style="background:#1e40af"></i>${escHtml(t('wz.band.adv'))}</span>
    </div>
  </div>`;
}

function wizardRange(version, levels) {
  const first = wizardLevelLabel(version, levels[0].level);
  const last = wizardLevelLabel(version, levels[levels.length - 1].level);
  return first === last ? `HSK ${first}` : `HSK ${first}–${last}`;
}

function wizardStep1HTML(s, view) {
  const v = s.version;
  const zh = { beg: '初级', int: '中级', adv: '高级' };
  const sum = list => list.reduce((n, l) => n + l.words, 0);
  const bands = view.bands.map(b => `<button type="button" id="wz-band-${b.id}" class="wz-band" data-wz="band:${b.id}" ${wizardPressed(b.id === view.band.id)}>
      <span class="wz-band-zh">${zh[b.id]}</span>
      <span><span class="wz-band-name">${escHtml(t('wz.band.' + b.id))}</span><span class="wz-band-desc">${escHtml(t('wz.band.' + b.id + '.desc'))}</span></span>
      <span><span class="wz-band-range">${escHtml(wizardRange(v, b.levels))}</span><span class="wz-band-words">${escHtml(t('wz.words', { n: wizardFormat(sum(b.levels)) }))}</span></span>
    </button>`).join('');

  const ladder = view.levels.map(l => {
    const cls = l.level === view.start ? 'start' : view.band.levels.includes(l) ? 'band' : (l.level < view.start && s.below === 'known') ? 'known' : '';
    const w = Math.min(Math.max(l.words, 500), 1400);
    return `<span class="${cls}" style="flex:${w}">HSK ${wizardLevelLabel(v, l.level)}</span>`;
  }).join('');

  const starts = view.band.levels.map(l => `<button type="button" id="wz-start-${l.level}" class="wz-chip" data-wz="start:${l.level}" ${wizardPressed(l.level === view.start)}>
      <b>HSK ${wizardLevelLabel(v, l.level)}</b><small>${escHtml(t('wz.words', { n: wizardFormat(l.words) }))}</small></button>`).join('');

  let below = '';
  if (view.plan.lower.length > 0) {
    const opts = [
      ['known', t('wz.below.known'), t('wz.below.knownDesc')],
      ['review', t('wz.below.review'), t('wz.below.reviewDesc')],
      ['include', t('wz.below.include'), t('wz.below.includeDesc', { n: wizardFormat(view.plan.lowerWords), level: wizardLevelLabel(v, view.start) })],
    ].map(([id, label, desc]) => `<button type="button" id="wz-below-${id}" class="wz-opt" data-wz="below:${id}" ${wizardPressed(s.below === id)}><b>${escHtml(label)}</b><small>${escHtml(desc)}</small></button>`).join('');
    const range = wizardRange(v, view.plan.lower);
    below = `<div class="wz-section" id="wz-below"><div class="wz-label">${escHtml(t('wz.below.title', { range }))}</div><div class="wz-opts">${opts}</div></div>`;
  }

  return `<div class="wz-pad wz-body">
    <h1 class="wz-h1">${escHtml(t('wz.l.title'))}</h1>
    <p class="wz-sub">${escHtml(t('wz.l.sub', { version: v + '.0' }))} <button type="button" class="wz-link" data-wz="step:0">${escHtml(t('wz.change'))}</button></p>
    <div class="wz-bands">${bands}</div>
    <div class="wz-section"><div class="wz-label">${escHtml(t('wz.path', { version: v + '.0' }))}</div><div class="wz-ladder">${ladder}</div></div>
    <div class="wz-section"><div class="wz-label">${escHtml(t('wz.startWith'))}</div><div class="wz-chips">${starts}</div></div>
    ${below}
  </div>`;
}

function wizardSeg(label, buttons) {
  return `<div class="wz-pref"><span class="wz-pref-label">${escHtml(label)}</span><div class="wz-seg">${buttons}</div></div>`;
}

function wizardStep2HTML(s, view) {
  const names = { 5: 'wz.pace.relaxed', 10: 'wz.pace.steady', 20: 'wz.pace.intense', 30: 'wz.pace.sprint' };
  const paces = WZ_PACES.map(n => `<button type="button" id="wz-pace-${n}" class="wz-pace" data-wz="pace:${n}" ${wizardPressed(s.pace === n)}><b>${n}</b><small>${escHtml(t(names[n]))}</small></button>`).join('');
  const est = wizardEstimate(s.pace, view.plan.words);
  const startLabel = `HSK ${wizardLevelLabel(s.version, view.start)}`;
  const toggle = (id, on, onLabel, offLabel) =>
    `<button type="button" id="${id}-on" data-wz="${id.slice(3)}:on" ${wizardPressed(on)}>${escHtml(onLabel)}</button><button type="button" id="${id}-off" data-wz="${id.slice(3)}:off" ${wizardPressed(!on)}>${escHtml(offLabel)}</button>`;
  const langs = [['en', 'English'], ['de', 'Deutsch']].map(([code, label]) =>
    `<button type="button" id="wz-lang-${code}" data-wz="lang:${code}" ${wizardPressed(s.langs.includes(code))}>${label}</button>`).join('');
  return `<div class="wz-pad wz-body">
    <h1 class="wz-h1">${escHtml(t('wz.p.title'))}</h1>
    <p class="wz-sub">${escHtml(t('wz.p.sub'))}</p>
    <div class="wz-label">${escHtml(t('wz.pace'))}</div>
    <div class="wz-paces">${paces}</div>
    <div class="wz-estimate" id="wz-estimate">${escHtml(t('wz.estimate', { minutes: est.minutes, start: startLabel, days: est.days }))}</div>
    <div class="wz-prefs">
      ${wizardSeg(t('wz.meaningIn'), langs)}
      ${wizardSeg(t('wz.gamification'), toggle('wz-game', s.game, t('wz.on'), t('wz.off')))}
      ${wizardSeg(t('wz.audio'), toggle('wz-audio', s.audio, t('wz.on'), t('wz.off')))}
    </div>
    ${s.error ? `<p class="wz-error" id="wz-error" role="alert">${escHtml(s.error)}</p>` : ''}
  </div>`;
}

function wizardFootHTML(s, view) {
  const back = s.step > 0 ? `<button type="button" id="wz-back" class="wz-btn wz-btn-secondary" data-wz="back">${escHtml(t('wz.back'))}</button>` : '';
  const custom = s.step === 0 ? `<button type="button" id="ob-qs-custom" class="wz-link" data-wz="custom">${escHtml(t('empty.qsCustom'))}</button>` : '';
  const label = s.step < 2 ? t('wz.continue') : s.busy ? t('empty.qsImporting') : t('wz.import', { n: wizardFormat(view.plan.words) });
  return `<div class="wz-pad wz-foot">${back}${custom}<button type="button" id="wz-next" class="wz-btn wz-btn-primary" data-wz="next" ${s.busy ? 'disabled' : ''}>${escHtml(label)}</button></div>`;
}

function wizardDoneHTML(s, view) {
  const bandName = t('wz.band.' + view.band.id);
  const pills = [
    `HSK ${s.version}.0`,
    bandName,
    t('wz.startingAt', { level: wizardLevelLabel(s.version, view.start) }),
    t('wz.perDay', { n: s.pace }),
    s.langs.map(l => (l === 'en' ? 'English' : 'Deutsch')).join(' + '),
  ].map(p => `<span>${escHtml(p)}</span>`).join('');
  return `<div class="wz-done" id="wz-done">
    <div class="wz-done-tile" aria-hidden="true">学</div>
    <h1 class="wz-h1">${escHtml(t('wz.ready', { n: wizardFormat(wizard.imported) }))}</h1>
    <p>${escHtml(t('wz.readyText', { n: s.pace }))}</p>
    <div class="wz-pills">${pills}</div>
    <div class="wz-done-actions">
      <button type="button" id="wz-start-training" class="wz-btn wz-btn-primary" data-wz="train">${escHtml(t('wz.startTraining'))}</button>
      <button type="button" id="wz-adjust" class="wz-btn wz-btn-secondary" data-wz="adjust">${escHtml(t('wz.adjust'))}</button>
    </div>
  </div>`;
}
