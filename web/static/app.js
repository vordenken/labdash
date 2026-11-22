import { t, locale, localizePage } from './core/i18n.js';
import { api, stream } from './core/api.js';
import { h, clear, toast, contrastFg, containerStatus, containerBuckets, containerLink, statusDot } from './core/dom.js';
import { icon, iconButton } from './core/icons.js';
import { get as getWidget } from './core/registry.js';
import { tileForm, titleForm, jsonForm, confirmDialog, enableDragDrop, appearanceForm, discoveryForm, accentColor } from './core/editor.js';
import { setupForm, applySetup } from './core/setup.js';
import './widgets/link.js';
import './widgets/docker.js';
import './widgets/clock.js';

const state = {
  session: null,
  layout: { title: 'labdash', theme: 'dark', appearance: { accent: 'indigo', hero: true, name: '' }, pages: [] },
  docker: { containers: {}, error: '' },
  page: localStorage.getItem('labdash.page') || null,
  editing: false,
  dirty: false,
  draft: null,          // config clone while editing
  instances: [],        // live widget instances for incremental updates
  es: null,
};

const $ = (id) => document.getElementById(id);
const app = $('app');
localizePage(document);

// --- boot -----------------------------------------------------------------

async function boot() {
  applyTheme(localStorage.getItem('labdash.theme'));
  state.session = await api.session();
  if (state.session.version) {
    const versionLabel = $('settings-version');
    versionLabel.textContent = t('Version {0}', state.session.version);
    versionLabel.hidden = false;
  }
  document.title = state.session.title;
  $('title').textContent = state.session.title;
  $('btn-logout').hidden = !(state.session.authRequired && state.session.authenticated);
  if (state.session.authRequired && !state.session.authenticated) {
    renderLogin();
    return;
  }
  connect();
}

function renderSkeleton() {
  app.className = 'dashboard skeleton';
  clear(app);
  for (let g = 0; g < 2; g++) {
    const tiles = h('div', { class: 'tiles' });
    for (let i = 0; i < 3; i++) tiles.append(h('div', { class: 'tile', 'aria-hidden': 'true' }));
    app.append(h('section', { class: 'group' }, h('div', { class: 'group-head' }, h('h2', { 'aria-hidden': 'true' })), tiles));
  }
  app.setAttribute('aria-busy', 'true');
}

function connect() {
  if (state.es) state.es.close();
  document.body.classList.remove('login');
  $('btn-logout').hidden = !state.session.authRequired;
  if (!state.layout.pages.length) renderSkeleton();
  state.es = stream({
    layout: receiveLayout,
    docker: (d) => { state.docker = d; updateDockerPill(); updateHeroStats(); updateInstances(); },
    __error: () => { const p = $('docker-state'); p.hidden = false; p.querySelector('.txt').textContent = 'offline'; p.className = 'pill err'; },
    __open: () => updateDockerPill(),
  });
}

function receiveLayout(layout) {
  state.layout = layout;
  app.removeAttribute('aria-busy');
  updateDockerPill();
  if (!state.editing) {
    applyAppearance();
    render();
  }
}

function renderSetup() {
  const section = $('setup');
  const visible = Boolean(state.layout.needsSetup) && !state.editing;
  if (visible === !section.hidden) return;
  if (visible) {
    section.append(setupForm(state.layout, async (values) => {
      try {
        const cfg = await api.getConfig();
        const layout = await api.completeSetup(applySetup(cfg, values));
        receiveLayout(layout);
        connect();
        toast(t('Saved'));
      } catch (e) {
        if (e.status !== 409) throw e;
        toast(t('Setup was already completed or the layout changed.'), true);
        receiveLayout(await api.layout());
      }
    }));
    section.hidden = false;
  } else {
    const hadFocus = section.contains(document.activeElement);
    section.hidden = true;
    clear(section);
    if (hadFocus) $('filter').focus();
  }
}

