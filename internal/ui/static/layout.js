// Pieces several pages share: the tree on the left, the path to a node, and
// what state an AP is in.

import { h, link, icon } from './dom.js';
import { ago } from './format.js';

// ordered is a tree in the order the left side shows it: depth first, and
// under each folder its APs, then its folders, each as the manager lists
// them.
function ordered(t) {
	const kids = new Map();
	for (const n of t.list)
		if (n.parent && t.nodes.has(n.parent)) {
			if (!kids.has(n.parent)) kids.set(n.parent, []);
			kids.get(n.parent).push(n);
		}
	const out = [];
	const walk = (n) => {
		out.push(n);
		const k = kids.get(n.id) || [];
		k.filter((c) => c.kind === 'ap').forEach(walk);
		k.filter((c) => c.kind !== 'ap').forEach(walk);
	};
	t.list.filter((n) => !n.parent || !t.nodes.has(n.parent)).forEach(walk);
	return out;
}

// treeAside draws a tree on the left, as in the mockup: the folders, and
// under each its APs, then its folders, each AP with a dot for how it is
// doing (status maps an AP to apStatus), so a change can be watched
// landing. keep is the tab to open on the folder or AP clicked (0047); it
// is not kept in an isolated folder, whose page has no tabs.
export function treeAside(ctx, tree, selected, status, keep = '') {
	const t = ctx.trees[tree];
	const aside = h('aside', { class: 'tree' });
	const draw = () => aside.replaceChildren(h('div', { class: 'tree-title' }, tree === 'locations' ? 'Locations' : 'Services'),
		...rows(ctx, t, tree, selected, status, keep, draw));
	draw();
	return aside;
}

// The folders whose APs this viewer has hidden in the tree, kept in the
// browser: a convenience of theirs, not a setting. Where the browser keeps
// nothing, they stay hidden until the page is loaded again.
const HIDDEN = 'aeolus.tree.hiddenAPs';
let hidden = null;

function hiddenFolders() {
	if (!hidden)
		try {
			hidden = new Set(JSON.parse(localStorage.getItem(HIDDEN) || '[]'));
		} catch {
			hidden = new Set();
		}
	return hidden;
}

function keepHidden() {
	try {
		localStorage.setItem(HIDDEN, JSON.stringify([...hiddenFolders()]));
	} catch { /* storage blocked */ }
}

// The worst of some APs' states, for a folder whose APs are hidden.
const WORST = ['bad', 'warn', 'idle', 'ok'];

// apToggle is a folder's switch for its APs in the tree (Griff, 2026-10-06):
// their count, and while they are hidden, one dot for the worst of them.
function apToggle(folder, aps, status, draw) {
	const shown = !hiddenFolders().has(folder.id);
	const sts = aps.map((a) => status?.get(a.id)).filter(Boolean);
	const worst = WORST.find((c) => sts.some((s) => s.cls === c));
	const counts = [...new Set(sts.map((s) => s.label))].map((l) => `${sts.filter((s) => s.label === l).length} ${l.toLowerCase()}`).join(', ');
	return h('button', {
		type: 'button', class: 'aptoggle', 'aria-pressed': shown ? 'true' : 'false',
		title: `${shown ? 'Hide' : 'Show'} the ${aps.length === 1 ? 'AP' : `${aps.length} APs`} in ${folder.name}${counts ? ` (${counts})` : ''}`,
		onclick: () => {
			if (shown) hiddenFolders().add(folder.id); else hiddenFolders().delete(folder.id);
			keepHidden();
			draw();
		},
	}, icon('ap'), String(aps.length), !shown && worst && h('span', { class: 'dot ' + worst }));
}

