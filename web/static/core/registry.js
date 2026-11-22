import { t } from './i18n.js';
// Widget registry. A widget module calls register() with:
//   type:   unique id (matches tile.type)
//   label:  human readable name shown in the editor
//   schema: array of { key, label, type, default, help, options }
//           type ∈ text | url | number | boolean | select | textarea
//   render(el, tile, ctx): fills the tile body; may return { update(ctx), destroy() }
//
// ctx = { docker: {containers}, session, editing, action(name, verb) }

const widgets = new Map();

export function register(def) {
  if (!def || !def.type) throw new Error('widget needs a type');
  widgets.set(def.type, { schema: [], ...def });
}

export function get(type) {
  return widgets.get(type) || widgets.get('link');
}

export function all() {
  return [...widgets.values()].sort((a, b) => a.label.localeCompare(b.label));
}

// Common fields every tile has regardless of widget type.
export const baseSchema = [
  { key: 'name', label: 'Name', type: 'text', required: true },
  { key: 'href', label: 'Link (URL)', type: 'url', help: t('Open this address when the tile is clicked.') },
  { key: 'icon', label: 'Icon', type: 'text', help: t('Icon name like “jellyfin” (Dashboard Icons) or “mdi-home” (Material Design Icons), or a file in your icon folder. Leave blank for initials.') },
  { key: 'description', label: t('Description'), type: 'text' },
  { key: 'container', label: t('Docker container'), type: 'container', help: t('Link a container by name to show its live status. No labels are required.') },
  { key: 'size', label: t('Size'), type: 'select', default: '', options: [{ value: '', label: 'Normal' }, { value: 'wide', label: t('Wide (2 columns)') }] },
];
