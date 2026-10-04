// The Networks tab of a Locations folder or AP (0047): the service folders
// that apply there and each network they offer, on the radios here. A
// network is edited where it is shown (0048), but it lives in its service
// folder (0013): an edit changes it there, for every location that uses that
// folder, and the preview names every AP it reaches. Band steering and
// multicast-to-unicast are switched right on each network, and what usteer
// does on each AP is shown below them (0051). A VXLAN transport picks one of
// the tunnels set where it is being edited (0055).

import { h, link } from '../dom.js';
import { get, schema } from '../api.js';
import { bandName, security, group, value, ago, probeOnly, PROBE_ONLY } from '../format.js';
import { fieldPanels, editing } from './fields.js';
import { only, configs } from './sections.js';
import { fieldsForm, changedValues } from './edit.js';
import { ask, confirm } from './confirm.js';

const BANDS = ['2g', '5g', '6g'];

// The order a network's fields are offered in. Any other field the schema
// has for a network follows under More settings, so a new one can be set
// before it gets a place here.
const MORE = 'More settings';
const SECTIONS = [
	[null, ['ssid', 'security', 'passphrase', 'bands', 'enabled', 'hidden', 'isolation', 'multicast_to_unicast']],
	['Roaming and steering', ['roaming.ft', 'roaming.rrm', 'roaming.btm', 'band_steering']],
	['Traffic', [
		'transport.primary.type', 'transport.primary.vlan', 'transport.primary.concentrator', 'transport.primary.vni', 'transport.primary.probe',
		'transport.fallback.type', 'transport.fallback.vlan', 'transport.fallback.concentrator', 'transport.fallback.vni', 'transport.fallback.probe',
		'transport.switching', 'transport.ha', 'transport.failback', 'transport.holddown',
	]],
];

export async function networksTab(ctx, id, page) {
	const [d, at, reports] = await Promise.all([schema(), networksAt(page), configs(page.hardware?.aps || [])]);
	const lib = tunnelsAt(page.fields);
	const bandsHere = new Set((page.hardware?.bands || []).map((b) => b.band));
	const writable = at.folders.filter((f) => f.canEdit);
	const addBox = h('div', { class: 'edit flush' });
	// The APs that say they have no usteer, which would refuse band steering.
	const noUsteer = new Set(reports.filter((r) => r.cfg?.condition?.state?.report?.steering?.installed === false).map((r) => r.ap.id));
	return [
		fieldPanels(ctx, 'locations', id, only(page.fields, (p) => p === 'services'), editing(ctx, 'locations', page)),
		at.nets.length === 0
			? h('div', { class: 'banner info' }, 'No network reaches here: no service folder that applies offers one.')
			: h('div', { class: 'bands' }, at.nets.map((n) => networkCard(ctx, d, n, bandsHere, noUsteer, lib))),
		writable.length > 0 && h('div', { class: 'below' },
			h('button', { type: 'button', class: 'button', onclick: () => addForm(ctx, d, writable, at.nets, addBox, lib) }, 'Add a network')),
		addBox,
		steeringStatus(ctx, reports),
		transportsStatus(reports),
	];
}

// tunnelsAt lists the tunnels set at a Locations node, from the values in
// force there (0055): [{id, address, port, mtu}] by name.
export function tunnelsAt(fields) {
	const out = new Map();
	for (const [path, r] of Object.entries(fields || {})) {
		const m = path.match(/^concentrators\.([^.]+)\.(address|port|mtu)$/);
		if (!m) continue;
		if (!out.has(m[1])) out.set(m[1], { id: m[1] });
		out.get(m[1])[m[2]] = r.value;
	}
	return [...out.values()].sort((a, b) => a.id.localeCompare(b.id));
}

