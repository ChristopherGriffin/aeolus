// One AP: whether it runs what it should, its settings in the same tabs as a
// folder's (0047), what it reported, and its history (0039, 0041).

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { bandName, when, ago } from '../format.js';
import { treeAside, crumbs, apStatus, fleetMap, tabBar, pick, SUBTABS } from '../layout.js';
import { editing } from './fields.js';
import { followButton } from './follow.js';
import { hardwareTab, systemSection } from './sections.js';
import { networksTab } from './networks.js';
import { interfacesTab } from './interfaces.js';

const TABS = [['overview', 'Overview'], ['hardware', 'Hardware'], ['networks', 'Networks'], ['interfaces', 'Interfaces'], ['system', 'System']];

export async function apPage(ctx, id, tab, sub) {
	const enc = encodeURIComponent(id);
	const [page, cfg, hist, fleet] = await Promise.all([
		get(`/v1/trees/locations/nodes/${enc}`),
		get(`/v1/aps/${enc}/config`),
		get(`/v1/aps/${enc}/history?limit=20`),
		get('/v1/aps'),
	]);
	const me = fleet.aps.find((a) => a.id === id);
	const st = me ? apStatus(me) : { chip: 'idle', label: 'Unknown', detail: '' };
	const cond = cfg.condition || {};
	const facts = page.facts || {};
	const edit = editing(ctx, 'locations', page);
	// Everything the AP sets for itself can go back to its folder in one
	// change (0046).
	const own = Object.keys(page.fields || {}).filter((p) => page.fields[p].origin === 'self').sort();
	const revertBox = h('div', { class: 'edit flush' });
	const base = `/aps/${enc}`;
	tab = pick(TABS, tab);
	const thisAP = { ap: { id, name: page.node.name }, cfg };
	const main = [
		crumbs(ctx, 'locations', page.ancestry),
		h('div', { class: 'head' },
			h('div', null,
				h('h1', null, page.node.name),
				h('div', { class: 'sub' }, ['AP', facts.model, cond.seen?.source].filter(Boolean).join(' · '))),
			edit && own.length > 0 && followButton(ctx, 'locations', id, page.node.name, edit.parentName, own, revertBox,
				`Revert to ${edit.parentName} (${own.length} custom setting${own.length === 1 ? '' : 's'})`)),
		revertBox,
		statusPanel(st, cfg, cond),
		cfg.check?.problems?.length > 0 && h('div', { class: 'banner problems' },
			h('strong', null, 'Its config breaks these rules, so it is not sent'),
			h('ul', null, cfg.check.problems.map((p) => h('li', null, p)))),
		tunnelTrouble(cond, base),
		tabBar(base, TABS, tab),
	];
	if (tab === 'overview') {
		main.push(h('div', { class: 'grid2' },
			h('div', { class: 'col' }, latest(cond)),
			h('div', { class: 'col' }, enrollment(facts))), history(hist));
	} else if (tab === 'hardware') main.push(await hardwareTab(ctx, base, id, page, sub, thisAP));
	else if (tab === 'networks') main.push(await networksTab(ctx, id, page));
	else if (tab === 'interfaces') main.push(await interfacesTab(ctx, base, id, page, sub, thisAP, edit));
	else main.push(await systemSection(ctx, id, page, edit));
	const keep = tab === 'overview' ? '' : `/${tab}${SUBTABS.has(tab) && sub ? '/' + sub : ''}`;
	return { aside: treeAside(ctx, 'locations', id, fleetMap(fleet.aps), keep), main, refresh: 30 };
}

// tunnelTrouble warns of a tunnel the AP's prober finds down, and of a port
// its loop guard took off its tunnels (0059). The Tunnels view says more.
function tunnelTrouble(cond, base) {
	const x = cond.state?.report?.vxlan;
	const down = (x?.tunnels || []).filter((t) => t.probe?.verdict === 'down');
	const loops = x?.loops || [];
	if (!down.length && !loops.length) return null;
	return h('div', { class: 'banner problems' },
		h('strong', null, 'Its tunnels need a look'),
		h('ul', null,
			down.map((t) => h('li', null, `The tunnel to ${t.peer}, VNI ${t.vni}, is down: ${t.probe.underlay === false ? `${t.peer} cannot be reached` : `nothing on VNI ${t.vni} answers`}.`)),
			loops.map((l) => h('li', null, `${l.port} is off its tunnels: VNI ${l.vni ?? '?'} loops.`))),
		h('div', null, link(`${base}/interfaces/tunnels`, 'See Interfaces › Tunnels')));
}

