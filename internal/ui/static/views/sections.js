// The parts of a Locations folder or AP page besides its radios (0047):
// the channels its APs picked, its ports, the networks that reach it and its
// system settings.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { bandName, security, ago, value } from '../format.js';
import { tabBar, pick } from '../layout.js';
import { fieldPanels, editing } from './fields.js';
import { radiosSection } from './hardware.js';

const BANDS = ['2g', '5g', '6g'];
const HARDWARE = [['radios', 'Radios'], ['channels', 'Channels'], ['ports', 'Ports']];

// hardwareTab draws the Hardware tab of a Locations node whose page is at
// base. ap ({ap, cfg}) is the AP itself, on an AP's page.
export async function hardwareTab(ctx, base, id, page, sub, ap) {
	sub = pick(HARDWARE, sub);
	let body;
	if (sub === 'radios') body = radiosSection(ctx, id, page.node.name, page, ap?.cfg?.condition?.state);
	else if (sub === 'channels') body = channelsSection(ctx, ap ? [ap] : await configs(page.hardware?.aps || []));
	else body = portsSection(ctx, id, page, editing(ctx, 'locations', page));
	return [tabBar(`${base}/hardware`, HARDWARE, sub, true), body];
}

// networksTab draws the Networks tab: an AP's own networks, as its config
// resolves them, or a folder's, from the service folders that apply there.
export async function networksTab(ctx, id, page, ap) {
	const nets = ap
		? Object.entries(ap.cfg?.networks || {}).map(([nid, n]) => ({ id: nid, from: n.from, fields: n.fields }))
		: await folderNetworks(page);
	const bandsHere = new Set((page.hardware?.bands || []).map((b) => b.band));
	return networksSection(ctx, id, page, editing(ctx, 'locations', page), nets, bandsHere);
}

// only keeps the fields whose paths pass keep.
export function only(fields, keep) {
	return Object.fromEntries(Object.entries(fields || {}).filter(([p]) => keep(p)));
}

// configs reads each AP's config and condition, for the live views.
export async function configs(aps) {
	const all = await Promise.allSettled(aps.map((a) => get(`/v1/aps/${encodeURIComponent(a.id)}/config`)));
	return aps.map((a, i) => ({ ap: a, cfg: all[i].status === 'fulfilled' ? all[i].value : null }));
}

// channelsSection lists the channel each radio is on now, as its AP last
// reported, beside what Aeolus sets. With automatic channels each AP picks
// its own when its radio starts (0045).
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
		h('h2', null, 'Channels now', h('span', { class: 'note' }, 'as each AP last reported')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'Band', 'Channel', 'Width', 'Clients', 'Aeolus sets', 'Reported'].map((c) => h('th', null, c))),
			lines));
}

// portsSection shows the port settings that reach the node.
export function portsSection(ctx, here, page, edit) {
	const ports = only(page.fields, (p) => p.startsWith('ports.'));
	if (!Object.keys(ports).length) return h('div', { class: 'banner info' }, 'No port settings here: each AP keeps its own.');
	return fieldPanels(ctx, 'locations', here, ports, edit);
}

// systemSection shows the system settings that reach the node.
export function systemSection(ctx, here, page, edit) {
	const sys = only(page.fields, (p) => p.startsWith('system.'));
	if (!Object.keys(sys).length) return h('div', { class: 'banner info' }, 'No system settings here: each AP keeps its own.');
	return fieldPanels(ctx, 'locations', here, sys, edit);
}

// folderNetworks reads the networks a folder's service folders offer, as
// each service folder resolves them (0013): [{id, from, fields}], with
// fields keyed below the network (ssid, transport.primary.vlan, ...).
export async function folderNetworks(page) {
	const assigned = page.fields?.services?.value || [];
	const pages = await Promise.allSettled(assigned.map((sid) => get(`/v1/trees/services/nodes/${encodeURIComponent(sid)}`)));
	const out = new Map();
	pages.forEach((p, i) => {
		if (p.status !== 'fulfilled') return;
		for (const [path, r] of Object.entries(p.value.fields || {})) {
			const m = path.match(/^network\.([^.]+)\.(.+)$/);
			if (!m) continue;
			if (!out.has(m[1])) out.set(m[1], { id: m[1], from: assigned[i], fields: {} });
			const n = out.get(m[1]);
			if (n.from === assigned[i]) n.fields[m[2]] = r;
		}
	});
	return [...out.values()];
}

// networksSection shows the service folders that apply at the node, and
// each network they offer on the radios here: a network asks for bands,
// and the radios here carry them (0047). bandsHere are the bands some AP
// here has a radio for.
export function networksSection(ctx, here, page, edit, nets, bandsHere) {
	return [
		fieldPanels(ctx, 'locations', here, only(page.fields, (p) => p === 'services'), edit),
		nets.length === 0
			? h('div', { class: 'banner info' }, 'No network reaches here: no service folder that applies offers one.')
			: h('div', { class: 'bands' }, nets.map((n) => networkCard(ctx, n, bandsHere))),
	];
}

function networkCard(ctx, n, bandsHere) {
	const f = (k) => n.fields?.[k]?.value;
	const asks = f('bands') || BANDS;
	const row = (label, v) => v != null && v !== '' && h('div', { class: 'row' }, h('div', { class: 'label' }, label), h('div', { class: 'value' }, v));
	const transport = (slot) => {
		const type = f(`transport.${slot}.type`);
		if (!type) return null;
		return type === 'vxlan'
			? `VXLAN ${f(`transport.${slot}.concentrator`)} · VNI ${f(`transport.${slot}.vni`)}`
			: `VLAN ${f(`transport.${slot}.vlan`)}`;
	};
	const roaming = [f('roaming.ft') && '11r', f('roaming.rrm') && '11k', f('roaming.btm') && '11v'].filter(Boolean);
	const limits = [f('rate_limit.down_kbps') && `down ${f('rate_limit.down_kbps')} kbps`, f('rate_limit.up_kbps') && `up ${f('rate_limit.up_kbps')} kbps`].filter(Boolean);
	return h('section', { class: 'panel' },
		h('h2', null, f('ssid') || n.id, f('enabled') === false && h('span', { class: 'chip idle' }, 'off'),
			h('span', { class: 'note' }, 'from ', link(`/services/${encodeURIComponent(n.from)}`, ctx.name('services', n.from)))),
		h('div', { class: 'row' },
			h('div', { class: 'label' }, 'On radios'),
			h('div', { class: 'value' }, BANDS.filter((b) => asks.includes(b)).map((b) => bandsHere.has(b)
				? h('span', { class: 'chip band' }, bandName(b))
				: h('span', { class: 'chip band none', title: 'No radio here for this band' }, bandName(b) + ' (no radio here)')))),
		row('Security', security(f('security'))),
		row('Travels over', transport('primary') && [transport('primary'), transport('fallback') && `, then ${transport('fallback')}`]),
		row('Roaming', roaming.length ? roaming.join(', ') : 'off'),
		f('hidden') && row('Hidden', 'yes'),
		f('isolation') && row('Client isolation', 'on'),
		limits.length > 0 && row('Rate limit', limits.join(', ')));
}
