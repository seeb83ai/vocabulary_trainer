// Pure helper: turns the overview payload into the ordered list of stat
// tiles rendered at the top of the page. Extracted so it can be unit tested
// without a DOM.
function buildStatTiles(ov) {
  return [
    { label: 'Total users', value: String(ov.users.total) },
    { label: 'Verified / Unverified', value: `${ov.users.verified} / ${ov.users.unverified}` },
    { label: 'Admin / Plus / Free', value: `${ov.users.admins} / ${ov.users.plus} / ${ov.users.free}` },
    { label: 'Active (7d)', value: String(ov.activity.active_last_7_days) },
    { label: 'Active (30d)', value: String(ov.activity.active_last_30_days) },
    { label: 'Dormant', value: String(ov.activity.dormant) },
  ];
}

function formatDateLabel(dateStr) {
  // "2026-03-04" -> "Mar 4"
  const parts = dateStr.split('-');
  const months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
  return months[parseInt(parts[1], 10) - 1] + ' ' + parseInt(parts[2], 10);
}

function renderStatTiles(ov) {
  const tiles = buildStatTiles(ov);
  $('stat-tiles').innerHTML = tiles.map(t => `
    <div class="bg-white rounded-2xl shadow p-4">
      <p class="text-xs text-gray-500 uppercase tracking-wide">${escHtml(t.label)}</p>
      <p class="text-xl font-bold text-gray-800 mt-1">${escHtml(t.value)}</p>
    </div>
  `).join('');
}

function renderDailyCountChart(canvasId, emptyId, days, label, color) {
  if (days.length === 0) {
    $(canvasId).style.display = 'none';
    show(emptyId);
    return;
  }
  const ctx = $(canvasId).getContext('2d');
  new Chart(ctx, {
    type: 'bar',
    data: {
      labels: days.map(d => formatDateLabel(d.date)),
      datasets: [{ label, data: days.map(d => d.count), backgroundColor: color }],
    },
    options: {
      responsive: true,
      scales: {
        x: { ticks: { maxRotation: 45, autoSkip: true, maxTicksLimit: 20 } },
        y: { beginAtZero: true },
      },
    },
  });
}

function renderQuizVolumeChart(days) {
  if (days.length === 0) {
    $('quiz-volume-chart').style.display = 'none';
    show('quiz-volume-empty');
    return;
  }
  const ctx = $('quiz-volume-chart').getContext('2d');
  new Chart(ctx, {
    type: 'bar',
    data: {
      labels: days.map(d => formatDateLabel(d.date)),
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
      ],
    },
    options: {
      responsive: true,
      scales: {
        x: { ticks: { maxRotation: 45, autoSkip: true, maxTicksLimit: 20 } },
        y: { beginAtZero: true, stacked: true },
      },
    },
  });
}

function renderUsageTable(tbodyId, rows) {
  if (rows.length === 0) {
    $(tbodyId).innerHTML = `<tr><td colspan="4" class="py-8 text-center text-gray-400">No data yet.</td></tr>`;
    return;
  }
  $(tbodyId).innerHTML = rows.map(r => `
    <tr class="border-t border-gray-100">
      <td class="py-2 px-4 font-medium">${escHtml(r.name)}</td>
      <td class="py-2 px-4">${r.total_count}</td>
      <td class="py-2 px-4">${r.unique_users}</td>
      <td class="py-2 px-4 text-gray-500">${escHtml(r.last_seen)}</td>
    </tr>
  `).join('');
}

document.addEventListener('DOMContentLoaded', async () => {
  let ov;
  try {
    ov = await apiFetch('/api/admin/overview');
  } catch (err) {
    $('load-error').textContent = `Failed to load admin overview: ${err.message}`;
    show('load-error');
    return;
  }
  if (!ov) return;

  renderStatTiles(ov);
  renderDailyCountChart('signups-chart', 'signups-empty', ov.signups, 'Signups', 'rgba(59, 130, 246, 0.7)');
  renderQuizVolumeChart(ov.quiz_volume);
  renderDailyCountChart('guest-activity-chart', 'guest-activity-empty', ov.guest_activity, 'Failed logins', 'rgba(239, 68, 68, 0.7)');

  setText('deepl-total-calls', String(ov.deepl_usage.total_calls));
  setText('deepl-unique-users', String(ov.deepl_usage.unique_users));
  setText('llm-total-calls', String(ov.llm_usage.total_calls));
  setText('llm-unique-users', String(ov.llm_usage.unique_users));

  renderUsageTable('page-views-tbody', ov.page_views);
  renderUsageTable('feature-usage-tbody', ov.feature_usage);
});
