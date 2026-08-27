// Shared hovering toast notification (issue #347).
//
// Before this, save-confirmation and error messages were inline <div>s
// scattered across the page (a different one per settings card, plus
// separate ones on other pages) that were toggled via show()/hide(). That
// meant the message could appear in a different spot depending on which
// control changed, and toggling `hidden` shifted the layout of whatever sat
// below it.
//
// This is a single fixed-position element, reused for every toast, so it
// never affects page layout and never appears in more than one place.
//
// Dedup/collapse behaviour: only one toast is ever shown at a time. A new
// toast while one is already visible replaces its content and restarts the
// auto-dismiss timer, rather than stacking a second message. If the *same*
// message fires again while still visible (e.g. two settings cards
// auto-saving within a moment of each other), a "(×N)" counter is appended
// so the user can still tell that more than one save happened, without
// piling up separate toasts. A genuinely different message always replaces
// the counter and starts over at 1.

const TOAST_DISMISS_MS = 2500;

let toastEl = null;
let toastTimer = null;
let toastState = null; // { baseText, count } while a toast is visible; null once dismissed.

// computeToastText is pure so it can be unit-tested without a DOM. `current`
// is the in-progress toast state (or null if nothing is showing right now);
// `incomingMessage` is the new save/error text about to be displayed.
function computeToastText(current, incomingMessage) {
  if (current && current.baseText === incomingMessage) {
    const count = current.count + 1;
    return { baseText: incomingMessage, count, text: `${incomingMessage} (×${count})` };
  }
  return { baseText: incomingMessage, count: 1, text: incomingMessage };
}

function ensureToastEl() {
  if (toastEl) return toastEl;
  const el = document.createElement('div');
  el.id = 'app-toast';
  el.setAttribute('data-testid', 'toast');
  el.setAttribute('role', 'status');
  el.setAttribute('aria-live', 'polite');
  el.className = 'hidden fixed bottom-5 right-5 z-[70] max-w-xs px-4 py-2.5 rounded-xl shadow-lg text-sm font-medium text-white transition-opacity duration-150';
  document.body.appendChild(el);
  toastEl = el;
  return el;
}

function hideToastNow() {
  const el = ensureToastEl();
  el.classList.add('hidden');
  toastState = null;
  clearTimeout(toastTimer);
}

function showToast(message, type) {
  const el = ensureToastEl();
  const wasVisible = !el.classList.contains('hidden');
  const state = computeToastText(wasVisible ? toastState : null, message);
  toastState = state;

  el.textContent = state.text;
  el.classList.remove('hidden', 'bg-green-600', 'bg-red-600');
  el.classList.add(type === 'error' ? 'bg-red-600' : 'bg-green-600');

  // Restart the pulse animation on every call (not just repeats) so each
  // individual save is felt, even when the toast never left the screen.
  el.classList.remove('toast-pulse');
  void el.offsetWidth; // force reflow so re-adding the class restarts the animation
  el.classList.add('toast-pulse');

  clearTimeout(toastTimer);
  toastTimer = setTimeout(hideToastNow, TOAST_DISMISS_MS);
}

function showSaved(message) {
  showToast(message || 'Saved.', 'success');
}

function showToastError(message) {
  showToast(message || 'Something went wrong.', 'error');
}
