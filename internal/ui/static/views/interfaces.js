// The Interfaces tab of a Locations folder or AP (0053): the Ethernet ports
// of its APs, as each last reported them, and what Aeolus sets for each
// port, with an editor built from the schema. A port is set by name, so a
// folder's setting for lan2 reaches every AP below with a lan2. The uplink
// carries the AP's management, so it is shown but not offered for editing.
// Tunnels is where tunnels are set, by name, for every AP below (0055), and
// shows each AP's VXLAN tunnels live, beside what its networks ask for (0054).

import { h, link } from '../dom.js';
import { schema } from '../api.js';
import { group, value, origin, ago } from '../format.js';
import { tabBar, pick } from '../layout.js';
import { configs } from './sections.js';
import { fieldsForm, changedValues } from './edit.js';
import { ask, confirm } from './confirm.js';
import { followButton } from './follow.js';
import { tunnelsAt } from './networks.js';

const INTERFACES = [['ethernet', 'Ethernet'], ['tunnels', 'Tunnels']];

// The port fields offered, in order. LACP and its bond are not applied yet,
// and the uplink is the agent's own setting.
const FIELDS = ['enabled', 'mode', 'untagged', 'tagged'];

const RELOAD = "Applying reloads each AP's network: wired clients on this port drop briefly. An AP that can no longer reach Aeolus puts its old settings back within 90 seconds.";

// A tunnel's fields, in order, and where a new one starts (0055).
const TUNNEL = ['address', 'port', 'mtu'];
const TUNNEL_START = { port: 4789, mtu: 1450 };
const TUNNEL_RELOAD = "Applying reloads the network, and restarts the Wi-Fi, on each AP whose networks use this tunnel. An AP that can no longer reach Aeolus puts its old settings back within 90 seconds.";

// interfacesTab draws the Interfaces tab of a Locations node whose page is
// at base. ap ({ap, cfg}) is the AP itself, on an AP's page.
export async function interfacesTab(ctx, base, id, page, sub, ap, edit) {
	sub = pick(INTERFACES, sub);
	const body = sub === 'tunnels' ? await tunnels(ctx, id, page, ap, edit) : await ethernet(ctx, id, page, ap, edit);
	return [tabBar(`${base}/interfaces`, INTERFACES, sub, true), body];
}

// tunnels shows the tunnels set here, each with its editor, and a way to add
// one (0055); and below them, for each AP here, the tunnels its networks run
// and what it reported, joined by VNI (0054).
async function tunnels(ctx, here, page, ap, edit) {
	const [d, rows] = await Promise.all([schema(), ap ? [ap] : configs(page.hardware?.aps || [])]);
	const fields = page.fields || {};
	const set = tunnelsAt(fields);
	const addBox = h('div', { class: 'edit flush' });
	const lines = rows.flatMap(({ ap, cfg }) => tunnelRows(ap, cfg));
	return [
		set.length
			? h('div', { class: 'bands' }, set.map((t) => tunnelCard(ctx, d, here, page.node.name, t.id, fields, edit)))
			: h('div', { class: 'banner info' }, 'No tunnel is set here. A tunnel set on a folder reaches every AP below it, and a network picks one in its VXLAN transport.'),
		edit && h('div', { class: 'below' },
			h('button', { type: 'button', class: 'button', onclick: () => addTunnel(ctx, d, here, page.node.name, fields, addBox) }, 'Add a tunnel')),
		addBox,
		lines.length
			? h('section', { class: 'panel' },
				h('h2', null, 'Tunnels now', h('span', { class: 'note' }, 'as each AP last reported')),
				h('table', { class: 'list' },
					h('tr', null, ['AP', 'Network', 'As', 'Tunnel', 'VNI', 'MTU', 'State'].map((c) => h('th', null, c))),
					lines))
			: h('div', { class: 'banner info' }, 'No network here travels over VXLAN.'),
	];
}

