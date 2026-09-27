// One AP: whether it runs what it should, what it offers, what it reported,
// and its history (0039, 0041).

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { security, bandName, when, ago } from '../format.js';
import { treeAside, crumbs, apStatus, fleetMap } from '../layout.js';
import { fieldPanels } from './fields.js';

export async function apPage(ctx, id) {
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
	const main = [
		crumbs(ctx, 'locations', page.ancestry),
		h('div', { class: 'head' },
			h('div', null,
				h('h1', null, page.node.name),
				h('div', { class: 'sub' }, ['AP', facts.model, cond.seen?.source].filter(Boolean).join(' · ')))),
		statusPanel(st, cfg, cond),
		cfg.check?.problems?.length > 0 && h('div', { class: 'banner problems' },
			h('strong', null, 'Its config breaks these rules, so it is not sent'),
			h('ul', null, cfg.check.problems.map((p) => h('li', null, p)))),
		h('div', { class: 'grid2' },
			h('div', { class: 'col' }, networks(ctx, cfg), radios(cond)),
			h('div', { class: 'col' }, latest(cond), enrollment(facts))),
		h('section', { class: 'panel' },
			h('h2', null, 'Location settings', h('span', { class: 'note' }, 'what it inherits, and from where'))),
		fieldPanels(ctx, 'locations', id, page.fields),
		history(hist),
	];
	return { aside: treeAside(ctx, 'locations', id, fleetMap(fleet.aps)), main, refresh: 30 };
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

// networks is what this AP offers: each network from its service folders,
// with how its traffic travels (0018).
function networks(ctx, cfg) {
	const nets = Object.entries(cfg.networks || {});
	const f = (n, k) => n.fields?.[k]?.value;
	const transport = (n, slot) => {
		const type = f(n, `transport.${slot}.type`);
		if (!type) return null;
		return type === 'vxlan'
			? `VXLAN ${f(n, `transport.${slot}.concentrator`)} · VNI ${f(n, `transport.${slot}.vni`)}`
			: `VLAN ${f(n, `transport.${slot}.vlan`)}`;
	};
	return h('section', { class: 'panel' },
		h('h2', null, 'Networks on this AP', h('span', { class: 'note' }, 'from its service folders')),
		nets.length === 0
			? h('div', { class: 'empty' }, cfg.unassigned ? 'None while it waits in Landing Zone.' : 'None: no service folder assigned above it offers a network.')
			: h('table', { class: 'list' },
				h('tr', null, ['SSID', 'Security', 'Travels over', 'From'].map((c) => h('th', null, c))),
				nets.map(([nid, n]) => h('tr', null,
					h('td', { class: 'mono' }, f(n, 'ssid') || nid, f(n, 'enabled') === false && ' (off)'),
					h('td', null, security(f(n, 'security')) || '—'),
					h('td', { class: 'mono' }, transport(n, 'primary') || '—', transport(n, 'fallback') && h('div', null, 'then ', transport(n, 'fallback'))),
					h('td', null, link(`/services/${encodeURIComponent(n.from)}`, ctx.name('services', n.from)))))));
}

// radios is what the AP last reported about its radios.
function radios(cond) {
	const report = cond.state?.report;
	return h('section', { class: 'panel' },
		h('h2', null, 'Radios now', report && h('span', { class: 'note' }, 'reported ' + ago(cond.state.at))),
		!report?.radios?.length
			? h('div', { class: 'empty' }, 'No state report yet.')
			: h('table', { class: 'list' },
				h('tr', null, ['Radio', 'Band', 'Channel', 'Width', 'Clients'].map((c) => h('th', null, c))),
				report.radios.map((r) => h('tr', null,
					h('td', { class: 'mono' }, r.radio),
					h('td', null, bandName(r.band)),
					h('td', null, r.channel || '—'),
					h('td', null, r.width ? r.width + ' MHz' : '—'),
					h('td', null, String(r.clients))))));
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

