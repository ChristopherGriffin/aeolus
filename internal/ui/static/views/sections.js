// The parts of a Locations folder or AP page besides its networks and
// interfaces (0047, 0053, 0072, 0075): its APs' radios as each last
// reported, on a folder's APs tab, and its system settings.

import { h, link } from '../dom.js';
import { get, schema } from '../api.js';
import { bandName, ago, value } from '../format.js';
import { fieldPanels } from './fields.js';
import { systemEditor } from './system.js';
import { ask, confirm, cancelButton } from './confirm.js';

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
// renamed, its hostname with it (0076).
export function channelsSection(ctx, rows, status, canEdit) {
	if (!rows.length) return h('div', { class: 'banner info' }, 'No APs here yet.');
	const box = h('div', { class: 'edit' });
	const lines = rows.flatMap(({ ap, cfg }) => {
		const rep = cfg?.condition?.state;
		const radios = rep?.report?.radios || [];
		const apLink = [link(`/aps/${encodeURIComponent(ap.id)}`, ap.name),
			canEdit && h('button', { type: 'button', class: 'button small rename', title: 'Rename this AP: its hostname changes with it',
				onclick: () => renameForm(ctx, ap, box) }, 'Rename…')];
		const st = status?.get(ap.id);
		const state = st && h('span', { class: 'chip ' + st.chip, title: st.detail }, st.label);
		if (!radios.length) return [h('tr', null, h('td', null, apLink), h('td', null, state), h('td', { colspan: 6, class: 'sub' }, cfg ? 'No report yet.' : 'You cannot see this AP.'))];
		return radios.map((r, i) => {
			const path = `radio.${r.band}.channel`;
			const set = cfg.location?.[path];
			return h('tr', null,
				h('td', null, i === 0 && apLink),
				h('td', null, i === 0 && state),
				h('td', null, bandName(r.band)),
				h('td', { class: 'mono' }, r.channel || 'starting'),
				h('td', { class: 'mono' }, r.width ? r.width + ' MHz' : '—'),
				h('td', null, String(r.clients ?? '—')),
				h('td', null, set ? value(path, set.value) : h('span', { class: 'sealed' }, 'not set; the AP keeps its own')),
				h('td', null, ago(rep.at)));
		});
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'APs', h('span', { class: 'note' }, 'radios as each last reported')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'State', 'Band', 'Channel', 'Width', 'Clients', 'Aeolus sets', 'Reported'].map((c) => h('th', null, c))),
			lines),
		box);
}

// HOSTNAME is what an AP's name must be, as its hostname (0076).
const HOSTNAME = '[A-Za-z0-9]([A-Za-z0-9\\-]{0,61}[A-Za-z0-9])?';

// renameForm opens, in box, a new name for an AP, previewed and logged as
// any change is. The AP's hostname changes with it, at once.
function renameForm(ctx, ap, box) {
	const input = h('input', { type: 'text', value: ap.name, maxlength: 63, pattern: HOSTNAME, spellcheck: 'false', 'aria-label': `New name for ${ap.name}` });
	const out = h('div');
	const msg = h('div', { class: 'error' });
	const review = async () => {
		msg.replaceChildren();
		const name = input.value.trim();
		if (!name || !input.checkValidity()) {
			msg.replaceChildren('An AP\'s name is its hostname: letters, digits and hyphens, at most 63, starting and ending with a letter or digit.');
			return;
		}
		if (name === ap.name) {
			msg.replaceChildren('That is its name already.');
			return;
		}
		const op = { kind: 'rename', tree: 'locations', node: ap.id, name };
		const p = await ask(out, op);
		if (!p) return;
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, 'Rename: '), ap.name, ' → ', name),
		], [
			h('div', { class: 'sub' }, 'The AP\'s hostname changes with it, as it applies the config. Nothing restarts.'),
		]);
	};
	input.addEventListener('keydown', (e) => { if (e.key === 'Enter') review(); });
	box.replaceChildren(h('div', { class: 'fieldform', 'data-editing': true },
		h('label', { class: 'field' }, h('span', { class: 'label' }, `New name for ${ap.name}`), input),
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review'),
			cancelButton(box)),
		out));
	input.focus();
	input.select();
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