// networksAt reads the networks offered by the service folders that apply at
// a node, as each folder resolves them, and whether the person may change
// each folder.
async function networksAt(page) {
	const assigned = page.fields?.services?.value || [];
	const pages = await Promise.allSettled(assigned.map((sid) => get(`/v1/trees/services/nodes/${encodeURIComponent(sid)}`)));
	const folders = [];
	const nets = new Map();
	pages.forEach((p, i) => {
		if (p.status !== 'fulfilled') return;
		const sid = assigned[i];
		const canEdit = p.value.role === 'operator' || p.value.role === 'admin';
		folders.push({ id: sid, canEdit });
		for (const [path, r] of Object.entries(p.value.fields || {})) {
			const m = path.match(/^network\.([^.]+)\.(.+)$/);
			if (!m) continue;
			if (!nets.has(m[1])) nets.set(m[1], { id: m[1], from: sid, canEdit, fields: {} });
			const n = nets.get(m[1]);
			if (n.from === sid) n.fields[m[2]] = r;
		}
	});
	return { folders, nets: [...nets.values()] };
}

function networkCard(ctx, d, n, bandsHere, noUsteer, lib) {
	const box = h('div', { class: 'edit' });
	const viewNow = () => view(ctx, n, bandsHere, box, noUsteer, lib);
	const body = h('div', null, viewNow());
	const close = () => body.replaceChildren(viewNow());
	return h('section', { class: 'panel' },
		h('h2', null, n.fields.ssid?.value || n.id, n.fields.enabled?.value === false && h('span', { class: 'chip idle' }, 'off'),
			h('span', { class: 'note' }, 'from ', link(`/services/${encodeURIComponent(n.from)}`, ctx.name('services', n.from))),
			n.canEdit && h('span', { class: 'controls' },
				h('button', { type: 'button', class: 'button small', onclick: () => { box.replaceChildren(); body.replaceChildren(editForm(ctx, d, n, close, lib)); } }, 'Edit'),
				n.fields.ssid?.origin === 'self' && h('button', { type: 'button', class: 'button small danger', onclick: () => { close(); deleteNetwork(ctx, n, box); } }, 'Delete'))),
		body,
		box);
}

// deleteNetwork deletes a network where it is defined: what its service
// folder sets for it is unset, in one change (0046, 0048). Every location
// that uses the folder stops offering it, and each AP removes the
// interfaces Aeolus made for it.
async function deleteNetwork(ctx, n, box) {
	const folderName = ctx.name('services', n.from);
	const own = Object.keys(n.fields).filter((k) => n.fields[k].origin === 'self').map((k) => `network.${n.id}.${k}`).sort();
	const op = { kind: 'unset', tree: 'services', node: n.from, paths: own };
	const p = await ask(box, op);
	if (!p) return;
	confirm(ctx, box, op, p, [
		h('div', null, h('strong', null, `Delete ${ssidOf(n)}`)),
		h('div', { class: 'sub' }, `It is removed from Services › ${folderName}, so every location that uses that folder stops offering it, and each AP listed removes its interfaces for it.`),
	], [
		h('div', { class: 'sub warn' }, 'Its clients on each AP listed are disconnected, and the other networks on those APs drop for a few seconds while the Wi-Fi restarts.'),
	]);
}

function view(ctx, n, bandsHere, box, noUsteer, lib) {
	const f = (k) => n.fields?.[k]?.value;
	const asks = f('bands') || BANDS;
	const row = (label, v) => v != null && v !== '' && h('div', { class: 'row' }, h('div', { class: 'label' }, label), h('div', { class: 'value' }, v));
	const transport = (slot) => {
		const type = f(`transport.${slot}.type`);
		if (!type) return null;
		return type === 'vxlan'
			? `VXLAN over ${tunnelName(lib, f(`transport.${slot}.concentrator`))} · VNI ${f(`transport.${slot}.vni`)}${f(`transport.${slot}.probe`) ? ` · asks ${f(`transport.${slot}.probe`)}` : ''}`
			: `VLAN ${f(`transport.${slot}.vlan`)}`;
	};
	const roaming = [f('roaming.ft') && '11r', f('roaming.rrm') && '11k', f('roaming.btm') && '11v'].filter(Boolean);
	const limits = [f('rate_limit.down_kbps') && `down ${f('rate_limit.down_kbps')} kbps`, f('rate_limit.up_kbps') && `up ${f('rate_limit.up_kbps')} kbps`].filter(Boolean);
	return [
		h('div', { class: 'row' },
			h('div', { class: 'label' }, 'On radios'),
			h('div', { class: 'value' }, BANDS.filter((b) => asks.includes(b)).map((b) => bandsHere.has(b)
				? h('span', { class: 'chip band' }, bandName(b))
				: h('span', { class: 'chip band none', title: 'No radio here for this band' }, bandName(b) + ' (no radio here)')))),
		row('Security', security(f('security'))),
		row('Travels over', transport('primary') && [transport('primary'), transport('fallback') && `, then ${transport('fallback')}`]),
		row('Roaming', roaming.length ? roaming.join(', ') : 'off'),
		steeringRow(ctx, n, box, noUsteer),
		multicastRow(ctx, n, box),
		f('hidden') && row('Hidden', 'yes'),
		f('isolation') && row('Client isolation', 'on'),
		limits.length > 0 && row('Rate limit', limits.join(', ')),
	];
}

