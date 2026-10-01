// Tab switching
function initTabs() {
  const tabs = [
    { btn: 'tab-words',      panel: 'panel-words' },
    { btn: 'tab-pinyin',     panel: 'panel-pinyin' },
    { btn: 'tab-components', panel: 'panel-components' },
    { btn: 'tab-mnemonics',  panel: 'panel-mnemonics' },
  ];
  tabs.forEach(({ btn, panel }) => {
    $(btn).addEventListener('click', () => {
      tabs.forEach(({ btn: b, panel: p }) => {
        const active = b === btn;
        $(b).setAttribute('aria-pressed', String(active));
        $(p).classList.toggle('hidden', !active);
      });
    });
  });
}

// Folded "Last 14 days" tables: a disclosure button toggles its table.
function initTableToggles() {
  ['stats-table', 'pinyin-table', 'comp-table'].forEach(id => {
    const btn = $(id + '-toggle');
    const wrap = $(id + '-wrap');
    if (!btn || !wrap) return;
    btn.addEventListener('click', () => {
      const open = wrap.classList.contains('hidden');
      wrap.classList.toggle('hidden', !open);
      btn.setAttribute('aria-expanded', String(open));
      btn.innerHTML = `${open ? '▾' : '▸'} <span>${escHtml(t(open ? 'stats.hideTable' : 'stats.showTable'))}</span>`;
    });
  });
}

function setTile(key, value, sub, subClass) {
  setText(`tile-${key}-value`, value);
  const el = $(`tile-${key}-sub`);
  if (!el) return;
  el.textContent = sub || '';
  el.className = 'sx-tile-sub' + (subClass ? ' ' + subClass : '');
}

// renderSummaryTiles fills the four summary tiles above the training history.
function renderSummaryTiles(days, quizStats, gamification) {
  const today = new Date().toISOString().slice(0, 10);
  const s = statsSummary(days, today);
  setTile('answers', String(s.answersToday), t('stats.mistakesCount', { n: s.mistakesToday }));
  let deltaText = '';
  let deltaClass = '';
  if (s.accuracyDelta !== null) {
    const sign = s.accuracyDelta > 0 ? '+' : s.accuracyDelta < 0 ? '−' : '±';
    deltaText = t('stats.vsPrevious', { delta: `${sign}${Math.abs(s.accuracyDelta)} %` });
    deltaClass = s.accuracyDelta > 0 ? 'sx-up' : s.accuracyDelta < 0 ? 'sx-down' : '';
  }
  setTile('accuracy', s.accuracy === null ? '—' : `${s.accuracy}%`, deltaText, deltaClass);
  if (quizStats) setText('tile-training-sub', t('stats.dueTodayCount', { n: quizStats.due_today || 0 }));
  if (gamification) {
    setText('tile-streak-label', t('stats.dayStreak'));
    setTile('streak', String(s.streak), s.streak > 0 ? '🔥' : '');
  } else {
    setText('tile-streak-label', t('stats.trainingTimeLong'));
    setTile('streak', formatTrainingTime(s.trainingSeconds), t('stats.last14Days'));
  }
}

