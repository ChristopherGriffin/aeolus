// A folder in Locations or Services: its values and where they come from, its
// overrides, and what it holds (0012, 0013). Landing Zone lists the APs
// waiting in it (0032).

import { h, link, icon } from '../dom.js';
import { get } from '../api.js';
import { group, value, ago } from '../format.js';
import { treeAside, crumbs, fleetMap } from '../layout.js';
import { fieldPanels } from './fields.js';
import { apPage } from './ap.js';

export async function treePage(ctx, tree, id) {
	const t = ctx.trees[tree];
	id = id || t.root;
	if (!id) return { main: [h('div', { class: 'banner info' }, 'You have no role in this tree.')] };
	// Links to where a value was set can point at an AP; it has its own page.
	if (tree === 'locations' && t.nodes.get(id)?.kind === 'ap') return apPage(ctx, id);
	const [page, fleet] = await Promise.all([
		get(`/v1/trees/${tree}/nodes/${encodeURIComponent(id)}`),
		tree === 'locations' ? get('/v1/aps') : null,
	]);
	const status = fleet ? fleetMap(fleet.aps) : null;
	const n = page.node;
	const branch = page.ancestry.map((a) => t.nodes.get(a)).find((x) => x?.broken);
	const kind = n.kind === 'org'
		? `Org · root of ${tree === 'locations' ? 'Locations' : 'Services'}`
		: n.isolated ? 'Isolated folder · APs wait here to be adopted' : 'Folder';

	const main = [
		crumbs(ctx, tree, page.ancestry),
		h('div', { class: 'head' },
			h('div', null,
				h('h1', null, n.name, n.broken && h('span', { class: 'chip break' }, 'Break Hierarchy')),
				h('div', { class: 'sub' }, kind, ' · your role here: ', page.role)),
			overrides(ctx, tree, page.overrides)),
		branch && h('div', { class: 'banner branch' },
			h('strong', null, 'Break Hierarchy'),
			h('span', null, `${branch.name} starts its own branch. Locks from above stop there, and the branch owns its configuration and the inheritance below it. Values marked Branch baseline were copied when the break was made.`)),
		page.problems?.length > 0 && h('div', { class: 'banner problems' },
			h('strong', null, `What is set here breaks ${page.problems.length} rule${page.problems.length === 1 ? '' : 's'}`),
			h('ul', null, page.problems.map((p) => h('li', null, p)))),
	];
	if (n.isolated) main.push(await landingZone(ctx, t, id, fleet));
	else main.push(fieldPanels(ctx, tree, id, page.fields));
	main.push(inside(ctx, tree, t, id, status));
	return { aside: treeAside(ctx, tree, id, status), main, refresh: n.isolated ? 30 : 0 };
}

// overrides is the mockup's Overrides menu (0012): what differs from above,
// here and below, each a link to where it was set.
function overrides(ctx, tree, ov) {
	const inEffect = ov?.in_effect || [];
	const below = ov?.below || [];
	const item = (o) => h('a', { href: `#/${tree}/${encodeURIComponent(o.node)}` },
		labelOf(o.path),
		h('small', null, h('span', { class: 'mono' }, value(o.path, o.value, (id) => ctx.name('services', id))), ' · set at ', ctx.name(tree, o.node)));
	const drop = h('div', { class: 'drop', hidden: true },
		inEffect.length > 0 && [h('h3', null, 'In effect here'), inEffect.map(item)],
		below.length > 0 && [h('h3', null, 'Made below this level'), below.map(item)],
		!inEffect.length && !below.length && h('div', { class: 'empty' }, 'No overrides: everything here comes straight from above.'));
	const button = h('button', { type: 'button', 'aria-expanded': 'false', onclick: () => {
		drop.hidden = !drop.hidden;
		button.setAttribute('aria-expanded', String(!drop.hidden));
	} }, 'Overrides', h('span', { class: 'count' }, inEffect.length + below.length), icon('chevron'));
	return h('div', { class: 'menu' }, button, drop);
}

function labelOf(path) {
	const g = group(path);
	return g.network ? `${g.network} · ${g.label}` : `${g.title} · ${g.label}`;
}

// landingZone lists the APs waiting to be adopted, with what they said about
// themselves when they enrolled (0033), so a person can tell a real AP from
// an impostor.
async function landingZone(ctx, t, id, fleet) {
	const waiting = t.list.filter((n) => n.parent === id && n.kind === 'ap');
	if (!waiting.length) return h('div', { class: 'banner info' }, 'No APs are waiting. An AP running the agent lands here when it enrolls.');
	const pages = await Promise.all(waiting.map((n) => get(`/v1/trees/locations/nodes/${encodeURIComponent(n.id)}`)));
	const seen = new Map((fleet?.aps || []).map((a) => [a.id, a.seen]));
	return h('section', { class: 'panel' },
		h('h2', null, 'Waiting to be adopted', h('span', { class: 'note' }, 'Adopting comes with editing (M6 part 2)')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'MAC', 'Model', 'OpenWrt', 'Enrolled from', 'Last seen'].map((c) => h('th', null, c))),
			pages.map((p) => {
				const f = p.facts || {};
				return h('tr', null,
					h('td', null, link(`/aps/${encodeURIComponent(p.node.id)}`, p.node.name)),
					h('td', { class: 'mono' }, f.mac || '—'),
					h('td', null, f.model || '—'),
					h('td', null, f.openwrt || '—'),
					h('td', { class: 'mono' }, f.source || '—'),
					h('td', null, ago(seen.get(p.node.id)?.at)));
			})));
}

// inside lists what a folder holds.
function inside(ctx, tree, t, id, status) {
	const kids = t.list.filter((n) => n.parent === id && !(t.nodes.get(id)?.isolated && n.kind === 'ap'));
	if (!kids.length) return null;
	return h('section', { class: 'panel' },
		h('h2', null, 'Inside ' + ctx.name(tree, id)),
		h('table', { class: 'list' }, kids.map((n) => {
			const ap = n.kind === 'ap';
			const st = ap && status ? status.get(n.id) : null;
			return h('tr', null,
				h('td', null, link(ap ? `/aps/${encodeURIComponent(n.id)}` : `/${tree}/${encodeURIComponent(n.id)}`, n.name)),
				h('td', null, ap ? 'AP' : n.isolated ? 'Isolated folder' : 'Folder', n.broken && ' · breaks hierarchy'),
				h('td', null, st && h('span', { class: 'chip ' + st.chip }, st.label)));
		})));
}
