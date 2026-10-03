// The Networks tab of a Locations folder or AP (0047): the service folders
// that apply there and each network they offer, on the radios here. A
// network is edited where it is shown (0048), but it lives in its service
// folder (0013): an edit changes it there, for every location that uses that
// folder, and the preview names every AP it reaches.

import { h, link } from '../dom.js';
import { get, schema } from '../api.js';
import { bandName, security, group, value } from '../format.js';
import { fieldPanels, editing } from './fields.js';
import { only } from './sections.js';
import { describe, input } from './edit.js';
import { ask, confirm } from './confirm.js';

const BANDS = ['2g', '5g', '6g'];

// The order a network's fields are offered in. Any other field the schema
// has for a network follows under More settings, so a new one can be set
// before it gets a place here.
const SECTIONS = [
	[null, ['ssid', 'security', 'passphrase', 'bands', 'enabled', 'hidden', 'isolation', 'multicast_to_unicast']],
	['Roaming', ['roaming.ft', 'roaming.rrm', 'roaming.btm']],
	['Traffic', ['transport.primary.type', 'transport.primary.vlan', 'transport.primary.concentrator', 'transport.primary.vni']],
];

export async function networksTab(ctx, id, page) {
	const [d, at] = await Promise.all([schema(), networksAt(page)]);
	const bandsHere = new Set((page.hardware?.bands || []).map((b) => b.band));
	const writable = at.folders.filter((f) => f.canEdit);
	const addBox = h('div', { class: 'edit flush' });
	return [
		fieldPanels(ctx, 'locations', id, only(page.fields, (p) => p === 'services'), editing(ctx, 'locations', page)),
		at.nets.length === 0
			? h('div', { class: 'banner info' }, 'No network reaches here: no service folder that applies offers one.')
			: h('div', { class: 'bands' }, at.nets.map((n) => networkCard(ctx, d, n, bandsHere))),
		writable.length > 0 && h('div', { class: 'below' },
			h('button', { type: 'button', class: 'button', onclick: () => addForm(ctx, d, writable, at.nets, addBox) }, 'Add a network')),
		addBox,
	];
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

function networkCard(ctx, d, n, bandsHere) {
	const body = h('div', null, view(n, bandsHere));
	const close = () => body.replaceChildren(view(n, bandsHere));
	return h('section', { class: 'panel' },
		h('h2', null, n.fields.ssid?.value || n.id, n.fields.enabled?.value === false && h('span', { class: 'chip idle' }, 'off'),
			h('span', { class: 'note' }, 'from ', link(`/services/${encodeURIComponent(n.from)}`, ctx.name('services', n.from))),
			n.canEdit && h('button', { type: 'button', class: 'button small', onclick: () => body.replaceChildren(editForm(ctx, d, n, close)) }, 'Edit')),
		body);
}

function view(n, bandsHere) {
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
	return [
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
		f('multicast_to_unicast') != null && row('Multicast to unicast', f('multicast_to_unicast') ? 'all multicast' : 'off'),
		limits.length > 0 && row('Rate limit', limits.join(', ')),
	];
}

// form lays out an input for every network field the schema has, with the
// values now in fields ({field: {value, from, origin}}). Fields locked
// above folder cannot be changed there.
function form(d, net, fields, folder) {
	const keys = Object.keys(d.fields).filter((k) => k.startsWith('network.*.')).map((k) => k.slice('network.*.'.length));
	const placed = new Set(SECTIONS.flatMap(([, ks]) => ks));
	const sections = [...SECTIONS, ['More settings', keys.filter((k) => !placed.has(k)).sort()]];
	const inputs = new Map();
	const rowFor = (k) => {
		const path = `network.${net}.${k}`;
		const f = describe(d, path);
		if (!f) return null;
		const cur = fields[k];
		const it = input(path, f, cur?.value);
		const locked = cur?.origin === 'locked' && cur.from !== folder;
		if (locked) it.el.disabled = true;
		else inputs.set(k, it);
		return h('label', { class: 'field' },
			h('span', { class: 'label' }, group(path).label),
			it.el,
			locked && h('span', { class: 'sub' }, 'locked above'));
	};
	const body = sections.map(([title, ks]) => {
		const rows = ks.map(rowFor).filter(Boolean);
		if (!rows.length) return null;
		if (title === 'More settings') return h('details', null, h('summary', null, title), h('div', { class: 'fields' }, rows));
		return [title && h('h3', null, title), h('div', { class: 'fields' }, rows)];
	});
	return { body, inputs };
}

// changes reads what the person changed into {path: value}, or throws with
// the first value that is wrong.
function changes(net, inputs) {
	const out = {};
	for (const [k, it] of inputs) {
		if (!it.changed()) continue;
		let v;
		try {
			v = it.read();
		} catch (e) {
			throw new Error(`${group(`network.${net}.${k}`).label}: ${e.message}`);
		}
		if (v !== undefined) out[`network.${net}.${k}`] = v;
	}
	return out;
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

function editForm(ctx, d, n, close) {
	const folderName = ctx.name('services', n.from);
	const { body, inputs } = form(d, n.id, n.fields, n.from);
	const box = h('div', { class: 'edit flush' });
	const msg = h('div', { class: 'error' });
	const review = async () => {
		msg.replaceChildren();
		let values;
		try {
			values = changes(n.id, inputs);
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
			h('div', { class: 'sub warn' }, 'Applying restarts the Wi-Fi on each AP listed; its clients drop for a few seconds and reconnect.'),
		]);
	};
	// The network can be removed where it is defined: what is set in this
	// service folder is unset, in one change (0046).
	const own = Object.keys(n.fields).filter((k) => n.fields[k].origin === 'self').map((k) => `network.${n.id}.${k}`).sort();
	const remove = async () => {
		const op = { kind: 'unset', tree: 'services', node: n.from, paths: own };
		const p = await ask(box, op);
		if (!p) return;
		confirm(ctx, box, op, p, [
			h('div', null, h('strong', null, `Remove ${n.fields.ssid?.value || n.id} from Services › ${folderName}`)),
			h('div', { class: 'sub' }, `Every location that uses Services › ${folderName} stops offering it.`),
		], [
			h('div', { class: 'sub warn' }, 'Its clients on each AP listed are disconnected.'),
		]);
	};
	return h('div', { class: 'netform', 'data-editing': true },
		body,
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review changes'),
			h('button', { type: 'button', class: 'button', onclick: close }, 'Cancel'),
			n.fields.ssid?.origin === 'self' && h('button', { type: 'button', class: 'button danger', onclick: remove }, 'Remove network')),
		box);
}

// addForm offers a new network in one of the service folders that apply
// here and that the person may change. Its name in the schema comes from
// its SSID.
function addForm(ctx, d, folders, nets, box) {
	const pickFolder = h('select', null, folders.map((f) => h('option', { value: f.id }, `Services › ${ctx.name('services', f.id)}`)));
	const { body, inputs } = form(d, 'new', {}, null);
	const msg = h('div', { class: 'error' });
	const out = h('div', { class: 'edit flush' });
	const review = async () => {
		msg.replaceChildren();
		let values;
		try {
			values = changes('new', inputs);
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
		h('div', { class: 'netform' },
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

// name makes a network's name from its SSID, as the schema's pattern allows
// and unlike any taken.
function name(ssid, pattern, taken) {
	let base = ssid.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 28) || 'network';
	if (!/^[a-z0-9]/.test(base)) base = 'n' + base;
	const ok = new RegExp(pattern);
	let id = base;
	for (let i = 2; taken.has(id) || !ok.test(id); i++) id = `${base}-${i}`;
	return id;
}