function renderLogin() {
  const pw = h('input', { type: 'password', id: 'login-pw', name: 'password', placeholder: t('Password'), autofocus: true, required: true, autocomplete: 'current-password' });
  const err = h('div', { class: 'error' });
  const form = h('form', { onsubmit: async (e) => {
    e.preventDefault();
    try {
      await api.login(pw.value);
      state.session = await api.session();
      connect();
    } catch (ex) {
      err.textContent = ex.status === 429 ? t('Too many attempts. Please wait a moment.') : t('Incorrect password.');
      pw.value = '';
      pw.focus();
    }
  } }, h('img', { class: 'logo', src: '/mark.svg', width: 44, height: 44, alt: '' }), h('h3', { text: state.session.title }),
    h('label', { class: 'visually-hidden', for: 'login-pw', text: t('Password') }), pw, err, h('button', { class: 'btn primary', text: t('Sign in') }));
  clear(app).append(h('div', { class: 'login' }, form));
  app.className = 'login-wrap';
  document.body.classList.add('login');
  $('brand-sub').hidden = true;
}

// --- rendering ------------------------------------------------------------

function ctx() {
  return {
    docker: state.docker,
    session: state.session,
    editing: state.editing,
    action: async (name, verb) => {
      const message = { start: 'Starting {0}.', stop: 'Stopping {0}.', restart: 'Restarting {0}.' }[verb];
      try { await api.containerAction(name, verb); toast(t(message, name)); }
      catch (ex) { toast(t('Container action failed for {0}: {1}', name, ex.message), true); }
    },
  };
}

function currentPages() {
  return state.editing ? state.draft.pages : state.layout.pages;
}

function render() {
  app.className = 'dashboard';
  destroyInstances();
  renderSetup();
  const pages = currentPages();
  if (!pages.find((p) => p.id === state.page)) state.page = pages[0]?.id;
  renderPageNav(pages);
  const page = pages.find((p) => p.id === state.page);
  clear(app);
  renderHeader();
  if (!page) return;
  let gi = 0;
  for (const group of page.groups) app.append(renderGroup(group, page, state.editing ? discoveredFor(group) : [], false, gi++));
  if (state.editing) for (const vg of virtualGroups(page)) app.append(renderGroup(vg, page, [], true, gi++));
  applyFilter();
  if (!page.groups.length && !state.editing && !state.layout.needsSetup) {
    app.append(h('div', { class: 'empty' },
      h('p', { text: t('No tiles yet. Containers with dashboard labels appear automatically.') }),
      h('button', { class: 'btn primary', onclick: enterEdit }, icon('edit'), t('Edit dashboard')),
    ));
  }
}

// --- header: greeting, clock, stats ----------------------------------------

let heroTimer;
function renderHeader() {
  const { appearance: ap = {}, title } = visibleSettings();
  const sub = $('brand-sub');
  clearInterval(heroTimer);
  if (!ap.hero) { sub.hidden = true; $('title').textContent = title || 'labdash'; updateHeroStats(); return; }
  sub.hidden = false;
  const tick = () => {
    const now = new Date();
    const hour = now.getHours();
    const word = hour < 5 ? t('Good night') : hour < 11 ? t('Good morning') : hour < 17 ? t('Hello') : hour < 22 ? t('Good evening') : t('Good night');
    const heading = clear($('title'));
    heading.append(word);
    if (ap.name) heading.append(', ', h('em', { text: ap.name }));
    $('hero-time').textContent = now.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' });
    $('hero-date').textContent = now.toLocaleDateString(locale, { weekday: 'short', day: 'numeric', month: 'short' });
  };
  tick();
  heroTimer = setInterval(tick, 10000);
  updateHeroStats();
}

