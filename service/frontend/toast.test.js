import { describe, it, expect } from 'vitest';

// ── computeToastText ──────────────────────────────────────────────────────────
// computeToastText is defined as a regular function in toast.js, which we
// inline here to keep tests self-contained and independent of module
// bundling (per project convention — see app.test.js).
//
// It decides what the single hovering toast should display next: a repeat of
// the message currently showing gets a "(×N)" counter appended so back-to-back
// identical saves are still felt individually, while a genuinely different
// message simply replaces it and the counter restarts at 1.

function computeToastText(current, incomingMessage) {
  if (current && current.baseText === incomingMessage) {
    const count = current.count + 1;
    return { baseText: incomingMessage, count, text: `${incomingMessage} (×${count})` };
  }
  return { baseText: incomingMessage, count: 1, text: incomingMessage };
}

describe('computeToastText', () => {
  it('shows the plain message when nothing was previously showing', () => {
    expect(computeToastText(null, 'Saved.')).toEqual({ baseText: 'Saved.', count: 1, text: 'Saved.' });
  });

  it('appends a ×2 counter when the same message repeats while still visible', () => {
    const first = computeToastText(null, 'Saved.');
    const second = computeToastText(first, 'Saved.');
    expect(second).toEqual({ baseText: 'Saved.', count: 2, text: 'Saved. (×2)' });
  });

  it('keeps incrementing the counter for further repeats', () => {
    let state = computeToastText(null, 'Saved.');
    state = computeToastText(state, 'Saved.');
    state = computeToastText(state, 'Saved.');
    expect(state).toEqual({ baseText: 'Saved.', count: 3, text: 'Saved. (×3)' });
  });

  it('replaces the toast and resets the counter when the message differs', () => {
    const first = computeToastText(null, 'Saved.');
    const second = computeToastText(first, 'Network error.');
    expect(second).toEqual({ baseText: 'Network error.', count: 1, text: 'Network error.' });
  });

  it('does not collapse when nothing is currently visible, even for a repeated message', () => {
    // Once a toast has fully dismissed, `current` resets to null, so the
    // next call — even with the same text — starts a fresh count.
    expect(computeToastText(null, 'Saved.')).toEqual({ baseText: 'Saved.', count: 1, text: 'Saved.' });
  });
});
