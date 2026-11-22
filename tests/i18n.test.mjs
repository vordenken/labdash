import assert from 'node:assert/strict';
import test from 'node:test';

async function load(preference) {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: { getItem: () => preference },
  });
  return import(`../web/static/core/i18n.js?preference=${preference}`);
}

test('a fresh browser uses its first supported language, or English as fallback', async () => {
  const { selectLocale } = await load(null);
  assert.equal(selectLocale(null, ['fr-FR', 'de-DE', 'en-US']), 'de');
  assert.equal(selectLocale(null, ['en-GB', 'de-DE']), 'en');
  assert.equal(selectLocale('fr', ['DE-AT']), 'de');
  assert.equal(selectLocale(null, ['fr-FR']), 'en');
});

test('an explicit language selection overrides browser preferences', async () => {
  const { selectLocale, locale, t } = await load('en');
  assert.equal(selectLocale('en', ['de-DE']), 'en');
  assert.equal(selectLocale('de', ['en-US']), 'de');
  assert.equal(locale, 'en');
  assert.equal(t('Open {0}', 'Medien'), 'Open Medien');
});

test('German translates UI text and attributes but not interpolated content', async () => {
  const { localizePage, t } = await load('de');
  const label = { textContent: 'Filter tiles' };
  const attributes = new Map([['title', 'Edit (E)'], ['aria-label', 'Edit (E)']]);
  const button = {
    dataset: { i18nAttrs: 'title aria-label' },
    getAttribute: name => attributes.get(name),
    setAttribute: (name, value) => attributes.set(name, value),
  };
  const root = {
    documentElement: {},
    querySelectorAll: selector => selector === '[data-i18n]' ? [label] : [button],
  };
  localizePage(root);
  assert.equal(root.documentElement.lang, 'de');
  assert.equal(label.textContent, 'Kacheln filtern');
  assert.deepEqual([...attributes.values()], ['Bearbeiten (E)', 'Bearbeiten (E)']);
  assert.equal(t('Open {0}', 'Save {0}'), 'Save {0} öffnen');
  assert.equal(t('Show containers: {0} ({1})', 'running', 3), 'Container anzeigen: running (3)');
});
