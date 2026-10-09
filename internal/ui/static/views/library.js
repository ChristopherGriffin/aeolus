// The library: AP templates (0085). Each is a named set of Locations
// settings for the APs of one or more boards, made at the Org or a folder
// and offered there and below; a folder picks one for a board, and the
// board's APs below take it. Each AP that takes one either follows it or has
// some of its fields replaced by settings closer to the AP.

import { h, link } from '../dom.js';
import { get } from '../api.js';

const val = (v) => (v && typeof v === 'object' && v.sealed ? '(sealed)' : JSON.stringify(v));

export async function libraryPage(ctx) {
	const { templates } = await get('/v1/library');
	return {
		main: [
			h('div', { class: 'head' },
				h('div', null,
					h('h1', null, 'Library'),
					h('div', { class: 'sub' }, 'AP templates: settings for every AP of a kind, picked by a folder for the APs below it. A new kind of AP gets an empty one when its first AP is adopted.'))),
			templates.length === 0
				? h('div', { class: 'banner info' }, 'No templates yet. Adopting an AP of a new kind makes one.')
				: h('section', { class: 'panel' },
					h('table', { class: 'list' },
						h('tr', null, ['Template', 'Boards', 'Made at', 'Settings', 'APs'].map((c) => h('th', null, c))),
						templates.map((t) => {
							const custom = t.aps.filter((a) => !a.follows);
							const paths = Object.keys(t.values).sort();
							return h('tr', null,
								h('td', null, h('div', null, t.name), h('div', { class: 'mono sub' }, t.id)),
								h('td', { class: 'mono' }, t.boards.map((b) => h('div', null, b))),
								h('td', null, link(`/locations/${encodeURIComponent(t.at)}`, ctx.name('locations', t.at))),
								h('td', null, paths.length === 0
									? h('span', { class: 'sub' }, 'none yet')
									: paths.map((p) => h('div', { class: 'mono' }, `${p} = ${val(t.values[p])}`))),
								h('td', null,
									h('div', null, `${t.aps.length} take it`),
									custom.length > 0 && h('div', { class: 'sub' }, `${custom.length} with settings of their own: `,
										custom.map((a, i) => [i > 0 && ', ', link(`/aps/${encodeURIComponent(a.ap)}`, ctx.name('locations', a.ap))]))));
						}))),
		],
	};
}