function statusPanel(st, cfg, cond) {
	return h('section', { class: 'panel' },
		h('div', { class: 'status' },
			h('span', { class: 'chip ' + st.chip + ' big' }, st.label),
			h('div', null,
				h('div', null, st.detail),
				h('div', { class: 'detail' },
					cond.seen ? `Last seen ${ago(cond.seen.at)} from ${cond.seen.source}` : 'Never seen',
					' · its version is ', String(cfg.version),
					cond.seen?.running != null && ` · running ${cond.seen.running}`))));
}

function latest(cond) {
	const c = cond.check;
	const a = cond.apply;
	return h('section', { class: 'panel' },
		h('h2', null, 'Latest check and apply'),
		h('dl', { class: 'kv' },
			h('dt', null, 'Render check'),
			h('dd', null, c ? `${c.result} · version ${c.version} · ${ago(c.at)}` : 'none yet'),
			h('dt', null, 'Apply'),
			h('dd', null, a ? `${a.ok ? 'worked' : 'failed'} · version ${a.version} · ${ago(a.at)}${a.checked ? '' : ' · NOT covered by an ok check'}` : 'none yet'),
			a?.error && [h('dt', null, 'Error'), h('dd', null, a.error)],
			h('dt', null, 'Uptime'),
			h('dd', null, cond.state?.report?.uptime != null ? uptime(cond.state.report.uptime) : '—'),
			h('dt', null, 'OpenWrt'),
			h('dd', null, cond.state?.report?.openwrt || '—')));
}

function uptime(s) {
	const d = Math.floor(s / 86400), hr = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
	return [d && d + ' d', hr && hr + ' h', m + ' min'].filter(Boolean).join(' ');
}

// enrollment is what the AP said about itself when it enrolled (0033).
function enrollment(facts) {
	if (!facts.mac) return null;
	return h('section', { class: 'panel' },
		h('h2', null, 'Enrollment'),
		h('dl', { class: 'kv' },
			h('dt', null, 'MAC'), h('dd', null, facts.mac),
			h('dt', null, 'All MACs'), h('dd', null, (facts.macs || []).join(', ')),
			h('dt', null, 'Hostname'), h('dd', null, facts.hostname || '—'),
			h('dt', null, 'Board'), h('dd', null, facts.board || '—'),
			h('dt', null, 'Radios'), h('dd', null, (facts.radios || []).map((r) => `${r.radio} ${bandName(r.band)}: ${(r.htmodes || []).join(' ')}`).map((x) => h('div', null, x))),
			h('dt', null, 'Enrolled from'), h('dd', null, facts.source || '—')));
}

// history lists recent checks (with the UCI the AP sent, secrets blanked:
// 0041), applies and state reports, newest first.
function history(hist) {
	const checks = hist.checks || [];
	const applies = hist.applies || [];
	const states = hist.states || [];
	return h('section', { class: 'panel' },
		h('h2', null, 'History', h('span', { class: 'note' }, 'newest first')),
		checks.map((c) => h('details', null,
			h('summary', null,
				h('span', { class: 'when' }, when(c.at)),
				h('span', { class: 'chip ' + (c.result === 'ok' ? 'ok' : c.result === 'stale' ? 'idle' : 'bad') }, 'check ' + c.result),
				h('span', { class: 'grow' }, `version ${c.version}`, c.problems?.length ? ` · ${c.problems.length} problem${c.problems.length === 1 ? '' : 's'}` : ''),
				h('span', { class: 'mono' }, c.hash.slice(0, 12))),
			c.problems?.length > 0 && h('ul', { class: 'problems' }, c.problems.map((p) => h('li', null, p))),
			c.uci ? h('pre', { class: 'uci' }, c.uci) : h('div', { class: 'empty' }, 'No UCI kept for this check.'))),
		applies.map((a) => h('details', null,
			h('summary', null,
				h('span', { class: 'when' }, when(a.at)),
				h('span', { class: 'chip ' + (a.ok ? 'ok' : 'bad') }, a.ok ? 'applied' : 'apply failed'),
				h('span', { class: 'grow' }, `version ${a.version}`, a.checked ? '' : ' · not covered by an ok check', a.error ? ' · ' + a.error : ''),
				h('span', { class: 'mono' }, a.hash.slice(0, 12))))),
		states.length > 0 && h('details', null,
			h('summary', null, h('span', { class: 'when' }, when(states[0].at)), h('span', { class: 'chip idle' }, 'state'),
				h('span', { class: 'grow' }, `${states.length} recent state reports`)),
			h('pre', { class: 'uci' }, states.map((s) => `${when(s.at)}  ${JSON.stringify(s.report)}`).join('\n'))),
		!checks.length && !applies.length && !states.length && h('div', { class: 'empty' }, 'Nothing yet.'));
}