const ssidOf = (n) => n.fields.ssid?.value || n.id;

// tunnelName names a tunnel with its far end, where it is set here.
export function tunnelName(lib, id) {
	const t = lib.find((c) => c.id === id);
	return t?.address ? `${id} (${t.address})` : id;
}

// lockedAbove says whether a network's field is locked above its folder,
// so it cannot be changed there.
const lockedAbove = (n, k) => n.fields[k]?.origin === 'locked' && n.fields[k].from !== n.from;

// whence says where a value in force comes from, when it is not the
// network's own folder.
function whence(ctx, n, r) {
	return r && r.from !== n.from ? h('span', { class: 'sub' }, `from ${ctx.name('services', r.from)}`) : null;
}

const RESTART = 'Applying restarts the Wi-Fi on each AP listed; its clients drop for a few seconds and reconnect.';

// steeringRow switches band steering for a network in its service folder
// (0050, 0051). The switch shows what is in force; a change is previewed
// first, and only then recorded.
function steeringRow(ctx, n, box, noUsteer) {
	const k = 'band_steering';
	const path = `network.${n.id}.${k}`;
	const field = n.fields[k];
	const on = field?.value === true;
	// Off: unset what this folder set, so it follows what is above; or, when
	// the value comes from above, set it off here.
	const op = on
		? (field.origin === 'self' ? { kind: 'unset', tree: 'services', node: n.from, path } : { kind: 'set', tree: 'services', node: n.from, path, value: false })
		: { kind: 'set', tree: 'services', node: n.from, path, value: true };
	const flip = async () => {
		const p = await ask(box, op);
		if (!p) return;
		const after = p.resolved?.[path];
		const missing = (p.reversioned || []).filter((ap) => noUsteer.has(ap)).map((ap) => ctx.name('locations', ap));
		confirm(ctx, box, op, p, [
			h('div', null, h('strong', null, `Band steering for ${ssidOf(n)}: `), on ? 'on' : 'off', ' → ', after?.value === true ? 'on' : 'off',
				after && after.from !== n.from ? ` (from ${ctx.name('services', after.from)})` : ''),
			h('div', { class: 'sub' }, `This changes the network in Services › ${ctx.name('services', n.from)}, for every location that uses it.`,
				!on ? ' Steering also turns on 802.11k and 802.11v for it.' : ''),
			missing.length > 0 && h('div', { class: 'banner problems' },
				`${missing.join(', ')} ${missing.length === 1 ? 'has' : 'have'} no usteer, so ${missing.length === 1 ? 'it' : 'they'} would refuse this until it is installed (apk add usteer).`),
		], [h('div', { class: 'sub warn' }, RESTART)]);
	};
	return h('div', { class: 'row' },
		h('div', { class: 'label' }, 'Band steering'),
		h('div', { class: 'value' },
			h('button', {
				type: 'button', class: 'switch' + (on ? ' on' : ''), role: 'switch', 'aria-checked': String(on),
				title: on ? 'Turn band steering off' : 'Turn band steering on', disabled: !n.canEdit || lockedAbove(n, k), onclick: flip,
			}, h('span', { class: 'knob' }), on ? 'On' : 'Off'),
			whence(ctx, n, field)));
}

