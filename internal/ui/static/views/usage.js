// Usage (0108): the Wi-Fi clients and traffic of the APs below a node over
// the last day, from their state reports. Traffic is drawn in bars, what
// the APs sent their clients and what they received stacked, and the most
// clients they had at once as a line; with the totals, the clients that
// moved the most (0110), and on a folder its busiest APs by traffic.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { size } from '../format.js';

const SVG = 'http://www.w3.org/2000/svg';
const W = 480, H = 120, TOP = 8, BOTTOM = 18;

function svg(tag, attrs, ...kids) {
	const el = document.createElementNS(SVG, tag);
	for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, String(v));
	for (const k of kids.flat()) if (k != null && k !== false) el.append(k instanceof Node ? k : document.createTextNode(String(k)));
	return el;
}

const clock = (t) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
const plural = (n, one) => `${n} ${one}${n === 1 ? '' : 's'}`;

// usagePanel draws the usage of the APs below node id; folder adds its
// busiest APs.
export async function usagePanel(id, folder) {
	let u;
	try {
		u = await get(`/v1/usage?under=${encodeURIComponent(id)}&hours=24`);
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
			svg('title', null, `${clock(b.at)}: ↓ ${size(b.down)} to clients, ↑ ${size(b.up)} from them; ${plural(b.clients, 'client')} at most`),
			svg('rect', { x, y: TOP, width: bw, height: high, class: 'hit' }),
			svg('rect', { x: x + 1, y: TOP + high - hd - hu, width: Math.max(1, bw - 2), height: hu, class: 'up' }),
			svg('rect', { x: x + 1, y: TOP + high - hd, width: Math.max(1, bw - 2), height: hd, class: 'down' }));
	});
	const line = svg('polyline', { class: 'clients', points: u.buckets.map((b, i) => `${((i + 0.5) * bw).toFixed(1)},${(TOP + high - (b.clients / peak) * high).toFixed(1)}`).join(' ') });
	const chart = svg('svg', { viewBox: `0 0 ${W} ${H}`, class: 'usage', role: 'img',
		'aria-label': `Over the last 24 hours, ${size(u.down)} to clients and ${size(u.up)} from them; at most ${plural(u.peak, 'client')} at once` },
		bars, line,
		svg('line', { x1: 0, x2: W, y1: TOP + high, y2: TOP + high, class: 'base' }),
		svg('text', { x: 0, y: H - 4, class: 'axis' }, clock(u.from)),
		svg('text', { x: W / 2, y: H - 4, class: 'axis', 'text-anchor': 'middle' }, clock(u.buckets[Math.floor(n / 2)].at)),
		svg('text', { x: W, y: H - 4, class: 'axis', 'text-anchor': 'end' }, 'now'));
	const aps = folder ? u.aps.slice(0, 10) : [];
	const me = !folder && u.aps[0];
	const top = (u.clients || []).slice(0, folder ? 10 : 5);
	return h('section', { class: 'panel' },
		h('h2', null, 'Usage', h('span', { class: 'note' }, "the last 24 hours, from the APs' reports; a bar shows its numbers on hover")),
		h('div', { class: 'sub' },
			h('span', { class: 'swatch down' }), ` ↓ ${size(u.down)} to clients · `,
			h('span', { class: 'swatch up' }), ` ↑ ${size(u.up)} from them · `,
			h('span', { class: 'swatch clients' }), ` at most ${plural(u.peak, 'client')} at once`),
		chart,
		top.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['Top clients', '↓ To it', '↑ From it', folder ? 'On' : null].filter(Boolean).map((t) => h('th', null, t))),
			top.map((c) => h('tr', null,
				h('td', null, c.host || h('span', { class: 'mono' }, c.mac), c.host && h('div', { class: 'sub mono' }, c.mac)),
				h('td', null, size(c.down)), h('td', null, size(c.up)),
				folder && h('td', { class: 'sub' }, c.aps.join(', '))))),
		me && h('div', { class: 'row' }, h('div', { class: 'label' }, 'Calling in'), h('div', { class: 'value' }, strip(u, me))),
		aps.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['AP', '↓ To clients', '↑ From them', 'Most clients', 'Calling in'].map((t) => h('th', null, t))),
			aps.map((a) => h('tr', null,
				h('td', null, link(`/aps/${encodeURIComponent(a.ap)}`, a.name)),
				h('td', null, size(a.down)), h('td', null, size(a.up)), h('td', null, String(a.peak)),
				h('td', null, strip(u, a))))));
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
		return svg('rect', { x: i * 4, y: 0, width: 3, height: 12, class: cls }, svg('title', null, `${clock(u.buckets[i].at)}: ${says}`));
	});
	return h('span', { class: 'strip', title: `reported in ${heard} of the last ${n} ${spans(u.bucket)}` },
		svg('svg', { viewBox: `0 0 ${n * 4} 12`, width: n * 4, height: 12, role: 'img', 'aria-label': `reported in ${heard} of ${n}` }, cells),
		h('span', { class: 'sub' }, ` ${heard} of ${n}`));
}
