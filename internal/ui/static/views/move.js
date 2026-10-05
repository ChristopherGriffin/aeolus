// Moving an AP or a folder to another folder (0076): an AP takes its new
// folder's settings from then on; a folder keeps everything set on it and
// inside it, and what it inherited comes from its new place. Previewed and
// logged as any change is; the preview says which settings the new place's
// locks would drop.

import { h } from '../dom.js';
import { group, value } from '../format.js';
import { ask, confirm, cancelButton } from './confirm.js';

// moveButton offers to move node ({ id, name, kind }) in tree to another
// folder, its form opening in box.
export function moveButton(ctx, tree, node, box, cls = 'rename') {
	const ap = node.kind === 'ap';
	return h('button', {
		type: 'button', class: `button small ${cls}`,
		title: ap ? 'Move this AP to another folder' : 'Move this folder, with everything in it, into another folder',
		onclick: () => moveForm(ctx, tree, node, box),
	}, 'Move…');
}

// destinations lists the folders node can go to in tree, each by its path
// from the Org: not itself or anything inside it, not where it is now, and
// not an isolated folder, such as Landing Zone, or anything inside one.
function destinations(ctx, tree, node) {
	const t = ctx.trees[tree];
	const up = (n, test) => {
		for (let x = n; x; x = t.nodes.get(x.parent)) if (test(x)) return true;
		return false;
	};
	const path = (n) => {
		const names = [];
		for (let x = n; x; x = t.nodes.get(x.parent)) names.unshift(x.name);
		return names.join(' › ');
	};
	const parent = t.nodes.get(node.id)?.parent;
	return t.list
		.filter((n) => n.kind !== 'ap' && n.id !== parent && !up(n, (x) => x.isolated || x.id === node.id))
		.map((n) => ({ id: n.id, path: path(n) }))
		.sort((a, b) => a.path.localeCompare(b.path));
}

function moveForm(ctx, tree, node, box) {
	const ap = node.kind === 'ap';
	const parent = ctx.trees[tree].nodes.get(node.id)?.parent;
	const choices = destinations(ctx, tree, node);
	if (!choices.length) {
		box.replaceChildren(h('div', { class: 'fieldform', 'data-editing': true },
			h('div', { class: 'sub' }, 'There is no other folder to move it to.'), h('div', { class: 'actions' }, cancelButton(box))));
		return;
	}
	const select = h('select', { 'aria-label': `New folder for ${node.name}` },
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
		const op = { kind: 'move', tree, node: node.id, parent: dest.id };
		const p = await ask(out, op);
		if (!p) return;
		const from = parent ? ctx.name(tree, parent) : '—';
		const removed = p.effect?.removed || [];
		const label = (o) => `${o.node !== node.id ? `${ctx.name(tree, o.node)}: ` : ''}${group(o.path).title} · ${group(o.path).label} (${value(o.path, o.value)})`;
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, `Move ${node.name}: `), from, ' → ', dest.path),
			!ap && h('div', { class: 'sub' }, `Everything set on ${node.name} and inside it stays. What it inherited from ${from} now comes from ${dest.path}.`),
			removed.length > 0 && h('div', null, h('strong', null, 'Dropped, as the new place locks them: '), removed.map(label).join(', ')),
		], [
			h('div', { class: 'sub warn' }, ap
				? 'The AP takes its new folder\'s settings as it applies the config. Where its networks or radios differ, its Wi-Fi restarts and its clients drop for a few seconds.'
				: 'Each AP listed takes what it now inherits as it applies the config. Where its networks or radios differ, its Wi-Fi restarts and its clients drop for a few seconds.'),
		]);
	};
	box.replaceChildren(h('div', { class: 'fieldform', 'data-editing': true },
		h('label', { class: 'field' }, h('span', { class: 'label' }, `Move ${node.name} into`), select),
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review'),
			cancelButton(box)),
		out));
	select.focus();
}
