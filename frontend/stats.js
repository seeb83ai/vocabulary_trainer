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
    return;
  }

  renderChart(days);
  renderTable(days);
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

function formatDateLabel(dateStr) {
  // "2026-03-04" -> "Mar 4"
  const parts = dateStr.split('-');
  const months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
  return months[parseInt(parts[1], 10) - 1] + ' ' + parseInt(parts[2], 10);
}
