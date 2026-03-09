document.addEventListener('DOMContentLoaded', async () => {
  let data;
  try {
    data = await apiFetch('/api/quiz/daily-stats');
  } catch (e) {
    $('stats-table-body').innerHTML =
      '<tr><td colspan="8" class="py-8 text-center text-red-500">Failed to load stats.</td></tr>';
    return;
  }

  const days = data.days || [];

  if (days.length === 0) {
    $('stats-chart').style.display = 'none';
    show('chart-empty');
    $('stats-table-body').innerHTML =
      '<tr><td colspan="8" class="py-8 text-center text-gray-400">No training data yet.</td></tr>';
  } else {
    renderChart(days);
    renderTable(days);
  }

  // Load word-level statistics
  try {
    const ws = await apiFetch('/api/quiz/word-stats');
    if (ws && ws.total_seen > 0) {
      renderWordStats(ws);
      show('word-stats-section');
    }
  } catch (_) {}
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
          label: 'Correct',
          data: days.map(d => d.attempts - d.mistakes),
          backgroundColor: 'rgba(34, 197, 94, 0.7)',
          stack: 'answers',
        },
        {
          label: 'Mistakes',
          data: days.map(d => d.mistakes),
          backgroundColor: 'rgba(239, 68, 68, 0.7)',
          stack: 'answers',
        },
        {
          label: 'Words Seen',
          data: days.map(d => d.words_seen),
          type: 'line',
          borderColor: 'rgba(168, 85, 247, 0.9)',
          backgroundColor: 'rgba(168, 85, 247, 0.1)',
          fill: false,
          yAxisID: 'y1',
          tension: 0.3,
          pointRadius: 2,
        },
        {
          label: 'Words Known',
          data: days.map(d => d.words_known),
          type: 'line',
          borderColor: 'rgba(59, 130, 246, 0.9)',
          backgroundColor: 'rgba(59, 130, 246, 0.1)',
          fill: true,
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
        y: { beginAtZero: true, title: { display: true, text: 'Answers' }, stacked: true },
        y1: {
          beginAtZero: true,
          position: 'right',
          title: { display: true, text: 'Words' },
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
              return `Accuracy: ${acc}%\nNew words: ${d.new_words}\nWords seen: ${d.words_seen}\nBest streak: ${d.correct_streak}`;
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
    tbody.innerHTML = '<tr><td colspan="8" class="py-8 text-center text-gray-400">No data in the last 14 days.</td></tr>';
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
      <td class="py-2 pr-4 text-right">${d.words_known}</td>
      <td class="py-2 pr-4 text-right">${d.new_words}</td>
      <td class="py-2 text-right">${d.correct_streak}</td>
    </tr>`;
  }).join('');
}

function renderWordStats(ws) {
  // Milestones bar chart
  const mCtx = $('milestones-chart').getContext('2d');
  const mLabels = ['1+', '3+', '5+', '10+'];
  const mData = mLabels.map(k => ws.milestones[k] || 0);
  new Chart(mCtx, {
    type: 'bar',
    data: {
      labels: mLabels.map(l => l + ' correct'),
      datasets: [{
        data: mData,
        backgroundColor: ['rgba(59,130,246,0.7)', 'rgba(34,197,94,0.7)', 'rgba(168,85,247,0.7)', 'rgba(245,158,11,0.7)'],
      }],
    },
    options: {
      responsive: true,
      plugins: { legend: { display: false } },
      scales: { y: { beginAtZero: true, ticks: { precision: 0 } } },
    },
  });

  // Accuracy distribution doughnut
  const aCtx = $('accuracy-chart').getContext('2d');
  const aData = TIERS.map(t => ws.accuracy_buckets[t.key] || 0);
  new Chart(aCtx, {
    type: 'doughnut',
    data: {
      labels: TIERS.map(t => t.label),
      datasets: [{
        data: aData,
        backgroundColor: TIERS.map(t => t.color + 'b3'), // 70% opacity
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
              return `${ctx.label}: ${ctx.raw} words (${pct}%)`;
            },
          },
        },
      },
    },
  });

  // Tier legend below chart
  const legend = $('tier-legend');
  const total = aData.reduce((a, b) => a + b, 0);
  legend.innerHTML = TIERS.map((t, i) => {
    const count = aData[i];
    const pct = total > 0 ? Math.round(count / total * 100) : 0;
    return `<div class="flex items-center justify-between py-1 border-b border-gray-50 last:border-0">
      <div class="flex items-center gap-2">
        <span class="inline-block w-2.5 h-2.5 rounded-full flex-shrink-0" style="background:${t.color}"></span>
        <span class="font-medium text-gray-700">${t.label}</span>
        <span class="text-gray-400 text-xs">${t.desc}</span>
      </div>
      <span class="text-gray-600 tabular-nums">${count} <span class="text-gray-400">(${pct}%)</span></span>
    </div>`;
  }).join('');

  // Aggregates table
  $('word-stats-total').textContent = `(${ws.total_seen} words seen)`;
  const agg = ws.aggregates;
  const rows = [
    { label: 'Correct answers', d: agg.correct, unit: '' },
    { label: 'Total attempts', d: agg.attempts, unit: '' },
    { label: 'Accuracy', d: agg.accuracy, unit: '%' },
    { label: 'Ease factor', d: agg.easiness, unit: '' },
  ];
  $('aggregates-body').innerHTML = rows.map(r =>
    `<tr class="border-b border-gray-100">
      <td class="py-2 pr-4 font-medium">${escHtml(r.label)}</td>
      <td class="py-2 pr-4 text-right">${r.d.avg}${r.unit}</td>
      <td class="py-2 pr-4 text-right">${r.d.median}${r.unit}</td>
      <td class="py-2 text-right">${r.d.p95}${r.unit}</td>
    </tr>`
  ).join('');

  // Hardest words
  renderWordTable('hardest-body', ws.hardest, ['accuracy', 'attempts']);
  // Most practiced
  renderWordTable('most-practiced-body', ws.most_practiced, ['attempts', 'accuracy']);
}

function renderWordTable(tbodyId, words, cols) {
  const tbody = $(tbodyId);
  if (!words || words.length === 0) {
    tbody.innerHTML = '<tr><td colspan="4" class="py-4 text-center text-gray-400">Not enough data yet.</td></tr>';
    return;
  }
  tbody.innerHTML = words.map(w => {
    const acc = Math.round(w.accuracy);
    const accColor = acc >= 80 ? 'text-green-600' : acc >= 50 ? 'text-yellow-600' : 'text-red-600';
    const zhLabel = escHtml(w.zh_text) + (w.pinyin ? ` <span class="text-gray-400">${escHtml(w.pinyin)}</span>` : '');
    const enLabel = (w.en_texts || []).map(t => escHtml(t)).join(', ');

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
