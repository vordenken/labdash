import { t, locale } from '../core/i18n.js';
import { register } from '../core/registry.js';
import { h } from '../core/dom.js';

// Example of a purely client-side widget with its own schema.
register({
  type: 'clock',
  label: t('Clock'),
  schema: [
    { key: 'seconds', label: t('Show seconds'), type: 'boolean', default: false },
    { key: 'format', label: t('Date format'), type: 'select', default: 'long',
      options: [{ value: 'long', label: t('Long (weekday and month)') }, { value: 'short', label: t('Short (numeric)') }] },
  ],
  render(el, tile) {
    const p = tile.params || {};
    const time = h('div', { class: 'clock' });
    const date = h('div', { class: 'clock-date' });
    el.append(h('div', { class: 'tile-body' }, time, date));
    const tick = () => {
      const now = new Date();
      time.textContent = now.toLocaleTimeString(locale, {
        hour: '2-digit', minute: '2-digit', second: p.seconds ? '2-digit' : undefined,
      });
      date.textContent = now.toLocaleDateString(locale, p.format === 'short'
        ? { day: '2-digit', month: '2-digit', year: 'numeric' }
        : { weekday: 'long', day: 'numeric', month: 'long' });
    };
    tick();
    const timer = setInterval(tick, p.seconds ? 1000 : 10000);
    return { destroy: () => clearInterval(timer) };
  },
});