function updateHeroStats() {
  const box = $('hero-stats');
  if (!visibleSettings().appearance?.hero || !state.session?.dockerEnabled) { clear(box); return; }
  const buckets = containerBuckets(state.docker.containers);
  const focusedKey = box.contains(document.activeElement) ? document.activeElement.dataset.statusKey : null;
  const chip = (key, cls, alert = false) => {
    const count = buckets[key].length;
    const label = t(key);
    const name = t('Show containers: {0} ({1})', label, count);
    return h('button', { type: 'button', class: 'stat-chip' + (alert && count ? ' alert' : ''),
      dataset: { statusKey: key }, 'aria-label': name, 'aria-haspopup': 'dialog', title: name,
      onclick: () => openStatusList(key, buckets[key]) },
    h('span', { class: `status ${cls}`, 'aria-hidden': 'true' }), h('span', { class: 'v', text: count }), h('span', { class: 'l', text: label }));
  };
  clear(box).append(
    chip('running', 'running'),
    chip('sleeping', 'sleeping'),
    chip('stopped', buckets.stopped.length ? 'exited' : 'unknown', true),
  );
  if (buckets.unhealthy.length) box.append(chip('unhealthy', 'unhealthy', true));
  if (focusedKey) (statusButton(focusedKey) || statusButton('running'))?.focus({ preventScroll: true });
  if (statusDialog.open) renderStatusList(buckets[statusKey]);
}

const statusDialog = $('container-status');
let statusKey;
let statusScroll;
const statusButton = (key) => $('hero-stats').querySelector(`[data-status-key="${key}"]`);

function openStatusList(key, containers) {
  statusKey = key;
  statusScroll = { top: window.scrollY, left: window.scrollX, behavior: 'instant' };
  renderStatusList(containers);
  statusDialog.showModal();
}

statusDialog.addEventListener('close', () => {
  (statusButton(statusKey) || statusButton('running') || $('filter')).focus({ preventScroll: true });
  // Native dialog focus restoration can scroll a sticky header's trigger back into view.
  window.scrollTo(statusScroll);
});

function renderStatusList(containers) {
  $('container-status-title').textContent = t('Container status: {0}', t(statusKey));
  $('container-status-empty').hidden = containers.length > 0;
  const list = clear($('container-status-list'));
  for (const container of [...containers].sort((a, b) => a.name.localeCompare(b.name, locale))) {
    const status = containerStatus(container);
    const dot = statusDot(status);
    if (status.severity !== 'ok') dot.setAttribute('aria-hidden', 'true');
    const url = containerLink(state.session?.containerLink, container);
    const label = t('Open {0} in container management', container.name);
    list.append(h('li', {},
      dot,
      h('span', { class: 'container-status-name', text: container.name }),
      status.severity === 'ok' ? null : h('span', { class: 'status-scope', text: status.label }),
      url ? h('a', { class: 'btn small icon container-link', href: url, target: '_blank', rel: 'noopener', title: label, 'aria-label': label }, icon('external')) : null));
  }
}

// --- live filter ------------------------------------------------------------

const filterInput = $('filter');
const filterClear = $('filter-clear');
let filterEmptyEl = null;

function tileHaystack(tile, group) {
  const s = tile.container ? containerStatus(state.docker.containers[tile.container]) : null;
  return [tile.name, tile.description, tile.container, group.title, s?.label, s?.severity === 'bad' ? 'problem fehler' : ''].filter(Boolean).join(' ').toLowerCase();
}

function clearFilter() {
  filterInput.value = '';
  applyFilter();
  filterInput.focus();
}

function applyFilter() {
  filterClear.hidden = !filterInput.value;
  $('filter-hint').hidden = !filterClear.hidden;
  const q = filterInput.value.trim().toLowerCase();
  const words = q.split(/\s+/).filter(Boolean);
  document.body.classList.toggle('filtering', words.length > 0);
  filterEmptyEl?.remove();
  filterEmptyEl = null;
  if (!words.length) return;
  let total = 0;
  for (const section of app.querySelectorAll('.group')) {
    let visible = 0;
    for (const el of section.querySelectorAll('.tile[data-tile-id]')) {
      const hay = el.dataset.hay || '';
      const ok = words.every((w) => hay.includes(w));
      el.classList.toggle('match', ok);
      if (ok) visible++;
    }
    section.classList.toggle('no-match', visible === 0);
    total += visible;
  }
  if (!total) {
    filterEmptyEl = h('p', { class: 'filter-empty', text: t('No results for “{0}”.', filterInput.value.trim()) });
    app.append(filterEmptyEl);
  }
}

