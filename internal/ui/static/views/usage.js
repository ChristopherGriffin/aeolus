// Usage (0108): the Wi-Fi clients and traffic of the APs below a node over
// a day, or as long as a year (0140), from their state reports. Traffic is drawn in bars, what
// the APs sent their clients and what they received stacked, and the most
// clients they had at once as a line; with the totals, the clients that
// moved the most (0110), and on a folder its busiest APs by traffic.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { size } from '../format.js';

const SVG = 'http://www.w3.org/2000/svg';
const W = 480, H = 106, TOP = 8, BOTTOM = 4;

function svg(tag, attrs, ...kids) {
	const el = document.createElementNS(SVG, tag);
	for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, String(v));
	for (const k of kids.flat()) if (k != null && k !== false) el.append(k instanceof Node ? k : document.createTextNode(String(k)));
	return el;
}

const clock = (t) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
const dated = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
const day = (t) => new Date(t).toLocaleDateString([], { month: 'short', day: 'numeric' });
// when says a time of a chart: by the clock over a day or two, with its
// date over longer, and by its date alone over more than a week or so.
const when = (u, t) => {
	const hours = (new Date(u.to) - new Date(u.from)) / 3600e3;
	return hours > 200 ? day(t) : hours > 49 ? dated(t) : clock(t);
};

// The times usage is drawn over, in hours. Over more than two days the
// manager reads it from what it keeps by the hour (0140).
const SPANS = [[24, 'A day'], [168, 'A week'], [720, '30 days'], [8760, 'A year']];
// What each node's usage was last drawn over, kept over a redraw.
const over = new Map();
const plural = (n, one) => `${n} ${one}${n === 1 ? '' : 's'}`;

// The folders whose tables were left open, over a redraw: each its own.
const more = new Set();

// usagePanel draws the usage of the APs below node id; folder adds its
// busiest APs, and has its tables a click down: on a folder's Overview the
// chart says the day (Griff, 2026-10-12).
export async function usagePanel(id, folder) {
	// Another time chosen draws the panel again in its place. Only the
	// latest asking is drawn: an answer to an earlier choice that comes in
	// late does not replace it.
	let box = null, asked = 0;
	const pick = async (hours) => {
		over.set(id, hours);
		const mine = ++asked;
		const next = await drawn(id, folder, pick);
		if (mine !== asked || !next || !box) return;
		box.replaceWith(next);
		box = next;
	};
	box = await drawn(id, folder, pick);
	return box;
}

