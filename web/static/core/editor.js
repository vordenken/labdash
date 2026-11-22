import { t } from './i18n.js';
// Editor: schema-driven forms and drag & drop reordering of a config draft.
import { h, clear, uid, contrastFg } from './dom.js';
import { icon, iconButton } from './icons.js';
import { all as allWidgets, get as getWidget, baseSchema } from './registry.js';

const dialog = () => document.getElementById('dialog');

let dialogSeq = 0;
function openDialog(formEl, { wide = false } = {}) {
  const d = dialog();
  clear(d).append(formEl);
  d.classList.toggle('wide', wide);
  const heading = formEl.querySelector('h3');
  if (heading) {
    heading.id ||= `dlg-title-${++dialogSeq}`;
    d.setAttribute('aria-labelledby', heading.id);
  }
  d.returnValue = '';
  d.showModal();
  return new Promise((resolve) => {
    d.onclose = () => { const v = d.returnValue || 'cancel'; clear(d); resolve(v); };
  });
}

function field(spec, value, extra = {}) {
  const id = `f-${spec.key}`;
  let input;
  switch (spec.type) {
    case 'boolean':
      input = h('input', { type: 'checkbox', id, name: spec.key });
      input.checked = value ?? spec.default ?? false;
      return h('label', { class: 'check' }, input, spec.label, spec.help ? h('span', { class: 'field-help', text: spec.help }) : null);
    case 'select':
      input = h('select', { id, name: spec.key },
        ...(spec.options || []).map((o) => h('option', { value: o.value, text: o.label })));
      input.value = value ?? spec.default ?? '';
      break;
    case 'textarea':
      input = h('textarea', { id, name: spec.key, rows: 4 });
      input.value = value ?? spec.default ?? '';
      break;
    case 'number':
      input = h('input', { type: 'number', id, name: spec.key, step: 'any' });
      input.value = value ?? spec.default ?? '';
      break;
    case 'container':
      input = h('input', { type: 'text', id, name: spec.key, list: 'containers-list', autocomplete: 'off' });
      input.value = value ?? '';
      break;
    default:
      input = h('input', { type: spec.type === 'url' ? 'url' : 'text', id, name: spec.key, placeholder: spec.placeholder || '' });
      input.value = value ?? spec.default ?? '';
  }
  Object.assign(input, extra);
  if (spec.required) input.required = true;
  return h('label', { class: 'field' + (spec.required ? ' required' : '') }, h('span', { text: spec.label }), input, spec.help ? h('span', { class: 'field-help', text: spec.help }) : null);
}

function readForm(form, schema) {
  const out = {};
  for (const spec of schema) {
    const el = form.elements[spec.key];
    if (!el) continue;
    if (spec.type === 'boolean') out[spec.key] = el.checked;
    else if (spec.type === 'number') out[spec.key] = el.value === '' ? undefined : Number(el.value);
    else out[spec.key] = el.value.trim() === '' ? undefined : el.value.trim();
  }
  return out;
}

function containerDatalist(containers) {
  const dl = h('datalist', { id: 'containers-list' });
  for (const name of Object.keys(containers || {}).sort()) dl.append(h('option', { value: name }));
  return dl;
}

// Returns { action: 'save'|'delete'|'cancel', tile }.
export async function tileForm(tile, containers) {
  const isNew = !tile.id;
  const draft = structuredClone(tile);
  draft.params ||= {};
  const typeSel = h('select', { name: 'type' },
    ...allWidgets().map((w) => h('option', { value: w.type, text: w.label })));
  typeSel.value = draft.type || 'link';
  const paramsBox = h('div', { style: 'display:grid;gap:.8rem' });
  const renderParams = () => {
    clear(paramsBox);
    for (const spec of getWidget(typeSel.value).schema) paramsBox.append(field(spec, draft.params[spec.key]));
  };
  typeSel.onchange = renderParams;
  renderParams();

  const form = h('form', { method: 'dialog' },
    h('h3', { text: isNew ? t('Add tile') : t('Edit tile') }),
    h('label', { class: 'field' }, t('Widget type'), typeSel),
    ...baseSchema.map((s) => field(s, draft[s.key])),
    paramsBox,
    containerDatalist(containers),
    h('div', { class: 'form-actions' },
      isNew ? null : h('button', { class: 'btn danger', value: 'delete', formnovalidate: true }, icon('trash'), t('Delete')),
      h('button', { class: 'btn', value: 'cancel', text: t('Cancel'), formnovalidate: true }),
      h('button', { class: 'btn primary', value: 'save', text: t('Done') }),
    ),
  );
  const result = await openDialog(form);
  if (result !== 'save') return { action: result };
  const base = readForm(form, baseSchema);
  const type = typeSel.value;
  const params = readForm(form, getWidget(type).schema);
  for (const k of Object.keys(params)) if (params[k] === undefined) delete params[k];
  return {
    action: 'save',
    tile: {
      id: draft.id || uid('t'),
      type,
      ...base,
      params: Object.keys(params).length ? params : undefined,
    },
  };
}