const MULTICAST = [
	['default', 'OpenWrt default', 'Only the groups clients joined (IGMP/MLD snooping)'],
	['all', 'All', 'All multicast (ARP, IPv4, IPv6), to each client as unicast'],
	['none', 'None', 'No multicast is sent as unicast'],
];

// multicastRow picks multicast-to-unicast for a network in its service
// folder (0049, 0051): OpenWrt's default (unset), all, or none.
function multicastRow(ctx, n, box) {
	const k = 'multicast_to_unicast';
	const path = `network.${n.id}.${k}`;
	const field = n.fields[k];
	const now = field?.value === true ? 'all' : field?.value === false ? 'none' : 'default';
	const about = (key) => MULTICAST.find(([x]) => x === key);
	const choose = async (want) => {
		const op = want === 'default'
			? { kind: 'unset', tree: 'services', node: n.from, path }
			: { kind: 'set', tree: 'services', node: n.from, path, value: want === 'all' };
		const p = await ask(box, op);
		if (!p) return;
		confirm(ctx, box, op, p, [
			h('div', null, h('strong', null, `Multicast to unicast for ${ssidOf(n)}: `), about(now)[1], ' → ', about(want)[1]),
			h('div', { class: 'sub' }, about(want)[2] + '.'),
			h('div', { class: 'sub' }, `This changes the network in Services › ${ctx.name('services', n.from)}, for every location that uses it.`),
		], [h('div', { class: 'sub warn' }, RESTART)]);
	};
	const fixed = !n.canEdit || lockedAbove(n, k);
	return h('div', { class: 'row' },
		h('div', { class: 'label' }, 'Multicast to unicast'),
		h('div', { class: 'value' },
			h('span', { class: 'segmented', role: 'group', 'aria-label': 'Multicast to unicast' }, MULTICAST.map(([key, text, help]) => h('button', {
				type: 'button', class: key === now ? 'on' : null, 'aria-pressed': String(key === now), title: help,
				// The default is unset here; a value set above cannot be unset here.
				disabled: key === now || fixed || (key === 'default' && field?.origin !== 'self'),
				onclick: () => choose(key),
			}, text))),
			whence(ctx, n, field)));
}

// steeringStatus shows what usteer is doing on each AP here, as each last
// reported (0051): whether it runs, which SSIDs it steers, and for those,
// the clients on each band and the clients it moved.
function steeringStatus(ctx, reports) {
	if (!reports.length) return null;
	const rows = reports.map(({ ap, cfg }) => {
		const st = cfg?.condition?.state;
		const g = st?.report?.steering;
		let usteer;
		if (!st) usteer = 'no report yet';
		else if (!g) usteer = 'its agent does not say';
		else if (!g.installed) usteer = 'not installed';
		else if (!g.running) usteer = 'installed, not running';
		else usteer = 'running';
		const steered = g?.running && g.interval > 0 ? g.ssids || [] : [];
		const bands = steered.map((ssid) => h('div', null, h('strong', null, ssid + ': '),
			(g.bss || []).filter((b) => b.ssid === ssid).map((b) =>
				`${bandName(b.band)} ${b.clients} client${b.clients === 1 ? '' : 's'}` +
				(b.steered_away ? `, ${b.steered_away} moved off` : '') + (b.steered_in ? `, ${b.steered_in} moved on` : '')).join(' · ')));
		return h('tr', null,
			h('td', null, link(`/aps/${encodeURIComponent(ap.id)}`, ap.name)),
			h('td', null, h('span', { class: 'chip ' + (usteer === 'running' ? 'ok' : g && !g.installed ? 'warn' : 'idle') }, usteer)),
			h('td', null, !g?.running ? '—' : g.interval > 0 ? `on for ${steered.join(', ')}` : 'off'),
			h('td', null, bands.length ? bands : '—'),
			h('td', null, st ? ago(st.at) : '—'));
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'Band steering on each AP', h('span', { class: 'note' }, 'as each last reported; moves are counted since usteer started')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'usteer', 'Steering', 'Steered networks now', 'Reported'].map((c) => h('th', null, c))),
			rows));
}

