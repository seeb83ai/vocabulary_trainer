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
      btn.disabled = latestStats.available_to_advance === 0;
    }
  });
  const label = document.getElementById('success-amount-label');
  if (label) label.textContent = checked ? t('success.difficultAmount') : t('success.learnMore');
}

// successAdvanceState computes the "learn more words" advance-button and
// introduce-new-word-button state shown on the success screen, from the
// latest /api/quiz/stats response. Shared by both success-screen render
// paths in train-card.js so they can't drift out of sync with each other.
function successAdvanceState(stats) {
  const allAdvanceDisabled = (stats?.available_to_advance || 0) === 0;
  const hasUnseen = (stats?.has_unseen || 0) > 0 || (stats?.new_available || 0) > 0;
  return { allAdvanceDisabled, showIntroduceNew: allAdvanceDisabled && hasUnseen };
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
    const letters = t('success.weekLetters').split(',');
    $('success-week').innerHTML = weekGrid(daily.days, localDateStr(0)).map(d =>
      `<div class="tr-week-day${d.trained ? ' is-trained' : ''}${d.today ? ' is-today' : ''}" title="${escHtml(d.date)}"><span class="tr-week-bar"></span><span class="tr-week-label">${escHtml(letters[d.weekday] || '')}</span></div>`).join('');
    const due = dueTomorrowCount(dist.dates, localDateStr(1));
    setText('success-streak', String(streak));
    // The day streak and its week grid are gamification elements; the
    // "words moved up" line in the same card stays.
    $('success-streak-title').classList.toggle('hidden', !_gamificationEnabled);
    $('success-week').classList.toggle('hidden', !_gamificationEnabled);
    setText('success-due-tomorrow', String(due));
    setText('success-comeback-msg', t(due > 0 ? 'success.comebackDue' : 'success.comebackNoDue'));
    if (wordsImproved > 0) {
      setText('success-improved-count', String(wordsImproved));
      show('success-improved');
    } else {
      hide('success-improved');
    }
    $('success-streak-card').classList.toggle('hidden', !_gamificationEnabled && !(wordsImproved > 0));
    show('success-comeback');
  } catch (e) {
    hide('success-comeback');
  }
}

// weekGrid returns the 7 days ending at `today` (YYYY-MM-DD) for the all-done
// streak card. weekday is 0 = Monday … 6 = Sunday.
function weekGrid(days, today) {
  const trained = new Set((days || []).filter(d => d.attempts > 0).map(d => d.date));
  const out = [];
  const cur = new Date(today + 'T00:00:00Z');
  cur.setUTCDate(cur.getUTCDate() - 6);
  for (let i = 0; i < 7; i++) {
    const date = cur.toISOString().slice(0, 10);
    out.push({ date, weekday: (cur.getUTCDay() + 6) % 7, trained: trained.has(date), today: date === today });
    cur.setUTCDate(cur.getUTCDate() + 1);
  }
  return out;
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

// Returns the {pct, min} i18n params when the accuracy baseline pauses new
// words (issue #484), else null.
function accuracyPauseParams(stats) {
  if (stats.accuracy_pause_pct === undefined || stats.accuracy_pause_min === undefined) return null;
  return { pct: stats.accuracy_pause_pct, min: stats.accuracy_pause_min };
}

// renderSessionProgress fills the session bar's "X of Y today" label and bar.
function renderSessionProgress(doneToday, dueLeft) {
  const p = sessionProgress(doneToday, dueLeft);
  setText('session-progress-label', t('session.progress', { done: p.done, total: p.total }));
  const bar = document.getElementById('session-progress-bar');
  if (bar) bar.style.width = p.pct + '%';
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
    const dueLeft = dueDisplayCount(stats, false);
    setText('stats-due', dueLeft);
    setText('stats-total', stats.total);
    setText('stats-new', t('session.newOf', { n: stats.new_today, max: stats.max_new_per_day }));
    renderSessionProgress(stats.today_attempts, dueLeft);
    const pause = accuracyPauseParams(stats);
    const pausedEl = document.getElementById('stats-new-paused');
    if (pausedEl) {
      pausedEl.textContent = pause ? t('statsBar.newPaused', pause) : '';
      pausedEl.classList.toggle('hidden', !pause);
    }
    renderDifficultDrill();
  } catch (_) {}
}
