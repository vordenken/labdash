import { t } from '../core/i18n.js';
import { register } from '../core/registry.js';
import { h, iconEl, safeHref, containerStatus, containerLink, applyTileState, statusDot, updateStatusDot, uid } from '../core/dom.js';
import { openButton } from '../core/icons.js';

// Default tile: icon, name, description and (if bound) container status
// shown as a dot, with visible text for non-running or unhealthy states.
register({
  type: 'link',
  label: 'Link / App',
  schema: [],
  render(el, tile, ctx) {
    const bound = !!tile.container;
    const dot = bound ? statusDot(containerStatus(ctx.docker.containers[tile.container])) : null;
    if (dot) dot.id = uid('status');
    const label = h('span', { class: 'state-label' });
    const foot = h('div', { class: 'tile-foot' }, label);
    const head = h('div', { class: 'tile-head' },
      iconEl(tile.icon, tile.name),
      h('div', { class: 'tile-text' },
        h('div', { class: 'tile-name', text: tile.name || tile.container || '–' }),
        tile.description ? h('div', { class: 'tile-desc', text: tile.description }) : null,
      ),
      dot,
    );
    const href = safeHref(tile.href);
    const wrap = href
      ? h('a', { href, target: '_blank', rel: 'noopener', 'aria-label': tile.name ? t('Open {0}', tile.name) : undefined, 'aria-describedby': dot?.id }, head, bound ? foot : null)
      : h('div', { class: 'tile-body' }, head, bound ? foot : null);
    el.append(wrap);

    const update = (c) => {
      if (!bound) return;
      const s = containerStatus(c.docker.containers[tile.container]);
      updateStatusDot(dot, s);
      applyTileState(el, s);
      foot.hidden = s.severity === 'ok';
      label.className = `state-label ${s.severity}`;
      label.textContent = s.label;
      foot.querySelector('.container-link')?.remove();
      const ctr = c.docker.containers[tile.container];
      const url = containerLink(c.session?.containerLink, ctr);
      if (url) foot.append(openButton(url, t('Open {0} in container management', tile.name || ctr.name)));
    };
    update(ctx);
    return { update };
  },
});