filterInput.addEventListener('input', applyFilter);
filterClear.append(icon('x', 16));
filterClear.onclick = clearFilter;
filterInput.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') { if (filterInput.value) clearFilter(); else filterInput.blur(); e.stopPropagation(); }
  if (e.key === 'Enter') {
    e.preventDefault();
    const first = app.querySelector(document.body.classList.contains('filtering') ? '.tile.match > a[href]' : '.tile > a[href]');
    if (first) window.open(first.href, '_blank', 'noopener');
  }
});

function visibleSettings() {
  return state.editing ? state.draft : state.layout;
}

function applyAppearance() {
  const settings = visibleSettings();
  const accent = accentColor(settings.appearance?.accent);
  document.documentElement.style.setProperty('--accent', accent);
  document.documentElement.style.setProperty('--accent-fg', contrastFg(accent));
  if (!localStorage.getItem('labdash.theme')) applyTheme(settings.theme);
  const title = settings.title || state.session?.title || 'labdash';
  document.title = title;
  $('title').textContent = title;
}

function renderPageNav(pages) {
  const nav = clear($('pages'));
  if (state.layout.needsSetup && !state.editing) return;
  if (pages.length === 1 && !state.editing) {
    nav.append(h('span', { class: 'page-current', 'aria-current': 'page', text: pages[0].title || pages[0].id }));
    return;
  }
  for (const p of pages) {
    const active = p.id === state.page;
    const b = h('span', { class: 'page-tab' + (active ? ' active' : '') },
      h('button', { type: 'button', 'aria-current': active ? 'page' : undefined, onclick: () => {
        state.page = p.id; localStorage.setItem('labdash.page', p.id); render();
      } }, p.title || p.id));
    if (state.editing && active) {
      b.append(iconButton('edit', t('Rename or delete page “{0}”', p.title), { class: 'small ghost', onclick: async (e) => {
        e.stopPropagation();
        const r = await titleForm('page', p.title);
        if (r.action === 'save') { p.title = r.title; commit(); }
        if (r.action === 'delete') {
          if (state.draft.pages.length <= 1) { toast(t('You cannot delete the last page.'), true); return; }
          if (await confirmDialog(t('Delete page “{0}” and its groups ({1})?', p.title, p.groups.length))) {
            const idx = state.draft.pages.indexOf(p);
            state.draft.pages.splice(idx, 1);
            commit();
            toast(t('Deleted page “{0}”', p.title), { undo: () => { state.draft.pages.splice(idx, 0, p); state.page = p.id; commit(); } });
          }
        }
      } }));
    }
    nav.append(b);
  }
}

function renderGroup(group, page, extras = [], virtual = false, gi = 0) {
  const tiles = h('div', { class: 'tiles', dataset: { groupId: group.id } });
  let i = 0;
  for (const tile of group.tiles) tiles.append(renderTile(tile, group, i++, gi));
  for (const tile of extras) tiles.append(renderTile(tile, group, i++, gi));
  if (state.editing) {
    tiles.append(h('button', { type: 'button', class: 'tile add', 'aria-label': t('Add a tile to “{0}”', group.title), onclick: async () => {
      const r = await tileForm({ type: 'link' }, state.docker.containers);
      if (r.action === 'save') { draftGroupFor(group).tiles.push(r.tile); commit(); }
    } }, icon('plus', 20), h('span', { text: t('Add tile') })));
  }
  const tools = h('div', { class: 'group-tools' });
  if (state.editing && !virtual) {
    tools.append(iconButton('edit', t('Rename or delete group “{0}”', group.title), { class: 'small', onclick: async () => {
      const r = await titleForm('group', group.title);
      if (r.action === 'save') { group.title = r.title; commit(); }
      if (r.action === 'delete' && await confirmDialog(t('Delete group “{0}” and its tiles ({1})?', group.title, group.tiles.length))) {
        const idx = page.groups.indexOf(group);
        page.groups.splice(idx, 1);
        commit();
        toast(t('Deleted group “{0}”', group.title), { undo: () => { page.groups.splice(idx, 0, group); commit(); } });
      }
    } }));
    const idx = page.groups.indexOf(group);
    tools.append(iconButton('up', t('Move group up'), { class: 'small', disabled: idx === 0, onclick: () => { page.groups.splice(idx, 1); page.groups.splice(idx - 1, 0, group); commit(); } }));
    tools.append(iconButton('down', t('Move group down'), { class: 'small', disabled: idx === page.groups.length - 1, onclick: () => { page.groups.splice(idx, 1); page.groups.splice(idx + 1, 0, group); commit(); } }));
  }
  const alert = h('span', { class: 'group-alert', role: 'status' });
  const all = [...group.tiles, ...extras];
  const updateAlert = (c) => {
    const bad = all.filter((t) => t.container && containerStatus(c.docker.containers[t.container]).severity === 'bad').length;
    clear(alert);
    if (bad) alert.append(h('span', { class: 'status unhealthy', 'aria-hidden': 'true' }), t('With issues: {0}', bad));
  };
  updateAlert(ctx());
  state.instances.push({ update: updateAlert });
  return h('section', { class: 'group' + (virtual ? ' virtual' : ''), 'aria-labelledby': `gh-${group.id}` },
    h('div', { class: 'group-head' }, h('h2', { id: `gh-${group.id}`, text: group.title || '' }), alert, tools),
    tiles,
  );
}

