// HIBP k-anonymity check
async function isPasswordPwned(password) {
  if (!crypto?.subtle) return false;
  try {
    const buf = await crypto.subtle.digest('SHA-1', new TextEncoder().encode(password));
    const hex = Array.from(new Uint8Array(buf))
      .map(b => b.toString(16).padStart(2, '0')).join('').toUpperCase();
    const prefix = hex.slice(0, 5), suffix = hex.slice(5);
    const res = await fetch('https://api.pwnedpasswords.com/range/' + prefix,
      { headers: { 'Add-Padding': 'true' } });
    const text = await res.text();
    return text.split('\r\n').some(l => l.split(':')[0] === suffix);
  } catch {
    return false;
  }
}

// Load account info
fetch('/api/me').then(r => {
  if (r.status === 401) { window.location.replace('/'); return null; }
  return r.json();
}).then(data => {
  if (data) document.getElementById('account-email').textContent = data.email;
}).catch(() => {});

// Change password form
document.getElementById('pw-form').addEventListener('submit', async e => {
  e.preventDefault();
  const errEl = document.getElementById('pw-error');
  const okEl = document.getElementById('pw-success');
  errEl.classList.add('hidden');
  okEl.classList.add('hidden');

  const btn = document.getElementById('pw-btn');
  const currentPw = document.getElementById('pw-current').value;
  const newPw = document.getElementById('pw-new').value;
  const confirmPw = document.getElementById('pw-confirm').value;

  if (newPw !== confirmPw) {
    errEl.textContent = 'New passwords do not match.';
    errEl.classList.remove('hidden');
    return;
  }
  if (newPw.length < 8) {
    errEl.textContent = 'New password must be at least 8 characters.';
    errEl.classList.remove('hidden');
    return;
  }

  btn.disabled = true;
  btn.textContent = 'Checking password…';

  const pwned = await isPasswordPwned(newPw);
  if (pwned) {
    errEl.textContent = 'This password has appeared in a data breach. Please choose a different password.';
    errEl.classList.remove('hidden');
    btn.disabled = false;
    btn.textContent = 'Update Password';
    return;
  }

  btn.textContent = 'Updating…';

  try {
    const res = await fetch('/api/change-password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ current_password: currentPw, new_password: newPw })
    });
    const data = await res.json();

    if (!res.ok) {
      errEl.textContent = data.error || 'Failed to update password.';
      errEl.classList.remove('hidden');
    } else {
      okEl.classList.remove('hidden');
      document.getElementById('pw-form').reset();
    }
  } catch {
    errEl.textContent = 'Network error. Please try again.';
    errEl.classList.remove('hidden');
  }

  btn.disabled = false;
  btn.textContent = 'Update Password';
});