export async function titleForm(kind, current) {
  const form = h('form', { method: 'dialog' },
    h('h3', { text: kind === 'page'
      ? t(current === undefined ? 'Add page' : 'Rename page')
      : t(current === undefined ? 'Add group' : 'Rename group') }),
    field({ key: 'title', label: t('Title'), type: 'text', required: true }, current, { autofocus: true }),
    h('div', { class: 'form-actions' },
      current === undefined ? null : h('button', { class: 'btn danger', value: 'delete', formnovalidate: true }, icon('trash'), t('Delete')),
      h('button', { class: 'btn', value: 'cancel', text: t('Cancel'), formnovalidate: true }),
      h('button', { class: 'btn primary', value: 'save', text: t('Done') }),
    ),
  );
  const result = await openDialog(form);
  return { action: result, title: form.elements.title.value.trim() };
}

export async function jsonForm(cfg) {
  const ta = h('textarea', { class: 'json', name: 'json', spellcheck: 'false' });
  ta.value = JSON.stringify(cfg, null, 2);
  const err = h('div', { class: 'error' });
  const form = h('form', { method: 'dialog' },
    h('h3', { text: t('Configuration (JSON)') }),
    h('label', { class: 'field' }, h('span', { text: t('config.json – do not store passwords or API keys here') }), ta),
    err,
    h('div', { class: 'form-actions' },
      h('button', { class: 'btn', value: 'cancel', text: t('Cancel'), formnovalidate: true }),
      h('button', { class: 'btn primary', value: 'save', text: t('Done') }),
    ),
  );
  form.addEventListener('submit', (e) => {
    if (e.submitter?.value !== 'save') return;
    try { JSON.parse(ta.value); } catch (ex) {
      e.preventDefault();
      const pos = /position (\d+)/.exec(ex.message)?.[1];
      const line = pos ? ta.value.slice(0, Number(pos)).split('\n').length : null;
      const detail = ex.message.replace(/^JSON\.parse: /, '');
      err.textContent = line ? t('Invalid JSON on line {0}: {1}', line, detail) : t('Invalid JSON: {0}', detail);
    }
  });
  const result = await openDialog(form, { wide: true });
  return result === 'save' ? JSON.parse(ta.value) : null;
}

export async function confirmDialog(text) {
  const form = h('form', { method: 'dialog' },
    h('p', { text }),
    h('div', { class: 'form-actions' },
      h('button', { class: 'btn', value: 'cancel', text: t('Cancel') }),
      h('button', { class: 'btn primary', value: 'ok', text: 'OK', autofocus: true }),
    ),
  );
  return (await openDialog(form)) === 'ok';
}

// --- drag & drop ----------------------------------------------------------

// Wires HTML5 drag & drop on tiles within root. moveTile(tileId, fromGroupId,
// toGroupId, index) is invoked on drop.
export function enableDragDrop(root, moveTile) {
  let dragging = null;
  root.addEventListener('dragstart', (e) => {
    const tile = e.target.closest('.tile[draggable="true"]');
    if (!tile) return;
    dragging = { tileId: tile.dataset.tileId, groupId: tile.dataset.groupId };
    tile.classList.add('dragging');
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', tile.dataset.tileId);
  });
  root.addEventListener('dragend', () => {
    dragging = null;
    root.querySelectorAll('.dragging,.drop-before,.drop-after').forEach((n) => n.classList.remove('dragging', 'drop-before', 'drop-after'));
    root.querySelectorAll('.drop-target').forEach((n) => n.classList.remove('drop-target'));
  });
  root.addEventListener('dragover', (e) => {
    if (!dragging) return;
    const tiles = e.target.closest('.tiles');
    if (!tiles) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    root.querySelectorAll('.drop-before,.drop-after').forEach((n) => n.classList.remove('drop-before', 'drop-after'));
    root.querySelectorAll('.drop-target').forEach((n) => n.classList.remove('drop-target'));
    const over = e.target.closest('.tile:not(.add)');
    if (over && over.dataset.tileId !== dragging.tileId) {
      const r = over.getBoundingClientRect();
      over.classList.add(e.clientX < r.left + r.width / 2 ? 'drop-before' : 'drop-after');
    } else {
      tiles.classList.add('drop-target');
    }
  });
  root.addEventListener('drop', (e) => {
    if (!dragging) return;
    const tiles = e.target.closest('.tiles');
    if (!tiles) return;
    e.preventDefault();
    const toGroup = tiles.dataset.groupId;
    const over = e.target.closest('.tile:not(.add)');
    let index = tiles.querySelectorAll('.tile:not(.add)[data-tile-id]').length;
    if (over && over.dataset.tileId !== dragging.tileId) {
      const siblings = [...tiles.querySelectorAll('.tile:not(.add)[data-tile-id]')];
      index = siblings.indexOf(over) + (over.classList.contains('drop-after') ? 1 : 0);
    }
    moveTile(dragging.tileId, dragging.groupId, toGroup, index);
    dragging = null;
  });
}

