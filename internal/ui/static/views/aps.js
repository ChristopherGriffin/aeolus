// Every AP at a glance (0042).

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { ago } from '../format.js';
import { apStatus } from '../layout.js';

export async function apsPage(ctx) {
	const { aps } = await get('/v1/aps');
	const where = (a) => a.ancestry.slice(1, -1).map((id) => ctx.name('locations', id)).join(' › ') || ctx.org;
	const counts = {};
	const rows = aps.map((a) => {
		const st = apStatus(a);
		counts[st.label] = (counts[st.label] || 0) + 1;
		return h('tr', null,
			h('td', null, h('span', { class: 'chip ' + st.chip }, st.label)),
			h('td', null, link(`/aps/${encodeURIComponent(a.id)}`, a.name)),
			h('td', null, where(a)),
			h('td', { class: 'mono' }, a.seen?.running != null ? `${a.seen.running} / ${a.version}` : `— / ${a.version}`),
			h('td', null, ago(a.seen?.at)),
			h('td', { class: 'mono' }, a.seen?.source || '—'));
	});
	return {
		main: [
			h('div', { class: 'head' },
				h('div', null,
					h('h1', null, 'APs'),
					h('div', { class: 'sub' }, aps.length === 0 ? 'No APs yet.' : Object.entries(counts).map(([k, n]) => `${n} ${k.toLowerCase()}`).join(' · ')))),
			aps.length > 0 && h('section', { class: 'panel' },
				h('table', { class: 'list' },
					h('tr', null, ['State', 'AP', 'Where', 'Running / its version', 'Last seen', 'From'].map((c) => h('th', null, c))),
					rows)),
		],
		refresh: 30,
	};
}
