// Building the page (0042). Everything shown comes from the API and is added
// as text, never as HTML, so a name, an SSID or a reason cannot inject markup.

// h makes an element. attrs: "class", "on<event>" handlers, "style" as an
// object of CSS properties (set through the DOM, which the page's CSP
// allows), or plain attributes. Children are nodes, strings or arrays;
// null and false are skipped.
export function h(tag, attrs, ...children) {
	const el = document.createElement(tag);
	for (const [k, v] of Object.entries(attrs || {})) {
		if (v == null || v === false) continue;
		if (k === 'class') el.className = v;
		else if (k === 'style') for (const [p, x] of Object.entries(v)) el.style.setProperty(p, x);
		else if (k.startsWith('on')) el.addEventListener(k.slice(2).toLowerCase(), v);
		else el.setAttribute(k, v === true ? '' : String(v));
	}
	add(el, children);
	return el;
}

function add(el, children) {
	for (const c of children.flat(Infinity)) {
		if (c == null || c === false) continue;
		el.append(c instanceof Node ? c : document.createTextNode(String(c)));
	}
}

// link makes an in-app link to a route.
export function link(route, ...children) {
	return h('a', { href: '#' + route }, ...children);
}

const SVG = 'http://www.w3.org/2000/svg';

// icon draws one of the few icons the UI uses.
export function icon(name) {
	const paths = {
		folder: ['M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z'],
		ap: ['M8.5 13.5a5 5 0 0 1 7 0', 'M5.5 10.5a9 9 0 0 1 13 0'],
		lock: ['M8 11V8a4 4 0 0 1 8 0v3'],
		chevron: ['M6 9l6 6 6-6'],
	}[name] || [];
	const svg = document.createElementNS(SVG, 'svg');
	for (const [k, v] of Object.entries({ width: 16, height: 16, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor',
		'stroke-width': 2, 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'aria-hidden': 'true' })) svg.setAttribute(k, v);
	for (const d of paths) {
		const p = document.createElementNS(SVG, 'path');
		p.setAttribute('d', d);
		svg.append(p);
	}
	if (name === 'ap') {
		const c = document.createElementNS(SVG, 'circle');
		for (const [k, v] of Object.entries({ cx: 12, cy: 17, r: 1.5 })) c.setAttribute(k, v);
		svg.append(c);
	}
	if (name === 'lock') {
		const r = document.createElementNS(SVG, 'rect');
		for (const [k, v] of Object.entries({ x: 5, y: 11, width: 14, height: 9, rx: 2 })) r.setAttribute(k, v);
		svg.append(r);
	}
	return svg;
}
