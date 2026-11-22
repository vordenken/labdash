import { t } from './i18n.js';
// Shared DOM helpers.

export function h(tag, attrs = {}, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === null || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k === 'text') el.textContent = v;
    // CSSOM assignment is allowed by the CSP; a style attribute is not.
    else if (k === 'style') el.style.cssText = v;
    else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2).toLowerCase(), v);
    else if (k === 'dataset') Object.assign(el.dataset, v);
    else if (v === true) el.setAttribute(k, '');
    else el.setAttribute(k, v);
  }
  for (const c of children.flat()) {
    if (c === null || c === undefined || c === false) continue;
    el.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return el;
}

export function clear(el) {
  while (el.firstChild) el.removeChild(el.firstChild);
  return el;
}

let toastTimer;
// toast(msg, true) for errors; toast(msg, { undo: fn }) adds an undo action.
export function toast(msg, opts = false) {
  const isError = opts === true;
  const undo = opts && typeof opts === 'object' ? opts.undo : null;
  const el = document.getElementById('toast');
  clear(el).append(document.createTextNode(msg));
  if (undo) {
    el.append(h('button', { class: 'btn small', text: t('Undo'), onclick: () => { el.hidden = true; undo(); } }));
  }
  el.className = 'toast glass' + (isError ? ' err' : '');
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.hidden = true; }, isError ? 6000 : undo ? 7000 : 2500);
}

// Picks a readable foreground for a hex background (WCAG relative luminance).
export function contrastFg(hex) {
  const m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex || '');
  if (!m) return '#0b0f17';
  const lin = (c) => { c = parseInt(c, 16) / 255; return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4; };
  const L = 0.2126 * lin(m[1]) + 0.7152 * lin(m[2]) + 0.0722 * lin(m[3]);
  // white text needs (1.05)/(L+0.05) >= 4.5  →  L <= 0.183
  return L <= 0.183 ? '#ffffff' : '#0b0f17';
}

export function uid(prefix = 't') {
  return `${prefix}-${Math.random().toString(36).slice(2, 8)}${Date.now().toString(36).slice(-3)}`;
}

// Resolves a tile icon to /icons/, where the server serves it from the icon
// folder or its download cache. The browser never loads images from other
// origins, so external URLs fall back to initials just like a missing icon.
export function iconEl(icon, name) {
  const box = h('div', { class: 'tile-icon' });
  const fallback = () => {
    box.className = 'tile-icon';
    box.textContent = initials(name);
  };
  const src = iconSrc(icon);
  if (!src) { fallback(); return box; }
  if (icon.startsWith('mdi-')) box.classList.add('mdi');
  box.append(h('img', { src, alt: '', loading: 'lazy', onerror: fallback }));
  return box;
}

export function iconSrc(icon) {
  if (!icon) return null;
  if (icon.startsWith('/') && !icon.startsWith('//')) return icon;
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(icon) || icon.includes('..')) return null;
  return `/icons/${icon}`;
}

const UNSAFE_SCHEMES = new Set(['javascript:', 'data:', 'vbscript:', 'blob:']);

// Returns href if it is safe to use as a link target, otherwise undefined.
// Mirrors config.SafeHref on the server for links from older configs.
export function safeHref(href) {
  if (!href || /[\u0000-\u0020\u007f]/.test(href)) return undefined;
  try {
    return UNSAFE_SCHEMES.has(new URL(href, 'http://labdash.invalid/').protocol) ? undefined : href;
  } catch {
    return undefined;
  }
}

function initials(name = '?') {
  const parts = name.trim().split(/\s+/);
  return (parts.length > 1 ? parts[0][0] + parts[1][0] : name.slice(0, 2)).toUpperCase();
}

export function relTime(unixSeconds) {
  if (!unixSeconds) return '';
  const diff = Date.now() / 1000 - unixSeconds;
  const units = [[86400, 'd'], [3600, 'h'], [60, 'min']];
  for (const [s, l] of units) if (diff >= s) return `${Math.floor(diff / s)}${l}`;
  return `${Math.floor(diff)}s`;
}

// Maps container state to the status class, label and severity used by
// widgets. severity: ok | warn | bad | sleep | unknown
export function containerStatus(c) {
  if (!c) return { cls: 'unknown', label: t('not found'), severity: 'unknown' };
  if (c.state === 'running') {
    if (c.health === 'unhealthy') return { cls: 'unhealthy', label: t('unhealthy'), severity: 'bad' };
    if (c.health === 'starting') return { cls: 'starting', label: t('starting'), severity: 'warn' };
    return { cls: 'running', label: c.health === 'healthy' ? t('running, healthy') : t('running'), severity: 'ok' };
  }
  if (isSleeping(c)) return { cls: 'sleeping', label: t('sleeping'), severity: 'sleep' };
  const map = { exited: t('stopped'), paused: t('paused'), restarting: t('restarting'), created: t('created'), dead: t('dead') };
  const severity = c.state === 'paused' || c.state === 'restarting' || c.state === 'created' ? 'warn' : 'bad';
  return { cls: c.state, label: map[c.state] || c.state, severity };
}

// URL that opens a problem container in an external management tool (e.g.
// Arcane), built from the LABDASH_CONTAINER_LINK template. Returns null for
// healthy, sleeping or unknown containers so the link only shows when useful.
export function containerLink(template, c) {
  if (!template || !c?.id) return null;
  const { severity } = containerStatus(c);
  if (severity !== 'bad' && severity !== 'warn') return null;
  const values = { id: c.id, name: c.name || '', project: c.project || '' };
  return safeHref(template.replace(/\{(id|name|project)\}/g, (_, k) => encodeURIComponent(values[k]))) ?? null;
}

function isSleeping(c) {
  return c.state === 'exited' && c.sablier;
}

export function containerBuckets(containers) {
  const buckets = { running: [], sleeping: [], stopped: [], unhealthy: [] };
  for (const c of Object.values(containers)) {
    if (c.state === 'running') {
      buckets.running.push(c);
      if (c.health === 'unhealthy') buckets.unhealthy.push(c);
    } else {
      buckets[isSleeping(c) ? 'sleeping' : 'stopped'].push(c);
    }
  }
  return buckets;
}

// Applies state-* classes to a tile element from a status.
export function applyTileState(el, s) {
  el.classList.remove('state-bad', 'state-warn', 'state-sleep');
  if (s.severity === 'bad') el.classList.add('state-bad');
  else if (s.severity === 'warn') el.classList.add('state-warn');
  else if (s.severity === 'sleep') el.classList.add('state-sleep');
}

export function statusDot(s) {
  const dot = h('span', { role: 'img' });
  updateStatusDot(dot, s);
  return dot;
}

export function updateStatusDot(dot, s) {
  dot.className = `status ${s.cls}`;
  dot.title = t('Status: {0}', s.label);
  dot.setAttribute('aria-label', dot.title);
}