function renderTile(tile, group, i = 0, gi = 0) {
  const discovered = tile.source === 'discovery';
  const el = h('div', {
    class: 'tile' + (discovered ? ' discovered' : '') + (tile.size === 'wide' ? ' wide' : ''),
    style: `--i:${i};--gi:${gi}`,
    dataset: { tileId: tile.id, groupId: group.id },
    draggable: state.editing && !discovered ? 'true' : undefined,
  });
  el.dataset.hay = tileHaystack(tile, group);
  const def = getWidget(tile.type);
  try {
    const inst = def.render(el, tile, ctx());
    if (inst) state.instances.push(inst);
    if (tile.container) state.instances.push({ update: () => { el.dataset.hay = tileHaystack(tile, group); } });
  } catch (e) {
    console.error('widget failed', tile, e);
    el.append(h('div', { class: 'tile-body error', text: t('Widget “{0}” failed', tile.type) }));
  }
  if (state.editing) {
    const edit = h('div', { class: 'tile-edit' });
    const label = tile.name || tile.container || t('Tile');
    if (discovered) {
      edit.append(h('span', { class: 'edit-note', text: t('From Docker labels') }));
      edit.append(iconButton('pin', t('Copy “{0}” to the dashboard to edit it', label), { class: 'small', onclick: () => adoptTile(tile, group) }));
      edit.append(iconButton('eyeOff', t('Hide “{0}”', label), { class: 'small', onclick: () => {
        state.draft.discovery.hidden.push(tile.container);
        commit();
        toast(t('Hid “{0}”', label), { undo: () => { state.draft.discovery.hidden = state.draft.discovery.hidden.filter((x) => x !== tile.container); commit(); } });
      } }));
    } else {
      const idx = group.tiles.indexOf(tile);
      edit.append(iconButton('left', t('Move “{0}” earlier', label), { class: 'small ghost', disabled: idx === 0, onclick: () => moveTileBy(tile, group, -1) }));
      edit.append(iconButton('right', t('Move “{0}” later', label), { class: 'small ghost', disabled: idx === group.tiles.length - 1, onclick: () => moveTileBy(tile, group, 1) }));
      edit.append(iconButton('edit', t('Edit “{0}”', label), { class: 'small', onclick: async () => {
        const r = await tileForm(tile, state.docker.containers);
        if (r.action === 'save') {
          for (const k of Object.keys(tile)) delete tile[k];
          Object.assign(tile, r.tile);
          commit();
        }
        if (r.action === 'delete') {
          const i = group.tiles.indexOf(tile);
          group.tiles.splice(i, 1);
          commit();
          toast(t('Deleted “{0}”', label), { undo: () => { group.tiles.splice(i, 0, tile); commit(); } });
        }
      } }));
    }
    el.append(edit);
  }
  return el;
}

