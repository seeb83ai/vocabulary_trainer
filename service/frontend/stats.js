// Tab switching
function initTabs() {
  const tabs = [
    { btn: 'tab-words',   panel: 'panel-words' },
    { btn: 'tab-pinyin',  panel: 'panel-pinyin' },
  ];
  tabs.forEach(({ btn, panel }) => {
    $(btn).addEventListener('click', () => {
      tabs.forEach(({ btn: b, panel: p }) => {
        const active = b === btn;
        $(b).className = `tab-btn px-5 py-2 rounded-lg text-sm font-medium transition ${active ? 'bg-blue-600 text-white' : 'text-gray-500 hover:text-gray-800'}`;
        $(p).classList.toggle('hidden', !active);
      });
    });
  });
}

document.addEventListener('DOMContentLoaded', async () => {
  initTabs();

  // Load both tabs in parallel
  const [wordsResult, pinyinResult] = await Promise.allSettled([
    apiFetch('/api/quiz/daily-stats'),
    apiFetch('/api/pinyin-quiz/daily-stats'),
  ]);

  // --- Words tab ---
  if (wordsResult.status === 'rejected') {
    $('stats-table-body').innerHTML =
      `<tr><td colspan="11" class="py-8 text-center text-red-500">${escHtml(t('stats.failedToLoad'))}</td></tr>`;
  } else {
    const days = (wordsResult.value.days) || [];
    if (days.length === 0) {
      $('stats-chart').style.display = 'none';
      show('chart-empty');
      $('stats-table-body').innerHTML =
        `<tr><td colspan="11" class="py-8 text-center text-gray-400">${escHtml(t('stats.noTrainingDataShort'))}</td></tr>`;
    } else {
      renderChart(days);
      renderBucketChart(days);
      renderTable(days);
    }
  }

  // Load word-level statistics
  try {
    const ws = await apiFetch('/api/quiz/word-stats');
    if (ws && ws.total_seen > 0) {
      renderWordStats(ws);
      show('word-stats-section');
    }
  } catch (_) {}

  // Load due-date distribution with tag filters
  await initDueDateChart();

  // --- Pinyin tab ---
  if (pinyinResult.status === 'rejected') {
    $('pinyin-table-body').innerHTML =
      `<tr><td colspan="5" class="py-8 text-center text-red-500">${escHtml(t('stats.failedToLoad'))}</td></tr>`;
  } else {
    const pdays = (pinyinResult.value.days) || [];
    if (pdays.length === 0) {
      $('pinyin-stats-chart').style.display = 'none';
      show('pinyin-chart-empty');
      $('pinyin-table-body').innerHTML =
        `<tr><td colspan="5" class="py-8 text-center text-gray-400">No pinyin training data yet.</td></tr>`;
    } else {
      renderPinyinChart(pdays);
      renderPinyinToneChart(pdays);
      renderPinyinTable(pdays);
    }
  }
});

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
          backgroundColor: 'rgba(34, 197, 94, 0.7)',
          stack: 'answers',
        },
        {
          label: t('chart.mistakes'),
          data: days.map(d => d.mistakes),
          backgroundColor: 'rgba(239, 68, 68, 0.7)',
          stack: 'answers',
        },
        {
          label: t('chart.wordsSeen'),
          data: days.map(d => d.words_seen),
          type: 'line',
          borderColor: 'rgba(168, 85, 247, 0.9)',
          backgroundColor: 'rgba(168, 85, 247, 0.1)',
          fill: false,
          yAxisID: 'y1',
          tension: 0.3,
          pointRadius: 2,
        },
      ],
    },
    options: {
      responsive: true,
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
              return `Accuracy: ${acc}%\nWords seen: ${d.words_seen}\nBest streak: ${d.correct_streak}\n` +
                `Buckets: ${d.bucket_new||0} new · ${d.bucket_struggling||0} struggling · ${d.bucket_learning||0} learning · ${d.bucket_practicing||0} practicing · ${d.bucket_mastered||0} mastered`;
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
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: { ticks: { maxRotation: 45, autoSkip: true, maxTicksLimit: 20 } },
        y: { beginAtZero: true, stacked: _bucketStacked, title: { display: true, text: 'Words' } },
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
    tbody.innerHTML = `<tr><td colspan="11" class="py-8 text-center text-gray-400">${escHtml(t('stats.noDataLast14'))}</td></tr>`;
    return;
  }
  tbody.innerHTML = recent.map(d => {
    const correct = d.attempts - d.mistakes;
    const acc = d.attempts > 0 ? Math.round((correct / d.attempts) * 100) : 0;
    const accColor = acc >= 80 ? 'text-green-600' : acc >= 50 ? 'text-yellow-600' : 'text-red-600';
    return `<tr class="border-b border-gray-100 hover:bg-gray-50">
      <td class="py-2 pr-4 font-medium">${escHtml(formatDateLabel(d.date))}</td>
      <td class="py-2 pr-4 text-right">${d.attempts}</td>
      <td class="py-2 pr-4 text-right">${d.mistakes}</td>
      <td class="py-2 pr-4 text-right ${accColor} font-medium">${acc}%</td>
      <td class="py-2 pr-4 text-right">${d.words_seen}</td>
      <td class="py-2 pr-4 text-right">${d.correct_streak}</td>
      <td class="py-2 pr-4 text-right text-violet-600">${d.bucket_new || 0}</td>
      <td class="py-2 pr-4 text-right text-red-600">${d.bucket_struggling || 0}</td>
      <td class="py-2 pr-4 text-right text-amber-600">${d.bucket_learning || 0}</td>
      <td class="py-2 pr-4 text-right text-blue-600">${d.bucket_practicing || 0}</td>
      <td class="py-2 text-right text-green-600">${d.bucket_mastered || 0}</td>
    </tr>`;
  }).join('');
}