// --- appearance -----------------------------------------------------------

export const ACCENTS = {
  indigo: '#6c8cff', teal: '#2dd4bf', amber: '#f6b73c', rose: '#fb7185', violet: '#a78bfa', emerald: '#34d399',
};

export function accentColor(accent) {
  return ACCENTS[accent] || (/^#[0-9a-fA-F]{6}$/.test(accent || '') ? accent : ACCENTS.indigo);
}

export async function appearanceForm(cfg) {
  const ap = { ...cfg.appearance };
  const custom = h('input', { type: 'color', name: 'custom', 'aria-label': t('Custom accent color') });
  custom.value = accentColor(ap.accent);
  const swatches = h('div', { class: 'swatches', role: 'group' });
  const paint = () => {
    swatches.querySelectorAll('.swatch').forEach((sw) => sw.setAttribute('aria-pressed', String(sw.dataset.accent === ap.accent)));
    document.documentElement.style.setProperty('--accent', accentColor(ap.accent));
    document.documentElement.style.setProperty('--accent-fg', contrastFg(accentColor(ap.accent)));
  };
  for (const [name, color] of Object.entries(ACCENTS)) {
    swatches.append(h('button', { type: 'button', class: 'swatch', 'aria-label': t('Accent: {0}', name), 'aria-pressed': 'false', style: `--c:${color}`, dataset: { accent: name },
      onclick: () => { ap.accent = name; custom.value = color; paint(); } }, icon('check', 16)));
  }
  custom.oninput = () => { ap.accent = custom.value; paint(); };
  swatches.append(custom);
  paint();

  const originalAccent = accentColor(cfg.appearance.accent);
  const form = h('form', { method: 'dialog' },
    h('h3', { text: t('Appearance') }),
    h('div', { class: 'form-row' },
      field({ key: 'title', label: t('Title'), type: 'text' }, cfg.title),
      field({ key: 'theme', label: t('Default theme'), type: 'select', options: [
        { value: 'dark', label: t('Dark') }, { value: 'light', label: t('Light') }, { value: 'auto', label: 'System' }] }, cfg.theme),
    ),
    h('fieldset', { class: 'field' }, h('legend', { text: t('Accent color') }), swatches),
    field({ key: 'hero', label: t('Show greeting, time and container summary'), type: 'boolean' }, ap.hero),
    field({ key: 'name', label: t('Greeting name'), type: 'text', help: t('Leave blank for a neutral greeting.') }, ap.name),
    h('div', { class: 'form-actions' },
      h('button', { class: 'btn', value: 'cancel', text: t('Cancel'), formnovalidate: true }),
      h('button', { class: 'btn primary', value: 'save', text: t('Done') }),
    ),
  );
  const result = await openDialog(form);
  if (result !== 'save') {
    document.documentElement.style.setProperty('--accent', originalAccent);
    document.documentElement.style.setProperty('--accent-fg', contrastFg(originalAccent));
    return null;
  }
  return {
    title: form.elements.title.value.trim() || cfg.title,
    theme: form.elements.theme.value,
    appearance: { accent: ap.accent, hero: form.elements.hero.checked, name: form.elements.name.value.trim() },
  };
}

export async function discoveryForm(discovery) {
  const homepage = h('input', { type: 'checkbox', autofocus: true, 'aria-describedby': 'homepage-help' });
  homepage.checked = discovery.homepage === true;
  const hidden = [...discovery.hidden];
  const hiddenList = h('div', { class: 'hidden-list' });
  const renderHidden = () => {
    clear(hiddenList);
    if (!hidden.length) { hiddenList.append(h('span', { class: 'field-help', text: t('No hidden containers.') })); return; }
    for (const name of hidden) {
      hiddenList.append(h('span', { class: 'chip' }, name,
        iconButton('x', t('Show {0} again', name), { class: 'small ghost', onclick: () => { hidden.splice(hidden.indexOf(name), 1); renderHidden(); } })));
    }
  };
  renderHidden();

  const form = h('form', { method: 'dialog' },
    h('h3', { text: t('Docker discovery') }),
    h('div', {},
      h('label', { class: 'check' }, homepage, t('Discover from Homepage labels')),
      h('p', { id: 'homepage-help', class: 'field-help', text: t('Uses homepage.* labels as a fallback to labdash.*. This changes automatic tiles after you save; saved tiles and container status are not affected.') }),
    ),
    h('fieldset', { class: 'field' }, h('legend', { text: t('Hidden containers') }), hiddenList),
    h('div', { class: 'form-actions' },
      h('button', { class: 'btn', value: 'cancel', text: t('Cancel') }),
      h('button', { class: 'btn primary', value: 'save', text: t('Done') }),
    ),
  );
  return await openDialog(form) === 'save' ? { homepage: homepage.checked, hidden } : null;
}