// How each verdict of a network's transport is shown (0059, 0061).
const VERDICT = {
	up: ['ok', 'answering'], down: ['bad', 'down'], unverified: ['warn', 'unverified'],
	unknown: ['idle', 'starting'], off: ['idle', 'standing by'],
};

// transportsStatus shows, for each network with a fallback on each AP here,
// which transport carries it and how each is doing, by the AP's prober
// (0061): a VLAN transport probed on the uplink, a VXLAN one on its tunnel.
// Under it, the last switch, or why a switch that is due cannot be made.
function transportsStatus(reports) {
	const rows = [];
	for (const { ap, cfg } of reports) {
		const st = cfg?.condition?.state;
		const report = st?.report;
		const vlans = new Map((report?.vlan_probes || []).map((v) => [v.vlan, v.probe]));
		const tunnels = new Map((report?.vxlan?.tunnels || []).map((t) => [t.vni, t.probe]));
		for (const [id, s] of Object.entries(report?.transports || {}).sort()) {
			const n = cfg.document?.network?.[id];
			const how = (slot) => {
				const t = n?.transport?.[slot];
				const [cls, word] = VERDICT[s[slot]] || ['idle', s[slot] || 'not reported'];
				const p = t?.type === 'vlan' ? vlans.get(t.vlan) : t?.type === 'vxlan' ? tunnels.get(t.vni) : null;
				return [
					h('span', { class: 'chip ' + cls }, `${!t ? slot : t.type === 'vlan' ? 'VLAN ' + t.vlan : 'VNI ' + t.vni}: ${word}`),
					p?.lease && h('div', { class: 'sub' }, `leased ${p.lease.address}${p.lease.server ? ' from ' + p.lease.server : ''}`),
				];
			};
			rows.push(h('tr', null,
				h('td', null, rows.length === 0 || rows[rows.length - 1].dataset.ap !== ap.id ? link(`/aps/${encodeURIComponent(ap.id)}`, ap.name) : null),
				h('td', null, n?.ssid || id),
				h('td', null,
					s.active === 'none' ? h('span', { class: 'chip bad' }, 'nothing') : s.active === 'fallback' ? h('span', { class: 'chip warn' }, 'fallback') : s.active,
					s.cannot_switch ? h('div', { class: 'sub' }, `cannot switch: ${s.cannot_switch}`)
						: s.last_switch && h('div', { class: 'sub' },
							`to ${s.last_switch.to} ${ago(new Date(new Date(st.at).getTime() - s.last_switch.ago * 1000))}: ${s.last_switch.why}`)),
				h('td', null, how('primary')),
				h('td', null, how('fallback')),
				h('td', null, ago(st.at))));
			rows[rows.length - 1].dataset.ap = ap.id;
		}
	}
	if (!rows.length) return null;
	return h('section', { class: 'panel' },
		h('h2', null, 'Transports on each AP', h('span', { class: 'note' }, 'networks with a fallback, as each AP last reported')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'Network', 'Carried by', 'Primary', 'Fallback', 'Reported'].map((c) => h('th', null, c))),
			rows));
}

// The fields a transport's type shows: a VLAN's ID, or a VXLAN's
// concentrator, VNI and probe address (0059). The others stay hidden, and
// are not sent.
const FOR_TYPE = { vlan: ['vlan'], vxlan: ['concentrator', 'vni', 'probe'] };

// What only means something once there is a fallback transport, and with
// automatic switching (0022, 0061).
const WITH_FALLBACK = ['transport.switching'];
const WITH_AUTOMATIC = ['transport.ha', 'transport.failback', 'transport.holddown'];

