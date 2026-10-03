// Following the folder again (0046): values a node sets for itself are unset
// in one change, so it inherits from above once more. The preview says what
// each field becomes and where that comes from.

import { h } from '../dom.js';
import { group, value } from '../format.js';
import { ask, confirm } from './confirm.js';

// followButton offers to unset paths at node, previewing in box. parentName
// is the folder above, or null at the root, where nothing is above.
export function followButton(ctx, tree, node, nodeName, parentName, paths, box, label) {
	label ??= parentName ? `Follow ${parentName}` : 'Stop setting it here';
	return h('button', { type: 'button', class: 'button small', onclick: () => follow(ctx, tree, node, nodeName, parentName, paths, box) }, label);
}

async function follow(ctx, tree, node, nodeName, parentName, paths, box) {
	const op = paths.length === 1
		? { kind: 'unset', tree, node, path: paths[0] }
		: { kind: 'unset', tree, node, paths };
	const p = await ask(box, op);
	if (!p) return;
	const names = (id) => ctx.name('services', id);
	const before = paths.length === 1 ? { [paths[0]]: p.effect?.before } : (p.effect?.before || {});
	const radios = paths.some((x) => x.startsWith('radio.'));
	confirm(ctx, box, op, p, [
		h('div', null, h('strong', null, parentName ? `${nodeName} follows ${parentName} again` : `${nodeName} stops setting ${paths.length === 1 ? 'this' : 'these'}`)),
		h('ul', { class: 'becomes' }, paths.map((path) => {
			const g = group(path);
			const after = p.resolved?.[path];
			return h('li', null,
				h('span', null, `${g.title} · ${g.label}: `),
				value(path, before[path], names), ' → ',
				after ? [value(path, after.value, names), ` (from ${ctx.name(tree, after.from)})`] : h('span', { class: 'sealed' }, 'not set by Aeolus any more'));
		})),
	], [
		radios && h('div', { class: 'sub warn' }, 'Applying restarts each radio whose settings change; its clients drop briefly and reconnect.'),
	]);
}