function moveTileBy(tile, group, delta) {
  const i = group.tiles.indexOf(tile);
  const j = i + delta;
  if (i < 0 || j < 0 || j >= group.tiles.length) return;
  group.tiles.splice(i, 1);
  group.tiles.splice(j, 0, tile);
  commit();
  requestAnimationFrame(() => document.querySelector(`[data-tile-id="${CSS.escape(tile.id)}"] .tile-edit button:nth-child(${delta < 0 ? 1 : 2})`)?.focus());
}

// Marks the draft as changed and re-renders.
function commit() {
  state.dirty = true;
  $('dirty-badge').hidden = false;
  render();
}

function destroyInstances() {
  for (const i of state.instances) i.destroy?.();
  state.instances = [];
}

function updateInstances() {
  const c = ctx();
  for (const i of state.instances) i.update?.(c);
  if (document.body.classList.contains('filtering')) applyFilter();
}

// The pill only reports a Docker failure. Container counts belong to the
// header summary, which the "hero" appearance option switches off.
function updateDockerPill() {
  const pill = $('docker-state');
  if (!state.session?.dockerEnabled || !state.docker.error) { pill.hidden = true; return; }
  pill.hidden = false;
  pill.className = 'pill err';
  const text = t('Docker unavailable');
  pill.querySelector('.txt').textContent = text;
  pill.setAttribute('aria-label', `${text}: ${state.docker.error}`);
  pill.title = state.docker.error;
}

// --- editing --------------------------------------------------------------

const editToolbar = $('edit-toolbar');
new ResizeObserver(() => {
  document.documentElement.style.setProperty('--edit-toolbar-height', `${editToolbar.offsetHeight}px`);
}).observe(editToolbar);

async function enterEdit() {
  if (state.editing || !state.session?.authenticated || !state.layout.pages.length || state.layout.needsSetup) return;
  try {
    state.draft = await api.getConfig();
  } catch (e) { toast(e.message, true); return; }
  state.draft.discovery.hidden ||= [];
  state.editing = true;
  state.dirty = false;
  $('dirty-badge').hidden = true;
  document.body.classList.add('editing');
  editToolbar.hidden = false;
  render();
  $('btn-add-tile').focus({ preventScroll: true });
}

async function cancelEdit() {
  if (state.dirty && !(await confirmDialog(t('Discard unsaved changes?')))) return;
  exitEdit();
}

function exitEdit(layout = state.layout) {
  const restoreFocus = editToolbar.contains(document.activeElement);
  state.editing = false;
  state.dirty = false;
  state.draft = null;
  document.body.classList.remove('editing');
  editToolbar.hidden = true;
  receiveLayout(layout);
  if (restoreFocus) $('btn-settings').focus({ preventScroll: true });
}

async function save() {
  const button = $('btn-save');
  if (button.disabled) return;
  button.disabled = true;
  try {
    const layout = await api.putConfig(stripRuntime(state.draft));
    exitEdit(layout);
    // Discard any older layout events queued on the previous stream.
    connect();
    toast(t('Saved'));
  } catch (e) {
    toast(t('Could not save. Your changes are still here: {0}', e.message), true);
  } finally {
    button.disabled = false;
  }
}

function stripRuntime(cfg) {
  const c = structuredClone(cfg);
  for (const p of c.pages) {
    delete p.virtual;
    for (const g of p.groups) {
      delete g.virtual;
      for (const t of g.tiles) delete t.source;
    }
  }
  return c;
}

// Discovered tiles (from the server layout) that belong to a draft group by
// title and are neither bound to a draft tile nor hidden.
function discoveredFor(group) {
  return discoveredTiles().filter((d) => d.groupTitle === (group.title || '').toLowerCase()).map((d) => d.tile);
}