// rows are the tree's rows: each folder, with a switch for its APs where it
// has any, and each AP not hidden with its folder's, the selected one
// always.
function rows(ctx, t, tree, selected, status, keep, draw) {
	const off = hiddenFolders();
	const apsOf = new Map();
	for (const n of t.list)
		if (n.kind === 'ap' && n.parent) {
			if (!apsOf.has(n.parent)) apsOf.set(n.parent, []);
			apsOf.get(n.parent).push(n);
		}
	const depth = (id) => {
		let d = 0;
		for (let n = t.nodes.get(id); n && n.parent && t.nodes.has(n.parent); n = t.nodes.get(n.parent)) d++;
		return d;
	};
	const inBranch = (id) => {
		for (let n = t.nodes.get(id); n; n = t.nodes.get(n.parent)) if (n.broken) return true;
		return false;
	};
	return ordered(t).filter((n) => n.kind !== 'ap' || !off.has(n.parent) || n.id === selected).map((n) => {
		const ap = n.kind === 'ap';
		const href = (ap ? `/aps/${encodeURIComponent(n.id)}` : `/${tree}/${encodeURIComponent(n.id)}`) + (n.isolated ? '' : keep);
		const cls = [n.id === selected ? 'on' : '', inBranch(n.id) ? 'branch' : ''].join(' ').trim();
		const st = ap && status ? status.get(n.id) : null;
		const row = h('a', { href: '#' + href, class: cls || null, style: { '--depth': depth(n.id) } },
			icon(ap ? 'ap' : 'folder'),
			h('span', { class: 'name' }, n.name),
			n.broken && h('span', { class: 'chip break' }, 'BREAK'),
			st && h('span', { class: 'dot ' + st.cls, title: st.label }));
		const aps = !ap && apsOf.get(n.id);
		return aps ? h('div', { class: 'treerow' }, row, apToggle(n, aps, status, draw)) : row;
	});
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
	// netifd lost its network.wireless object (2026-10-06): the radios run,
	// but Aeolus there can't see them.
	if (a.wireless_missing) return { cls: 'warn', chip: 'warn', label: 'Radios unseen',
		detail: `netifd lost its network.wireless object, so Aeolus on this AP can't see its radios, clients or neighbours, and checks the Wi-Fi after a change only by counting its networks. Restarting the network on the AP brings it back; its Wi-Fi drops for about 30 seconds.` };
	if (a.in_sync === true) {
		// Its agent (0079), once it says which it runs: amber while that
		// isn't the one it should run.
		const g = a.agent, runs = g?.runs?.hash;
		if (runs !== undefined && g?.wants && runs !== g.wants.hash) {
			const u = g.runs.update;
			const short = (v) => (v || '').replace(/-[0-9a-f]{7,}$/, '') || 'unknown';
			if (u && u.hash === g.wants.hash && (u.state === 'rolled-back' || u.state === 'failed'))
				return { cls: 'warn', chip: 'warn', label: u.state === 'failed' ? 'Agent update failed' : 'Agent rolled back', detail: u.why || '' };
			return { cls: 'warn', chip: 'warn', label: 'Agent updating', detail: `Its agent is ${short(g.runs.version)}; it should run ${short(g.wants.version)}.` };
		}
		return { cls: 'ok', chip: 'ok', label: 'In sync', detail: 'Running version ' + a.version + '.' };
	}
	if (a.in_sync === false) return { cls: 'warn', chip: 'warn', label: 'Out of sync', detail: `Running version ${a.seen.running}; its version is ${a.version}.` };
	return { cls: 'warn', chip: 'warn', label: 'No config yet', detail: 'It has not reported running a version.' };
}

// fleet reads every AP's state, by ID.
export function fleetMap(aps) {
	return new Map(aps.map((a) => [a.id, apStatus(a)]));
}

// tabBar draws a page's tabs as links under base ([[key, label]]); current
// is the one shown. sub draws the smaller second row (0047), and 'minor' a
// third, smaller still, for the views of one part (0072).
export function tabBar(base, tabs, current, sub) {
	return h('nav', { class: sub === 'minor' ? 'subtabs minor' : sub ? 'subtabs' : 'pagetabs', 'aria-label': sub === 'minor' ? 'View of this part' : sub ? 'Part of this section' : 'Sections of this page' },
		tabs.map(([key, label]) => h('a', { href: `#${base}/${key}`, class: key === current ? 'on' : null, 'aria-current': key === current ? 'page' : null }, label)));
}

// SUBTABS are the page tabs with tabs of their own; moving to another node
// keeps the choice of those too.
export const SUBTABS = new Set(['interfaces']);

// keepPath is the part of a node's address that moving to another node
// keeps: its tab, and the tab's part and view.
export function keepPath(tab, sub, view) {
	if (!SUBTABS.has(tab) || !sub) return `/${tab}`;
	return `/${tab}/${sub}${view ? '/' + view : ''}`;
}

// moved reads an address from before 0072, when Radios and Channels were a
// Hardware tab of their own, as the Interfaces tab's, and puts the new
// address in place of the old one, so links and bookmarks keep working.
export function moved(base, tab, sub, view) {
	if (tab !== 'hardware') return [tab, sub, view];
	const now = ['interfaces', 'radios', sub === 'channels' ? 'channels' : undefined];
	try { history.replaceState(null, '', `#${base}${keepPath(...now)}`); } catch { /* the address stays as it was */ }
	return now;
}

// pick returns the tab asked for if there is one, or the first.
export function pick(tabs, key) {
	return tabs.some(([k]) => k === key) ? key : tabs[0][0];
}
