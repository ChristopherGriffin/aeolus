// Moving an AP to another folder (0076): it takes the new folder's settings
// from then on. Previewed and logged as any change is; the preview says
// which of the AP's own settings the new folder's locks would drop.

import { h } from '../dom.js';
import { group, value } from '../format.js';
import { ask, confirm, cancelButton } from './confirm.js';

// moveButton offers to move ap ({ id, name }) to another Locations folder,
// its form opening in box.
export function moveButton(ctx, ap, box, cls = 'rename') {
	return h('button', {
		type: 'button', class: `button small ${cls}`, title: 'Move this AP to another folder',
		onclick: () => moveForm(ctx, ap, box),
	}, 'Move…');
}

// folders lists the Locations folders an AP can go to, each by its path from
// the Org: every folder but the isolated ones, such as Landing Zone, and
// those below them.
function folders(ctx) {
	const t = ctx.trees.locations;
	const path = (n) => {
		const names = [];
		for (let x = n; x; x = t.nodes.get(x.parent)) names.unshift(x.name);
		return names.join(' › ');
	};
	const isolated = (n) => {
		for (let x = n; x; x = t.nodes.get(x.parent)) if (x.isolated) return true;
		return false;
	};
	return t.list.filter((n) => n.kind !== 'ap' && !isolated(n))
		.map((n) => ({ id: n.id, path: path(n) }))
		.sort((a, b) => a.path.localeCompare(b.path));
}

function moveForm(ctx, ap, box) {
	const parent = ctx.trees.locations.nodes.get(ap.id)?.parent;
	const choices = folders(ctx).filter((f) => f.id !== parent);
	if (!choices.length) {
		box.replaceChildren(h('div', { class: 'fieldform', 'data-editing': true },
			h('div', { class: 'sub' }, 'There is no other folder to move it to.'), h('div', { class: 'actions' }, cancelButton(box))));
		return;
	}
	const select = h('select', { 'aria-label': `New folder for ${ap.name}` },
		h('option', { value: '' }, 'Choose a folder'),
		choices.map((f) => h('option', { value: f.id }, f.path)));
	const out = h('div');
	const msg = h('div', { class: 'error' });
	const review = async () => {
		msg.replaceChildren();
		const dest = choices.find((f) => f.id === select.value);
		if (!dest) {
			msg.replaceChildren('Choose a folder.');
			return;
		}
		const op = { kind: 'move', tree: 'locations', node: ap.id, parent: dest.id };
		const p = await ask(out, op);
		if (!p) return;
		const from = parent ? ctx.name('locations', parent) : '—';
		const removed = p.effect?.removed || [];
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, `Move ${ap.name}: `), from, ' → ', dest.path),
			removed.length > 0 && h('div', null, h('strong', null, 'Dropped, as the new folder locks them: '),
				removed.map((o) => `${group(o.path).title} · ${group(o.path).label} (${value(o.path, o.value)})`).join(', ')),
		], [
			h('div', { class: 'sub warn' }, 'The AP takes its new folder\'s settings as it applies the config. Where its networks or radios differ, its Wi-Fi restarts and its clients drop for a few seconds.'),
		]);
	};
	box.replaceChildren(h('div', { class: 'fieldform', 'data-editing': true },
		h('label', { class: 'field' }, h('span', { class: 'label' }, `Move ${ap.name} to`), select),
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review'),
			cancelButton(box)),
		out));
	select.focus();
}
