// The radio neighbours as a map (0073): each AP a node, each pair of
// neighbours on a band a line, coloured by how strongly they hear each
// other, the weaker way: the mesh RRM and power control (0077) work over.
// Lines are drawn shorter the stronger the signal, so APs that hear each
// other well sit close. A line is solid where hellos go both ways, dashed
// where one is only heard. APs outside this folder are drawn faint.

import { h } from '../dom.js';
import { bandName } from '../format.js';

const SVG = 'http://www.w3.org/2000/svg';
const W = 640, MIN_H = 200, MAX_H = 560, R = 16;

// The signal classes, strongest first, with the weakest signal in each.
const CLASSES = [['strong', -60, '−60 dBm or better'], ['good', -70, 'to −70'], ['fair', -80, 'to −80'], ['weak', -Infinity, 'weaker']];
const classOf = (s) => (s == null ? 'unknown' : CLASSES.find(([, min]) => s >= min)[0]);

function svg(tag, attrs, ...kids) {
	const el = document.createElementNS(SVG, tag);
	for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, String(v));
	for (const k of kids.flat()) if (k != null && k !== false) el.append(k instanceof Node ? k : document.createTextNode(String(k)));
	return el;
}

// links reads, from the APs' reports (rows), every pair of neighbours on
// each band: how strongly each hears the other, and whether hellos go both
// ways. A pair both report is joined: each AP's own reading counts first.
function links(rows) {
	const out = new Map();
	for (const { ap, cfg } of rows)
		for (const n of cfg?.condition?.state?.report?.rrm?.neighbours || [])
			for (const b of n.bands || []) {
				const [x, y] = [ap.id, n.ap].sort();
				const k = `${b.band} ${x} ${y}`;
				const l = out.get(k) || { band: b.band, a: x, b: y, hears: {}, up: false };
				// hears[p] is how strongly p hears the other.
				if (b.signal != null) l.hears[ap.id] = b.signal;
				if (b.their_signal != null && l.hears[n.ap] == null) l.hears[n.ap] = b.their_signal;
				l.up = l.up || n.state === 'up';
				out.set(k, l);
			}
	for (const l of out.values()) {
		const s = Object.values(l.hears);
		l.weaker = s.length ? Math.min(...s) : null;
	}
	return [...out.values()].filter((l) => l.weaker != null);
}

// place lays the nodes out from a circle in the given order: each line
// pulls its ends to a length that follows its signal, a strong one short,
// and every node pushes the others away.
function place(order, edges) {
	const n = order.length;
	const p = new Map(order.map((id, i) => [id, { x: Math.cos((2 * Math.PI * i) / n) * 150, y: Math.sin((2 * Math.PI * i) / n) * 150 }]));
	if (n === 1) p.get(order[0]).x = p.get(order[0]).y = 0;
	const rest = (s) => 120 + Math.min(1, Math.max(0, (-45 - s) / 45)) * 180;
	const pts = [...p.values()];
	for (let it = 0, step = 10; it < 400; it++, step *= 0.988) {
		const f = new Map(pts.map((q) => [q, { x: 0, y: 0 }]));
		for (let i = 0; i < n; i++)
			for (let j = i + 1; j < n; j++) {
				const a = pts[i], b = pts[j];
				const dx = a.x - b.x, dy = a.y - b.y, d2 = Math.max(dx * dx + dy * dy, 1), d = Math.sqrt(d2);
				const push = 30000 / d2;
				f.get(a).x += (push * dx) / d; f.get(a).y += (push * dy) / d;
				f.get(b).x -= (push * dx) / d; f.get(b).y -= (push * dy) / d;
			}
		for (const e of edges) {
			const a = p.get(e.a), b = p.get(e.b);
			const dx = b.x - a.x, dy = b.y - a.y, d = Math.max(Math.hypot(dx, dy), 1);
			const pull = (d - rest(e.weaker)) * 0.08;
			f.get(a).x += (pull * dx) / d; f.get(a).y += (pull * dy) / d;
			f.get(b).x -= (pull * dx) / d; f.get(b).y -= (pull * dy) / d;
		}
		// A node near a line it isn't on is pushed off it, and the line the
		// other way.
		for (const e of edges) {
			const a = p.get(e.a), b = p.get(e.b);
			const dx = b.x - a.x, dy = b.y - a.y, l2 = dx * dx + dy * dy || 1;
			for (const q of pts) {
				if (q === a || q === b) continue;
				const s = Math.max(0, Math.min(1, ((q.x - a.x) * dx + (q.y - a.y) * dy) / l2));
				let ox = q.x - a.x - s * dx, oy = q.y - a.y - s * dy, d = Math.hypot(ox, oy);
				if (d >= 70) continue;
				if (d < 0.5) { ox = -dy; oy = dx; d = Math.hypot(ox, oy) || 1; }
				const push = (70 - d) * 0.6;
				f.get(q).x += (push * ox) / d; f.get(q).y += (push * oy) / d;
				f.get(a).x -= (push * ox) / d / 2; f.get(a).y -= (push * oy) / d / 2;
				f.get(b).x -= (push * ox) / d / 2; f.get(b).y -= (push * oy) / d / 2;
			}
		}
		for (const [q, v] of f) {
			const m = Math.hypot(v.x, v.y);
			if (m > 0) { q.x += (v.x / m) * Math.min(m, step); q.y += (v.y / m) * Math.min(m, step); }
		}
	}
	// No two nodes closer than GAP, so their names don't run together.
	const GAP = 120;
	for (let it = 0; it < 60; it++)
		for (let i = 0; i < n; i++)
			for (let j = i + 1; j < n; j++) {
				const a = pts[i], b = pts[j];
				const dx = a.x - b.x, dy = a.y - b.y, d = Math.max(Math.hypot(dx, dy), 0.01);
				if (d >= GAP) continue;
				const m = (GAP - d) / 2;
				a.x += (dx / d) * m; a.y += (dy / d) * m;
				b.x -= (dx / d) * m; b.y -= (dy / d) * m;
			}
	return p;
}

