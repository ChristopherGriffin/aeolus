// Pieces several pages share: the tree on the left, the path to a node, and
// what state an AP is in.

import { h, link, icon } from './dom.js';
import { ago } from './format.js';

// treeAside draws a tree on the left, as in the mockup. status maps an AP
// to how it is doing (apStatus), for the dot beside its name.
export function treeAside(ctx, tree, selected, status) {
	const t = ctx.trees[tree];
	const depth = (id) => {
		let d = 0;
		for (let n = t.nodes.get(id); n && n.parent && t.nodes.has(n.parent); n = t.nodes.get(n.parent)) d++;
		return d;
	};
	const inBranch = (id) => {
		for (let n = t.nodes.get(id); n; n = t.nodes.get(n.parent)) if (n.broken) return true;
		return false;
	};
	return h('aside', { class: 'tree' },
		h('div', { class: 'tree-title' }, tree === 'locations' ? 'Locations' : 'Services'),
		t.list.map((n) => {
			const ap = n.kind === 'ap';
			const href = ap ? `/aps/${encodeURIComponent(n.id)}` : `/${tree}/${encodeURIComponent(n.id)}`;
			const cls = [n.id === selected ? 'on' : '', inBranch(n.id) ? 'branch' : ''].join(' ').trim();
			const st = ap && status ? status.get(n.id) : null;
			return h('a', { href: '#' + href, class: cls || null, style: { '--depth': depth(n.id) } },
				icon(ap ? 'ap' : 'folder'),
				h('span', { class: 'name' }, n.name),
				n.broken && h('span', { class: 'chip break' }, 'BREAK'),
				st && h('span', { class: 'dot ' + st.cls, title: st.label }));
		}));
}

// crumbs draws the path above a node.
export function crumbs(ctx, tree, ancestry) {
	return h('nav', { class: 'crumbs', 'aria-label': 'Path' },
		ancestry.slice(0, -1).map((id, i) => [i > 0 && '›', link(`/${tree}/${encodeURIComponent(id)}`, ctx.name(tree, id))]));
}

// A quiet AP is one not heard from in three poll intervals of the default
// length.
const QUIET = 180;

// apStatus sums up an AP from the fleet view (GET /v1/aps): a dot class, a
// short label, and a sentence.
export function apStatus(a) {
	const seen = a.seen?.at;
	const quiet = seen && (Date.now() - new Date(seen).getTime()) / 1000 > QUIET;
	if (a.config === 'unassigned') return { cls: 'idle', chip: 'idle', label: 'In Landing Zone', detail: 'Waiting for a person to adopt it.' };
	if (!seen) return { cls: 'idle', chip: 'idle', label: 'Never seen', detail: 'It has not called the manager.' };
	if (quiet) return { cls: 'bad', chip: 'bad', label: 'Not heard from', detail: 'Last seen ' + ago(seen) + '.' };
	if (a.config === 'held') return { cls: 'bad', chip: 'bad', label: 'Held', detail: `Its config breaks ${a.problems} rule${a.problems === 1 ? '' : 's'}; it keeps running what it has.` };
	if (a.in_sync === true) return { cls: 'ok', chip: 'ok', label: 'In sync', detail: 'Running version ' + a.version + '.' };
	if (a.in_sync === false) return { cls: 'warn', chip: 'warn', label: 'Out of sync', detail: `Running version ${a.seen.running}; its version is ${a.version}.` };
	return { cls: 'warn', chip: 'warn', label: 'No config yet', detail: 'It has not reported running a version.' };
}

// fleet reads every AP's state, by ID.
export function fleetMap(aps) {
	return new Map(aps.map((a) => [a.id, apStatus(a)]));
}
