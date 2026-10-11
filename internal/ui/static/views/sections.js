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

// channelsSection lists each AP below a folder on its APs tab (0075): its
// state, such as In sync (status, by AP ID, from the fleet), and its radios
// as it last reported them: the channel each is on, its width and clients,
// beside what Aeolus sets. With automatic channels each AP picks its own
// (0045), within the band's channel set. With canEdit, each AP can be
// renamed, its hostname with it, and moved to another folder (0076).
// agentCell is an AP's agent (0079): the release it runs, and whether that
// is the one it should run: current, updating, behind, rolled back or
// failed, or held where its folder pins a release the manager no longer
// keeps.
function agentCell(a) {
	if (!a) return '—';
	const { runs, wants } = a;
	const short = (v) => (v || '').replace(/-[0-9a-f]{7,}$/, '') || 'unknown';
	const u = runs?.update;
	const chip = (cls, label, title) => h('span', { class: 'chip ' + cls, title: title || null }, label);
	let state;
	if (a.missing_pin) state = chip('warn', 'pin missing', `Its folder pins ${a.missing_pin}, which the manager no longer keeps, so it stays as it is.`);
	else if (u?.state === 'updating') state = chip('warn', 'updating', `Trying ${short(u.version)}`);
	else if (runs?.hash && runs.hash === wants?.hash) state = chip('ok', 'current');
	else if (u && u.hash === wants?.hash && u.state === 'rolled-back') state = chip('warn', 'rolled back', u.why);
	else if (u && u.hash === wants?.hash && u.state === 'failed') state = chip('warn', 'failed', u.why);
	else state = chip('warn', 'behind', wants && `It should run ${short(wants.version)}`);
	return [h('span', { class: 'mono' }, short(runs?.version)), ' ', state];
}

export function channelsSection(ctx, rows, status, canEdit) {
	if (!rows.length) return h('div', { class: 'banner info' }, 'No APs here yet.');
	const box = h('div', { class: 'edit' });
	const lines = rows.flatMap(({ ap, cfg }) => {
		const rep = cfg?.condition?.state;
		const radios = rep?.report?.radios || [];
		const apLink = [link(`/aps/${encodeURIComponent(ap.id)}`, ap.name),
			canEdit && renameButton(ctx, 'locations', { id: ap.id, name: ap.name, kind: 'ap' }, box),
			canEdit && moveButton(ctx, 'locations', { id: ap.id, name: ap.name, kind: 'ap' }, box)];
		const st = status?.get(ap.id);
		const state = st && h('span', { class: 'chip ' + st.chip, title: st.detail }, st.label);
		const agent = cfg && agentCell(cfg.agent);
		// How it is doing, by its latest report (0119); its Health tab has the rest.
		const brief = healthBrief(rep?.report?.health);
		const health = brief && h('a', { class: 'chip ' + brief.grade, href: `#/aps/${encodeURIComponent(ap.id)}/health`, title: 'By its latest report. Open its health and history' }, brief.text);
		if (!radios.length) return [h('tr', null, h('td', null, apLink), h('td', null, state), h('td', null, agent), h('td', { class: 'nowrap' }, health), h('td', { colspan: 6, class: 'sub' }, cfg ? 'No report yet.' : 'You cannot see this AP.'))];
		return radios.map((r, i) => {
			const path = `radio.${r.band}.channel`;
			const set = cfg.location?.[path];
			return h('tr', null,
				h('td', null, i === 0 && apLink),
				h('td', null, i === 0 && state),
				h('td', null, i === 0 && agent),
				h('td', { class: 'nowrap' }, i === 0 && health),
				h('td', null, bandName(r.band)),
				h('td', { class: 'mono' }, r.channel || 'starting'),
				h('td', { class: 'mono' }, r.width ? r.width + ' MHz' : '—'),
				h('td', null, String(r.clients ?? '—')),
				h('td', null, set ? value(path, set.value) : h('span', { class: 'sealed' }, 'not set')),
				h('td', null, ago(rep.at)));
		});
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'APs', h('span', { class: 'note' }, 'radios as each last reported')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'State', 'Agent', 'Health', 'Band', 'Channel', 'Width', 'Clients', 'Aeolus sets', 'Reported'].map((c) => h('th', null, c))),
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
