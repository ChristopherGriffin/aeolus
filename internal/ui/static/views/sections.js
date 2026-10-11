// The parts of a Locations folder or AP page besides its networks and
// interfaces (0047, 0053, 0072, 0075): its APs' radios as each last
// reported, on a folder's APs tab, and its system settings.

import { h, link } from '../dom.js';
import { get, schema } from '../api.js';
import { bandName, ago, value } from '../format.js';
import { fieldPanels } from './fields.js';
import { systemEditor } from './system.js';
import { renameButton } from './rename.js';
import { moveButton } from './move.js';
import { healthBrief } from './health.js';

// only keeps the fields whose paths pass keep.
export function only(fields, keep) {
	return Object.fromEntries(Object.entries(fields || {}).filter(([p]) => keep(p)));
}

// configs reads each AP's config and condition, for the live views.
export async function configs(aps) {
	const all = await Promise.allSettled(aps.map((a) => get(`/v1/aps/${encodeURIComponent(a.id)}/config`)));
	return aps.map((a, i) => ({ ap: a, cfg: all[i].status === 'fulfilled' ? all[i].value : null }));
}

// channelsSection lists each AP below a folder on its APs tab (0075), a
// line each: its state, such as In sync (status, by AP ID, from the fleet),
// how it is doing, its radios as it last reported them, and its clients.
// The rest is a step away: a radio says its clients, and what Aeolus sets
// for it, to whoever points at it; the AP's own page has everything. With
// automatic channels each AP picks its own (0045), within the band's
// channel set. With canEdit, each AP can be renamed, its hostname with it,
// and moved to another folder (0076).
//
// agentChip is an AP's agent (0079) where it is not the release it should
// run: updating, behind, rolled back or failed, or held where its folder
// pins a release the manager no longer keeps. Nothing where it is current:
// that is as it should be, and its page says which.
function agentChip(a) {
	if (!a) return null;
	const { runs, wants } = a;
	const short = (v) => (v || '').replace(/-[0-9a-f]{7,}$/, '') || 'unknown';
	const u = runs?.update;
	const chip = (label, why) => h('span', { class: 'chip warn', title: [`It runs agent ${short(runs?.version)}.`, why].filter(Boolean).join(' ') }, `agent ${label}`);
	if (a.missing_pin) return chip('pin missing', `Its folder pins ${a.missing_pin}, which the manager no longer keeps, so it stays as it is.`);
	if (u?.state === 'updating') return chip('updating', `Trying ${short(u.version)}.`);
	if (runs?.hash && runs.hash === wants?.hash) return null;
	if (u && u.hash === wants?.hash && u.state === 'rolled-back') return chip('rolled back', u.why);
	if (u && u.hash === wants?.hash && u.state === 'failed') return chip('failed', u.why);
	return chip('behind', wants && `It should run ${short(wants.version)}.`);
}

export function channelsSection(ctx, rows, status, canEdit) {
	if (!rows.length) return h('div', { class: 'banner info' }, 'No APs here yet.');
	const box = h('div', { class: 'edit' });
	const lines = rows.map(({ ap, cfg }) => {
		const rep = cfg?.condition?.state;
		const radios = rep?.report?.radios || [];
		const st = status?.get(ap.id);
		const state = st && h('span', { class: 'chip ' + st.chip, title: st.detail }, st.label);
		const agent = cfg && agentChip(cfg.agent);
		// How it is doing, by its latest report (0119); its Health tab has the rest.
		const brief = healthBrief(rep?.report?.health);
		const health = brief && h('a', { class: 'chip ' + brief.grade, href: `#/aps/${encodeURIComponent(ap.id)}/health`, title: 'By its latest report. Open its health and history' }, brief.text);
		// A radio: its band, with the channel it is on and its width.
		const radio = (r) => {
			const path = `radio.${r.band}.channel`;
			const set = cfg.location?.[path];
			const n = r.clients ?? 0;
			return h('span', { class: 'chip idle radio',
				title: `${bandName(r.band)}: ${r.channel ? `channel ${r.channel}` : 'starting'}${r.width ? `, ${r.width} MHz` : ''}, ${n} client${n === 1 ? '' : 's'}. Aeolus sets its channel: ${set ? value(path, set.value) : 'not set'}.` },
			bandName(r.band), h('span', { class: 'mono' }, r.channel ? `${r.channel}${r.width ? `/${r.width}` : ''}` : 'starting'));
		};
		return h('tr', null,
			h('td', null, link(`/aps/${encodeURIComponent(ap.id)}`, ap.name)),
			h('td', null, state, agent && ' ', agent),
			h('td', { class: 'nowrap' }, health || h('span', { class: 'sub' }, '—')),
			h('td', null, radios.length ? h('span', { class: 'radiochips' }, radios.map(radio)) : h('span', { class: 'sub' }, cfg ? 'No report yet' : 'You cannot see this AP')),
			h('td', null, radios.length ? String(radios.reduce((n, r) => n + (r.clients || 0), 0)) : '—'),
			h('td', { class: 'sub nowrap' }, rep?.at ? ago(rep.at) : '—'),
			canEdit && h('td', { class: 'rowacts' },
				renameButton(ctx, 'locations', { id: ap.id, name: ap.name, kind: 'ap' }, box),
				moveButton(ctx, 'locations', { id: ap.id, name: ap.name, kind: 'ap' }, box)));
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'APs', h('span', { class: 'note' }, 'as each last reported')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'State', 'Health', 'Radios', 'Clients', 'Reported', canEdit && ''].filter((c) => c !== false).map((c) => h('th', null, c))),
			lines),
		box);
}

// systemSection shows the system settings that reach the node, and for
// someone who may change it, an editor for them (0052).
export async function systemSection(ctx, here, page, edit) {
	const sys = only(page.fields, (p) => p.startsWith('system.') || p.startsWith('notify.'));
	const panels = Object.keys(sys).length
		? fieldPanels(ctx, 'locations', here, sys, edit)
		: h('div', { class: 'banner info' }, 'No system settings here.');
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
