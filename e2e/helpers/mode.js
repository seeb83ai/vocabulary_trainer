/**
 * Force the new-word introduction ladder (new_word_mode_0/1/2) to the given
 * mode, so a word still in the intro phase (learning_new_word=true) is served
 * in this exact mode too — matching whatever overall training mode the test
 * configures via /api/training-filters or localStorage.
 *
 * Needed because mode selection during the intro phase always follows
 * new_word_mode_0/1/2, regardless of the selected training mode (issue #435).
 * The main seeded words (see global-setup.js) stay in the intro phase for a
 * long time across a spec's tests: its short, minute-scale retry intervals
 * keep them due again quickly, unlike a real graduated word's day-scale SM-2
 * intervals — so tests must not force real graduation to get a specific mode
 * shown, or they'll run out of due words partway through a spec file.
 * @param {import('@playwright/test').Page} page
 * @param {string} mode
 */
export async function syncNewWordMode(page, mode) {
  const res = await page.request.get('/api/settings');
  const settings = await res.json();
  await page.request.patch('/api/settings', {
    data: { ...settings, new_word_mode_0: mode, new_word_mode_1: mode, new_word_mode_2: mode },
  });
}
