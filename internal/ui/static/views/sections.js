// The parts of a Locations folder or AP page besides its networks and
// interfaces (0047, 0053, 0072, 0075): its APs' radios as each last
// reported, on a folder's APs tab, and its system settings.

import { h, link } from '../dom.js';
import { get, schema } from '../api.js';
import { bandName, ago, value } from '../format.js';
import { fieldPanels } from './fields.js';
import { systemEditor } from './system.js';

// only keeps the fields whose paths pass keep.
export function only(fields, keep) {
	return Object.fromEntries(Object.entries(fields || {}).filter(([p]) => keep(p)));
}

// configs reads each AP's config and condition, for the live views.
export async function configs(aps) {
	const all = await Promise.allSettled(aps.map((a) => get(`/v1/aps/${encodeURIComponent(a.id)}/config`)));
	return aps.map((a, i) => ({ ap: a, cfg: all[i].status === 'fulfilled' ? all[i].value : null }));
}

// channelsSection lists each AP's radios as it last reported them, on a
// folder's APs tab (0075): the channel each is on, its width and clients,
// beside what Aeolus sets. With automatic channels each AP picks its own
// (0045), within the band's channel set.
export function channelsSection(ctx, rows) {
	if (!rows.length) return h('div', { class: 'banner info' }, 'No APs here yet.');
	const lines = rows.flatMap(({ ap, cfg }) => {
		const rep = cfg?.condition?.state;
		const radios = rep?.report?.radios || [];
		const apLink = link(`/aps/${encodeURIComponent(ap.id)}`, ap.name);
		if (!radios.length) return [h('tr', null, h('td', null, apLink), h('td', { colspan: 6, class: 'sub' }, cfg ? 'No report yet.' : 'You cannot see this AP.'))];
		return radios.map((r, i) => {
			const path = `radio.${r.band}.channel`;
			const set = cfg.location?.[path];
			return h('tr', null,
				h('td', null, i === 0 && apLink),
				h('td', null, bandName(r.band)),
				h('td', { class: 'mono' }, r.channel || 'starting'),
				h('td', { class: 'mono' }, r.width ? r.width + ' MHz' : '—'),
				h('td', null, String(r.clients ?? '—')),
				h('td', null, set ? value(path, set.value) : h('span', { class: 'sealed' }, 'not set; the AP keeps its own')),
				h('td', null, ago(rep.at)));
		});
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'Radios now', h('span', { class: 'note' }, 'as each AP last reported')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'Band', 'Channel', 'Width', 'Clients', 'Aeolus sets', 'Reported'].map((c) => h('th', null, c))),
			lines));
}

// systemSection shows the system settings that reach the node, and for
// someone who may change it, an editor for them (0052).
export async function systemSection(ctx, here, page, edit) {
	const sys = only(page.fields, (p) => p.startsWith('system.'));
	const panels = Object.keys(sys).length
		? fieldPanels(ctx, 'locations', here, sys, edit)
		: h('div', { class: 'banner info' }, 'No system settings here: each AP keeps its own.');
	if (!edit) return panels;
	const d = await schema();
	const box = h('div', { class: 'edit flush' });
	return [
		h('div', { class: 'below' },
			h('button', { type: 'button', class: 'button', onclick: () => systemEditor(ctx, d, here, page.node.name, page.fields, box) }, 'Edit system settings')),
		box,
		panels,
	];
}