// tunnelCard shows one tunnel by name: its far end, port and MTU, and where
// each comes from. Someone who may change this node can edit it, or have it
// follow the folder above again; one set only here is deleted that way.
function tunnelCard(ctx, d, here, nodeName, name, fields, edit) {
	const paths = TUNNEL.map((k) => `concentrators.${name}.${k}`).filter((p) => fields[p]);
	const own = paths.filter((p) => fields[p].origin === 'self');
	const onlyHere = own.length === paths.length;
	const body = h('div', null);
	const box = h('div', { class: 'edit' });
	const names = (id) => ctx.name('locations', id);
	const close = () => body.replaceChildren(h('div', null, paths.map((path) => {
		const r = fields[path];
		return h('div', { class: 'row' },
			h('div', { class: 'label' }, group(path).label),
			h('div', { class: 'value' }, value(path, r.value)),
			origin('locations', here, r, names));
	})));
	close();
	return h('section', { class: 'panel' },
		h('h2', null, name,
			edit && h('span', { class: 'controls' },
				h('button', { type: 'button', class: 'button small', onclick: () => { box.replaceChildren(); body.replaceChildren(tunnelForm(ctx, d, here, nodeName, name, fields, false, close)); } }, 'Edit'),
				own.length > 0 && followButton(ctx, 'locations', here, edit.nodeName, edit.parentName, own, box,
					onlyHere ? 'Delete' : `Follow ${edit.parentName}`, onlyHere ? `Delete tunnel ${name} from ${nodeName}` : null))),
		body,
		box);
}

// addTunnel sets up a new tunnel here, by name.
function addTunnel(ctx, d, here, nodeName, fields, box) {
	const pattern = new RegExp(d.names.concentrators || '^[a-z0-9][a-z0-9-]{0,31}$');
	const nameIn = h('input', { type: 'text', maxlength: 32, placeholder: 'core' });
	const slot = h('div', null);
	const msg = h('div', { class: 'error' });
	const next = () => {
		msg.replaceChildren();
		const name = nameIn.value.trim();
		if (!pattern.test(name)) {
			msg.replaceChildren('A tunnel name is lower case letters, digits and hyphens, such as core or dc-east.');
			return;
		}
		if (fields[`concentrators.${name}.address`]) {
			msg.replaceChildren(`A tunnel named ${name} is already set here; edit it instead.`);
			return;
		}
		slot.replaceChildren(tunnelForm(ctx, d, here, nodeName, name, fields, true, () => box.replaceChildren()));
	};
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, 'Add a tunnel'),
		h('div', { class: 'fieldform' },
			h('label', { class: 'field' }, h('span', { class: 'label' }, 'Name'), nameIn),
			msg,
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button', onclick: next }, 'Next'),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel')),
			slot)));
}

// tunnelForm edits a tunnel's fields on this node in one change. A new one
// starts at port 4789 and an MTU of 1450, and is set whole.
function tunnelForm(ctx, d, here, nodeName, name, fields, fresh, close) {
	const prefix = `concentrators.${name}.`;
	const { body, inputs, rows } = fieldsForm(d, [[null, TUNNEL.map((k) => prefix + k)]], fields, here);
	if (fresh)
		for (const [k, v] of Object.entries(TUNNEL_START)) {
			const el = rows.get(prefix + k)?.it.el;
			if (el) el.value = String(v);
		}
	const out = h('div', { class: 'edit flush' });
	const msg = h('div', { class: 'error' });
	const review = async () => {
		msg.replaceChildren();
		let values;
		try {
			values = changedValues(inputs, rows);
			if (fresh)
				for (const k of TUNNEL) {
					const v = rows.get(prefix + k)?.it.read();
					if (v === undefined) throw new Error(`${group(prefix + k).label}: set it`);
					values[prefix + k] = v;
				}
		} catch (e) {
			msg.replaceChildren(e.message);
			return;
		}
		const paths = Object.keys(values);
		if (!paths.length) {
			msg.replaceChildren('Nothing has changed.');
			return;
		}
		const op = paths.length === 1
			? { kind: 'set', tree: 'locations', node: here, path: paths[0], value: values[paths[0]] }
			: { kind: 'set', tree: 'locations', node: here, values };
		const p = await ask(out, op);
		if (!p) return;
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, `${fresh ? 'Add tunnel' : 'Tunnel'} ${name} on ${nodeName}`)),
			h('ul', { class: 'becomes' }, paths.map((path) => h('li', null,
				`${group(path).label}: `,
				fields[path] ? [value(path, fields[path].value), ' → '] : '', value(path, values[path])))),
			h('div', { class: 'sub' }, fresh
				? `Every AP here can use it once a network picks ${name} in its VXLAN transport.`
				: 'APs below that set their own keep theirs.'),
		], [h('div', { class: 'sub warn' }, TUNNEL_RELOAD)]);
	};
	return h('div', { class: 'fieldform', 'data-editing': true },
		body,
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review changes'),
			h('button', { type: 'button', class: 'button', onclick: close }, 'Cancel')),
		out);
}

