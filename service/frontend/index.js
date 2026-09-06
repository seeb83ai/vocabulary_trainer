// Landing page logic. Extracted from inline <script> so the app can enforce
// a strict CSP (script-src 'self', no 'unsafe-inline').
//
// Sign-in/create-account lives in a modal (#auth-modal), opened by any
// element carrying [data-show-tab="signin"|"register"] — the header
// buttons, the hero CTA, the demo-finish CTA and the closing band.

// activeTabClasses returns the Tailwind classes for a modal tab button,
// depending on whether it is the selected tab.
function activeTabClasses(isActive) {
  return 'flex-1 py-2 rounded-lg text-sm font-semibold transition ' +
    (isActive ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-800');
}

// initialTab picks which modal tab a visitor lands on based on the URL
// hash, so a shared "/#register" link opens straight to registration.
function initialTab(hash) {
  return hash === '#register' ? 'register' : 'signin';
}

function showTab(tab) {
  const isSignin = tab === 'signin';
  document.getElementById('tab-signin').className = activeTabClasses(isSignin);
  document.getElementById('tab-register').className = activeTabClasses(!isSignin);
  document.getElementById('panel-signin').classList.toggle('hidden', !isSignin);
  document.getElementById('panel-register').classList.toggle('hidden', isSignin);
  history.replaceState(null, '', isSignin ? '/' : '/#register');
}

function openModal(tab) {
  showTab(tab);
  document.getElementById('auth-modal').classList.remove('hidden');
  document.getElementById(tab === 'register' ? 'reg-email' : 'signin-email')?.focus();
}

function closeModal() {
  document.getElementById('auth-modal').classList.add('hidden');
}

async function isPasswordPwned(password) {
  if (!crypto?.subtle) return false;
  try {
    const buf = await crypto.subtle.digest('SHA-1', new TextEncoder().encode(password));
    const hex = Array.from(new Uint8Array(buf))
      .map(b => b.toString(16).padStart(2, '0')).join('').toUpperCase();
    const prefix = hex.slice(0, 5), suffix = hex.slice(5);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 2000);
    let res;
    try {
      res = await fetch('https://api.pwnedpasswords.com/range/' + prefix,
        { headers: { 'Add-Padding': 'true' }, signal: controller.signal });
    } finally {
      clearTimeout(timer);
    }
    if (!res.ok) return false;
    const text = await res.text();
    return text.split('\r\n').some(l => l.split(':')[0] === suffix);
  } catch {
    return false;
  }
}

document.addEventListener('DOMContentLoaded', () => {
  const langSelect = document.getElementById('lang-select');
  langSelect.value = getUILang();
  applyTranslations();
  langSelect.addEventListener('change', () => {
    setUILang(langSelect.value);
    applyTranslations();
  });

  // Redirect to /train if already authenticated
  fetch('/api/auth/status').then(r => r.json()).then(d => {
    if (d.auth) {
      fetch('/api/me').then(r => {
        if (r.ok) window.location.replace('/train');
      });
    }
  }).catch(() => {});

  document.getElementById('tab-signin').addEventListener('click', () => showTab('signin'));
  document.getElementById('tab-register').addEventListener('click', () => showTab('register'));
  for (const el of document.querySelectorAll('[data-show-tab]')) {
    el.addEventListener('click', () => openModal(el.dataset.showTab));
  }

  document.getElementById('auth-modal-close').addEventListener('click', closeModal);
  document.getElementById('auth-modal').addEventListener('click', e => {
    if (e.target === e.currentTarget) closeModal();
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && !document.getElementById('auth-modal').classList.contains('hidden')) {
      closeModal();
    }
  });

  if (location.hash === '#register') openModal(initialTab(location.hash));

  // Sign In
  document.getElementById('signin-form').addEventListener('submit', async e => {
    e.preventDefault();
    const errEl = document.getElementById('signin-error');
    const infoEl = document.getElementById('signin-info');
    errEl.classList.add('hidden');
    infoEl.classList.add('hidden');
    const btn = document.getElementById('signin-btn');
    btn.disabled = true;
    btn.textContent = 'Signing in…';

    const email = document.getElementById('signin-email').value.trim();
    const password = document.getElementById('signin-password').value;

    try {
      const res = await fetch('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password })
      });
      const data = await res.json();

      if (!res.ok) {
        if (data.error === 'email_not_verified') {
          infoEl.textContent = 'Please check your inbox and click the confirmation link to activate your account.';
          infoEl.classList.remove('hidden');
        } else {
          errEl.textContent = data.error || 'Login failed. Please try again.';
          errEl.classList.remove('hidden');
        }
        btn.disabled = false;
        btn.textContent = 'Sign In';
        return;
      }

      const pwned = await isPasswordPwned(password);
      if (pwned) {
        document.getElementById('hibp-warning').classList.remove('hidden');
        btn.disabled = false;
        btn.textContent = 'Sign In';
        return;
      }

      window.location.replace('/train');
    } catch {
      errEl.textContent = 'Network error. Please try again.';
      errEl.classList.remove('hidden');
      btn.disabled = false;
      btn.textContent = 'Sign In';
    }
  });

  // Register
  document.getElementById('register-form').addEventListener('submit', async e => {
    e.preventDefault();
    const errEl = document.getElementById('register-error');
    errEl.classList.add('hidden');
    const btn = document.getElementById('register-btn');

    const email = document.getElementById('reg-email').value.trim();
    const password = document.getElementById('reg-password').value;
    const confirm = document.getElementById('reg-confirm').value;

    if (password !== confirm) {
      errEl.textContent = 'Passwords do not match.';
      errEl.classList.remove('hidden');
      return;
    }
    if (password.length < 8) {
      errEl.textContent = 'Password must be at least 8 characters.';
      errEl.classList.remove('hidden');
      return;
    }

    btn.disabled = true;
    btn.textContent = 'Checking password…';

    const pwned = await isPasswordPwned(password);
    if (pwned) {
      errEl.textContent = 'This password has appeared in a data breach. Please choose a different password.';
      errEl.classList.remove('hidden');
      btn.disabled = false;
      btn.textContent = 'Create Account';
      return;
    }

    btn.textContent = 'Creating account…';

    try {
      const res = await fetch('/api/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password })
      });
      const data = await res.json();

      if (!res.ok) {
        errEl.textContent = data.error || 'Registration failed. Please try again.';
        errEl.classList.remove('hidden');
        btn.disabled = false;
        btn.textContent = 'Create Account';
        return;
      }

      if (data.auto_login) {
        window.location.replace(data.redirect || '/train');
        return;
      }

      document.getElementById('register-form').classList.add('hidden');
      document.getElementById('register-pending-email').textContent = email;
      document.getElementById('register-pending').classList.remove('hidden');
    } catch {
      errEl.textContent = 'Network error. Please try again.';
      errEl.classList.remove('hidden');
      btn.disabled = false;
      btn.textContent = 'Create Account';
    }
  });

  const urlParams = new URLSearchParams(location.search);
  const urlError = urlParams.get('error');
  if (urlError) {
    openModal('signin');
    const errEl = document.getElementById('signin-error');
    if (urlError === 'invalid_token') {
      errEl.textContent = 'This verification link is invalid or has expired. Please register again.';
    } else {
      errEl.textContent = 'An error occurred. Please try again.';
    }
    errEl.classList.remove('hidden');
  }
});