function renderWordStats(ws) {
  // Accuracy distribution doughnut
  const aCtx = $('accuracy-chart').getContext('2d');
  const aData = TIERS.map(t => ws.accuracy_buckets[t.key] || 0);
  new Chart(aCtx, {
    type: 'doughnut',
    data: {
      labels: TIERS.map(tier => t('tier.' + tier.label.toLowerCase())),
      datasets: [{
        data: aData,
        backgroundColor: TIERS.map(t => t.color + 'b3'),
      }],
    },
    options: {
      responsive: true,
      plugins: {
        legend: { display: false },
        tooltip: {
          callbacks: {
            label(ctx) {
              const total = ctx.dataset.data.reduce((a, b) => a + b, 0);
              const pct = total > 0 ? Math.round(ctx.raw / total * 100) : 0;
              return `${ctx.label}: ${t('stats.wordsCount', { count: ctx.raw, pct })}`;
            },
          },
        },
      },
    },
  });

  // Tier legend
  // Safety: t.color values come from the hardcoded TIERS array in app.js, never from user input.
  const legend = $('tier-legend');
  const total = aData.reduce((a, b) => a + b, 0);
  legend.innerHTML = TIERS.map((tier, i) => {
    const count = aData[i];
    const pct = total > 0 ? Math.round(count / total * 100) : 0;
    return `<div class="flex items-center justify-between py-1 border-b border-gray-50 last:border-0">
      <div class="flex items-center gap-2">
        <span class="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style="background:${tier.color}"></span>
        <span class="font-medium text-gray-700">${escHtml(t('tier.' + tier.label.toLowerCase()))}</span>
        <span class="text-gray-400 text-xs">${escHtml(tier.desc)}</span>
      </div>
      <span class="text-gray-600 tabular-nums">${count} <span class="text-gray-400">(${pct}%)</span></span>
    </div>`;
  }).join('');

  // Hardest words
  renderWordTable('hardest-body', ws.hardest, ['accuracy', 'attempts']);
  // Most practiced
  renderWordTable('most-practiced-body', ws.most_practiced, ['attempts', 'accuracy']);
}

