// The Network tab of a Locations folder or the Org (0141): how long the
// networks' own services take to answer the APs below it. An address from
// DHCP, the gateway, a name looked up, and the RADIUS server, each as what
// it typically takes and as a line over the time chosen; of every VLAN
// together, or of the one picked. It reads what the client tests and the
// state reports already time, and sets nothing.

import { h } from '../dom.js';
import { get } from '../api.js';

const SVG = 'http://www.w3.org/2000/svg';
const W = 240, H = 56, TOP = 4, BOTTOM = 2;

function svg(tag, attrs, ...kids) {
	const el = document.createElementNS(SVG, tag);
	for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, String(v));
	for (const k of kids.flat()) if (k != null && k !== false) el.append(k instanceof Node ? k : document.createTextNode(String(k)));
	return el;
}

// The times it is drawn over, in hours, as a graph's are anywhere.
const SPANS = [[24, 'A day'], [168, 'A week'], [720, '30 days'], [8760, 'A year']];

// The steps shown, with what each is in full for a hover. A whole sign-in
// is shown only where one was tried, whether or not any was answered:
// where a network signs clients in.
const STEPS = [
	['dhcp', 'DHCP', 'From asking for an address to having one, as a client test found.'],
	['gateway', 'Gateway', 'How long the gateway took to answer, as a client test found.'],
	['dns', 'DNS', 'How long a name took to look up, as a client test found.'],
	['aaa', 'AAA', "How long the network's RADIUS server took to answer the AP, as each state report says."],
	['signin', 'Sign-in', 'A whole 802.1X sign-in, as a client test through the air found.'],
];

// ms writes a time: 4.1 ms, 284 ms, 2.0 s.
const ms = (n) => (n == null ? '—' : n >= 1000 ? `${(n / 1000).toFixed(1)} s` : n >= 10 ? `${Math.round(n)} ms` : `${n.toFixed(1)} ms`);
const clock = (t) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
const dated = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
const day = (t) => new Date(t).toLocaleDateString([], { month: 'short', day: 'numeric' });
const when = (hours, t) => (hours > 200 ? day(t) : hours > 49 ? dated(t) : clock(t));

// What a segment is called: its VLAN or its tunnel, and the networks on it.
function named(s) {
	const nets = s.networks.map((n) => n.ssid || n.network).join(', ');
	if (s.vlan != null) return `VLAN ${s.vlan} · ${nets}`;
	if (s.vni != null) return `Tunnel ${s.vni} · ${nets}`;
	return nets;
}

// What each folder's tab was last drawn over, and of which segment, kept
// while the page is.
const view = new Map();

// line draws one step's average over the time: a line where it was timed,
// broken where it was not, each stretch saying its own on hover.
function line(v, key, label) {
	const pts = v.series, n = pts.length;
	const high = H - TOP - BOTTOM, bw = W / n;
	const most = Math.max(1e-9, ...pts.map((p) => p.steps[key] ?? 0));
	const y = (x) => TOP + high - (x / most) * high;
	const runs = [];
	let run = [];
	pts.forEach((p, i) => {
		const x = p.steps[key];
		if (x == null) {
			if (run.length) runs.push(run);
			run = [];
			return;
		}
		run.push(`${((i + 0.5) * bw).toFixed(1)},${y(x).toFixed(1)}`);
	});
	if (run.length) runs.push(run);
	return svg('svg', { viewBox: `0 0 ${W} ${H}`, class: 'usage steptime', role: 'img', 'aria-label': `${label} over ${v.hours} hours` },
		pts.map((p, i) => svg('g', null,
			svg('title', null, `${when(v.hours, p.at)}: ${p.steps[key] == null ? 'not timed' : ms(p.steps[key])}`),
			svg('rect', { x: i * bw, y: TOP, width: bw, height: high, class: 'hit' }))),
		// A stretch of one point alone is a dot: a line needs two.
		runs.map((r) => (r.length > 1 ? svg('polyline', { class: 'clients', points: r.join(' ') })
			: svg('circle', { class: 'dot', cx: r[0].split(',')[0], cy: r[0].split(',')[1], r: 1.5 }))),
		svg('line', { x1: 0, x2: W, y1: TOP + high, y2: TOP + high, class: 'base' }));
}