// tunnelRows joins what an AP's config asks for with what it reported.
function tunnelRows(ap, cfg) {
	if (!cfg) return [];
	const doc = cfg.document || {};
	const report = cfg.condition?.state?.report;
	const reported = new Map((report?.vxlan?.tunnels || []).map((t) => [t.vni, t]));
	const want = [];
	for (const [id, n] of Object.entries(doc.network || {}).sort()) {
		if (n.enabled === false) continue;
		for (const slot of ['primary', 'fallback']) {
			const t = n.transport?.[slot];
			if (t?.type === 'vxlan') want.push({ id, ssid: n.ssid || id, slot, t, conc: doc.concentrators?.[t.concentrator] });
		}
	}
	const apLink = link(`/aps/${encodeURIComponent(ap.id)}`, ap.name);
	const lines = want.map((w, i) => h('tr', null,
		h('td', null, i === 0 && apLink),
		h('td', null, w.ssid),
		h('td', null, w.slot === 'primary' ? 'primary' : 'fallback'),
		h('td', null, w.t.concentrator, w.conc && h('span', { class: 'sub' }, ` ${w.conc.address}:${w.conc.port}`)),
		h('td', { class: 'mono' }, String(w.t.vni)),
		h('td', null, w.conc ? String(w.conc.mtu) : '—'),
		h('td', null, tunnelState(report, reported.get(w.t.vni)))));
	// What the AP runs that nothing asks for any more, until it applies.
	const asked = new Set(want.map((w) => w.t.vni));
	for (const t of reported.values())
		if (!asked.has(t.vni))
			lines.push(h('tr', null,
				h('td', null, lines.length === 0 && apLink),
				h('td', { class: 'sub', colspan: 3 }, `to ${t.peer}:${t.port}; no network asks for it`),
				h('td', { class: 'mono' }, String(t.vni)),
				h('td', null, String(t.mtu || '—')),
				h('td', null, t.up ? h('span', { class: 'chip ok' }, 'up') : h('span', { class: 'chip idle' }, 'down'))));
	return lines;
}

// tunnelState says what the AP reported for a tunnel its config asks for.
function tunnelState(report, t) {
	if (!report) return h('span', { class: 'sub' }, 'no report yet');
	if (!report.vxlan) return h('span', { class: 'sub' }, 'its agent does not report tunnels; update it');
	if (report.vxlan.installed === false) return h('span', { class: 'chip warn' }, 'vxlan is not installed (apk add vxlan)');
	if (!t) return h('span', { class: 'sub' }, 'not on the AP yet');
	if (t.up) return h('span', { class: 'chip ok' }, 'up');
	if (t.standby) return h('span', { class: 'chip idle' }, 'standing by');
	return h('span', { class: 'chip warn' }, 'down');
}