// tangles counts what makes a layout hard to read: lines that cross, and
// lines that pass close by a node they don't join, which count more.
function tangles(p, edges) {
	const cross = (a, b, c, d) => {
		const o = (p1, p2, p3) => Math.sign((p2.x - p1.x) * (p3.y - p1.y) - (p2.y - p1.y) * (p3.x - p1.x));
		return o(a, b, c) * o(a, b, d) < 0 && o(c, d, a) * o(c, d, b) < 0;
	};
	const near = (q, a, b) => {
		const dx = b.x - a.x, dy = b.y - a.y, l2 = dx * dx + dy * dy || 1;
		const t = Math.max(0, Math.min(1, ((q.x - a.x) * dx + (q.y - a.y) * dy) / l2));
		return Math.hypot(q.x - a.x - t * dx, q.y - a.y - t * dy) < 40;
	};
	let n = 0;
	for (let i = 0; i < edges.length; i++) {
		const a = p.get(edges[i].a), b = p.get(edges[i].b);
		for (let j = i + 1; j < edges.length; j++) {
			const e = edges[j];
			if ([e.a, e.b].some((x) => x === edges[i].a || x === edges[i].b)) continue;
			if (cross(a, b, p.get(e.a), p.get(e.b))) n += 1;
		}
		for (const [id, q] of p)
			if (id !== edges[i].a && id !== edges[i].b && near(q, a, b)) n += 3;
	}
	return n;
}

// layout tries the nodes in a few orders and keeps the least tangled; turns
// it so it runs across, the drawing being wider than tall; and fits it,
// scaled down only where it must be, in a drawing as tall as it needs. The
// same reports always give the same map. It returns the places and the
// height.
function layout(ids, edges) {
	// An order is the IDs sorted by a hash of each, mixed with the try's
	// number.
	const hash = (s) => [...s].reduce((x, ch) => (Math.imul(x, 31) + ch.charCodeAt(0)) | 0, 7);
	const mix = (x) => {
		x = Math.imul(x ^ (x >>> 16), 0x7feb352d);
		x = Math.imul(x ^ (x >>> 15), 0x846ca68b);
		return (x ^ (x >>> 16)) >>> 0;
	};
	let best = null, least = Infinity;
	for (let k = 0; k < (ids.length > 10 ? 6 : 24) && least > 0; k++) {
		const key = (id) => mix(hash(id) ^ Math.imul(k, 0x9e3779b9));
		const order = k === 0 ? ids : [...ids].sort((a, b) => key(a) - key(b));
		const p = place(order, edges);
		const n = tangles(p, edges);
		if (n < least) { best = p; least = n; }
	}
	const pts = ids.map((id) => best.get(id));
	// Turn the long axis across.
	const mx = pts.reduce((s, q) => s + q.x, 0) / pts.length, my = pts.reduce((s, q) => s + q.y, 0) / pts.length;
	let sxx = 0, syy = 0, sxy = 0;
	for (const q of pts) { sxx += (q.x - mx) ** 2; syy += (q.y - my) ** 2; sxy += (q.x - mx) * (q.y - my); }
	const th = 0.5 * Math.atan2(2 * sxy, sxx - syy), c = Math.cos(-th), s = Math.sin(-th);
	for (const q of pts) {
		const x = q.x - mx, y = q.y - my;
		q.x = x * c - y * s;
		q.y = x * s + y * c;
	}
	const xs = pts.map((q) => q.x), ys = pts.map((q) => q.y);
	const [x0, x1, y0, y1] = [Math.min(...xs), Math.max(...xs), Math.min(...ys), Math.max(...ys)];
	const k = Math.min(1, (W - 160) / Math.max(x1 - x0, 1), (MAX_H - 120) / Math.max(y1 - y0, 1));
	const height = Math.max(MIN_H, (y1 - y0) * k + 120);
	for (const q of pts) {
		q.x = W / 2 + (q.x - (x0 + x1) / 2) * k;
		q.y = height / 2 - 12 + (q.y - (y0 + y1) / 2) * k;
	}
	return { pos: best, height };
}

