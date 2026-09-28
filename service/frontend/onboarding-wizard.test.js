import { describe, it, expect } from 'vitest';

// Mirrors the pure helpers in onboarding-wizard.js.

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

// Topic lists in the shared library are tags named "topic-<id>". The labels
// and Chinese names come from the design; an unknown topic falls back to its
// capitalised id.
const WZ_TOPIC_NAMES = {
  arts: ['Arts', '艺术'], body: ['Body', '身体'], business: ['Business', '商务'],
  clothing: ['Clothing', '服装'], colors: ['Colors', '颜色'], culture: ['Culture', '文化'],
  directions: ['Directions', '方向'], family: ['Family', '家庭'], feelings: ['Feelings', '感情'],
  food: ['Food', '食物'], gardening: ['Gardening', '园艺'], greetings: ['Greetings', '问候'],
  health: ['Health', '健康'], hobbies: ['Hobbies', '爱好'], home: ['Home', '家'],
  law: ['Law', '法律'], nature: ['Nature', '自然'], numbers: ['Numbers', '数字'],
  politics: ['Politics', '政治'], school: ['School', '学校'], science: ['Science', '科学'],
  shopping: ['Shopping', '购物'], sports: ['Sports', '运动'], technology: ['Technology', '科技'],
  time: ['Time', '时间'], travel: ['Travel', '旅行'], weather: ['Weather', '天气'], work: ['Work', '工作'],
};