// pickers swaps a transport's tunnel field for a list of the tunnels set
// here (0055). A tunnel the list lacks, set where it was, stays on offer,
// marked.
function pickers(prefix, rows, inputs, lib) {
	for (const slot of ['primary', 'fallback']) {
		const path = `${prefix}transport.${slot}.concentrator`;
		const row = rows.get(path);
		if (!row) continue;
		const cur = row.it.read();
		const sel = h('select', null,
			h('option', { value: '' }, lib.length ? '—' : 'no tunnel is set here yet (Interfaces › Tunnels)'),
			lib.map((t) => h('option', { value: t.id, selected: t.id === cur }, tunnelName(lib, t.id))),
			cur && !lib.some((t) => t.id === cur) && h('option', { value: cur, selected: true }, `${cur} (not set here)`));
		const read = () => sel.value || undefined;
		const initial = JSON.stringify(read());
		const it = { el: sel, read, changed: () => JSON.stringify(read()) !== initial };
		sel.disabled = row.it.el.disabled;
		row.it.el.replaceWith(sel);
		row.it = it;
		if (inputs.has(path)) inputs.set(path, it);
	}
}

// form lays out an input for every network field the schema has, with the
// values now in fields ({field: {value, from, origin}}). Fields locked
// above folder cannot be changed there. A transport shows only the fields
// its type uses; sync shows the right ones after a type is set, and the
// switching settings only with a fallback. A VXLAN transport's tunnel is
// picked from lib, the tunnels set here.
function form(d, net, fields, folder, lib) {
	const prefix = `network.${net}.`;
	const keys = Object.keys(d.fields).filter((k) => k.startsWith('network.*.')).map((k) => k.slice('network.*.'.length));
	const placed = new Set(SECTIONS.flatMap(([, ks]) => ks));
	const sections = [...SECTIONS, [MORE, keys.filter((k) => !placed.has(k)).sort()]]
		.map(([title, ks]) => [title, ks.map((k) => prefix + k)]);
	const byPath = Object.fromEntries(Object.entries(fields).map(([k, r]) => [prefix + k, r]));
	const { body, inputs, rows } = fieldsForm(d, sections, byPath, folder, MORE);
	pickers(prefix, rows, inputs, lib);
	const typeOf = (slot) => rows.get(`${prefix}transport.${slot}.type`)?.it.el.value;
	const show = (k, on) => {
		const r = rows.get(prefix + k)?.row;
		if (r) r.hidden = !on;
	};
	const sync = () => {
		for (const slot of ['primary', 'fallback'])
			for (const [t, ks] of Object.entries(FOR_TYPE))
				for (const k of ks) show(`transport.${slot}.${k}`, typeOf(slot) === t);
		const fallback = Boolean(typeOf('fallback'));
		for (const k of WITH_FALLBACK) show(k, fallback);
		const automatic = rows.get(`${prefix}transport.switching`)?.it.el.value === 'automatic';
		for (const k of WITH_AUTOMATIC) show(k, fallback && automatic);
	};
	for (const k of ['transport.primary.type', 'transport.fallback.type', 'transport.switching'])
		rows.get(prefix + k)?.it.el.addEventListener('change', sync);
	sync();
	return { body, inputs, rows, sync };
}

function setOp(folder, values) {
	const paths = Object.keys(values);
	return paths.length === 1
		? { kind: 'set', tree: 'services', node: folder, path: paths[0], value: values[paths[0]] }
		: { kind: 'set', tree: 'services', node: folder, values };
}

// shown writes a value for the preview; a secret is never shown.
function shown(path, v) {
	return path.endsWith('.passphrase') ? 'a new passphrase' : value(path, v);
}

