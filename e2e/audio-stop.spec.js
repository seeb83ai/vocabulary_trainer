// @ts-check
import { test, expect } from '@playwright/test';
import { syncNewWordMode } from './helpers/mode.js';

// Issue #620: a slow audio request must not play on the next screen, and a
// second tap on play while the clip still loads must not play it twice.
//
// The audio route is held until the test releases it, so "slow" is
// deterministic. The empty MP3 body makes the browser reject play(), which
// sends playAudio to its Web Speech fallback — so every sound, real or
// fallback, is recorded in window.__sounds.

test.describe('Audio – stop on screen change, no double play (issue #620)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  async function useMode(page, mode) {
    await page.request.patch('/api/training-filters', {
      data: { mode, langs: ['en'], bucket: '', mnemonics: false, components: false, tags: [] },
    });
    await syncNewWordMode(page, mode);
    await page.addInitScript((m) => {
      localStorage.setItem('quizMode', m);
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    }, mode);
  }

  async function recordSounds(page) {
    await page.addInitScript(() => {
      window.__sounds = [];
      if ('speechSynthesis' in window) {
        speechSynthesis.speak = (u) => window.__sounds.push({ kind: 'speak', text: u.text });
      }
      const play = HTMLMediaElement.prototype.play;
      HTMLMediaElement.prototype.play = function () {
        this.addEventListener('playing', () => window.__sounds.push({ kind: 'play', src: this.src }), { once: true });
        return play.call(this);
      };
    });
  }

  // holdAudio holds every audio request until release(); requests that come
  // after release() are answered at once.
  async function holdAudio(page) {
    const held = [];
    let released = false;
    const fulfill = (route) =>
      route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) }).catch(() => {});
    await page.route('**/api/audio/**', (route) => { released ? fulfill(route) : held.push(route); });
    const release = async () => {
      released = true;
      for (const route of held.splice(0)) await fulfill(route);
    };
    return { held, release };
  }

  test('result-screen audio that is still loading does not play on the next card', async ({ page }) => {
    await recordSounds(page);
    const audio = await holdAudio(page);
    await useMode(page, 'transl_to_zh');
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#autoplay-toggle-btn').click();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'true');

    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
    // The result screen asked for the answer's audio; it is still loading.
    await expect.poll(() => audio.held.length, { timeout: 5_000 }).toBeGreaterThan(0);

    await page.locator('#next-btn').click();
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });

    // The old clip finishes loading only now, on the new card.
    await audio.release();
    await page.waitForTimeout(500);
    expect(await page.evaluate(() => window.__sounds)).toEqual([]);
  });

  test('a second tap on play while the clip loads plays it only once', async ({ page }) => {
    await recordSounds(page);
    const audio = await holdAudio(page);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#play-btn')).toBeVisible();

    await page.locator('#play-btn').click();
    await page.locator('#play-btn').click();
    await expect.poll(() => audio.held.length, { timeout: 5_000 }).toBeGreaterThan(0);

    await audio.release();
    await expect.poll(() => page.evaluate(() => window.__sounds.length), { timeout: 5_000 }).toBeGreaterThan(0);
    await page.waitForTimeout(1_000);
    expect(await page.evaluate(() => window.__sounds.length)).toBe(1);
  });
});