// Discovered groups whose title matches no draft group; shown on the first
// page like the server does.
function virtualGroups(page) {
  if (page !== state.draft.pages[0]) return [];
  const known = new Set(state.draft.pages.flatMap((p) => p.groups.map((g) => (g.title || '').toLowerCase())));
  const map = new Map();
  for (const d of discoveredTiles()) {
    if (known.has(d.groupTitle)) continue;
    if (!map.has(d.groupTitle)) map.set(d.groupTitle, { id: `disc-${d.groupTitle.replace(/[^a-z0-9_-]/g, '-')}`, title: d.title, tiles: [], virtual: true });
    map.get(d.groupTitle).tiles.push(d.tile);
  }
  return [...map.values()].sort((a, b) => a.title.localeCompare(b.title));
}

function discoveredTiles() {
  if (!state.draft?.discovery?.docker) return [];
  const bound = new Set();
  for (const p of state.draft.pages) for (const g of p.groups) for (const t of g.tiles) if (t.container) bound.add(t.container);
  const hidden = new Set(state.draft.discovery.hidden);
  const out = [];
  for (const p of state.layout.pages) for (const g of p.groups) for (const t of g.tiles) {
    if (t.source !== 'discovery' || bound.has(t.container) || hidden.has(t.container)) continue;
    out.push({ groupTitle: (g.title || '').toLowerCase(), title: g.title, tile: t });
  }
  return out;
}

// Returns the draft group for a rendered group. Groups that only exist through
// Docker labels are created in the draft on first use, so tiles can be added.
function draftGroupFor(group) {
  let target = group.virtual ? null : state.draft.pages.flatMap((p) => p.groups).find((g) => g.id === group.id);
  if (!target) {
    target = { id: group.id.replace(/^disc-/, 'g-'), title: group.title, tiles: [] };
    (state.draft.pages.find((p) => p.id === state.page) || state.draft.pages[0]).groups.push(target);
  }
  return target;
}

function adoptTile(tile, group) {
  const copy = { ...tile, id: `t-${tile.container}`.replace(/[^a-zA-Z0-9_-]/g, '-') };
  delete copy.source;
  const target = draftGroupFor(group);
  if (target.tiles.some((t) => t.id === copy.id)) copy.id += `-${Date.now().toString(36)}`;
  target.tiles.push(copy);
  commit();
}

function moveTile(tileId, fromGroupId, toGroupId, index) {
  const groups = state.draft.pages.flatMap((p) => p.groups);
  const from = groups.find((g) => g.id === fromGroupId);
  const to = groups.find((g) => g.id === toGroupId);
  if (!from || !to) return;
  const i = from.tiles.findIndex((t) => t.id === tileId);
  if (i < 0) return;
  const [tile] = from.tiles.splice(i, 1);
  if (from === to && i < index) index--;
  to.tiles.splice(Math.min(index, to.tiles.length), 0, tile);
  commit();
}

// --- theme & wiring -------------------------------------------------------

function applyTheme(preference) {
  let theme = preference || visibleSettings().theme || 'dark';
  if (theme === 'auto') theme = matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
  document.body.dataset.theme = theme;
}

const settingsButton = $('btn-settings');
const settingsPanel = $('settings-panel');
const settingsLanguage = $('settings-language');
const settingsTheme = $('settings-theme');