function editForm(ctx, d, n, close, lib) {
	const folderName = ctx.name('services', n.from);
	const { body, inputs, rows } = form(d, n.id, n.fields, n.from, lib);
	const box = h('div', { class: 'edit flush' });
	const msg = h('div', { class: 'error' });
	const review = async () => {
		msg.replaceChildren();
		let values;
		try {
			values = changedValues(inputs, rows);
		} catch (e) {
			msg.replaceChildren(e.message);
			return;
		}
		if (!Object.keys(values).length) {
			msg.replaceChildren('Nothing has changed.');
			return;
		}
		const op = setOp(n.from, values);
		const p = await ask(box, op);
		if (!p) return;
		confirm(ctx, box, op, p, [
			h('div', null, h('strong', null, `${n.fields.ssid?.value || n.id} in Services › ${folderName}`)),
			h('ul', { class: 'becomes' }, Object.entries(values).map(([path, v]) => h('li', null,
				`${group(path).label}: `, n.fields[path.split('.').slice(2).join('.')] && !path.endsWith('.passphrase')
					? [value(path, n.fields[path.split('.').slice(2).join('.')].value), ' → '] : '', shown(path, v)))),
			h('div', { class: 'sub' }, `This changes the network in Services › ${folderName}, for every location that uses it.`),
		], [
			h('div', { class: 'sub warn' }, probeOnly(Object.keys(values)) ? PROBE_ONLY : 'Applying restarts the Wi-Fi on each AP listed; its clients drop for a few seconds and reconnect.'),
		]);
	};
	return h('div', { class: 'fieldform', 'data-editing': true },
		body,
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review changes'),
			h('button', { type: 'button', class: 'button', onclick: close }, 'Cancel')),
		box);
}

// addForm offers a new network in one of the service folders that apply
// here and that the person may change. Its name in the schema comes from
// its SSID.
function addForm(ctx, d, folders, nets, box, lib) {
	const pickFolder = h('select', null, folders.map((f) => h('option', { value: f.id }, `Services › ${ctx.name('services', f.id)}`)));
	const { body, inputs, rows, sync } = form(d, 'new', {}, null, lib);
	// A new network travels over a VLAN unless the person picks otherwise.
	const primary = inputs.get('network.new.transport.primary.type');
	if (primary) {
		primary.el.value = 'vlan';
		sync();
	}
	const msg = h('div', { class: 'error' });
	const out = h('div', { class: 'edit flush' });
	const review = async () => {
		msg.replaceChildren();
		let values;
		try {
			values = changedValues(inputs, rows);
		} catch (e) {
			msg.replaceChildren(e.message);
			return;
		}
		const ssid = values['network.new.ssid'];
		if (!ssid) {
			msg.replaceChildren('Give it an SSID.');
			return;
		}
		const id = name(ssid, d.names.network, new Set(nets.map((n) => n.id)));
		const renamed = Object.fromEntries(Object.entries(values).map(([p, v]) => [p.replace(/^network\.new\./, `network.${id}.`), v]));
		const op = setOp(pickFolder.value, renamed);
		const p = await ask(out, op);
		if (!p) return;
		const folderName = ctx.name('services', pickFolder.value);
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, `Add ${ssid} to Services › ${folderName}`)),
			h('ul', { class: 'becomes' }, Object.entries(renamed).map(([path, v]) => h('li', null, `${group(path).label}: `, shown(path, v)))),
			h('div', { class: 'sub' }, `Every location that uses Services › ${folderName} offers it. It is saved as network "${id}".`),
		], [
			h('div', { class: 'sub warn' }, 'Applying restarts the Wi-Fi on each AP listed; its clients drop for a few seconds and reconnect.'),
		]);
	};
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, 'Add a network'),
		h('div', { class: 'fieldform' },
			folders.length > 1
				? h('label', { class: 'field' }, h('span', { class: 'label' }, 'In'), pickFolder)
				: h('div', { class: 'sub' }, `In Services › ${ctx.name('services', folders[0].id)}, for every location that uses it.`),
			body,
			msg,
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review'),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel')),
			out)));
}

// The names a network cannot have: Aeolus keeps them for its own sections on
// an AP (0054).
const KEPT = /^(vlan[0-9]+$|port-)/;

// name makes a network's name from its SSID, as the schema's pattern allows
// and unlike any taken. It starts with a letter.
function name(ssid, pattern, taken) {
	let base = ssid.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 28) || 'network';
	if (!/^[a-z]/.test(base) || KEPT.test(base)) base = 'n' + base;
	const ok = new RegExp(pattern);
	let id = base;
	for (let i = 2; taken.has(id) || !ok.test(id) || KEPT.test(id); i++) id = `${base}-${i}`;
	return id;
}
