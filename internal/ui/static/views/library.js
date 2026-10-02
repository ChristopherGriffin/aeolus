// The library: concentrators and their labeled VNIs (0021, 0023).

import { h } from '../dom.js';
import { get } from '../api.js';

export async function libraryPage(ctx) {
	const { concentrators } = await get('/v1/library');
	return {
		main: [
			h('div', { class: 'head' },
				h('div', null,
					h('h1', null, 'Library'),
					h('div', { class: 'sub' }, 'Concentrators for VXLAN transports. Defined once for the Org; networks pick one and a VNI from here.'))),
			concentrators.length === 0
				? h('div', { class: 'banner info' }, 'The library is empty.')
				: h('section', { class: 'panel' },
					h('table', { class: 'list' },
						h('tr', null, ['Concentrator', 'Address', 'Port', 'MTU', 'May be used at', 'VNIs'].map((c) => h('th', null, c))),
						concentrators.map((k) => h('tr', null,
							h('td', null, h('div', null, k.name), h('div', { class: 'mono sub' }, k.id)),
							h('td', { class: 'mono' }, k.address),
							h('td', { class: 'mono' }, String(k.port)),
							h('td', { class: 'mono' }, String(k.mtu)),
							h('td', null, k.scope?.length ? k.scope.map((id) => ctx.name('locations', id)).join(', ') : 'everywhere'),
							h('td', null, Object.entries(k.vnis || {}).map(([vni, label]) => h('div', null, h('span', { class: 'mono' }, vni), ' · ', label))))))),
		],
	};
}