function setSettingsOpen(open) {
  if (open) {
    settingsLanguage.value = localStorage.getItem('labdash.language') || '';
    settingsLanguage.disabled = state.editing;
    $('settings-language-hint').hidden = !state.editing;
    settingsTheme.value = localStorage.getItem('labdash.theme') || '';
    $('btn-edit').hidden = state.editing || !state.session?.authenticated || !state.layout.pages.length || state.layout.needsSetup;
  }
  settingsPanel.hidden = !open;
  settingsButton.setAttribute('aria-expanded', String(open));
}
settingsButton.append(icon('settings'));
settingsButton.onclick = () => setSettingsOpen(settingsPanel.hidden);
for (const event of ['pointerdown', 'focusin']) {
  document.addEventListener(event, (e) => {
    if (!settingsPanel.hidden && !settingsButton.parentElement.contains(e.target)) setSettingsOpen(false);
  });
}
settingsLanguage.onchange = () => {
  if (settingsLanguage.value) localStorage.setItem('labdash.language', settingsLanguage.value);
  else localStorage.removeItem('labdash.language');
  location.reload();
};
settingsTheme.onchange = () => {
  if (settingsTheme.value) localStorage.setItem('labdash.theme', settingsTheme.value);
  else localStorage.removeItem('labdash.theme');
  applyTheme(settingsTheme.value);
};
$('btn-edit').prepend(icon('edit'));
$('btn-edit').onclick = () => {
  setSettingsOpen(false);
  settingsButton.focus({ preventScroll: true });
  enterEdit();
};
document.querySelector('.search-field .search-icon').append(icon('search', 16));
$('btn-logout').append(icon('logout'));
$('btn-logout').onclick = async () => { await api.logout(); location.reload(); };
$('btn-add-tile').prepend(icon('plus'));
$('btn-add-group').prepend(icon('rows'));
$('btn-add-page').prepend(icon('file'));
$('btn-appearance').prepend(icon('palette'));
$('btn-discovery').prepend(icon('layers'));
$('btn-json').prepend(icon('braces'));
$('btn-cancel').onclick = cancelEdit;
$('btn-save').onclick = save;
// Adds to the first group on the current page; the per-group "+" tiles pick
// a specific group.
$('btn-add-tile').onclick = () => {
  const add = app.querySelector('.tile.add');
  if (add) add.click();
  else toast(t('Add a group first.'), true);
};
$('btn-add-group').onclick = async () => {
  const r = await titleForm('group');
  if (r.action !== 'save' || !r.title) return;
  const page = state.draft.pages.find((p) => p.id === state.page) || state.draft.pages[0];
  page.groups.push({ id: `g-${Date.now().toString(36)}`, title: r.title, tiles: [] });
  commit();
};
$('btn-add-page').onclick = async () => {
  const r = await titleForm('page');
  if (r.action !== 'save' || !r.title) return;
  const id = `p-${Date.now().toString(36)}`;
  state.draft.pages.push({ id, title: r.title, groups: [] });
  state.page = id;
  commit();
};
$('btn-appearance').onclick = async () => {
  const r = await appearanceForm(state.draft);
  if (!r) return;
  state.draft.title = r.title;
  state.draft.theme = r.theme;
  state.draft.appearance = r.appearance;
  applyAppearance();
  updateDockerPill();
  commit();
};
$('btn-discovery').onclick = async () => {
  const discovery = state.draft.discovery;
  const result = await discoveryForm(discovery);
  if (!result) return;
  Object.assign(discovery, result);
  commit();
};
$('btn-json').onclick = async () => {
  const next = await jsonForm(stripRuntime(state.draft));
  if (next) { state.draft = next; state.draft.discovery ||= { docker: true, hidden: [] }; state.draft.discovery.hidden ||= []; commit(); }
};
document.addEventListener('keydown', (e) => {
  const anyDialogOpen = document.querySelector('dialog[open]');
  if (e.key === 'Escape' && !settingsPanel.hidden && !anyDialogOpen) {
    if (e.defaultPrevented) return;
    e.preventDefault();
    setSettingsOpen(false);
    settingsButton.focus({ preventScroll: true });
    return;
  }
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault();
    if (!anyDialogOpen && !document.body.classList.contains('login')) { filterInput.focus(); filterInput.select(); }
    return;
  }
  if (e.target.matches('input,textarea,select') || anyDialogOpen) return;
  if (e.key === '/' && !document.body.classList.contains('login')) { e.preventDefault(); filterInput.focus(); }
  if (e.key === 'e' && !e.metaKey && !e.ctrlKey && !e.altKey) {
    setSettingsOpen(false);
    enterEdit();
  }
  if (e.key === 'Escape' && state.editing) cancelEdit();
});
window.addEventListener('beforeunload', (e) => { if (state.editing && state.dirty) e.preventDefault(); });
enableDragDrop(app, moveTile);

boot().catch((e) => { console.error(e); toast(t('Could not start: {0}', e.message), true); });
