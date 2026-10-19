// Public credits page: show it in the UI language and let the visitor change it.

document.addEventListener('DOMContentLoaded', () => {
  const langSelect = document.getElementById('lang-select');
  langSelect.value = getUILang();
  applyTranslations();
  langSelect.addEventListener('change', () => {
    setUILang(langSelect.value);
    applyTranslations();
  });
});