export async function networkTab(ctx, id) {
	const box = h('div');
	const at = view.get(id) || { hours: SPANS[0][0], segment: '' };
	view.set(id, at);
	// Only the latest asking is drawn: an answer to an earlier choice that
	// comes in late does not replace it.
	let asked = 0;
	const draw = async () => {
		const mine = ++asked;
		let v;
		try {
			v = await get(`/v1/network?under=${encodeURIComponent(id)}&hours=${at.hours}${at.segment ? `&segment=${encodeURIComponent(at.segment)}` : ''}`);
		} catch (e) {
			// An older manager has none of it.
			if (mine === asked) box.replaceChildren(h('div', { class: 'banner info' }, e.status === 404 ? 'This manager does not keep these times yet.' : e.message));
			return;
		}
		if (mine !== asked) return;
		// A segment picked that this time holds nothing of is let go.
		if (at.segment && !v.segments.some((s) => s.id === at.segment)) {
			at.segment = '';
			return draw();
		}
		const pick = (set) => { set(); draw(); };
		const spans = h('span', { class: 'segmented', role: 'group', 'aria-label': 'Over how long' }, SPANS.map(([t, name]) => h('button', {
			type: 'button', class: at.hours === t ? 'on' : '', 'aria-pressed': at.hours === t, onclick: () => pick(() => { at.hours = t; }),
		}, name)));
		const which = h('select', { 'aria-label': 'Which VLAN', onchange: (e) => pick(() => { at.segment = e.target.value; }) },
			h('option', { value: '', selected: !at.segment }, 'All VLANs'),
			v.segments.map((s) => h('option', { value: s.id, selected: at.segment === s.id }, named(s))));
		const of = at.segment ? v.segments.find((s) => s.id === at.segment).steps : v.all;
		const any = v.segments.length > 0;
		const shown = STEPS.filter(([key]) => key !== 'signin' || tried(of.signin));
		const cards = h('div', { class: 'tiles steptimes' }, shown.map(([key, label, says]) => {
			const s = of[key];
			return h('div', { class: 'tile', title: says },
				h('div', { class: 'tlabel' }, label),
				h('div', { class: 'tbig' }, ms(s?.ms)),
				h('div', { class: 'tsub' }, !s || (!s.n && !s.failed) ? 'not timed' : [s.most != null && `slowest ${ms(s.most)}`, s.failed > 0 && `${s.failed} without an answer`].filter(Boolean).join(' · ')),
				s?.n > 0 && line(v, key, label));
		}));
		const cell = (s) => h('td', { class: s?.failed ? 'warnish' : '', title: s && (s.n || s.failed) ? `timed ${s.n} time${s.n === 1 ? '' : 's'}${s.most != null ? `, slowest ${ms(s.most)}` : ''}${s.failed ? `; ${s.failed} without an answer` : ''}` : null },
			ms(s?.ms), s?.failed > 0 && h('span', { class: 'sub' }, ` · ${s.failed} failed`));
		const table = v.segments.length > 0 && h('section', { class: 'panel' },
			h('h2', null, 'By VLAN', h('span', { class: 'note' }, 'what each typically takes')),
			h('table', { class: 'list' },
				h('tr', null, ['VLAN', 'Networks', ...shownAll(v).map(([, label]) => label)].map((t) => h('th', null, t))),
				v.segments.map((s) => h('tr', { class: at.segment === s.id ? 'picked' : '' },
					h('td', null, h('a', { href: '#', title: 'Draw this one alone', onclick: (e) => { e.preventDefault(); pick(() => { at.segment = at.segment === s.id ? '' : s.id; }); } },
						s.vlan != null ? `VLAN ${s.vlan}` : s.vni != null ? `Tunnel ${s.vni}` : '—')),
					h('td', null, s.networks.map((n) => n.ssid || n.network).join(', ')),
					shownAll(v).map(([key]) => cell(s.steps[key]))))));
		box.replaceChildren(
			h('section', { class: 'panel' },
				h('h2', { title: 'How long each took on average, as the APs here timed it: in their client tests, and of the RADIUS server in their reports.' },
					h('span', { class: 'title' }, 'Response times', spans), which),
				any ? [cards, h('div', { class: 'usageaxis', 'aria-hidden': 'true' }, h('span', null, when(v.hours, v.from)), h('span', null, 'now'))]
					: h('p', { class: 'sub pad' }, 'Nothing was timed here in this time. Client tests on a schedule time each network\'s DHCP, gateway and DNS (Assurance); a network that signs clients in has its RADIUS server timed with every report.')),
			table);
	};
	await draw();
	return box;
}

// tried says whether a step was ever tried: timed, or left unanswered.
const tried = (s) => !!s && (s.n > 0 || s.failed > 0);

// shownAll is the steps the table has a column for: a sign-in only where
// some segment tried one.
function shownAll(v) {
	return STEPS.filter(([key]) => key !== 'signin' || v.segments.some((s) => tried(s.steps.signin)));
}
