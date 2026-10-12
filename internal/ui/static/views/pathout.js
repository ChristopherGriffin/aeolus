// An AP's Path tab (0142): its own way out, a hop at a time. Its management
// address and VLAN, its uplink and the switch port it is on, its gateway
// and the internet, each with whether it answers and how long it took; and
// under them what it leans on to work at all: its name servers, its time
// and the manager. From its latest report: its prober asks each every 30
// seconds. It sets nothing.

import { h } from '../dom.js';
import { uplinkBrief, switchPort } from '../format.js';
import { pathOf } from './path.js';

// took writes a time: 4.1 ms, 284 ms, 2.0 s.
const took = (n) => (n == null ? null : n >= 1000 ? `${(n / 1000).toFixed(1)} s` : n >= 10 ? `${Math.round(n)} ms` : `${n.toFixed(1)} ms`);

// How a thing on the way is doing, as the path colours it and as it is said.
const STATE = { up: 'ok', silent: 'bad', unresolved: 'warn' };
const SAYS = { up: 'answers', silent: 'no answer', unknown: 'not asked for long yet', unresolved: 'no address for the name' };

const chip = (state) => h('span', { class: `chip ${STATE[state] || 'idle'}` }, SAYS[state] || 'not said');

// hop is the gateway or the internet as the path draws it.
function hop(top, main, x, also) {
	return {
		top, main,
		sub: x.state === 'up' ? took(x.ms) : SAYS[x.state] || null,
		state: STATE[x.state] || null,
		title: [also, x.state === 'up' && x.ago != null && `It last answered ${x.ago} s before the report.`,
			x.state === 'silent' && 'Three asks in a row, 30 seconds apart, went unanswered.'].filter(Boolean).join(' ') || null,
	};
}

export function pathTab(cond) {
	const r = cond?.state?.report;
	if (!r) return h('div', { class: 'banner info' }, 'This AP has sent no report yet.');
	const p = r.path;
	const wire = uplinkBrief(r);
	const vlan = r.uplink_port?.management_vlan;
	const hops = [{
		top: 'This AP',
		main: p?.address ? `${p.address}${p.mask != null ? `/${p.mask}` : ''}` : 'its address not said',
		sub: [vlan != null && `VLAN ${vlan}`, p?.device].filter(Boolean).join(' · ') || null,
		title: 'Its management address, and the interface its own traffic leaves by.',
	}];
	if (wire) {
		hops.push({ top: 'Uplink', main: wire.text, sub: switchPort(r.uplink_neighbor) || null, state: wire.grade === 'ok' ? 'ok' : wire.grade,
			title: [wire.says, wire.errors?.says].filter(Boolean).join(' ') });
	}
	if (p?.gateway) hops.push(hop('Gateway', p.gateway.address, p.gateway));
	if (p) {
		hops.push(p.internet
			? hop('Internet', p.internet.target, p.internet, p.internet.address && p.internet.address !== p.internet.target && `${p.internet.target} is ${p.internet.address}.`)
			: { top: 'Internet', main: 'not asked', sub: 'no target set', title: 'Set the internet check in a folder\'s System settings, and its APs ask it.' });
	}
	const said = hops.map((x) => `${x.top} ${x.main}${x.sub ? `, ${x.sub}` : ''}`).join('; ');

	// What it leans on: a line each.
	const rows = [];
	for (const d of p?.dns || []) {
		rows.push(['DNS', d.server, d.state ? chip(d.state) : h('span', { class: 'sub' }, 'not asked'), took(d.state === 'up' ? d.ms : null),
			[p.name && `asked for ${p.name}`, d.rcode ? `it answered with error ${d.rcode}` : null].filter(Boolean).join('; ')]);
	}
	const t = r.time;
	if (t) {
		rows.push(['Time', (t.servers || []).join(', ') || 'no server set',
			h('span', { class: `chip ${t.synced === true ? 'ok' : t.synced === false ? 'bad' : 'idle'}` }, t.synced === true ? 'in step' : t.synced === false ? 'not in step' : 'not said'),
			null,
			[t.stratum != null && `stratum ${t.stratum}`, t.offset != null && `${(t.offset * 1000).toFixed(1)} ms off`].filter(Boolean).join(', ')]);
	}
	if (p?.manager) rows.push(['Manager', 'this manager', h('span', { class: 'chip ok' }, 'answers'), took(p.manager.ms), 'its last poll for its config: the whole request, its connection too']);

	return [
		h('section', { class: 'panel' },
			h('h2', null, 'Its way out', h('span', { class: 'note' }, 'as it last reported')),
			pathOf(hops, `Its way out. ${said}.`),
			!p && h('p', { class: 'sub pad' }, 'Its agent does not time its gateway, its name servers or the internet yet: that comes with a later agent, or needs its prober running.')),
		rows.length > 0 && h('section', { class: 'panel' },
			h('h2', null, 'What it leans on'),
			h('table', { class: 'list' },
				h('tr', null, ['', 'Where', 'State', 'Took', ''].map((c) => h('th', null, c))),
				rows.map(([what, where, state, time, more]) => h('tr', null,
					h('td', null, what), h('td', { class: 'mono' }, where), h('td', null, state),
					h('td', { class: 'nowrap' }, time || h('span', { class: 'sub' }, '—')), h('td', { class: 'sub' }, more))))),
	].filter(Boolean);
}