function wizardCapitalize(text) {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

// wizardTopics returns the topic lists in the library tags ({name, word_count}),
// sorted by label.
function wizardTopics(tags) {
  const topics = [];
  for (const tg of tags) {
    const m = /^topic-(.+)$/.exec(tg.name);
    if (!m) continue;
    const id = m[1];
    const [label, zh] = WZ_TOPIC_NAMES[id] || [wizardCapitalize(id), ''];
    topics.push({ id, tag: tg.name, label, zh, words: tg.word_count || 0 });
  }
  return topics.sort((a, b) => a.label.localeCompare(b.label));
}

// wizardTopicMatches matches a topic by its English or Chinese name.
function wizardTopicMatches(topic, query) {
  const q = query.trim().toLowerCase();
  return !q || topic.label.toLowerCase().includes(q) || topic.id.includes(q) || topic.zh.includes(q);
}

// wizardSteps lists the wizard screens; the topics step only exists when the
// library has topic lists.
function wizardSteps(hasTopics) {
  return hasTopics ? ['version', 'level', 'topics', 'pace'] : ['version', 'level', 'pace'];
}

// wizardImportGroups orders the imports: learn levels first, then the levels
// below the start (known / review / include), then the picked topics. Words
// only get the mode of the first list that creates them, so a topic word that
// sits in a "known" HSK level stays known.
function wizardImportGroups(plan, below, topicTags) {
  const groups = [];
  if (plan.learn.length) groups.push({ tags: plan.learn.map(l => l.tag), mode: 'include' });
  if (plan.lower.length) groups.push({ tags: plan.lower.map(l => l.tag), mode: below });
  if (topicTags.length) groups.push({ tags: topicTags, mode: 'include' });
  return groups;
}

// wizardTagLabel names a library tag for the done screens.
function wizardTagLabel(tag) {
  const m = /^hsk(\d+)-(\d+)$/.exec(tag);
  if (m) return `HSK ${m[1]}.0 · ${wizardLevelLabel(Number(m[1]), Number(m[2]))}`;
  const topic = /^topic-(.+)$/.exec(tag);
  return topic ? wizardTopics([{ name: tag }])[0].label : tag;
}


const lib = [
  ...[500, 772, 973, 1000, 1071, 1140].map((n, i) => ({ name: `hsk3-${i + 1}`, word_count: n })),
  { name: 'hsk3-7', word_count: 5636 },
  ...[150, 150, 300, 600, 1300, 2500].map((n, i) => ({ name: `hsk2-${i + 1}`, word_count: n })),
  { name: 'food', word_count: 20 },
];

describe('wizardLevels', () => {
  it('returns the levels of one HSK version, lowest first, with live word counts', () => {
    const levels = wizardLevels([...lib].reverse(), 3);
    expect(levels.map(l => l.level)).toEqual([1, 2, 3, 4, 5, 6, 7]);
    expect(levels[0]).toEqual({ level: 1, tag: 'hsk3-1', words: 500 });
    expect(levels[6].words).toBe(5636);
  });

  it('ignores other versions and unrelated tags', () => {
    expect(wizardLevels(lib, 2).map(l => l.tag)).toEqual(['hsk2-1', 'hsk2-2', 'hsk2-3', 'hsk2-4', 'hsk2-5', 'hsk2-6']);
    expect(wizardLevels([{ name: 'hsk3' }, { name: 'xhsk3-1' }], 3)).toEqual([]);
  });
});

describe('wizardLevelLabel', () => {
  it('shows HSK 3.0 level 7 as the combined 7–9 list', () => {
    expect(wizardLevelLabel(3, 7)).toBe('7–9');
    expect(wizardLevelLabel(3, 2)).toBe('2');
    expect(wizardLevelLabel(2, 6)).toBe('6');
  });
});

describe('wizardBands', () => {
  it('splits HSK 3.0 into beginner 1–3, intermediate 4–6 and advanced 7–9', () => {
    const bands = wizardBands(3, wizardLevels(lib, 3));
    expect(bands.map(b => [b.id, b.levels.map(l => l.level)])).toEqual([['beg', [1, 2, 3]], ['int', [4, 5, 6]], ['adv', [7]]]);
  });

  it('splits HSK 2.0 into pairs of levels', () => {
    const bands = wizardBands(2, wizardLevels(lib, 2));
    expect(bands.map(b => b.levels.map(l => l.level))).toEqual([[1, 2], [3, 4], [5, 6]]);
  });

  it('drops bands that have no list in the library', () => {
    const bands = wizardBands(3, wizardLevels([{ name: 'hsk3-1', word_count: 5 }], 3));
    expect(bands.map(b => b.id)).toEqual(['beg']);
  });
});

describe('wizardPlan', () => {
  const levels = wizardLevels(lib, 3);
  const beg = wizardBands(3, levels)[0].levels;
  const int = wizardBands(3, levels)[1].levels;

  it('starting at the lowest level imports the whole band and nothing below', () => {
    const plan = wizardPlan(levels, beg, 1, 'known');
    expect(plan.learn.map(l => l.level)).toEqual([1, 2, 3]);
    expect(plan.lower).toEqual([]);
    expect(plan.words).toBe(2245);
  });

  it('starting higher learns from the start level to the end of the band', () => {
    const plan = wizardPlan(levels, beg, 2, 'known');
    expect(plan.learn.map(l => l.level)).toEqual([2, 3]);
    expect(plan.lower.map(l => l.level)).toEqual([1]);
    expect(plan.words).toBe(772 + 973);
  });

  it('counts the lower levels only when they are learned too', () => {
    expect(wizardPlan(levels, int, 5, 'known').words).toBe(1071 + 1140);
    expect(wizardPlan(levels, int, 5, 'review').words).toBe(1071 + 1140);
    const all = wizardPlan(levels, int, 5, 'include');
    expect(all.lowerWords).toBe(500 + 772 + 973 + 1000);
    expect(all.words).toBe(1071 + 1140 + all.lowerWords);
  });
});

describe('wizardEstimate', () => {
  it('estimates minutes per day and days to finish', () => {
    expect(wizardEstimate(10, 500)).toEqual({ minutes: 15, days: 50 });
    expect(wizardEstimate(5, 8)).toEqual({ minutes: 8, days: 2 });
    expect(wizardEstimate(30, 2245)).toEqual({ minutes: 45, days: 75 });
  });
});

describe('wizardToggleLang', () => {
  it('adds and removes a language', () => {
    expect(wizardToggleLang(['en'], 'de')).toEqual(['en', 'de']);
    expect(wizardToggleLang(['en', 'de'], 'en')).toEqual(['de']);
  });

  it('never removes the last language', () => {
    expect(wizardToggleLang(['en'], 'en')).toEqual(['en']);
  });
});

describe('wizardSettingsPatch', () => {
  it('keeps every other setting and applies pace, gamification and audio', () => {
    const current = { max_new_words_per_day: 5, gamification_enabled: true, autoplay_always: false, blur_pinyin: true };
    expect(wizardSettingsPatch(current, { pace: 20, game: false, audio: true })).toEqual({
      max_new_words_per_day: 20, gamification_enabled: false, autoplay_always: true, blur_pinyin: true,
    });
  });
});

const topicLib = [
  ...lib,
  { name: 'topic-travel', word_count: 40 },
  { name: 'topic-food', word_count: 60 },
  { name: 'topic-e2eother', word_count: 5 },
];

describe('wizardTopics', () => {
  it('lists only topic-* tags, sorted by label, with Chinese names', () => {
    const topics = wizardTopics(topicLib);
    expect(topics.map(tp => tp.id)).toEqual(['e2eother', 'food', 'travel']);
    expect(topics[1]).toEqual({ id: 'food', tag: 'topic-food', label: 'Food', zh: '食物', words: 60 });
  });

  it('falls back to the capitalised id for a topic without a Chinese name', () => {
    const [topic] = wizardTopics([{ name: 'topic-e2eother' }]);
    expect(topic).toMatchObject({ label: 'E2eother', zh: '', words: 0 });
  });

  it('is empty when the library has no topics', () => {
    expect(wizardTopics(lib)).toEqual([]);
  });
});

describe('wizardTopicMatches', () => {
  const food = { id: 'food', label: 'Food', zh: '食物' };
  it('matches everything for an empty query', () => {
    expect(wizardTopicMatches(food, '  ')).toBe(true);
  });

  it('matches the English name case-insensitively and the Chinese name', () => {
    expect(wizardTopicMatches(food, 'FOO')).toBe(true);
    expect(wizardTopicMatches(food, '食')).toBe(true);
    expect(wizardTopicMatches(food, 'travel')).toBe(false);
  });
});

describe('wizardSteps', () => {
  it('has a topics step only when the library has topics', () => {
    expect(wizardSteps(true)).toEqual(['version', 'level', 'topics', 'pace']);
    expect(wizardSteps(false)).toEqual(['version', 'level', 'pace']);
  });
});

describe('wizardImportGroups', () => {
  const levels = wizardLevels(lib, 3);
  const beg = wizardBands(3, levels)[0].levels;

  it('imports learn levels first, then lower levels in their mode, then topics', () => {
    const plan = wizardPlan(levels, beg, 2, 'known');
    expect(wizardImportGroups(plan, 'known', ['topic-food'])).toEqual([
      { tags: ['hsk3-2', 'hsk3-3'], mode: 'include' },
      { tags: ['hsk3-1'], mode: 'known' },
      { tags: ['topic-food'], mode: 'include' },
    ]);
  });

  it('leaves out groups that have no lists', () => {
    const plan = wizardPlan(levels, beg, 1, 'known');
    expect(wizardImportGroups(plan, 'known', [])).toEqual([{ tags: ['hsk3-1', 'hsk3-2', 'hsk3-3'], mode: 'include' }]);
  });
});

describe('wizardTagLabel', () => {
  it('names HSK lists and topics for the done screen', () => {
    expect(wizardTagLabel('hsk3-1')).toBe('HSK 3.0 · 1');
    expect(wizardTagLabel('hsk3-7')).toBe('HSK 3.0 · 7–9');
    expect(wizardTagLabel('hsk2-6')).toBe('HSK 2.0 · 6');
    expect(wizardTagLabel('topic-food')).toBe('Food');
  });
});
