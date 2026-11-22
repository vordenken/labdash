import { t } from '../core/i18n.js';
import { register } from '../core/registry.js';
import { h, clear, iconEl, safeHref, containerStatus, containerLink, applyTileState, statusDot, updateStatusDot, uid, relTime } from '../core/dom.js';
import { iconButton, openButton } from '../core/icons.js';

// Container card: state, uptime and start/stop/restart controls.
register({
  type: 'docker',
  label: t('Docker container'),
  schema: [
    { key: 'showImage', label: t('Show image name instead of description'), type: 'boolean', default: false },
  ],
  render(el, tile, ctx) {
    const p = tile.params || {};
    const dot = statusDot(containerStatus(ctx.docker.containers[tile.container]));
    dot.id = uid('status');
    const desc = h('div', { class: 'tile-desc' });
    const foot = h('div', { class: 'tile-foot' });
    const head = h('div', { class: 'tile-head' },
      iconEl(tile.icon, tile.name || tile.container),
      h('div', { class: 'tile-text' }, h('div', { class: 'tile-name', text: tile.name || tile.container }), desc),
      dot);
    const href = safeHref(tile.href);
    if (href) {
      el.append(h('a', { href, target: '_blank', rel: 'noopener', 'aria-label': t('Open {0}', tile.name || tile.container), 'aria-describedby': dot.id }, head, foot));
      foot.addEventListener('click', (e) => { if (e.target.closest('button')) e.preventDefault(); });
    } else {
      el.append(h('div', { class: 'tile-body' }, head, foot));
    }

    let pending = null;
    const update = (c) => {
      const ctr = c.docker.containers[tile.container];
      const s = containerStatus(ctr);
      updateStatusDot(dot, s);
      applyTileState(el, s);
      desc.textContent = p.showImage && ctr ? ctr.image : (tile.description || '');
      desc.hidden = !desc.textContent;
      clear(foot);
      if (!ctr) { foot.append(h('span', { class: 'state-label bad', text: t('Container not found') })); return; }
      if (pending || s.severity !== 'ok') foot.append(h('span', { class: `state-label ${s.severity}`, text: pending ? `${pending}…` : s.label }));
      if (ctr.state === 'running' && ctr.created && !pending) foot.append(h('span', { class: 'uptime', text: t('Created {0} ago', relTime(ctr.created)) }));
      const url = containerLink(c.session?.containerLink, ctr);
      const manage = url ? openButton(url, t('Open {0} in container management', tile.name || ctr.name)) : null;
      if (c.session.containerActions) {
        const name = tile.name || ctr.name;
        const act = (verb, word) => async (e) => {
          e.preventDefault(); e.stopPropagation();
          if (verb !== 'start' && !confirm(t(verb === 'stop' ? 'Stop {0}?' : 'Restart {0}?', name))) return;
          pending = word;
          update(c);
          await c.action(ctr.name, verb);
          pending = null;
        };
        const btns = h('div', { class: 'tile-actions' });
        if (ctr.state === 'running') {
          btns.append(iconButton('restart', t('Restart {0}', name), { class: 'small', disabled: !!pending, onclick: act('restart', t('restarting')) }));
          btns.append(iconButton('stop', t('Stop {0}', name), { class: 'small danger', disabled: !!pending, onclick: act('stop', t('stopping')) }));
        } else {
          btns.append(iconButton('play', t('Start {0}', name), { class: 'small', disabled: !!pending, onclick: act('start', t('starting')) }));
        }
        if (manage) btns.prepend(manage);
        foot.append(btns);
      } else if (manage) {
        foot.append(h('div', { class: 'tile-actions' }, manage));
      }
    };
    update(ctx);
    return { update };
  },
});