document.addEventListener('DOMContentLoaded', async () => {
  initTabs();
  initTableToggles();
  if (window.Chart) {
    Chart.defaults.datasets.bar.maxBarThickness = 28;
    Chart.defaults.datasets.bar.borderRadius = 4;
  }

  // Load all tabs in parallel
  const [wordsResult, pinyinResult, compResult, hmmResult, compDueDateResult, quizStatsResult, settingsResult] = await Promise.allSettled([
    apiFetch('/api/quiz/daily-stats'),
    apiFetch('/api/pinyin-quiz/daily-stats'),
    apiFetch('/api/component/stats'),
    apiFetch('/api/hmm/breakdown'),
    apiFetch('/api/component/due-date-distribution'),
    apiFetch('/api/quiz/stats'),
    apiFetch('/api/settings'),
  ]);
  const quizStats = quizStatsResult.status === 'fulfilled' ? quizStatsResult.value : null;
  const gamification = settingsResult.status === 'fulfilled' && !!settingsResult.value.gamification_enabled;

  // --- Words tab ---
  if (wordsResult.status === 'rejected') {
    $('stats-table-body').innerHTML =
      `<tr><td colspan="12" class="sx-empty sx-error">${escHtml(t('stats.failedToLoad'))}</td></tr>`;
  } else {
    const days = (wordsResult.value.days) || [];
    renderSummaryTiles(days, quizStats, gamification);
    if (days.length === 0) {
      $('stats-chart').parentElement.style.display = 'none';
      show('chart-empty');
      $('stats-table-body').innerHTML =
        `<tr><td colspan="12" class="sx-empty">${escHtml(t('stats.noTrainingDataShort'))}</td></tr>`;
    } else {
      renderChart(days);
      renderBucketChart(days);
      renderTable(days);
    }
  }

  // Load word-level statistics, with a tag filter for the bucket breakdown
  await initWordStatsTagFilter();

  // Load due-date distribution with tag filters
  await initDueDateChart();

  // --- Components tab ---
  if (compResult.status === 'rejected') {
    $('comp-table-body').innerHTML =
      `<tr><td colspan="4" class="sx-empty sx-error">${escHtml(t('stats.failedToLoad'))}</td></tr>`;
  } else {
    const cdays = (compResult.value.days) || [];
    if (cdays.length === 0) {
      $('comp-stats-chart').parentElement.style.display = 'none';
      show('comp-chart-empty');
      $('comp-table-body').innerHTML =
        `<tr><td colspan="4" class="sx-empty">${escHtml(t('stats.noCompTrainingData'))}</td></tr>`;
    } else {
      renderCompChart(cdays);
      renderCompTable(cdays);
    }
  }

  if (compDueDateResult.status === 'fulfilled') {
    const cdates = compDueDateResult.value.dates || [];
    const canvas = $('comp-due-date-chart');
    if (cdates.length === 0) {
      canvas.parentElement.style.display = 'none';
      show('comp-due-chart-empty');
    } else {
      canvas.parentElement.style.display = '';
      hide('comp-due-chart-empty');
      renderCompDueDateChart(cdates);
    }
  } else {
    $('comp-due-date-chart').parentElement.style.display = 'none';
    show('comp-due-chart-empty');
  }

  // --- Mnemonics tab ---
  if (hmmResult.status === 'rejected') {
    $('hmm-breakdown-body').innerHTML =
      `<tr><td colspan="5" class="sx-empty sx-error">${escHtml(t('stats.failedToLoad'))}</td></tr>`;
  } else {
    const breakdown = (hmmResult.value.breakdown) || [];
    if (breakdown.length === 0) {
      show('hmm-breakdown-empty');
      $('hmm-breakdown-body').innerHTML = '';
    } else {
      renderHMMBreakdown(breakdown);
    }
  }

  // --- Pinyin tab ---
  if (pinyinResult.status === 'rejected') {
    $('pinyin-table-body').innerHTML =
      `<tr><td colspan="5" class="sx-empty sx-error">${escHtml(t('stats.failedToLoad'))}</td></tr>`;
  } else {
    const pdays = (pinyinResult.value.days) || [];
    if (pdays.length === 0) {
      $('pinyin-stats-chart').parentElement.style.display = 'none';
      show('pinyin-chart-empty');
      $('pinyin-table-body').innerHTML =
        `<tr><td colspan="5" class="sx-empty">${escHtml(t('stats.noPinyinTrainingData'))}</td></tr>`;
    } else {
      renderPinyinChart(pdays);
      renderPinyinToneChart(pdays);
      renderPinyinTable(pdays);
    }
  }
});

// statsSummary computes the summary tiles from the daily stats: today's
// answers, 14-day accuracy and its change vs the 14 days before, the day
// streak and the 14-day training time. Pure for unit testing.
function statsSummary(days, today) {
  const byDate = new Map((days || []).map(d => [d.date, d]));
  const shift = (date, n) => {
    const d = new Date(date + 'T00:00:00Z');
    d.setUTCDate(d.getUTCDate() + n);
    return d.toISOString().slice(0, 10);
  };
  const windowTotals = (fromOffset, toOffset) => {
    let attempts = 0, mistakes = 0, seconds = 0;
    for (let i = fromOffset; i <= toOffset; i++) {
      const d = byDate.get(shift(today, -i));
      if (!d) continue;
      attempts += d.attempts || 0;
      mistakes += d.mistakes || 0;
      seconds += d.training_seconds || 0;
    }
    return { attempts, mistakes, seconds };
  };
  const pct = w => (w.attempts > 0 ? Math.round(((w.attempts - w.mistakes) / w.attempts) * 100) : null);
  const current = windowTotals(0, 13);
  const previous = windowTotals(14, 27);
  const accuracy = pct(current);
  const prevAccuracy = pct(previous);
  // The streak ends today, or yesterday while today has no answers yet.
  let streak = 0;
  let offset = (byDate.get(today)?.attempts || 0) > 0 ? 0 : 1;
  while ((byDate.get(shift(today, -offset))?.attempts || 0) > 0) {
    streak++;
    offset++;
  }
  const todayRow = byDate.get(today);
  return {
    answersToday: todayRow?.attempts || 0,
    mistakesToday: todayRow?.mistakes || 0,
    accuracy,
    accuracyDelta: accuracy !== null && prevAccuracy !== null ? accuracy - prevAccuracy : null,
    streak,
    trainingSeconds: current.seconds,
  };
}