function renderWordTable(tbodyId, words, cols) {
  const tbody = $(tbodyId);
  if (!words || words.length === 0) {
    tbody.innerHTML = `<tr><td colspan="4" class="py-4 text-center text-gray-400">${escHtml(t('stats.notEnoughData'))}</td></tr>`;
    return;
  }
  tbody.innerHTML = words.map(w => {
    const acc = Math.round(w.accuracy);
    const accColor = acc >= 80 ? 'text-green-600' : acc >= 50 ? 'text-yellow-600' : 'text-red-600';
    const zhLabel = escHtml(w.zh_text) + (w.pinyin ? ` <span class="text-gray-400">${escHtml(w.pinyin)}</span>` : '');
    const enLabel = Object.values(w.translations || {}).flat().map(t => escHtml(t)).join(', ');

    if (cols[0] === 'accuracy') {
      return `<tr class="border-b border-gray-100 hover:bg-gray-50">
        <td class="py-2 pr-4">${zhLabel}</td>
        <td class="py-2 pr-4 text-gray-600">${enLabel}</td>
        <td class="py-2 pr-4 text-right ${accColor} font-medium">${acc}%</td>
        <td class="py-2 text-right">${w.total_attempts}</td>
      </tr>`;
    }
    return `<tr class="border-b border-gray-100 hover:bg-gray-50">
      <td class="py-2 pr-4">${zhLabel}</td>
      <td class="py-2 pr-4 text-gray-600">${enLabel}</td>
      <td class="py-2 pr-4 text-right">${w.total_attempts}</td>
      <td class="py-2 text-right ${accColor} font-medium">${acc}%</td>
    </tr>`;
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
    pill.className = `px-2.5 py-0.5 rounded-full text-xs font-medium transition cursor-pointer ${active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}`;
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
    canvas.style.display = 'none';
    show('due-chart-empty');
    if (dueChart) { dueChart.destroy(); dueChart = null; }
    return;
  }
  canvas.style.display = '';
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
          backgroundColor: 'rgba(34, 197, 94, 0.7)',
          stack: 'answers',
        },
        {
          label: t('chart.mistakes'),
          data: days.map(d => d.mistakes),
          backgroundColor: 'rgba(239, 68, 68, 0.7)',
          stack: 'answers',
        },
        {
          label: 'Sounds seen',
          data: days.map(d => d.sounds_seen),
          type: 'line',
          borderColor: 'rgba(168, 85, 247, 0.9)',
          backgroundColor: 'rgba(168, 85, 247, 0.1)',
          fill: false,
          yAxisID: 'y1',
          tension: 0.3,
          pointRadius: 2,
        },
      ],
    },
    options: {
      responsive: true,
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: { ticks: { maxRotation: 45, autoSkip: true, maxTicksLimit: 20 } },
        y: { beginAtZero: true, title: { display: true, text: t('stats.answers') }, stacked: true },
        y1: {
          beginAtZero: true,
          position: 'right',
          title: { display: true, text: 'Sounds' },
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
              return `Accuracy: ${acc}%\nSounds seen: ${d.sounds_seen}`;
            },
          },
        },
      },
    },
  });
}

// Tone labels with superscript tone marks for display
const TONE_LABELS = ['Tone 1 (ā)', 'Tone 2 (á)', 'Tone 3 (ǎ)', 'Tone 4 (à)', 'Tone 5 (a·)'];
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
    $('pinyin-tone-chart').style.display = 'none';
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
      labels: TONE_LABELS,
      datasets: [
        {
          label: 'Correct',
          data: correct,
          backgroundColor: 'rgba(34, 197, 94, 0.7)',
          stack: 'tone',
        },
        {
          label: 'Wrong',
          data: wrong,
          backgroundColor: 'rgba(239, 68, 68, 0.7)',
          stack: 'tone',
        },
      ],
    },
    options: {
      responsive: true,
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: {},
        y: { beginAtZero: true, stacked: true, title: { display: true, text: 'Answers' }, ticks: { precision: 0 } },
      },
      plugins: {
        tooltip: {
          callbacks: {
            afterBody(items) {
              const idx = items[0].dataIndex;
              const total = correct[idx] + wrong[idx];
              const acc = total > 0 ? Math.round(correct[idx] / total * 100) : 0;
              return `Accuracy: ${acc}%  (${correct[idx]}/${total})`;
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
    tbody.innerHTML = `<tr><td colspan="5" class="py-8 text-center text-gray-400">${escHtml(t('stats.noDataLast14'))}</td></tr>`;
    return;
  }
  tbody.innerHTML = recent.map(d => {
    const correct = d.attempts - d.mistakes;
    const acc = d.attempts > 0 ? Math.round((correct / d.attempts) * 100) : 0;
    const accColor = acc >= 80 ? 'text-green-600' : acc >= 50 ? 'text-yellow-600' : 'text-red-600';
    return `<tr class="border-b border-gray-100 hover:bg-gray-50">
      <td class="py-2 pr-4 font-medium">${escHtml(formatDateLabel(d.date))}</td>
      <td class="py-2 pr-4 text-right">${d.attempts}</td>
      <td class="py-2 pr-4 text-right">${d.mistakes}</td>
      <td class="py-2 pr-4 text-right ${accColor} font-medium">${acc}%</td>
      <td class="py-2 text-right">${d.sounds_seen}</td>
    </tr>`;
  }).join('');
}

function renderDueDateChart(dates) {
  const today = new Date().toISOString().slice(0, 10);
  const labels = dates.map(d => d.date === today ? t('stats.today') : formatDateLabel(d.date));
  const colors = dates.map(d => {
    if (d.date <= today) return 'rgba(239, 68, 68, 0.7)';   // overdue/today = red
    return 'rgba(59, 130, 246, 0.7)';                        // future = blue
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
      scales: {
        x: { ticks: { maxRotation: 45, autoSkip: true, maxTicksLimit: 20 } },
        y: { beginAtZero: true, title: { display: true, text: 'Words' }, ticks: { precision: 0 } },
      },
      plugins: {
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
