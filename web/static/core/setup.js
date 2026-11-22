import { h } from './dom.js';
import { t } from './i18n.js';

export function setupForm(layout, onSave) {
  const field = (label, input) => h('label', { class: 'field' }, h('span', { text: label }), input);
  const theme = h('select', { name: 'theme' },
    h('option', { value: 'dark', text: t('Dark') }),
    h('option', { value: 'light', text: t('Light') }),
    h('option', { value: 'auto', text: t('System') }));
  theme.value = layout.theme;
  const err = h('p', { class: 'error', role: 'alert' });
  const save = h('button', { type: 'submit', class: 'btn primary', text: t('Finish setup') });
  const skip = h('button', { type: 'submit', class: 'btn ghost', formnovalidate: true, text: t('Skip setup') });
  const form = h('form', { class: 'setup-form' },
    h('h2', { id: 'setup-title', text: t('Set up your dashboard') }),
    h('p', { class: 'setup-intro', text: t('Name your dashboard and first page. Add a link now or start with an empty page.') }),
    h('div', { class: 'form-row' },
      field(t('Dashboard name'), h('input', { name: 'title', value: layout.title, maxlength: 64, required: true })),
      field(t('First page name'), h('input', { name: 'page', value: t('Home'), maxlength: 64, required: true })),
      field(t('Greeting name (optional)'), h('input', { name: 'greeting', autocomplete: 'given-name', maxlength: 64 })),
      field(t('Default theme'), theme)),
    h('fieldset', { class: 'field setup-link' },
      h('legend', { text: t('First link (optional)') }),
      h('div', { class: 'form-row' },
        field(t('Link name'), h('input', { name: 'linkName', maxlength: 64, placeholder: t('e.g. Lab wiki') })),
        field('URL', h('input', { name: 'url', type: 'url', placeholder: 'https://wiki.example.org' })))),
    err,
    h('div', { class: 'form-actions' }, skip, save));
  const { url, linkName } = form.elements;
  url.oninput = () => url.setCustomValidity('');
  linkName.oninput = () => url.setCustomValidity('');
  form.onsubmit = async (e) => {
    e.preventDefault();
    const skipping = e.submitter === skip;
    if (!skipping && (url.value || linkName.value.trim()) && !/^https?:\/\//i.test(url.value)) {
      url.setCustomValidity(t('Enter an HTTP or HTTPS address.'));
      url.reportValidity();
      return;
    }
    err.textContent = '';
    form.setAttribute('aria-busy', 'true');
    save.disabled = skip.disabled = true;
    try {
      await onSave(skipping ? null : {
        title: form.elements.title.value.trim(),
        page: form.elements.page.value.trim(),
        name: form.elements.greeting.value.trim(),
        theme: theme.value,
        url: url.value,
        linkName: linkName.value.trim(),
      });
    } catch (e) {
      err.textContent = t('Could not finish setup: {0}', e.message);
    } finally {
      form.removeAttribute('aria-busy');
      save.disabled = skip.disabled = false;
    }
  };
  return form;
}

export function applySetup(cfg, values) {
  if (!values) return cfg;
  cfg.title = values.title || cfg.title;
  cfg.theme = values.theme;
  cfg.appearance.name = values.name;
  const page = cfg.pages[0];
  page.title = values.page || page.title;
  if (values.url) page.groups.push({
    id: 'links', title: t('Links'),
    tiles: [{ id: 'first-link', type: 'link', name: values.linkName || new URL(values.url).hostname, href: values.url }],
  });
  return cfg;
}