function formatTrainingTime(seconds) {
  if (!seconds || seconds <= 0) return '—';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${seconds}s`;
}

function renderChart(days) {
  const labels = days.map(d => formatDateLabel(d.date));
  const ctx = $('stats-chart').getContext('2d');
  new Chart(ctx, {
    type: 'bar',
    data: {
      labels,
      datasets: [
        {
          label: t('chart.correct'),
          data: days.map(d => d.attempts - d.mistakes),
          backgroundColor: '#2563eb',
          stack: 'answers',
        },
        {
          label: t('chart.mistakes'),
          data: days.map(d => d.mistakes),
          backgroundColor: '#fca5a5',
          stack: 'answers',
        },
        {
          label: t('chart.wordsSeen'),
          data: days.map(d => d.words_seen),
          type: 'line',
          borderColor: '#9ca3af',
          backgroundColor: 'rgba(156, 163, 175, 0.1)',
          fill: false,
          yAxisID: 'y1',
          tension: 0.3,
          pointRadius: 2,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: { ticks: { maxRotation: 45, autoSkip: true, maxTicksLimit: 20 } },
        y: { beginAtZero: true, title: { display: true, text: t('stats.answers') }, stacked: true },
        y1: {
          beginAtZero: true,
          position: 'right',
          title: { display: true, text: t('stats.words') },
          grid: { drawOnChartArea: false },
        },
      },
      plugins: {
        tooltip: {
          callbacks: {
            afterBody(items) {
              const idx = items[0].dataIndex;
              const d = days[idx];
              const acc = d.attempts > 0 ? Math.round(((d.attempts - d.mistakes) / d.attempts) * 100) : 0;
              return t('stats.accuracyTooltip', {
                acc, seen: d.words_seen, streak: d.correct_streak,
                new: d.bucket_new || 0, struggling: d.bucket_struggling || 0,
                learning: d.bucket_learning || 0, practicing: d.bucket_practicing || 0,
                mastered: d.bucket_mastered || 0,
              });
            },
          },
        },
      },
    },
  });
}

let _bucketChart = null;
let _bucketStacked = true;

function renderBucketChart(days) {
  // Only show if at least one day has bucket data
  const hasBuckets = days.some(d => (d.bucket_new||0) + (d.bucket_struggling||0) + (d.bucket_learning||0) + (d.bucket_practicing||0) + (d.bucket_mastered||0) > 0);
  if (!hasBuckets) return;
  show('bucket-chart-section');

  const toggle = $('bucket-stack-toggle');
  toggle.addEventListener('click', () => {
    _bucketStacked = !_bucketStacked;
    toggle.textContent = _bucketStacked ? t('stats.unstacked') : t('stats.stacked');
    drawBucketChart(days);
  });

  drawBucketChart(days);
}

function drawBucketChart(days) {
  if (_bucketChart) {
    _bucketChart.destroy();
    _bucketChart = null;
  }

  const labels = days.map(d => formatDateLabel(d.date));
  const ctx = $('bucket-chart').getContext('2d');
  _bucketChart = new Chart(ctx, {
    type: 'line',
    data: {
      labels,
      datasets: [
        { label: t('tier.mastered'),   data: days.map(d => d.bucket_mastered   || 0), backgroundColor: '#22c55eb3', borderColor: '#22c55e', fill: _bucketStacked, tension: 0.3, pointRadius: 2 },
        { label: t('tier.practicing'), data: days.map(d => d.bucket_practicing || 0), backgroundColor: '#3b82f6b3', borderColor: '#3b82f6', fill: _bucketStacked, tension: 0.3, pointRadius: 2 },
        { label: t('tier.learning'),   data: days.map(d => d.bucket_learning   || 0), backgroundColor: '#f59e0bb3', borderColor: '#f59e0b', fill: _bucketStacked, tension: 0.3, pointRadius: 2 },
        { label: t('tier.struggling'), data: days.map(d => d.bucket_struggling || 0), backgroundColor: '#ef4444b3', borderColor: '#ef4444', fill: _bucketStacked, tension: 0.3, pointRadius: 2 },
        { label: t('tier.new'),        data: days.map(d => d.bucket_new        || 0), backgroundColor: '#8b5cf6b3', borderColor: '#8b5cf6', fill: _bucketStacked, tension: 0.3, pointRadius: 2 },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: { ticks: { maxRotation: 45, autoSkip: true, maxTicksLimit: 20 } },
        y: { beginAtZero: true, stacked: _bucketStacked, title: { display: true, text: t('stats.words') } },
      },
      plugins: {
        tooltip: {
          callbacks: {
            footer(items) {
              const total = items.reduce((s, i) => s + i.raw, 0);
              return t('stats.total', { n: total });
            },
          },
        },
      },
    },
  });
}

function renderTable(days) {
  // Show last 14 days, most recent first
  const recent = days.slice(-14).reverse();
  const tbody = $('stats-table-body');
  if (recent.length === 0) {
    tbody.innerHTML = `<tr><td colspan="12" class="sx-empty">${escHtml(t('stats.noDataLast14'))}</td></tr>`;
    return;
  }
  tbody.innerHTML = recent.map(d => {
    const correct = d.attempts - d.mistakes;
    const acc = d.attempts > 0 ? Math.round((correct / d.attempts) * 100) : 0;
    const accColor = accClass(acc);
    return `<tr>
      <td class="sx-td-date">${escHtml(formatDateLabel(d.date))}</td>
      <td class="num">${d.attempts}</td>
      <td class="num">${d.mistakes}</td>
      <td class="num ${accColor}">${acc}%</td>
      <td class="num">${d.words_seen}</td>
      <td class="num">${d.correct_streak}</td>
      <td class="num sx-t-new">${d.bucket_new || 0}</td>
      <td class="num sx-t-struggling">${d.bucket_struggling || 0}</td>
      <td class="num sx-t-learning">${d.bucket_learning || 0}</td>
      <td class="num sx-t-practicing">${d.bucket_practicing || 0}</td>
      <td class="num sx-t-mastered">${d.bucket_mastered || 0}</td>
      <td class="num sx-muted">${formatTrainingTime(d.training_seconds)}</td>
    </tr>`;
  }).join('');
}

// --- Word Statistics / Bucket Breakdown Tag Filter ---

let wordStatsSelectedTags = [];

async function initWordStatsTagFilter() {
  let allTags = [];
  try { allTags = await apiFetch('/api/tags'); } catch (_) {}
  renderWordStatsTagChips(allTags);
  await loadWordStats();
}

function renderWordStatsTagChips(allTags) {
  const container = $('bucket-tag-chips');
  if (!container) return;
  container.innerHTML = '';
  if (allTags.length === 0) return;
  for (const tag of allTags) {
    const pill = document.createElement('button');
    const active = wordStatsSelectedTags.includes(tag);
    pill.type = 'button';
    pill.className = 'ui-chip sx-chip';
    pill.setAttribute('aria-pressed', String(active));
    pill.textContent = tag;
    pill.addEventListener('click', () => {
      if (wordStatsSelectedTags.includes(tag)) {
        wordStatsSelectedTags = wordStatsSelectedTags.filter(t => t !== tag);
      } else {
        wordStatsSelectedTags.push(tag);
      }
      renderWordStatsTagChips(allTags);
      loadWordStats();
    });
    container.appendChild(pill);
  }
}

async function loadWordStats() {
  let url = '/api/quiz/word-stats';
  if (wordStatsSelectedTags.length > 0) {
    url += '?tags=' + encodeURIComponent(wordStatsSelectedTags.join(','));
  }
  let ws;
  try { ws = await apiFetch(url); } catch (_) { return; }
  if (ws && wordStatsSelectedTags.length === 0) setText('tile-training-value', String(ws.total_seen || 0));
  if (ws && (ws.total_seen > 0 || (ws.accuracy_buckets.unseen || 0) > 0)) {
    renderWordStats(ws);
    show('word-stats-section');
  } else {
    hide('word-stats-section');
  }
}

// Words never seen yet — only shown in Levels, not a quiz tier.
const UNSEEN_BUCKET = { key: 'unseen', i18nKey: 'tier.unseen', color: '#d1d5db', icon: '○', fg: '#6b7280' };

// Accuracy colour class for a percentage: green ≥ 80, amber ≥ 50, else red.
function accClass(acc) {
  return acc >= 80 ? 'sx-good' : acc >= 50 ? 'sx-mid' : 'sx-bad';
}

function renderWordStats(ws) {
  // Safety: colours and icons come from the hardcoded TIERS array in app.js, never from user input.
  const buckets = [UNSEEN_BUCKET, ...TIERS];
  const counts = buckets.map(b => ws.accuracy_buckets[b.key] || 0);
  const total = counts.reduce((a, b) => a + b, 0);
  setText('levels-total', total === 1 ? t('stats.wordsTotalOne') : t('stats.wordsTotal', { n: total }));
  $('levels-bar').innerHTML = buckets.map((b, i) =>
    counts[i] > 0 ? `<span style="flex:${counts[i]};background:${b.color}" title="${escHtml(t(b.i18nKey))}: ${counts[i]}"></span>` : ''
  ).join('');
  $('tier-legend').innerHTML = buckets.map((b, i) => {
    const pct = total > 0 ? Math.round(counts[i] / total * 100) : 0;
    return `<div class="sx-level-row">
      <span class="sx-level-icon">${b.icon}</span>
      <span class="sx-level-name"><span style="color:${b.fg}">${escHtml(t(b.i18nKey))}</span><span class="sx-level-desc">${escHtml(t('stats.tierDesc.' + b.key))}</span></span>
      <span class="sx-level-n">${counts[i]}</span>
      <span class="sx-level-pct">${pct}%</span>
    </div>`;
  }).join('');

  renderWordTable('hardest-body', ws.hardest, 'accuracy');
  renderWordTable('most-practiced-body', ws.most_practiced, 'attempts');
}

// renderWordTable renders ranked word rows: word, pinyin, meanings, a bar
// and the value (accuracy for "hardest", attempts for "most practiced").
function renderWordTable(containerId, words, by) {
  const box = $(containerId);
  if (!words || words.length === 0) {
    box.innerHTML = `<div class="sx-empty">${escHtml(t('stats.notEnoughData'))}</div>`;
    return;
  }
  const maxAttempts = Math.max(...words.map(w => w.total_attempts), 1);
  box.innerHTML = words.map(w => {
    const acc = Math.round(w.accuracy);
    const meanings = Object.values(w.translations || {}).flat().map(x => escHtml(x)).join(', ');
    const width = by === 'accuracy' ? acc : Math.round(w.total_attempts / maxAttempts * 100);
    const barColor = by === 'accuracy' ? (acc < 50 ? '#ef4444' : acc < 80 ? '#f59e0b' : '#22c55e') : '#2563eb';
    const value = by === 'accuracy' ? `${acc}%` : String(w.total_attempts);
    const title = by === 'accuracy'
      ? t('stats.attemptsCount', { n: w.total_attempts })
      : `${acc}%`;
    return `<div class="sx-rank-row" title="${escHtml(title)}">
      <span class="sx-rank-word"><span class="hanzi sx-rank-zh">${escHtml(w.zh_text)}</span>${w.pinyin ? `<span class="sx-rank-py">${escHtml(w.pinyin)}</span>` : ''}<span class="sx-rank-en">${meanings}</span></span>
      <span class="sx-rank-track"><span style="width:${width}%;background:${barColor}"></span></span>
      <span class="sx-rank-value">${value}</span>
    </div>`;
  }).join('');
}

function formatDateLabel(dateStr) {
  // "2026-03-04" -> "Mar 4"
  const parts = dateStr.split('-');
  const months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
  return months[parseInt(parts[1], 10) - 1] + ' ' + parseInt(parts[2], 10);
}

// --- Due Date Distribution Chart with Tag Filters ---

let dueSelectedTags = [];
let dueChart = null;

async function initDueDateChart() {
  let allTags = [];
  try { allTags = await apiFetch('/api/tags'); } catch (_) {}
  renderDueTagChips(allTags);
  await loadDueDateChart();
}

function renderDueTagChips(allTags) {
  const container = $('due-tag-chips');
  container.innerHTML = '';
  if (allTags.length === 0) return;
  for (const tag of allTags) {
    const pill = document.createElement('button');
    const active = dueSelectedTags.includes(tag);
    pill.type = 'button';
    pill.className = 'ui-chip sx-chip';
    pill.setAttribute('aria-pressed', String(active));
    pill.textContent = tag;
    pill.addEventListener('click', () => {
      if (dueSelectedTags.includes(tag)) {
        dueSelectedTags = dueSelectedTags.filter(t => t !== tag);
      } else {
        dueSelectedTags.push(tag);
      }
      renderDueTagChips(allTags);
      loadDueDateChart();
    });
    container.appendChild(pill);
  }
}

async function loadDueDateChart() {
  let url = '/api/quiz/due-date-distribution';
  if (dueSelectedTags.length > 0) {
    url += '?tags=' + encodeURIComponent(dueSelectedTags.join(','));
  }
  let data;
  try { data = await apiFetch(url); } catch (_) { return; }
  const dates = data.dates || [];
  const canvas = $('due-date-chart');
  if (dates.length === 0) {
    canvas.parentElement.style.display = 'none';
    show('due-chart-empty');
    if (dueChart) { dueChart.destroy(); dueChart = null; }
    return;
  }
  canvas.parentElement.style.display = '';
  hide('due-chart-empty');
  renderDueDateChart(dates);
}

function renderPinyinChart(days) {
  const labels = days.map(d => formatDateLabel(d.date));
  const ctx = $('pinyin-stats-chart').getContext('2d');
  new Chart(ctx, {
    type: 'bar',
    data: {
      labels,
      datasets: [
        {
          label: t('chart.correct'),
          data: days.map(d => d.attempts - d.mistakes),
          backgroundColor: '#7c3aed',
          stack: 'answers',
        },
        {
          label: t('chart.mistakes'),
          data: days.map(d => d.mistakes),
          backgroundColor: '#fca5a5',
          stack: 'answers',
        },
        {
          label: t('stats.soundsSeen'),
          data: days.map(d => d.sounds_seen),
          type: 'line',
          borderColor: '#9ca3af',
          backgroundColor: 'rgba(156, 163, 175, 0.1)',
          fill: false,
          yAxisID: 'y1',
          tension: 0.3,
          pointRadius: 2,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: { ticks: { maxRotation: 45, autoSkip: true, maxTicksLimit: 20 } },
        y: { beginAtZero: true, title: { display: true, text: t('stats.answers') }, stacked: true },
        y1: {
          beginAtZero: true,
          position: 'right',
          title: { display: true, text: t('stats.sounds') },
          grid: { drawOnChartArea: false },
        },
      },
      plugins: {
        tooltip: {
          callbacks: {
            afterBody(items) {
              const idx = items[0].dataIndex;
              const d = days[idx];
              const acc = d.attempts > 0 ? Math.round(((d.attempts - d.mistakes) / d.attempts) * 100) : 0;
              return t('stats.pinyinAccuracyTooltip', { acc, seen: d.sounds_seen });
            },
          },
        },
      },
    },
  });
}

// Tone labels with superscript tone marks for display
const TONE_MARKS = ['ā', 'á', 'ǎ', 'à', 'a·'];
function toneLabels() {
  return TONE_MARKS.map((mark, i) => t('stats.toneLabel', { n: i + 1, mark }));
}
const TONE_COLORS = ['#3b82f6', '#22c55e', '#f59e0b', '#ef4444', '#8b5cf6'];

function renderPinyinToneChart(days) {
  // Aggregate correct/wrong per tone across the last 14 days only
  const recent = days.slice(-14);
  const correct = [0, 0, 0, 0, 0];
  const wrong   = [0, 0, 0, 0, 0];
  for (const d of recent) {
    correct[0] += d.tone1_correct || 0; wrong[0] += d.tone1_wrong || 0;
    correct[1] += d.tone2_correct || 0; wrong[1] += d.tone2_wrong || 0;
    correct[2] += d.tone3_correct || 0; wrong[2] += d.tone3_wrong || 0;
    correct[3] += d.tone4_correct || 0; wrong[3] += d.tone4_wrong || 0;
    correct[4] += d.tone5_correct || 0; wrong[4] += d.tone5_wrong || 0;
  }
  const hasData = correct.some(v => v > 0) || wrong.some(v => v > 0);
  if (!hasData) {
    $('pinyin-tone-chart').parentElement.style.display = 'none';
    show('pinyin-tone-chart-empty');
    return;
  }

  // Show the date range covered by the aggregated data
  const rangeEl = $('pinyin-tone-chart-range');
  if (rangeEl && recent.length > 0) {
    const first = formatDateLabel(recent[0].date);
    const last  = formatDateLabel(recent[recent.length - 1].date);
    rangeEl.textContent = first === last ? first : `${first} – ${last}`;
  }
  const ctx = $('pinyin-tone-chart').getContext('2d');
  new Chart(ctx, {
    type: 'bar',
    data: {
      labels: toneLabels(),
      datasets: [
        {
          label: t('chart.correct'),
          data: correct,
          backgroundColor: '#7c3aed',
          stack: 'tone',
        },
        {
          label: t('chart.mistakes'),
          data: wrong,
          backgroundColor: '#fca5a5',
          stack: 'tone',
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: {},
        y: { beginAtZero: true, stacked: true, title: { display: true, text: t('stats.answers') }, ticks: { precision: 0 } },
      },
      plugins: {
        tooltip: {
          callbacks: {
            afterBody(items) {
              const idx = items[0].dataIndex;
              const total = correct[idx] + wrong[idx];
              const acc = total > 0 ? Math.round(correct[idx] / total * 100) : 0;
              return t('stats.toneAccuracyTooltip', { acc, correct: correct[idx], total });
            },
          },
        },
      },
    },
  });
}

function renderPinyinTable(days) {
  const recent = days.slice(-14).reverse();
  const tbody = $('pinyin-table-body');
  if (recent.length === 0) {
    tbody.innerHTML = `<tr><td colspan="5" class="sx-empty">${escHtml(t('stats.noDataLast14'))}</td></tr>`;
    return;
  }
  tbody.innerHTML = recent.map(d => {
    const correct = d.attempts - d.mistakes;
    const acc = d.attempts > 0 ? Math.round((correct / d.attempts) * 100) : 0;
    const accColor = accClass(acc);
    return `<tr>
      <td class="sx-td-date">${escHtml(formatDateLabel(d.date))}</td>
      <td class="num">${d.attempts}</td>
      <td class="num">${d.mistakes}</td>
      <td class="num ${accColor}">${acc}%</td>
      <td class="num">${d.sounds_seen}</td>
    </tr>`;
  }).join('');
}

// --- Components tab ---

function renderCompChart(days) {
  const labels = days.map(d => formatDateLabel(d.date));
  const ctx = $('comp-stats-chart').getContext('2d');
  new Chart(ctx, {
    type: 'bar',
    data: {
      labels,
      datasets: [
        {
          label: t('chart.correct'),
          data: days.map(d => d.correct),
          backgroundColor: '#2563eb',
          stack: 'answers',
        },
        {
          label: t('chart.mistakes'),
          data: days.map(d => d.wrong),
          backgroundColor: '#fca5a5',
          stack: 'answers',
        },
        {
          label: t('stats.componentsInTraining'),
          data: days.map(d => d.components_total),
          type: 'line',
          borderColor: '#9ca3af',
          backgroundColor: 'rgba(156, 163, 175, 0.1)',
          fill: false,
          yAxisID: 'y1',
          tension: 0.3,
          pointRadius: 2,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: { ticks: { maxRotation: 45, autoSkip: true, maxTicksLimit: 20 } },
        y: { beginAtZero: true, stacked: true, title: { display: true, text: t('stats.answers') } },
        y1: {
          beginAtZero: true,
          position: 'right',
          title: { display: true, text: t('vocab.viewComponents') },
          grid: { drawOnChartArea: false },
        },
      },
      plugins: {
        tooltip: {
          callbacks: {
            afterBody(items) {
              const idx = items[0].dataIndex;
              const d = days[idx];
              const total = d.correct + d.wrong;
              const acc = total > 0 ? Math.round(d.correct / total * 100) : 0;
              return t('stats.compAccuracyTooltip', { acc, n: d.components_total });
            },
          },
        },
      },
    },
  });
}

function renderCompTable(days) {
  const recent = days.slice(-14).reverse();
  const tbody = $('comp-table-body');
  if (recent.length === 0) {
    tbody.innerHTML = `<tr><td colspan="5" class="sx-empty">${escHtml(t('stats.noDataLast14'))}</td></tr>`;
    return;
  }
  tbody.innerHTML = recent.map(d => {
    const total = d.correct + d.wrong;
    const acc = total > 0 ? Math.round(d.correct / total * 100) : 0;
    const accColor = accClass(acc);
    return `<tr>
      <td class="sx-td-date">${escHtml(formatDateLabel(d.date))}</td>
      <td class="num">${total}</td>
      <td class="num">${d.wrong}</td>
      <td class="num ${accColor}">${acc}%</td>
      <td class="num sx-t-new">${d.components_total || 0}</td>
    </tr>`;
  }).join('');
}

let compDueChart = null;

function renderCompDueDateChart(dates) {
  const today = new Date().toISOString().slice(0, 10);
  const labels = dates.map(d => d.date === today ? t('stats.today') : formatDateLabel(d.date));
  const colors = dates.map(d => {
    if (d.date <= today) return '#2563eb';   // overdue/today = solid blue
    return '#bfdbfe';                        // future = light blue
  });
  const ctx = $('comp-due-date-chart').getContext('2d');
  if (compDueChart) compDueChart.destroy();
  compDueChart = new Chart(ctx, {
    type: 'bar',
    data: {
      labels,
      datasets: [{
        label: t('vocab.viewComponents'),
        data: dates.map(d => d.count),
        backgroundColor: colors,
      }],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { ticks: { maxRotation: 45, autoSkip: true, maxTicksLimit: 20 } },
        y: { beginAtZero: true, title: { display: true, text: t('vocab.viewComponents') }, ticks: { precision: 0 } },
      },
      plugins: {
        legend: { display: false },
        tooltip: {
          callbacks: {
            title(items) {
              const idx = items[0].dataIndex;
              return dates[idx].date;
            },
          },
        },
      },
    },
  });
}

// --- Mnemonics tab ---

function hmmTypeLabels() {
  return {
    actor:     t('stats.hmmType.actors'),
    location:  t('stats.hmmType.locations'),
    tone_room: t('stats.hmmType.toneRooms'),
    prop:      t('stats.hmmType.props'),
  };
}

function renderHMMBreakdown(breakdown) {
  const tbody = $('hmm-breakdown-body');
  const typeLabels = hmmTypeLabels();
  let totalRow = { total: 0, due_today: 0, total_attempts: 0, total_correct: 0 };
  const rows = breakdown.map(b => {
    const acc = b.total_attempts > 0 ? Math.round(b.accuracy) : null;
    const accColor = acc === null ? 'sx-muted' : accClass(acc);
    totalRow.total         += b.total;
    totalRow.due_today     += b.due_today;
    totalRow.total_attempts += b.total_attempts;
    totalRow.total_correct  += b.total_correct;
    return `<tr>
      <td class="sx-td-date">${escHtml(typeLabels[b.entity_type] || b.entity_type)}</td>
      <td class="num">${b.total}</td>
      <td class="num">${b.due_today > 0 ? `<span class="sx-due">${b.due_today}</span>` : b.due_today}</td>
      <td class="num">${b.total_attempts}</td>
      <td class="num ${accColor}">${acc !== null ? acc + '%' : '—'}</td>
    </tr>`;
  });
  const totalAcc = totalRow.total_attempts > 0 ? Math.round(totalRow.total_correct / totalRow.total_attempts * 100) : null;
  const totalAccColor = totalAcc === null ? 'sx-muted' : accClass(totalAcc);
  rows.push(`<tr class="sx-total-row">
    <td>${escHtml(t('stats.totalCol'))}</td>
    <td class="num">${totalRow.total}</td>
    <td class="num">${totalRow.due_today > 0 ? `<span class="sx-due">${totalRow.due_today}</span>` : totalRow.due_today}</td>
    <td class="num">${totalRow.total_attempts}</td>
    <td class="num ${totalAccColor}">${totalAcc !== null ? totalAcc + '%' : '—'}</td>
  </tr>`);
  tbody.innerHTML = rows.join('');

  // Summary tiles: one per entity type with total, due today and accuracy.
  $('hmm-tiles').innerHTML = breakdown.map(b => {
    const acc = b.total_attempts > 0 ? Math.round(b.accuracy) : null;
    const meta = [t('stats.dueTodayCount', { n: b.due_today })];
    if (acc !== null) meta.push(t('stats.accuracyPct', { n: acc }));
    return `<div class="sx-tile sx-tile-mnem">
      <div class="sx-tile-label">${escHtml(typeLabels[b.entity_type] || b.entity_type)}</div>
      <div class="sx-tile-value">${b.total}</div>
      <div class="sx-tile-sub">${escHtml(meta.join(' · '))}</div>
    </div>`;
  }).join('');
}

function renderDueDateChart(dates) {
  const today = new Date().toISOString().slice(0, 10);
  const labels = dates.map(d => d.date === today ? t('stats.today') : formatDateLabel(d.date));
  const colors = dates.map(d => {
    if (d.date <= today) return '#2563eb';   // overdue/today = solid blue
    return '#bfdbfe';                        // future = light blue
  });
  const ctx = $('due-date-chart').getContext('2d');
  if (dueChart) dueChart.destroy();
  dueChart = new Chart(ctx, {
    type: 'bar',
    data: {
      labels,
      datasets: [{
        label: t('stats.words'),
        data: dates.map(d => d.count),
        backgroundColor: colors,
      }],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { ticks: { maxRotation: 45, autoSkip: true, maxTicksLimit: 20 } },
        y: { beginAtZero: true, title: { display: true, text: t('stats.words') }, ticks: { precision: 0 } },
      },
      plugins: {
        legend: { display: false },
        tooltip: {
          callbacks: {
            title(items) {
              const idx = items[0].dataIndex;
              return dates[idx].date;
            },
          },
        },
      },
    },
  });
}
