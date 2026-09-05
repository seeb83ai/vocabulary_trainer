/**
 * Sign-in/create-account moved from an always-visible inline card on the
 * landing page to a modal opened from CTA buttons (any element with
 * [data-show-tab]). Tests that used to interact with #signin-form /
 * #tab-register directly after page.goto('/') now need to open the modal
 * first.
 * @param {import('@playwright/test').Page} page
 * @param {'signin'|'register'} [tab]
 */
export async function openAuthModal(page, tab = 'signin') {
  await page.locator(tab === 'register' ? '#btn-signup' : '#btn-login').click();
}