async function drawn(id, folder, pick) {
	const hours = over.get(id) || SPANS[0][0];
	let u;
	try {
		u = await get(`/v1/usage?under=${encodeURIComponent(id)}&hours=${hours}`);
	} catch {
		return null; // an older manager has none
	}
	const n = u.buckets.length;
	const bw = W / n, high = H - TOP - BOTTOM;
	const most = Math.max(1, ...u.buckets.map((b) => b.down + b.up));
	const peak = Math.max(1, ...u.buckets.map((b) => b.clients));
	const bars = u.buckets.map((b, i) => {
		const x = i * bw, hd = (b.down / most) * high, hu = (b.up / most) * high;
		return svg('g', null,
			svg('title', null, `${when(u, b.at)}: ↓ ${size(b.down)} down, ↑ ${size(b.up)} up; ${plural(b.clients, 'client')} at most`),
			svg('rect', { x, y: TOP, width: bw, height: high, class: 'hit' }),
			svg('rect', { x: x + 1, y: TOP + high - hd - hu, width: Math.max(1, bw - 2), height: hu, class: 'up' }),
			svg('rect', { x: x + 1, y: TOP + high - hd, width: Math.max(1, bw - 2), height: hd, class: 'down' }));
	});
	const line = svg('polyline', { class: 'clients', points: u.buckets.map((b, i) => `${((i + 0.5) * bw).toFixed(1)},${(TOP + high - (b.clients / peak) * high).toFixed(1)}`).join(' ') });
	const chart = svg('svg', { viewBox: `0 0 ${W} ${H}`, class: 'usage', role: 'img',
		'aria-label': `Over ${SPANS.find(([t]) => t === hours)?.[1].toLowerCase() || `${hours} hours`}, ${size(u.down)} down and ${size(u.up)} up; at most ${plural(u.peak, 'client')} at once` },
		bars, line,
		svg('line', { x1: 0, x2: W, y1: TOP + high, y2: TOP + high, class: 'base' }));
	// Its times are a row under it: text in a drawing that fills the
	// page's width grows with it.
	const axis = h('div', { class: 'usageaxis', 'aria-hidden': 'true' },
		h('span', null, when(u, u.from)), h('span', null, when(u, u.buckets[Math.floor(n / 2)].at)), h('span', null, 'now'));
	const aps = folder ? u.aps.slice(0, 10) : [];
	const me = !folder && u.aps[0];
	const top = (u.clients || []).slice(0, folder ? 10 : 5);
	// under is the tables under the chart: as they are on an AP's page, and
	// folded on a folder's.
	const under = (tables) => {
		if (!folder || !tables.length) return tables;
		const d = h('details', { class: 'fold', open: more.has(id) }, h('summary', null, 'Top clients and APs'), tables);
		d.addEventListener('toggle', () => (d.open ? more.add(id) : more.delete(id)));
		return d;
	};
	return h('section', { class: 'panel' },
		h('h2', { title: "From the APs' reports. Down is what the APs sent their clients, up what they received from them. A bar shows its numbers on hover." },
			h('span', { class: 'title' }, 'Usage', h('span', { class: 'segmented', role: 'group', 'aria-label': 'Over how long' }, SPANS.map(([t, name]) => h('button', {
				type: 'button', class: hours === t ? 'on' : '', 'aria-pressed': hours === t, onclick: () => pick(t),
			}, name))))),
		h('div', { class: 'sub usagekey' },
			h('span', { class: 'swatch down' }), ` ↓ ${size(u.down)} down · `,
			h('span', { class: 'swatch up' }), ` ↑ ${size(u.up)} up · `,
			h('span', { class: 'swatch clients' }), ` at most ${plural(u.peak, 'client')} at once`),
		chart,
		axis,
		under([top.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['Top clients', '↓ Down', '↑ Up', folder ? 'AP' : null].filter(Boolean).map((t) => h('th', null, t))),
			top.map((c) => h('tr', null,
				h('td', null, c.host || h('span', { class: 'mono' }, c.mac), c.host && h('div', { class: 'sub mono' }, c.mac),
					c.tester && h('div', { class: 'sub', title: "An AP's scan radio: what it used is the speed measurements of its client tests." }, `Aeolus test client${c.tester === 'an AP' ? '' : `, ${c.tester}'s scan radio`}`)),
				h('td', null, size(c.down)), h('td', null, size(c.up)),
				folder && h('td', { class: 'sub' }, c.aps.join(', '))))),
		me && h('div', { class: 'row' }, h('div', { class: 'label' }, 'Calling in'), h('div', { class: 'value' }, strip(u, me))),
		aps.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['AP', '↓ Down', '↑ Up', 'Most clients', 'Calling in'].map((t) => h('th', null, t))),
			aps.map((a) => h('tr', null,
				h('td', null, link(`/aps/${encodeURIComponent(a.ap)}`, a.name)),
				h('td', null, size(a.down)), h('td', null, size(a.up)), h('td', null, String(a.peak)),
				h('td', null, strip(u, a)))))].filter(Boolean)));
}

// spans names buckets of b seconds: half-hours, hours, 8-minute spans.
const spans = (b) => (b === 1800 ? 'half-hours' : b === 3600 ? 'hours' : b % 3600 === 0 ? `${b / 3600}-hour spans` : `${b / 60}-minute spans`);

// strip is an AP's connectivity over the span: a cell for each bucket, filled
// where it reported. The last, not over yet, is left pale until it does, as
// are those before the manager kept usage, which say nothing.
function strip(u, a) {
	const n = a.heard.length;
	const heard = a.heard.filter(Boolean).length;
	const from = u.recorded_from ? new Date(u.recorded_from).getTime() : Infinity;
	const ends = (i) => new Date(u.buckets[i].at).getTime() + u.bucket * 1000;
	const state = (on, i) => (on ? ['on', 'reported'] : i === n - 1 ? ['pending', 'not yet'] : ends(i) <= from ? ['pending', 'no record'] : ['off', 'did not report']);
	const cells = a.heard.map((on, i) => {
		const [cls, says] = state(on, i);
		return svg('rect', { x: i * 4, y: 0, width: 3, height: 12, class: cls }, svg('title', null, `${when(u, u.buckets[i].at)}: ${says}`));
	});
	return h('span', { class: 'strip', title: `reported in ${heard} of the last ${n} ${spans(u.bucket)}` },
		svg('svg', { viewBox: `0 0 ${n * 4} 12`, width: n * 4, height: 12, role: 'img', 'aria-label': `reported in ${heard} of ${n}` }, cells),
		h('span', { class: 'sub' }, ` ${heard} of ${n}`));
}
