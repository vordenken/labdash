import assert from 'node:assert/strict';
import test from 'node:test';

Object.defineProperty(globalThis, 'localStorage', {
  configurable: true,
  value: { getItem: () => null },
});
const { containerBuckets, containerLink, containerStatus } = await import('../web/static/core/dom.js');
const { applySetup } = await import('../web/static/core/setup.js');

test('container counts and tile status agree for sleeping, paused and unhealthy containers', () => {
  const asleep = { state: 'exited', sablier: true };
  const paused = { state: 'paused', sablier: true };
  const unhealthy = { state: 'running', health: 'unhealthy', sablier: true };
  const stopped = { state: 'exited', sablier: false };
  const buckets = containerBuckets({ asleep, paused, unhealthy, stopped });
  assert.deepEqual(buckets.sleeping, [asleep]);
  assert.deepEqual(buckets.stopped, [paused, stopped]);
  assert.deepEqual(buckets.running, [unhealthy]);
  assert.deepEqual(buckets.unhealthy, [unhealthy]);
  assert.equal(containerStatus(asleep).cls, 'sleeping');
  assert.equal(containerStatus(paused).cls, 'paused');
});

test('setup preserves discovery settings and creates an optional first link', () => {
  const original = {
    title: 'labdash', theme: 'dark', appearance: { name: '', accent: 'teal' },
    pages: [{ title: 'Home', groups: [] }], discovery: { docker: false },
  };
  assert.deepEqual(applySetup(structuredClone(original), null), original);
  const values = { title: 'Workshop', theme: 'light', name: 'Morgan', page: 'Tools' };
  const noLink = applySetup(structuredClone(original), values);
  assert.equal(noLink.title, 'Workshop');
  assert.equal(noLink.theme, 'light');
  assert.deepEqual(noLink.appearance, { name: 'Morgan', accent: 'teal' });
  assert.deepEqual(noLink.discovery, original.discovery);
  assert.deepEqual(noLink.pages, [{ title: 'Tools', groups: [] }]);
  const withLink = applySetup(structuredClone(original), {
    ...values, url: 'https://wiki.example.org', linkName: '',
  });
  assert.deepEqual(withLink.pages[0].groups[0].tiles, [{
    id: 'first-link', type: 'link', name: 'wiki.example.org', href: 'https://wiki.example.org',
  }]);
});

test('tile icons are only loaded from this server', async () => {
  const { iconSrc } = await import('../web/static/core/dom.js');
  assert.equal(iconSrc('jellyfin'), '/icons/jellyfin');
  assert.equal(iconSrc('mdi-home'), '/icons/mdi-home');
  assert.equal(iconSrc('/custom/logo.png'), '/custom/logo.png');
  for (const external of ['https://cdn.example.org/a.png', '//cdn.example.org/a.png', '../config.json', 'a/b', '']) {
    assert.equal(iconSrc(external), null, external);
  }
});

test('tile links cannot run scripts', async () => {
  const { safeHref } = await import('../web/static/core/dom.js');
  for (const ok of ['https://media.example.org', '/local', 'smb://nas/share']) assert.equal(safeHref(ok), ok);
  for (const bad of ['javascript:alert(1)', 'JAVASCRIPT:alert(1)', 'java\tscript:alert(1)', ' javascript:x', 'data:text/html,x', undefined]) {
    assert.equal(safeHref(bad), undefined, String(bad));
  }
});

test('container management link only targets problem containers', () => {
  const tmpl = 'https://arcane.example.org/containers/{id}?env=0&n={name}&p={project}';
  const stopped = { id: 'abc', name: 'my app', project: 'a/b', state: 'exited' };
  assert.equal(containerLink(tmpl, stopped), 'https://arcane.example.org/containers/abc?env=0&n=my%20app&p=a%2Fb');
  assert.ok(containerLink(tmpl, { id: 'x', state: 'running', health: 'unhealthy' }));
  assert.ok(containerLink(tmpl, { id: 'x', state: 'restarting' }));
  assert.equal(containerLink(tmpl, { id: 'x', state: 'running' }), null);
  assert.equal(containerLink(tmpl, { id: 'x', state: 'exited', sablier: true }), null);
  assert.equal(containerLink(tmpl, undefined), null);
  assert.equal(containerLink('', stopped), null);
  assert.equal(containerLink('javascript:alert({id})', stopped), null);
});
