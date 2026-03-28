// Internationalization — English / Chinese (Simplified) UI translations

const I18N = {
  en: {
    // Nav & titles
    'nav.title': '词汇训练 · Vocab Trainer',
    'nav.train': 'Train',
    'nav.vocabulary': 'Vocabulary',
    'nav.mismatches': 'Mismatches',
    'nav.stats': 'Stats',
    'nav.signOut': 'Sign out',

    // Language selector
    'lang.en': 'English',
    'lang.zh': '中文',

    // Stats bar
    'statsBar.dueToday': 'Due today:',
    'statsBar.totalWords': 'Total words:',
    'statsBar.newToday': 'New today:',

    // Quiz modes
    'mode.progressive': 'Progressive',
    'mode.random': 'Random',
    'mode.en_to_zh': 'EN → ZH',
    'mode.zh_to_en': 'ZH → EN',
    'mode.zh_pinyin_to_en': 'ZH + Pinyin → EN',

    // Mode labels (result display)
    'modeLabel.en_to_zh': 'English → Chinese',
    'modeLabel.zh_to_en': 'Chinese → English',
    'modeLabel.zh_pinyin_to_en': 'Chinese + Pinyin → English',
    'modeLabel.new_word': 'New Word',

    // Tier/Level labels
    'tier.all': 'All',
    'tier.new': 'New',
    'tier.struggling': 'Struggling',
    'tier.learning': 'Learning',
    'tier.practicing': 'Practicing',
    'tier.mastered': 'Mastered',
    'filter.level': 'Level:',
    'filter.tags': 'Tags:',
    'filter.quizMode': 'Quiz Mode',
    'filter.done': 'Done',
    'filter.edit': '▼ Edit',

    // Empty / success / error states
    'empty.title': 'No vocabulary yet',
    'empty.msg': 'Add some words to start training.',
    'empty.addBtn': 'Add Vocabulary',
    'success.title': 'All done for today!',
    'success.learnMore': 'Learn more words?',
    'success.alsoNew': 'Also introduce new words today',
    'error.icon': '⚠️',

    // Card area
    'card.placeholder': 'Type your answer…',
    'card.submit': 'Submit',
    'card.readAloud': 'Read aloud',

    // New word area
    'newWord.label': 'New Word',
    'newWord.skip': 'Skip (7 days)',
    'newWord.gotIt': 'Got it',
    'newWord.noNew': 'No new words for now',

    // Result area
    'result.correct': '✓ Correct!',
    'result.wrong': '✗ Wrong',
    'result.correctLabel': 'Correct',
    'result.yourAnswer': 'Your answer',
    'result.belongsTo': 'Your answer belongs to',
    'result.addTranslation': 'Add "{answer}" as correct answer',
    'result.added': '✓ Added',
    'result.charBreakdown': 'Character breakdown',
    'result.flagReview': 'Flag for Review',
    'result.flagged': '✓ Flagged',
    'result.next': 'Next →',
    'result.graduated': 'Graduated! Word is now in the regular review queue.',
    'result.learning': 'Learning — get {n} correct in a row to graduate',
    'result.nextReview': 'Next review in {n} day(s)',
    'result.correctStats': 'Correct: {eff} / {total}',
    'result.streakBonus': '+{n} streak bonus',
    'result.streak': 'Streak: {n}',
    'result.streakProgress': 'Streak: {n} / {total}',

    // Vocab page
    'vocab.addWord': 'Add Word',
    'vocab.editWord': 'Edit Word',
    'vocab.chinese': 'Chinese',
    'vocab.pinyin': 'Pinyin',
    'vocab.englishTranslations': 'English Translation(s)',
    'vocab.tags': 'Tags',
    'vocab.addTranslation': '+ Add translation',
    'vocab.addTag': 'Add tag…',
    'vocab.startTraining': 'Start training immediately',
    'vocab.save': 'Save',
    'vocab.cancel': 'Cancel',
    'vocab.reviewNotice': 'This word is flagged for review — the flag will be cleared when you save.',
    'vocab.listTitle': 'Vocabulary List',
    'vocab.search': 'Search…',
    'vocab.hideUnseen': 'Hide Unseen',
    'vocab.needsReview': 'Needs Review',
    'vocab.dueToday': 'Due Today',
    'vocab.dueTomorrow': 'Due Tomorrow',
    'vocab.english': 'English',
    'vocab.level': 'Level',
    'vocab.due': 'Due',
    'vocab.actions': 'Actions',
    'vocab.edit': 'Edit',
    'vocab.delete': 'Delete',
    'vocab.unseen': 'Unseen',
    'vocab.noEntries': 'No vocabulary entries found.',
    'vocab.entries': '{n} entries',
    'vocab.perPage': '/ page',
    'vocab.confirmDelete': 'Delete this word and all its translations? This cannot be undone.',
    'vocab.zhRequired': 'Chinese text is required.',
    'vocab.enRequired': 'At least one English translation is required.',
    'vocab.replacePinyin': 'Replace pinyin "{old}" with "{new}"?',
    'vocab.englishPlaceholder': 'English translation',
    'vocab.zhPlaceholder': 'e.g. 你好',
    'vocab.pinyinPlaceholder': 'e.g. nǐ hǎo',
    'vocab.autoTranslate': 'Auto-translate',
    'vocab.translating': 'Translating…',
    'vocab.enterTextFirst': 'Enter Chinese or English text first.',
    'vocab.createTag': 'Create "{tag}"',
    'vocab.review': 'review',
    'vocab.dueLabel': 'Due',
    'vocab.inDays': 'in {n}d',

    // Download modal
    'download.title': 'Download Vocabulary',
    'download.columns': 'Columns',
    'download.translations': 'Translations',
    'download.accuracy': 'Accuracy',
    'download.attempts': 'Attempts',
    'download.dueDate': 'Due Date',
    'download.format': 'Format',
    'download.plainText': 'Plain text',
    'download.download': 'Download',
    'download.downloading': 'Downloading…',

    // Stats page
    'stats.trainingHistory': 'Training History',
    'stats.noTrainingData': 'No training data yet. Start a quiz to see your progress!',
    'stats.bucketBreakdown': 'Bucket Breakdown',
    'stats.unstacked': 'Unstacked',
    'stats.stacked': 'Stacked',
    'stats.wordsByDueDate': 'Words by Due Date',
    'stats.noSeenWords': 'No seen words yet.',
    'stats.last14Days': 'Last 14 Days',
    'stats.date': 'Date',
    'stats.attempts': 'Attempts',
    'stats.mistakes': 'Mistakes',
    'stats.accuracy': 'Accuracy',
    'stats.wordsSeen': 'Words Seen',
    'stats.bestStreak': 'Best Streak',
    'stats.loading': 'Loading...',
    'stats.failedToLoad': 'Failed to load stats.',
    'stats.noDataLast14': 'No data in the last 14 days.',
    'stats.noTrainingDataShort': 'No training data yet.',
    'stats.accuracyDistribution': 'Accuracy Distribution',
    'stats.hardestWords': 'Hardest Words',
    'stats.mostPracticed': 'Most Practiced',
    'stats.word': 'Word',
    'stats.translation': 'Translation',
    'stats.notEnoughData': 'Not enough data yet.',
    'stats.words': 'Words',
    'stats.answers': 'Answers',
    'stats.total': 'Total: {n}',
    'stats.today': 'Today',

    // Chart labels
    'chart.correct': 'Correct',
    'chart.mistakes': 'Mistakes',
    'chart.wordsSeen': 'Words Seen',

    // Mismatches page
    'mismatches.title': 'Confusion Pairs',
    'mismatches.subtitle': 'Words you confused with other known vocabulary.',
    'mismatches.noConfusions': 'No confusions recorded yet.',
    'mismatches.wordTested': 'Word tested',
    'mismatches.confusedWith': 'Confused with',
    'mismatches.mode': 'Mode',
    'mismatches.count': 'Count',
    'mismatches.lastSeen': 'Last seen',
    'mismatches.today': 'Today',
    'mismatches.yesterday': 'Yesterday',
    'mismatches.daysAgo': '{n}d ago',

    // Login page
    'login.title': '词汇训练 · Vocab Trainer',
    'login.subtitle': 'Sign in to continue',
    'login.username': 'Username',
    'login.password': 'Password',
    'login.signIn': 'Sign in',
    'login.failed': 'Login failed',
    'login.networkError': 'Network error',

    // Stats format
    'stats.attemptsAndMistakes': '{attempts} attempts · {mistakes} mistakes',
    'stats.accuracyTooltip': 'Accuracy: {acc}%\nWords seen: {seen}\nBest streak: {streak}\nBuckets: {new} new · {struggling} struggling · {learning} learning · {practicing} practicing · {mastered} mastered',
    'stats.wordsCount': '{count} words ({pct}%)',

    // Decomposition
    'decompose.radical': 'radical: {r}',
  },

  zh: {
    // Nav & titles
    'nav.title': '词汇训练',
    'nav.train': '训练',
    'nav.vocabulary': '词汇',
    'nav.mismatches': '混淆',
    'nav.stats': '统计',
    'nav.signOut': '退出',

    // Language selector
    'lang.en': 'English',
    'lang.zh': '中文',

    // Stats bar
    'statsBar.dueToday': '今日待复习：',
    'statsBar.totalWords': '总词数：',
    'statsBar.newToday': '今日新词：',

    // Quiz modes
    'mode.progressive': '渐进',
    'mode.random': '随机',
    'mode.en_to_zh': '英→中',
    'mode.zh_to_en': '中→英',
    'mode.zh_pinyin_to_en': '中+拼音→英',

    // Mode labels (result display)
    'modeLabel.en_to_zh': '英语 → 中文',
    'modeLabel.zh_to_en': '中文 → 英语',
    'modeLabel.zh_pinyin_to_en': '中文 + 拼音 → 英语',
    'modeLabel.new_word': '新词',

    // Tier/Level labels
    'tier.all': '全部',
    'tier.new': '新词',
    'tier.struggling': '困难',
    'tier.learning': '学习中',
    'tier.practicing': '练习中',
    'tier.mastered': '已掌握',
    'filter.level': '等级：',
    'filter.tags': '标签：',
    'filter.quizMode': '测验模式',
    'filter.done': '完成',
    'filter.edit': '▼ 编辑',

    // Empty / success / error states
    'empty.title': '还没有词汇',
    'empty.msg': '添加一些词汇开始训练吧。',
    'empty.addBtn': '添加词汇',
    'success.title': '今天全部完成！',
    'success.learnMore': '继续学习更多词汇？',
    'success.alsoNew': '同时引入今天的新词',
    'error.icon': '⚠️',

    // Card area
    'card.placeholder': '输入你的答案…',
    'card.submit': '提交',
    'card.readAloud': '朗读',

    // New word area
    'newWord.label': '新词',
    'newWord.skip': '跳过（7天）',
    'newWord.gotIt': '知道了',
    'newWord.noNew': '暂时不学新词',

    // Result area
    'result.correct': '✓ 正确！',
    'result.wrong': '✗ 错误',
    'result.correctLabel': '正确答案',
    'result.yourAnswer': '你的答案',
    'result.belongsTo': '你的答案属于',
    'result.addTranslation': '将"{answer}"添加为正确答案',
    'result.added': '✓ 已添加',
    'result.charBreakdown': '汉字拆解',
    'result.flagReview': '标记复习',
    'result.flagged': '✓ 已标记',
    'result.next': '下一个 →',
    'result.graduated': '毕业！该词已进入常规复习队列。',
    'result.learning': '学习中——连续答对 {n} 次即可毕业',
    'result.nextReview': '{n} 天后复习',
    'result.correctStats': '正确：{eff} / {total}',
    'result.streakBonus': '+{n} 连续奖励',
    'result.streak': '连续：{n}',
    'result.streakProgress': '连续：{n} / {total}',

    // Vocab page
    'vocab.addWord': '添加词汇',
    'vocab.editWord': '编辑词汇',
    'vocab.chinese': '中文',
    'vocab.pinyin': '拼音',
    'vocab.englishTranslations': '英语翻译',
    'vocab.tags': '标签',
    'vocab.addTranslation': '+ 添加翻译',
    'vocab.addTag': '添加标签…',
    'vocab.startTraining': '立即开始训练',
    'vocab.save': '保存',
    'vocab.cancel': '取消',
    'vocab.reviewNotice': '该词已标记复习——保存后将取消标记。',
    'vocab.listTitle': '词汇列表',
    'vocab.search': '搜索…',
    'vocab.hideUnseen': '隐藏未见',
    'vocab.needsReview': '需要复习',
    'vocab.dueToday': '今天到期',
    'vocab.dueTomorrow': '明天到期',
    'vocab.english': '英语',
    'vocab.level': '等级',
    'vocab.due': '到期',
    'vocab.actions': '操作',
    'vocab.edit': '编辑',
    'vocab.delete': '删除',
    'vocab.unseen': '未见',
    'vocab.noEntries': '没有找到词汇条目。',
    'vocab.entries': '{n} 条目',
    'vocab.perPage': '/ 页',
    'vocab.confirmDelete': '删除此词及所有翻译？此操作无法撤销。',
    'vocab.zhRequired': '中文文本为必填项。',
    'vocab.enRequired': '至少需要一个英语翻译。',
    'vocab.replacePinyin': '将拼音 "{old}" 替换为 "{new}"？',
    'vocab.englishPlaceholder': '英语翻译',
    'vocab.zhPlaceholder': '例如 你好',
    'vocab.pinyinPlaceholder': '例如 nǐ hǎo',
    'vocab.autoTranslate': '自动翻译',
    'vocab.translating': '翻译中…',
    'vocab.enterTextFirst': '请先输入中文或英文文本。',
    'vocab.createTag': '创建"{tag}"',
    'vocab.review': '复习',
    'vocab.dueLabel': '到期',
    'vocab.inDays': '{n}天后',

    // Download modal
    'download.title': '下载词汇',
    'download.columns': '列',
    'download.translations': '翻译',
    'download.accuracy': '准确率',
    'download.attempts': '尝试次数',
    'download.dueDate': '到期日期',
    'download.format': '格式',
    'download.plainText': '纯文本',
    'download.download': '下载',
    'download.downloading': '下载中…',

    // Stats page
    'stats.trainingHistory': '训练历史',
    'stats.noTrainingData': '暂无训练数据。开始测验查看进度！',
    'stats.bucketBreakdown': '等级分布',
    'stats.unstacked': '展开',
    'stats.stacked': '堆叠',
    'stats.wordsByDueDate': '按到期日期分布',
    'stats.noSeenWords': '暂无已学词汇。',
    'stats.last14Days': '最近14天',
    'stats.date': '日期',
    'stats.attempts': '尝试',
    'stats.mistakes': '错误',
    'stats.accuracy': '准确率',
    'stats.wordsSeen': '已见词汇',
    'stats.bestStreak': '最佳连续',
    'stats.loading': '加载中...',
    'stats.failedToLoad': '加载统计失败。',
    'stats.noDataLast14': '最近14天无数据。',
    'stats.noTrainingDataShort': '暂无训练数据。',
    'stats.accuracyDistribution': '准确率分布',
    'stats.hardestWords': '最难词汇',
    'stats.mostPracticed': '练习最多',
    'stats.word': '词汇',
    'stats.translation': '翻译',
    'stats.notEnoughData': '数据不足。',
    'stats.words': '词汇',
    'stats.answers': '回答',
    'stats.total': '总计：{n}',
    'stats.today': '今天',

    // Chart labels
    'chart.correct': '正确',
    'chart.mistakes': '错误',
    'chart.wordsSeen': '已见词汇',

    // Mismatches page
    'mismatches.title': '混淆词对',
    'mismatches.subtitle': '你与其他已知词汇混淆的词。',
    'mismatches.noConfusions': '暂无混淆记录。',
    'mismatches.wordTested': '测试词汇',
    'mismatches.confusedWith': '混淆词汇',
    'mismatches.mode': '模式',
    'mismatches.count': '次数',
    'mismatches.lastSeen': '最后出现',
    'mismatches.today': '今天',
    'mismatches.yesterday': '昨天',
    'mismatches.daysAgo': '{n}天前',

    // Login page
    'login.title': '词汇训练',
    'login.subtitle': '登录以继续',
    'login.username': '用户名',
    'login.password': '密码',
    'login.signIn': '登录',
    'login.failed': '登录失败',
    'login.networkError': '网络错误',

    // Stats format
    'stats.attemptsAndMistakes': '{attempts} 次尝试 · {mistakes} 次错误',
    'stats.accuracyTooltip': '准确率：{acc}%\n已见词汇：{seen}\n最佳连续：{streak}\n等级：{new} 新词 · {struggling} 困难 · {learning} 学习中 · {practicing} 练习中 · {mastered} 已掌握',
    'stats.wordsCount': '{count} 词（{pct}%）',

    // Decomposition
    'decompose.radical': '部首：{r}',
  },
};

// Current UI language — persisted in localStorage
let _uiLang = localStorage.getItem('uiLang') || 'en';

function getUILang() {
  return _uiLang;
}

function setUILang(lang) {
  _uiLang = lang;
  localStorage.setItem('uiLang', lang);
}

// Translate a key, with optional named interpolation: t('key', {n: 5})
function t(key, params) {
  const str = (I18N[_uiLang] && I18N[_uiLang][key]) || (I18N.en && I18N.en[key]) || key;
  if (!params) return str;
  return str.replace(/\{(\w+)\}/g, (_, k) => (params[k] !== undefined ? params[k] : `{${k}}`));
}

// Apply translations to all elements with data-i18n attribute
function applyTranslations() {
  document.querySelectorAll('[data-i18n]').forEach(el => {
    el.textContent = t(el.dataset.i18n);
  });
  document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
    el.placeholder = t(el.dataset.i18nPlaceholder);
  });
  document.querySelectorAll('[data-i18n-title]').forEach(el => {
    el.title = t(el.dataset.i18nTitle);
  });
  // Update the language selector display
  const langSelect = document.getElementById('lang-select');
  if (langSelect) langSelect.value = _uiLang;
}
