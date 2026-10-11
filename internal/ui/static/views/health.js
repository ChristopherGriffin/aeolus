// An AP's own health, and what happened to it (0119): how busy its
// processor is, its memory, storage and temperature now and over a day or a
// week, from each of its state reports; and its history, newest first: its
// restarts and silences, agent and firmware changes, radios moving channel,
// configs applied, alerts and what it was asked to do.

import { h } from '../dom.js';
import { get } from '../api.js';
import { ago } from '../format.js';

const SVG = 'http://www.w3.org/2000/svg';
const W = 480, H = 130, TOP = 8, BOTTOM = 18;

function svg(tag, attrs, ...kids) {
	const el = document.createElementNS(SVG, tag);
	for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, String(v));
	for (const k of kids.flat()) if (k != null && k !== false) el.append(k instanceof Node ? k : document.createTextNode(String(k)));
	return el;
}

const clock = (t, days) => new Date(t).toLocaleString([], days ? { month: 'short', day: 'numeric', hour: '2-digit' } : { hour: '2-digit', minute: '2-digit' });
const stamp = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
const mb = (kb) => (kb >= 1048576 ? `${(kb / 1048576).toFixed(1)} GB` : `${Math.round(kb / 1024)} MB`);

function upFor(s) {
	if (s < 120) return `${s} s`;
	if (s < 7200) return `${Math.round(s / 60)} min`;
	if (s < 172800) return `${Math.round(s / 3600)} h`;
	return `${Math.round(s / 86400)} days`;
}

// What the page shows: a day or a week of health, a week or a month of
// history. Kept while the page is redrawn.
const view = { hours: 24, history: 168 };

// tiles are how the AP is doing now, by its latest report.
function tiles(now) {
	const x = now?.health;
	if (!x) return h('div', { class: 'banner info' }, 'It has not said how it is doing yet: an AP reports its health from agent v0.70.0 on.');
	const tile = (label, big, sub, cls) => h('div', { class: `tile ${cls || ''}` },
		h('div', { class: 'tlabel' }, label), h('div', { class: 'tbig' }, big), h('div', { class: 'tsub' }, sub));
	const out = [];
	const load = x.load ? `load ${x.load[0]} on ${x.cores || '?'} core${x.cores === 1 ? '' : 's'}` : '';
	out.push(tile('Processor', x.cpu != null ? `${x.cpu}%` : '—', x.cpu != null ? `busy since its last report · ${load}` : `${load || 'no figure yet'}`,
		x.cpu >= 90 ? 'bad' : x.cpu >= 70 ? 'warn' : x.cpu != null ? 'ok' : ''));
	if (x.mem_total) {
		const free = Math.floor((100 * x.mem_available) / x.mem_total);
		out.push(tile('Memory', `${100 - free}%`, `in use · ${mb(x.mem_available)} free of ${mb(x.mem_total)}`, free < 5 ? 'bad' : free < 10 ? 'warn' : 'ok'));
	}
	if (x.storage_total) {
		const free = Math.floor((100 * x.storage_free) / x.storage_total);
		out.push(tile('Storage', `${100 - free}%`, `in use · ${mb(x.storage_free)} free of ${mb(x.storage_total)}, where its config is kept`, free < 5 ? 'warn' : 'ok'));
	}
	if (x.temp != null) out.push(tile('Temperature', `${x.temp} °C`, 'its hottest sensor', x.temp >= 95 ? 'bad' : x.temp >= 85 ? 'warn' : 'ok'));
	out.push(tile('Up', upFor(now.uptime), `since ${stamp(new Date(now.at).getTime() - now.uptime * 1000)}`, ''));
	if (x.procs) out.push(tile('Running', String(x.procs), `processes${x.conntrack_max ? ` · ${x.conntrack} connections tracked of ${x.conntrack_max}` : ''}`, ''));
	return h('div', { class: 'tiles' }, out);
}

