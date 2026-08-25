import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ── shouldAutoPlay ──────────────────────────────────────────────────────────────
// Mirrors the pure eligibility check in train.js that decides whether a newly
// shown card should trigger auto-play audio (when the user has the auto-play
// toggle enabled). Must never fire for transl_to_zh (would reveal the answer)
// or hmm cards (no audio exists).

function shouldAutoPlay(currentCard) {
  if (!currentCard) return false;
  if (currentCard.mode === 'new_word') return true;
  if (currentCard.card_type === 'component') return true;
  if (currentCard.card_type === 'hmm') return false;
  return currentCard.mode === 'zh_to_transl' || currentCard.mode === 'zh_pinyin_to_transl' || currentCard.mode === 'voice_to_transl';
}

describe('shouldAutoPlay', () => {
  it('returns true for zh_to_transl mode', () => {
    expect(shouldAutoPlay({ mode: 'zh_to_transl' })).toBe(true);
  });

  it('returns true for zh_pinyin_to_transl mode', () => {
    expect(shouldAutoPlay({ mode: 'zh_pinyin_to_transl' })).toBe(true);
  });

  it('returns false for transl_to_zh mode (would reveal the answer)', () => {
    expect(shouldAutoPlay({ mode: 'transl_to_zh' })).toBe(false);
  });

  it('returns true for the new-word introduction screen', () => {
    expect(shouldAutoPlay({ mode: 'new_word' })).toBe(true);
  });

  it('returns true for a component card (including new-component introduction)', () => {
    expect(shouldAutoPlay({ card_type: 'component', is_new: true })).toBe(true);
    expect(shouldAutoPlay({ card_type: 'component', is_new: false })).toBe(true);
  });

  it('returns false for hmm cards (no audio exists)', () => {
    expect(shouldAutoPlay({ card_type: 'hmm' })).toBe(false);
  });

  it('returns false when there is no current card', () => {
    expect(shouldAutoPlay(null)).toBe(false);
    expect(shouldAutoPlay(undefined)).toBe(false);
  });

  it('returns false for the Chinese (no sound) mode', () => {
    expect(shouldAutoPlay({ mode: 'zh_to_transl_no_sound' })).toBe(false);
  });

  it('returns true for voice_to_transl mode', () => {
    expect(shouldAutoPlay({ mode: 'voice_to_transl' })).toBe(true);
  });
});

// ── shouldAutoPlayResult ────────────────────────────────────────────────────────
// Mirrors the pure eligibility check in train.js that decides whether the
// result screen should read out the Chinese answer. Fires whenever auto-play
// is on and the answer wasn't already read out on the question screen (either
// because the mode never plays audio there, e.g. transl_to_zh or
// zh_to_transl_no_sound, or because the question-screen play was skipped for
// some other reason, e.g. the blur guard in autoPlayCard) — except for hmm
// cards which have no audio at all (issue #272).

function shouldAutoPlayResult(currentCard, autoPlayEnabled, alreadyPlayed) {
  if (!autoPlayEnabled || !currentCard) return false;
  if (currentCard.card_type === 'hmm') return false;
  return !alreadyPlayed;
}

describe('shouldAutoPlayResult', () => {
  it('returns true for transl_to_zh when auto-play is enabled and not already played', () => {
    expect(shouldAutoPlayResult({ mode: 'transl_to_zh' }, true, false)).toBe(true);
  });

  it('returns false for transl_to_zh when auto-play is disabled', () => {
    expect(shouldAutoPlayResult({ mode: 'transl_to_zh' }, false, false)).toBe(false);
  });

  it('returns true for other modes when not already played on the question screen', () => {
    expect(shouldAutoPlayResult({ mode: 'zh_to_transl' }, true, false)).toBe(true);
    expect(shouldAutoPlayResult({ mode: 'zh_pinyin_to_transl' }, true, false)).toBe(true);
  });

  it('returns false when already played on the question screen', () => {
    expect(shouldAutoPlayResult({ mode: 'zh_to_transl' }, true, true)).toBe(false);
  });

  it('returns true for zh_to_transl_no_sound on result screen (question screen was silent, result reveals the answer)', () => {
    expect(shouldAutoPlayResult({ mode: 'zh_to_transl_no_sound' }, true, false)).toBe(true);
  });

  it('returns false for hmm cards (no audio exists)', () => {
    expect(shouldAutoPlayResult({ card_type: 'hmm' }, true, false)).toBe(false);
  });

  it('returns true for component cards when not already played', () => {
    expect(shouldAutoPlayResult({ card_type: 'component', mode: undefined }, true, false)).toBe(true);
  });

  it('returns false for new_word when already marked as played (no result screen is shown for it in practice)', () => {
    expect(shouldAutoPlayResult({ mode: 'new_word' }, true, true)).toBe(false);
  });

  it('returns false when there is no current card', () => {
    expect(shouldAutoPlayResult(null, true, false)).toBe(false);
    expect(shouldAutoPlayResult(undefined, true, false)).toBe(false);
  });
});

