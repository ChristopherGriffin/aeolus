// Removing a folder that holds nothing (0125): no folder and no AP in it,
// and nothing that still leans on it, which the manager says in the preview
// where something does. What was set at it goes with it, and is listed
// first. Previewed and logged as any change is.

import { h } from '../dom.js';
import { group, value } from '../format.js';
import { ask, confirm } from './confirm.js';

// builtIn says whether a folder is one every Org has (0032), which stays:
// Sandbox in both trees, Landing Zone in Locations.
const builtIn = (tree, id) => id === 'sandbox' || (tree === 'locations' && id === 'landing-zone');

// removable says whether the page may offer to remove node: a folder that
// is not the Org's, not built in, and has nothing below it.
export function removable(ctx, tree, node) {
	return node.kind !== 'org' && !node.isolated && !builtIn(tree, node.id) && !ctx.trees[tree].list.some((n) => n.parent === node.id);
}

// removeFolderButton offers to remove node ({ id, name }) from tree, its
// preview opening in box. Once removed, the page is its parent's.
export function removeFolderButton(ctx, tree, node, box, cls = 'rename') {
	return h('button', {
		type: 'button', class: `button small ${cls}`, title: 'Remove this folder: it holds nothing',
		onclick: async () => {
			const out = h('div');
			box.replaceChildren(out);
			const op = { kind: 'remove-folder', tree, node: node.id };
			const p = await ask(out, op);
			if (!p) return;
			const parent = ctx.trees[tree].nodes.get(node.id)?.parent;
			const set = p.effect?.removed || [];
			confirm(ctx, out, op, p, [
				h('div', null, h('strong', null, 'Remove the folder: '), node.name),
				set.length > 0
					? h('div', null, h('strong', null, 'What is set at it goes with it: '), set.map((o) => `${group(o.path).title} · ${group(o.path).label} (${value(o.path, o.value, (id) => ctx.name('services', id))})`).join(', '))
					: h('div', { class: 'sub' }, 'Nothing is set at it.'),
				h('div', { class: 'sub' }, 'Roles granted on it go with it too. It cannot be brought back, only made again.'),
			], [], () => { location.hash = `#/${tree}${parent ? `/${encodeURIComponent(parent)}` : ''}`; });
		},
	}, 'Remove…');
}
