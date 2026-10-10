// Usage (0108): the Wi-Fi clients and traffic of the APs below a node over
// the last day, from their state reports. Traffic is drawn in bars, what
// the APs sent their clients and what they received stacked, and the most
// clients they had at once as a line; with the totals, and on a folder its
// busiest APs by traffic.

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
	const busy = folder ? u.aps.filter((a) => a.down + a.up > 0).slice(0, 6) : [];
	return h('section', { class: 'panel' },
		h('h2', null, 'Usage', h('span', { class: 'note' }, "the last 24 hours, from the APs' reports; a bar shows its numbers on hover")),
		h('div', { class: 'sub' },
			h('span', { class: 'swatch down' }), ` ↓ ${size(u.down)} to clients · `,
			h('span', { class: 'swatch up' }), ` ↑ ${size(u.up)} from them · `,
			h('span', { class: 'swatch clients' }), ` at most ${plural(u.peak, 'client')} at once`),
		chart,
		busy.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['AP', '↓ To clients', '↑ From them', 'Most clients'].map((t) => h('th', null, t))),
			busy.map((a) => h('tr', null,
				h('td', null, link(`/aps/${encodeURIComponent(a.ap)}`, a.name)),
				h('td', null, size(a.down)), h('td', null, size(a.up)), h('td', null, String(a.peak))))));
}
