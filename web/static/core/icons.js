// Inline SVG icon set (Lucide-style, 24px grid, 2px stroke). One source of
// truth so every control shares stroke weight and optical size.
const paths = {
  edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
  pin: '<path d="M12 17v5"/><path d="M9 3h6l-1 7 3 2v2H7v-2l3-2z"/>',
  eyeOff: '<path d="M9.9 4.2A10 10 0 0 1 12 4c7 0 10 8 10 8a17 17 0 0 1-2.2 3.2"/><path d="M6.6 6.6A17 17 0 0 0 2 12s3 8 10 8a10 10 0 0 0 5.4-1.6"/><path d="M14.1 14.1a3 3 0 1 1-4.2-4.2"/><path d="m2 2 20 20"/>',
  eye: '<path d="M2 12s3-8 10-8 10 8 10 8-3 8-10 8S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/>',
  up: '<path d="m18 15-6-6-6 6"/>',
  down: '<path d="m6 9 6 6 6-6"/>',
  left: '<path d="m15 18-6-6 6-6"/>',
  right: '<path d="m9 18 6-6-6-6"/>',
  restart: '<path d="M21 12a9 9 0 1 1-3-6.7"/><path d="M21 3v6h-6"/>',
  stop: '<rect x="6" y="6" width="12" height="12" rx="2"/>',
  play: '<path d="M7 4v16l13-8z"/>',
  plus: '<path d="M12 5v14"/><path d="M5 12h14"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>',
  settings: '<path d="M10 2h4l.6 2.4 1.7 1 2.4-.7 2 3.5-1.8 1.7v2.2l1.8 1.7-2 3.5-2.4-.7-1.7 1-.6 2.4h-4l-.6-2.4-1.7-1-2.4.7-2-3.5 1.8-1.7V9.9L3.3 8.2l2-3.5 2.4.7 1.7-1L10 2Z" transform="translate(0 1)"/><circle cx="12" cy="12" r="3"/>',
  logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/>',
  palette: '<circle cx="13.5" cy="6.5" r="1"/><circle cx="17.5" cy="10.5" r="1"/><circle cx="8.5" cy="7.5" r="1"/><circle cx="6.5" cy="12.5" r="1"/><path d="M12 2a10 10 0 0 0 0 20 2 2 0 0 0 1.9-2.6 2 2 0 0 1 1.9-2.4H18a4 4 0 0 0 4-4A10 10 0 0 0 12 2Z"/>',
  braces: '<path d="M8 3H7a2 2 0 0 0-2 2v5a2 2 0 0 1-2 2 2 2 0 0 1 2 2v5a2 2 0 0 0 2 2h1"/><path d="M16 21h1a2 2 0 0 0 2-2v-5a2 2 0 0 1 2-2 2 2 0 0 1-2-2V5a2 2 0 0 0-2-2h-1"/>',
  trash: '<path d="M3 6h18"/><path d="M8 6V4h8v2"/><path d="M19 6l-1 14H6L5 6"/><path d="M10 11v6m4-6v6"/>',
  x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
  check: '<path d="M20 6 9 17l-5-5"/>',
  rows: '<rect x="3" y="4" width="18" height="6" rx="1.5"/><rect x="3" y="14" width="18" height="6" rx="1.5"/>',
  layers: '<path d="m12 2 10 5-10 5L2 7Z"/><path d="m2 12 10 5 10-5"/><path d="m2 17 10 5 10-5"/>',
  file: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z"/><path d="M14 2v6h6"/>',
  undo: '<path d="M3 7v6h6"/><path d="M21 17a9 9 0 0 0-15-6.7L3 13"/>',
  external: '<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5"/>',
  moveTo: '<path d="M5 12h14"/><path d="m12 5 7 7-7 7"/>',
};

export function icon(name, size = 18) {
  const p = paths[name];
  if (!p) throw new Error(`unknown icon ${name}`);
  const tpl = document.createElement('template');
  tpl.innerHTML = `<svg class="ico" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">${p}</svg>`;
  return tpl.content.firstElementChild;
}

// Small button that opens url in a new tab. Used inside tiles, which are
// already links themselves, so a nested <a> would be invalid HTML.
export function openButton(url, label) {
  return iconButton('external', label, {
    class: 'small container-link',
    onclick: (e) => {
      e.preventDefault(); e.stopPropagation();
      window.open(url, '_blank', 'noopener');
    },
  });
}

// Icon-only button with a proper accessible name.
export function iconButton(name, label, attrs = {}) {
  const b = document.createElement('button');
  b.type = 'button';
  b.className = 'btn icon' + (attrs.class ? ' ' + attrs.class : '');
  b.setAttribute('aria-label', label);
  b.title = label;
  for (const [k, v] of Object.entries(attrs)) {
    if (k === 'class') continue;
    if (k.startsWith('on') && typeof v === 'function') b.addEventListener(k.slice(2).toLowerCase(), v);
    else if (k === 'disabled') b.disabled = !!v;
    else if (v !== undefined && v !== null && v !== false) b.setAttribute(k, v === true ? '' : v);
  }
  b.append(icon(name));
  return b;
}
