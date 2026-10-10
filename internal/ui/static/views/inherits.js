// The bar above a node's settings of one kind, its band settings or its
// ports (Griff, 2026-10-09): Inherits from the folder directly above, which
// is where a node inherits from, with an arrow to the same view there; on
// the right, Customize, or, where the node sets some of them itself,
// Customized here with Inherit again. A node that sets none of them shows
// them greyed out until Customize is pressed. The Org, with nothing above,
// has no bar, and its settings are open.

import { h } from '../dom.js';
import { followButton } from './follow.js';

// inheritsBar builds the bar for the fields starting with prefix, or also,
// linking to tab (such as interfaces/radios) on the folder above. onToggle(open) is
// called when Customize is pressed, or pressed again to cancel. It returns
// the bar, null on the Org, and whether the settings start open.
export function inheritsBar(ctx, node, page, { prefix, also, tab, box, onToggle }) {
	const may = page.role === 'operator' || page.role === 'admin';
	const above = (page.ancestry || []).filter((a) => a !== node);
	const setHere = Object.entries(page.fields || {}).filter(([p, r]) => (p.startsWith(prefix) || (also && p.startsWith(also))) && r.from === node).map(([p]) => p);
	if (!above.length) return { bar: null, open: true };
	const parent = above[above.length - 1];
	const parentName = ctx.name('locations', parent);
	const go = h('a', { class: 'go', href: `#/locations/${encodeURIComponent(parent)}/${tab}`, title: `Go to ${parentName}` }, '→');
	let right = null;
	if (setHere.length) {
		right = [h('span', { class: 'chip' }, 'Customized here'),
			may && followButton(ctx, 'locations', node, page.node.name, parentName, setHere, box, 'Inherit again')];
	} else if (may) {
		let open = false;
		right = h('button', { type: 'button', class: 'button small primary', onclick: (e) => {
			open = !open;
			e.currentTarget.textContent = open ? 'Cancel' : 'Customize';
			e.currentTarget.className = open ? 'button small' : 'button small primary';
			onToggle(open);
		} }, 'Customize');
	}
	const bar = h('div', { class: 'inherits' }, h('span', { class: 'label' }, 'Inherits from'),
		h('strong', { class: 'from' }, parentName), go, h('span', { class: 'gap' }), right);
	return { bar, open: setHere.length > 0 };
}