// draw draws the map for one band.
function draw(ctx, rows, band) {
	const here = new Map(rows.map(({ ap, cfg }) => [ap.id, { ap, report: cfg?.condition?.state?.report }]));
	const edges = links(rows).filter((l) => l.band === band);
	const ids = [...new Set([...here.keys(), ...edges.flatMap((e) => [e.a, e.b])])].sort();
	if (!ids.length) return h('div', { class: 'sub' }, 'No APs here yet.');
	const { pos, height } = layout(ids, edges);
	const name = (id) => here.get(id)?.ap.name || ctx.name('locations', id);
	const lines = edges.map((e) => {
		const a = pos.get(e.a), b = pos.get(e.b);
		const way = (x, y) => (e.hears[x] != null ? `${name(x)} hears ${name(y)} at ${e.hears[x]} dBm` : `${name(x)} doesn't say how it hears ${name(y)}`);
		const both = e.hears[e.a] != null && e.hears[e.b] != null;
		// The label sits beside the line's middle, on its upper side.
		const len = Math.hypot(b.x - a.x, b.y - a.y) || 1;
		let nx = -(b.y - a.y) / len, ny = (b.x - a.x) / len;
		if (ny > 0) { nx = -nx; ny = -ny; }
		const mx = (a.x + b.x) / 2 + nx * 10, my = (a.y + b.y) / 2 + ny * 10 + 4;
		return svg('g', { class: `edge ${classOf(e.weaker)}${e.up ? '' : ' heard'}` },
			svg('title', null, `${way(e.a, e.b)}; ${way(e.b, e.a)}; ${e.up ? 'hellos go both ways' : 'not neighbours both ways'}`),
			svg('line', { x1: a.x, y1: a.y, x2: b.x, y2: b.y }),
			svg('text', { x: mx, y: my, 'text-anchor': 'middle' }, both && e.hears[e.a] !== e.hears[e.b] ? `${e.hears[e.a]} / ${e.hears[e.b]}` : `${e.weaker}`));
	});
	const nodes = ids.map((id) => {
		const q = pos.get(id), at = here.get(id);
		const radio = (at?.report?.radios || []).find((r) => r.band === band);
		const held = (at?.report?.rrm?.apc || []).find((x) => x.band === band && x.power != null);
		const facts = [radio?.channel && `ch ${radio.channel}`, held ? `${held.power} dBm` : null].filter(Boolean).join(' · ');
		return svg('g', { class: `node${at ? '' : ' away'}` },
			svg('title', null, at ? `${name(id)}${facts ? `: ${facts}` : ''}` : `${name(id)}: not in this folder`),
			svg('circle', { cx: q.x, cy: q.y, r: R }),
			svg('path', { d: `M${q.x - 6} ${q.y + 1}a8 8 0 0 1 12 0M${q.x - 9.5} ${q.y - 3}a13 13 0 0 1 19 0`, class: 'waves' }),
			svg('circle', { cx: q.x, cy: q.y + 5, r: 1.8, class: 'dot' }),
			svg('text', { x: q.x, y: q.y + R + 15, 'text-anchor': 'middle', class: 'name' }, name(id)),
			facts && svg('text', { x: q.x, y: q.y + R + 29, 'text-anchor': 'middle', class: 'facts' }, facts));
	});
	return [
		svg('svg', { viewBox: `0 0 ${W} ${Math.round(height)}`, class: 'nmap', role: 'img',
			'aria-label': `${bandName(band)} neighbours: ${edges.map((e) => `${name(e.a)} and ${name(e.b)} at ${e.weaker} dBm`).join('; ') || 'none'}` },
		lines, nodes),
		!edges.length && h('div', { class: 'sub' }, `No two APs here hear each other on ${bandName(band)}.`),
	];
}

// neighbourMap is the map's panel, with a switch between the bands, 2.4 GHz
// first, where APs hear each other furthest.
export function neighbourMap(ctx, rows) {
	const bands = ['2g', '5g', '6g'].filter((b) => links(rows).some((l) => l.band === b) ||
		rows.some(({ cfg }) => (cfg?.condition?.state?.report?.radios || []).some((r) => r.band === b)));
	if (!bands.length) return null;
	const body = h('div', { class: 'nmapbox' });
	const tabs = h('span', { class: 'controls seg' });
	const show = (band) => {
		tabs.replaceChildren(...bands.map((b) => h('button', { type: 'button', class: `button small${b === band ? ' on' : ''}`, 'aria-pressed': b === band,
			onclick: () => show(b) }, bandName(b))));
		body.replaceChildren(...[draw(ctx, rows, band)].flat().filter(Boolean));
	};
	show(bands[0]);
	return h('section', { class: 'panel' },
		h('h2', null, 'Neighbour map', h('span', { class: 'note' }, 'how strongly each pair hears each other, the weaker way'), tabs),
		body,
		h('div', { class: 'nmaplegend' },
			CLASSES.map(([c, , text]) => h('span', { class: `key ${c}` }, text)),
			h('span', { class: 'key heard' }, 'heard, no hellos both ways')));
}