// ── isZhPromptWithSound ────────────────────────────────────────────────────────
// Mirrors the pure play-button-visibility check in train.js's showCard(): the
// button is only shown for the two modes where the Chinese prompt has audio.
// zh_to_transl_no_sound deliberately behaves like a Chinese prompt for every
// other purpose but is excluded here on purpose, not by accidental omission.

function isZhPromptWithSound(mode) {
  return mode === 'zh_to_transl' || mode === 'zh_pinyin_to_transl' || mode === 'voice_to_transl';
}

describe('isZhPromptWithSound', () => {
  it('returns true for zh_to_transl', () => {
    expect(isZhPromptWithSound('zh_to_transl')).toBe(true);
  });

  it('returns true for zh_pinyin_to_transl', () => {
    expect(isZhPromptWithSound('zh_pinyin_to_transl')).toBe(true);
  });

  it('returns true for voice_to_transl', () => {
    expect(isZhPromptWithSound('voice_to_transl')).toBe(true);
  });

  it('returns false for zh_to_transl_no_sound', () => {
    expect(isZhPromptWithSound('zh_to_transl_no_sound')).toBe(false);
  });

  it('returns false for transl_to_zh', () => {
    expect(isZhPromptWithSound('transl_to_zh')).toBe(false);
  });

  it('returns false for an unknown mode', () => {
    expect(isZhPromptWithSound('cycle')).toBe(false);
  });
});

// ── isAutoPlayBlockedByBlur ───────────────────────────────────────────────────
// Mirrors the noAutoVoiceOnBlur guard inside autoPlayCard(). Intro screens
// (new_word, new-component) never blur their pinyin display (see train.js's
// setText('new-word-pinyin', ...) / setText('new-component-pinyin', ...),
// which never route through applyPinyinBlur()), so the guard must not apply
// to them — regression for issue #273 ("Sound missing" on the new-word screen).

function isVoiceOnlyMode(mode) {
  return mode === 'voice_to_transl';
}

function isAutoPlayBlockedByBlur(currentCard, noAutoVoiceOnBlur, blurPinyin) {
  const isIntroScreen = currentCard.mode === 'new_word' ||
    (currentCard.card_type === 'component' && currentCard.is_new);
  if (isIntroScreen || isVoiceOnlyMode(currentCard.mode)) return false;
  return noAutoVoiceOnBlur && (blurPinyin || !currentCard.pinyin);
}

describe('isAutoPlayBlockedByBlur', () => {
  it('does not block a new_word intro with blur and no-auto-voice-on-blur both on', () => {
    expect(isAutoPlayBlockedByBlur({ mode: 'new_word', pinyin: 'bīngxiāng' }, true, true)).toBe(false);
  });

  it('does not block a new_word intro with no pinyin at all', () => {
    expect(isAutoPlayBlockedByBlur({ mode: 'new_word', pinyin: undefined }, true, false)).toBe(false);
  });

  it('does not block a new-component intro with blur and no-auto-voice-on-blur both on', () => {
    expect(isAutoPlayBlockedByBlur({ card_type: 'component', is_new: true, pinyin: 'bīng' }, true, true)).toBe(false);
  });

  it('still blocks a regular (already-reviewed) component with blur and no-auto-voice-on-blur both on', () => {
    expect(isAutoPlayBlockedByBlur({ card_type: 'component', is_new: false, pinyin: 'bīng' }, true, true)).toBe(true);
  });

  it('still blocks a regular zh_to_transl card with blur and no-auto-voice-on-blur both on', () => {
    expect(isAutoPlayBlockedByBlur({ mode: 'zh_to_transl', pinyin: 'nǐhǎo' }, true, true)).toBe(true);
  });

  it('does not block a regular zh_to_transl card when no-auto-voice-on-blur is off', () => {
    expect(isAutoPlayBlockedByBlur({ mode: 'zh_to_transl', pinyin: 'nǐhǎo' }, false, true)).toBe(false);
  });

  it('does not block voice_to_transl (already exempt as voice-only mode)', () => {
    expect(isAutoPlayBlockedByBlur({ mode: 'voice_to_transl', pinyin: 'nǐhǎo' }, true, true)).toBe(false);
  });
});
