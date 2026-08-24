/**
 * PR review screenshots — reuses the e2e tests already written for a
 * user-visible change instead of a separate capture script.
 *
 * Calls are no-ops unless PR_SCREENSHOTS=1 is set, so normal `make test-e2e`
 * / CI runs are unaffected. Enable with:
 *   PR_SCREENSHOTS=1 make screenshots-pr FILE=e2e/<spec>.spec.js
 */
import { mkdirSync } from 'node:fs';

const DIR = 'pr-screenshots';

/**
 * @param {import('@playwright/test').Page} page
 * @param {string} name - used as the PNG filename (without extension)
 */
export async function captureForPR(page, name) {
  if (process.env.PR_SCREENSHOTS !== '1') return;
  mkdirSync(DIR, { recursive: true });
  await page.screenshot({ path: `${DIR}/${name}.png` });
}