// chart draws the processor and memory over the time shown, each from 0 to
// 100 percent; a point shows its numbers on hover.
function chart(hl) {
	const pts = hl.points || [];
	if (pts.length < 2) return h('p', { class: 'sub pad' }, 'A chart needs two reports with its health; they come every five minutes.');
	const high = H - TOP - BOTTOM;
	const t0 = new Date(pts[0].at).getTime(), t1 = Math.max(new Date(pts[pts.length - 1].at).getTime(), t0 + 1);
	const x = (p) => ((new Date(p.at).getTime() - t0) / (t1 - t0)) * W;
	const y = (v) => TOP + high - (Math.min(100, Math.max(0, v)) / 100) * high;
	const line = (key, cls) => {
		const have = pts.filter((p) => p[key] != null);
		return have.length > 1 && svg('polyline', { class: cls, points: have.map((p) => `${x(p).toFixed(1)},${y(p[key]).toFixed(1)}`).join(' ') });
	};
	const bw = W / pts.length;
	const days = hl.hours > 48;
	const hits = pts.map((p) => svg('g', null,
		svg('title', null, `${clock(p.at, days)}: processor ${p.cpu != null ? `${p.cpu}%` : '—'}, memory ${p.mem != null ? `${p.mem}%` : '—'}${p.temp != null ? `, ${p.temp} °C` : ''}, ${p.clients} client${p.clients === 1 ? '' : 's'}`),
		svg('rect', { x: Math.max(0, x(p) - bw / 2), y: TOP, width: bw, height: high, class: 'hit' })));
	const mid = pts[Math.floor(pts.length / 2)];
	return svg('svg', { viewBox: `0 0 ${W} ${H}`, class: 'usage healthchart', role: 'img', 'aria-label': `Its processor and memory over the last ${hl.hours} hours` },
		svg('line', { x1: 0, x2: W, y1: y(50), y2: y(50), class: 'half' }),
		hits, line('mem', 'mem'), line('cpu', 'cpu'),
		svg('line', { x1: 0, x2: W, y1: TOP + high, y2: TOP + high, class: 'base' }),
		svg('text', { x: 0, y: H - 4, class: 'axis' }, clock(pts[0].at, days)),
		svg('text', { x: W / 2, y: H - 4, class: 'axis', 'text-anchor': 'middle' }, clock(mid.at, days)),
		svg('text', { x: W, y: H - 4, class: 'axis', 'text-anchor': 'end' }, 'now'));
}

// The kinds of thing that happen to an AP, as they are named.
const KIND = {
	restart: 'Restart', silence: 'Silence', agent: 'Agent', firmware: 'Firmware', radio: 'Radio', channel: 'Channel',
	uplink: 'Uplink', first: 'First report', config: 'Config', alert: 'Alert', action: 'Asked to',
};
const SEVERITY = { critical: 'bad', warning: 'warn', info: 'idle' };

function history(tl) {
	const ev = tl.events || [];
	if (!ev.length) return h('p', { class: 'sub pad' }, 'Nothing has happened to it in that time that was recorded. Its history is kept from manager v0.70.0 on.');
	return h('table', { class: 'list' },
		h('tr', null, ['When', 'What', ''].map((t) => h('th', null, t))),
		ev.map((e) => h('tr', null,
			h('td', { class: 'nowrap', title: stamp(e.at) }, ago(e.at), h('div', { class: 'sub' }, stamp(e.at))),
			h('td', null, h('span', { class: `chip ${SEVERITY[e.severity] || 'idle'}` }, KIND[e.kind] || e.kind)),
			h('td', null, e.text))));
}

// healthTab draws an AP's Health tab.
export async function healthTab(ctx, id) {
	const enc = encodeURIComponent(id);
	const box = h('div');
	const seg = (now, set, choices) => h('span', { class: 'segmented', role: 'group' }, choices.map(([v, name]) => h('button', {
		type: 'button', class: now === v ? 'on' : '', 'aria-pressed': now === v, onclick: () => { set(v); draw(); },
	}, name)));
	const draw = async () => {
		let hl, tl;
		try {
			[hl, tl] = await Promise.all([get(`/v1/aps/${enc}/health?hours=${view.hours}`), get(`/v1/aps/${enc}/timeline?hours=${view.history}`)]);
		} catch (e) {
			box.replaceChildren(h('div', { class: 'error' }, e.message));
			return;
		}
		box.replaceChildren(
			tiles(hl.now),
			h('section', { class: 'panel' },
				h('h2', null, h('span', { class: 'title' }, 'Processor and memory', seg(view.hours, (v) => { view.hours = v; }, [[24, 'A day'], [168, 'A week']])),
					h('span', { class: 'note' }, 'from each of its reports; where there are many, the worst of each run')),
				h('div', { class: 'sub pad' }, h('span', { class: 'swatch cpu' }), ' processor busy · ', h('span', { class: 'swatch mem' }), ' memory in use, each from 0 to 100%'),
				chart(hl)),
			h('section', { class: 'panel' },
				h('h2', null, h('span', { class: 'title' }, 'History', seg(view.history, (v) => { view.history = v; }, [[168, 'A week'], [720, '30 days']])),
					h('span', { class: 'note' }, 'what happened to it, newest first')),
				history(tl)));
	};
	await draw();
	return box;
}
