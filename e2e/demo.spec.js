// @ts-check
import { test, expect } from '@playwright/test';

test.describe('Demo quiz (no account)', () => {
  test('visitor can answer demo cards and sees flexible matching feedback', async ({ page }) => {
    await page.goto('/');

    await expect(page.locator('#demo-card')).toBeVisible();
    await expect(page.locator('#demo-zh')).toHaveText('你好');
    await expect(page.locator('#demo-progress')).toContainText('1');

    // Correct answer
    await page.locator('#demo-input').fill('hello');
    await page.locator('#demo-submit').click();
    await expect(page.locator('#demo-feedback')).toContainText(/correct/i);

    // Advance, then answer wrong: feedback must reveal the accepted answers
    await page.locator('#demo-next').click();
    await expect(page.locator('#demo-zh')).toHaveText('谢谢');
    await page.locator('#demo-input').fill('zzz');
    await page.locator('#demo-submit').click();
    await expect(page.locator('#demo-feedback')).toContainText('thank you');
  });

  test('finishing the demo leads to the create-account prompt', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('#demo-card')).toBeVisible();

    const answers = ['hello', 'thank you', 'cat', 'water', 'China'];
    for (const answer of answers) {
      await page.locator('#demo-input').fill(answer);
      await page.locator('#demo-submit').click();
      await expect(page.locator('#demo-feedback')).toBeVisible();
      await page.locator('#demo-next').click();
    }

    await expect(page.locator('#demo-done')).toBeVisible();
    await page.locator('#demo-done-cta').click();
    await expect(page.locator('#panel-register')).toBeVisible();
  });
});
