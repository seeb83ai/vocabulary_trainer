// train-stats.js — streak / due counts / difficult drill

// renderDifficultDrill shows/hides the temporary filter-bar pill and updates its
// remaining-count label from the latest stats.
function renderDifficultDrill() {
  const bar = document.getElementById('difficult-drill-bar');
  if (!bar) return;
  if (difficultDrill) {
    bar.classList.remove('hidden');
    const n = latestStats && typeof latestStats.difficult_remaining === 'number'
      ? latestStats.difficult_remaining : null;
    setText('difficult-drill-count', n != null ? `(${n})` : '');
  } else {
    bar.classList.add('hidden');
  }
}

// exitDifficultDrill ends the drill; clearServer also drops any remaining flags.
async function exitDifficultDrill(clearServer) {
  difficultDrill = false;
  localStorage.removeItem('quizDifficultDrill');
  renderDifficultDrill();
  if (clearServer) {
    try { await apiFetch('/api/quiz/difficult/clear', { method: 'POST' }); } catch (_) {}
  }
}

// updateAdvanceButtonsForDifficult re-enables the amount buttons when the
// "drill my hardest words" checkbox is ticked (they then flag that many difficult
// words rather than advancing due dates) and swaps the amount label.
function updateAdvanceButtonsForDifficult() {
  const checked = !!(document.getElementById('difficult-words-checkbox') || {}).checked;
  document.querySelectorAll('.advance-btn').forEach(btn => {
    if (checked) {
      btn.disabled = false;
    } else if (latestStats) {
      btn.disabled = latestStats.available_to_advance < parseInt(btn.dataset.advance);
    }
  });
  const label = document.getElementById('success-amount-label');
  if (label) label.textContent = checked ? t('success.difficultAmount') : t('success.learnMore');
}

// computeDayStreak returns the number of consecutive training days
// (attempts > 0) ending at `today` (YYYY-MM-DD). `days` may be unordered.
function computeDayStreak(days, today) {
  const trained = new Set((days || []).filter(d => d.attempts > 0).map(d => d.date));
  let streak = 0;
  const cur = new Date(today + 'T00:00:00Z');
  while (trained.has(cur.toISOString().slice(0, 10))) {
    streak++;
    cur.setUTCDate(cur.getUTCDate() - 1);
  }
  return streak;
}

// dueTomorrowCount extracts the review count scheduled for `tomorrow`
// (YYYY-MM-DD) from a due-date distribution.
function dueTomorrowCount(dates, tomorrow) {
  const hit = (dates || []).find(d => d.date === tomorrow);
  return hit ? hit.count : 0;
}

// localDateStr formats today + offsetDays in the browser's local timezone,
// matching the server-local dates used by daily stats and due scheduling.
function localDateStr(offsetDays) {
  const d = new Date();
  d.setDate(d.getDate() + (offsetDays || 0));
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

// loadComebackInfo fills the "come back tomorrow" block on the success
// screen: current day streak, how many reviews come due tomorrow, and (when
// wordsImproved is a positive number) how many words moved up a proficiency
// bucket today.
async function loadComebackInfo(wordsImproved) {
  if (!$('success-comeback')) return;
  try {
    const params = selectedTags.length ? '?tags=' + encodeURIComponent(selectedTags.join(',')) : '';
    const [daily, dist] = await Promise.all([
      apiFetch('/api/quiz/daily-stats'),
      apiFetch('/api/quiz/due-date-distribution' + params),
    ]);
    const streak = computeDayStreak(daily.days, localDateStr(0));
    const due = dueTomorrowCount(dist.dates, localDateStr(1));
    setText('success-streak', String(streak));
    setText('success-due-tomorrow', String(due));
    setText('success-comeback-msg', t(due > 0 ? 'success.comebackDue' : 'success.comebackNoDue'));
    if (wordsImproved > 0) {
      setText('success-improved-count', String(wordsImproved));
      show('success-improved');
    } else {
      hide('success-improved');
    }
    show('success-comeback');
  } catch (e) {
    hide('success-comeback');
  }
}

// dueDisplayCount computes the "remaining today" number shown to the user.
// GetNextCard may serve a not-yet-due (session_extension) card to avoid
// immediately repeating a just-answered word (see #186); that word isn't
// counted in stats.due_today, so it must be added back in here to keep the
// displayed count in sync with what the user will actually be asked.
function dueDisplayCount(stats, sessionExtension, newWordIntro = false) {
  return stats.due_today + (stats.hmm_due_today || 0) + (stats.components_due_today || 0)
    + (sessionExtension ? 1 : 0) + (newWordIntro ? 1 : 0);
}

async function loadStats() {
  try {
    const params = new URLSearchParams();
    if (selectedTags.length) params.set('tags', selectedTags.join(','));
    if (selectedBucket) params.set('bucket', selectedBucket);
    if (!includeMnemonics) params.set('mnemonics', 'false');
    if (includeComponents) params.set('trainComponents', '1');
    const qs = params.toString();
    const statsUrl = qs ? `/api/quiz/stats?${qs}` : '/api/quiz/stats';
    const stats = await apiFetch(statsUrl);
    latestStats = stats;
    setText('stats-due', dueDisplayCount(stats, false));
    setText('stats-total', stats.total);
    setText('stats-new', `${stats.new_today} / ${stats.max_new_per_day}`);
    renderDifficultDrill();
  } catch (_) {}
}
