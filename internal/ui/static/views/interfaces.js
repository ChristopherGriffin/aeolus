// The Interfaces tab of a Locations folder or AP (0053): the Ethernet ports
// of its APs, as each last reported them, and what Aeolus sets for each
// port, with an editor built from the schema. A port is set by name, so a
// folder's setting for lan2 reaches every AP below with a lan2. The uplink
// carries the AP's management, so it is shown but not offered for editing.
// A port in tunnel mode carries VNIs instead of VLANs, each untagged or on a
// VLAN of its own (0058).
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
import { tunnelsAt, tunnelName } from './networks.js';

const INTERFACES = [['ethernet', 'Ethernet'], ['tunnels', 'Tunnels']];

// The port fields offered, in order. LACP and its bond are not applied yet,
// and the uplink is the agent's own setting.
const FIELDS = ['enabled', 'mode', 'untagged', 'tagged'];

const RELOAD = "Applying reloads each AP's network: wired clients on this port drop briefly. An AP that can no longer reach Aeolus puts its old settings back within 90 seconds.";

// A tunnel's fields, in order, and where a new one starts (0055). Its MTU is
// left to the default unless someone sets one (0056).
const TUNNEL = ['address', 'port', 'mtu'];
const TUNNEL_START = { port: 4789 };

// defaultMTU is a tunnel's MTU when none is set: what a 1500-byte uplink
// carries once VXLAN's headers are added (0056).
function defaultMTU(address) {
	return String(address ?? '').includes(':') ? 1430 : 1450;
}
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
	const mtuPath = `concentrators.${name}.mtu`;
	const close = () => body.replaceChildren(h('div', null,
		paths.map((path) => {
			const r = fields[path];
			const rowBox = h('div', { class: 'edit' });
			return [h('div', { class: 'row' },
				h('div', { class: 'label' }, group(path).label),
				h('div', { class: 'value' }, value(path, r.value)),
				origin('locations', here, r, names),
				edit && r.origin === 'self' && !onlyHere && followButton(ctx, 'locations', here, edit.nodeName, edit.parentName, [path], rowBox),
				edit && path === mtuPath && r.origin === 'self' && onlyHere && followButton(ctx, 'locations', here, edit.nodeName, edit.parentName, [path], rowBox,
					'Use the default', `Tunnel ${name} on ${nodeName} goes back to the default MTU`)),
			rowBox];
		}),
		!fields[mtuPath] && h('div', { class: 'row' },
			h('div', { class: 'label' }, 'MTU'),
			h('div', { class: 'value' }, `default, ${defaultMTU(fields[`concentrators.${name}.address`]?.value)}`))));
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
	// Left empty, the MTU is the default; a number is a custom MTU, which the
	// AP's uplink must carry (0056).
	const mtuEl = rows.get(prefix + 'mtu')?.it.el;
	if (mtuEl) mtuEl.placeholder = 'default: 1450 (1430 over IPv6)';
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
					if (v === undefined && k !== 'mtu') throw new Error(`${group(prefix + k).label}: set it`);
					if (v !== undefined) values[prefix + k] = v;
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
	if (report.vxlan.loaded === false) return h('span', { class: 'chip warn' }, 'vxlan is installed, but netifd has not loaded it: restart the network');
	if (!t) return h('span', { class: 'sub' }, 'not on the AP yet');
	if (t.up) return h('span', { class: 'chip ok' }, 'up');
	if (t.standby) return h('span', { class: 'chip idle' }, 'standing by');
	return h('span', { class: 'chip warn' }, 'down');
}

async function ethernet(ctx, here, page, ap, edit) {
	const [d, rows] = await Promise.all([schema(), ap ? [ap] : configs(page.hardware?.aps || [])]);
	// Each port by name: the APs here that reported it, and whether it is
	// the uplink on any of them.
	const seen = new Map();
	for (const r of rows)
		for (const p of r.cfg?.condition?.state?.report?.ports || []) {
			if (!seen.has(p.name)) seen.set(p.name, { aps: [], uplinkOn: [] });
			seen.get(p.name).aps.push(r.ap);
			if (p.uplink) seen.get(p.name).uplinkOn.push(r.ap);
		}
	const fields = page.fields || {};
	const named = Object.keys(fields).filter((p) => p.startsWith('ports.')).map((p) => p.split('.')[1]);
	const names = [...new Set([...seen.keys(), ...named])].sort(byPort);
	const addBox = h('div', { class: 'edit flush' });
	return [
		names.length
			? h('div', { class: 'bands' }, names.map((name) => portCard(ctx, d, here, page.node.name, name, fields, seen.get(name), edit, !ap)))
			: h('div', { class: 'banner info' }, 'No AP here has reported its ports yet, and no port is set here.'),
		edit && h('div', { class: 'below' },
			h('button', { type: 'button', class: 'button', onclick: () => addPort(ctx, d, here, page.node.name, fields, addBox, edit) }, 'Set up another port')),
		addBox,
		portsNow(rows),
	];
}

// byPort orders port names as people count: lan2 before lan10.
function byPort(a, b) {
	return a.localeCompare(b, undefined, { numeric: true });
}

// portCard shows one port by name; on a folder, folder is true, and the
// card says how many of its APs have that port.
function portCard(ctx, d, here, nodeName, name, fields, seen, edit, folder) {
	const f = (k) => fields[`ports.${name}.${k}`];
	const uplinkOn = seen?.uplinkOn || [];
	const body = h('div', null);
	const viewNow = () => view(ctx, here, nodeName, name, fields, edit);
	const close = () => body.replaceChildren(h('div', null, viewNow()));
	close();
	return h('section', { class: 'panel' },
		h('h2', null, name,
			uplinkOn.length > 0 && h('span', { class: 'chip from' }, 'uplink'),
			f('enabled')?.value === false && h('span', { class: 'chip idle' }, 'off'),
			folder && h('span', { class: 'note' }, seen ? `on ${seen.aps.length} AP${seen.aps.length === 1 ? '' : 's'}` : 'no AP here has reported it'),
			edit && uplinkOn.length === 0 && h('span', { class: 'controls' },
				h('button', { type: 'button', class: 'button small', onclick: () => body.replaceChildren(portForm(ctx, d, here, nodeName, name, fields, close, edit)) }, 'Edit'))),
		uplinkOn.length > 0 && h('div', { class: 'sub' },
			`The uplink on ${uplinkOn.map((a) => a.name).join(', ')}: it carries the AP's management, so Aeolus leaves it alone there.`),
		body);
}

function view(ctx, here, nodeName, name, fields, edit) {
	const set = FIELDS.map((k) => `ports.${name}.${k}`).filter((p) => fields[p]);
	const vnis = vnisOf(fields, name);
	if (!set.length && !vnis.length) return h('div', { class: 'sub' }, 'Not set: each AP leaves this port as it is.');
	const names = (id) => ctx.name('locations', id);
	// What the mode leaves out stays set, but does nothing (0058).
	const tunnel = fields[`ports.${name}.mode`]?.value === 'tunnel';
	const unused = (path) => (tunnel ? /\.(untagged|tagged)$/.test(path) : /\.vxlan\./.test(path));
	const row = (label, path, shown, r, paths) => {
		const box = h('div', { class: 'edit' });
		const own = paths.filter((p) => fields[p]?.origin === 'self');
		return [h('div', { class: 'row' },
			h('div', { class: 'label' }, label, unused(path) && h('span', { class: 'sub' }, ' (unused in this mode)')),
			h('div', { class: 'value' }, shown),
			origin('locations', here, r, names),
			edit && own.length > 0 && followButton(ctx, 'locations', here, edit.nodeName, edit.parentName, own, box)),
		box];
	};
	return [
		set.map((path) => row(group(path).label, path, value(path, fields[path].value), fields[path], [path])),
		vnis.map((m) => row(onWire(m.vlan, true), vniPath(name, m.vlan, 'vni'), carried(m), m.tunnel ?? m.vni,
			['tunnel', 'vni'].map((k) => vniPath(name, m.vlan, k)))),
	];
}

// vniPath is where a tunnel port sets one of a VNI's fields (0058).
const vniPath = (name, vlan, k) => `ports.${name}.vxlan.${vlan}.${k}`;

// vnisOf reads a tunnel port's VNIs from the values in force (by path, each
// {value, from, origin}) as [{vlan, tunnel, vni}], untagged first, then by
// VLAN.
function vnisOf(fields, name) {
	const prefix = `ports.${name}.vxlan.`;
	const out = new Map();
	for (const [path, r] of Object.entries(fields || {})) {
		if (!path.startsWith(prefix)) continue;
		const [vlan, k] = path.slice(prefix.length).split('.');
		if (k !== 'tunnel' && k !== 'vni') continue;
		if (!out.has(vlan)) out.set(vlan, { vlan });
		out.get(vlan)[k] = r;
	}
	const rank = (v) => (v === 'untagged' ? 0 : Number(v));
	return [...out.values()].sort((a, b) => rank(a.vlan) - rank(b.vlan));
}

// onWire says how a port carries a VNI: untagged, or on a VLAN.
function onWire(vlan, capital) {
	if (vlan === 'untagged') return capital ? 'Untagged' : 'untagged';
	return `VLAN ${vlan}`;
}

// carried says where a VNI goes: "arista · VNI 50".
function carried(m) {
	return `${m.tunnel?.value ?? '(no tunnel)'} · VNI ${m.vni?.value ?? '(none)'}`;
}

// portForm edits one port's settings on this node, in one change. Only what
// the mode uses is shown: an access port's one VLAN, a trunk's untagged VLAN
// and tagged ones, or a tunnel port's VNIs (0058).
function portForm(ctx, d, here, nodeName, name, fields, close, edit) {
	const prefix = `ports.${name}.`;
	const { body, inputs, rows } = fieldsForm(d, [[null, FIELDS.map((k) => prefix + k)]], fields, here);
	const mode = rows.get(prefix + 'mode')?.it.el;
	// LACP is in the schema, but not applied yet (0053).
	if (mode && mode.value !== 'lacp') mode.querySelector('option[value="lacp"]')?.remove();
	const out = h('div', { class: 'edit flush' });
	const vnis = vniTable(ctx, here, nodeName, name, fields, edit, out);
	const sync = () => {
		const m = mode?.value;
		const show = (k, on) => {
			const r = rows.get(prefix + k)?.row;
			if (r) r.hidden = !on;
		};
		show('untagged', m === 'access' || m === 'trunk');
		show('tagged', m === 'trunk');
		vnis.el.hidden = m !== 'tunnel';
	};
	mode?.addEventListener('change', sync);
	sync();
	const msg = h('div', { class: 'error' });
	const review = async () => {
		msg.replaceChildren();
		let values;
		try {
			values = changedValues(inputs, rows);
			if (!vnis.el.hidden) Object.assign(values, vnis.read());
		} catch (e) {
			msg.replaceChildren(e.message);
			return;
		}
		const now = (k) => (prefix + k in values ? values[prefix + k] : fields[prefix + k]?.value);
		// An access port carries no tagged VLANs, so a list in force goes.
		if (now('mode') === 'access' && now('tagged')?.length) values[prefix + 'tagged'] = [];
		if (now('mode') === 'access' && !now('untagged')) {
			msg.replaceChildren('An access port needs its untagged VLAN.');
			return;
		}
		if (now('mode') === 'tunnel' && !vnis.count()) {
			msg.replaceChildren('A tunnel port needs a VNI: add one under VNIs.');
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
			h('div', null, h('strong', null, `Port ${name} on ${nodeName}`)),
			h('ul', { class: 'becomes' }, paths.map((path) => h('li', null,
				`${group(path).label}: `,
				fields[path] ? [value(path, fields[path].value), ' → '] : '', value(path, values[path])))),
			h('div', { class: 'sub' }, `Every AP here with a port named ${name} uses it; APs below that set their own keep theirs.`),
		], [h('div', { class: 'sub warn' }, RELOAD)]);
	};
	return h('div', { class: 'fieldform', 'data-editing': true },
		body,
		vnis.el,
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review changes'),
			h('button', { type: 'button', class: 'button', onclick: close }, 'Cancel')),
		out);
}

// vniTable edits a tunnel port's VNIs a row at a time (0058). A row in force
// can take another tunnel or VNI, and one more row can be added, both with
// the rest of the port's change; a row set here can be removed, which is a
// change of its own, previewed in out. read() returns {path: value} to set,
// or throws with what is wrong; count() says how many VNIs the port would
// carry.
function vniTable(ctx, here, nodeName, name, fields, edit, out) {
	const lib = tunnelsAt(fields);
	const names = (id) => ctx.name('locations', id);
	const picker = (cur) => h('select', null,
		h('option', { value: '' }, lib.length ? 'tunnel…' : 'no tunnel is set here yet (Interfaces › Tunnels)'),
		lib.map((t) => h('option', { value: t.id, selected: t.id === cur }, tunnelName(lib, t.id))),
		cur && !lib.some((t) => t.id === cur) && h('option', { value: cur, selected: true }, `${cur} (not set here)`));
	const number = (cur) => h('input', { type: 'number', min: 1, max: 16777215, step: 1, value: cur ?? '', placeholder: 'VNI' });
	const vniIn = (el, vlan) => {
		if (el.value === '') return undefined;
		const n = Number(el.value);
		if (!Number.isInteger(n) || n < 1 || n > 16777215) throw new Error(`${onWire(vlan, true)}: the VNI is a whole number from 1 to 16777215`);
		return n;
	};
	const have = vnisOf(fields, name);
	const lines = have.map((m) => {
		const tunnel = picker(m.tunnel?.value);
		const vni = number(m.vni?.value);
		tunnel.disabled = vni.disabled = [m.tunnel, m.vni].some((r) => r?.origin === 'locked' && r.from !== here);
		const own = ['tunnel', 'vni'].map((k) => vniPath(name, m.vlan, k)).filter((p) => fields[p]?.origin === 'self');
		const heading = `Port ${name} on ${nodeName}: ${onWire(m.vlan)} leaves VNI ${m.vni?.value}`;
		return {
			m, tunnel, vni,
			row: h('div', { class: 'field' },
				h('span', { class: 'label' }, onWire(m.vlan, true)),
				tunnel, vni,
				origin('locations', here, m.tunnel ?? m.vni, names),
				own.length > 0 && followButton(ctx, 'locations', here, nodeName, edit?.parentName, own, out, 'Remove', heading)),
		};
	});
	const vlanIn = h('input', { type: 'text', maxlength: 8, placeholder: 'untagged, or a VLAN' });
	const newTunnel = picker(undefined);
	const newVni = number(undefined);
	const read = () => {
		const values = {};
		for (const l of lines) {
			if (l.tunnel.disabled) continue;
			const t = l.tunnel.value || undefined;
			const v = vniIn(l.vni, l.m.vlan);
			if (t !== l.m.tunnel?.value) {
				if (!t) throw new Error(`${onWire(l.m.vlan, true)}: pick its tunnel, or remove the row`);
				values[vniPath(name, l.m.vlan, 'tunnel')] = t;
			}
			if (v !== l.m.vni?.value) {
				if (v === undefined) throw new Error(`${onWire(l.m.vlan, true)}: set its VNI, or remove the row`);
				values[vniPath(name, l.m.vlan, 'vni')] = v;
			}
		}
		const vlan = vlanIn.value.trim().toLowerCase();
		if (!vlan && !newTunnel.value && newVni.value === '') return values;
		if (!/^(untagged|[1-9][0-9]{0,3})$/.test(vlan) || (vlan !== 'untagged' && Number(vlan) > 4094))
			throw new Error('The new VNI is carried untagged, or on a VLAN from 1 to 4094.');
		if (have.some((m) => m.vlan === vlan)) throw new Error(`${onWire(vlan, true)} carries a VNI already; change that row instead.`);
		const v = vniIn(newVni, vlan);
		if (!newTunnel.value) throw new Error(`${onWire(vlan, true)}: pick its tunnel.`);
		if (v === undefined) throw new Error(`${onWire(vlan, true)}: set its VNI.`);
		values[vniPath(name, vlan, 'tunnel')] = newTunnel.value;
		values[vniPath(name, vlan, 'vni')] = v;
		return values;
	};
	const el = h('div', { class: 'vnis' },
		h('h3', null, 'VNIs'),
		h('div', { class: 'sub' }, 'Each VNI is carried untagged, or tagged with a VLAN of its own, over a tunnel set here. The port leaves the uplink\'s bridge.'),
		h('div', { class: 'fields' },
			lines.map((l) => l.row),
			h('div', { class: 'field' }, h('span', { class: 'label' }, have.length ? 'Add another' : 'Add one'), vlanIn, newTunnel, newVni)));
	return { el, read, count: () => have.length + (vlanIn.value.trim() ? 1 : 0) };
}

// addPort sets up a port no AP here has reported yet, by its name, such as
// one on APs that are still to be adopted.
function addPort(ctx, d, here, nodeName, fields, box, edit) {
	const pattern = new RegExp(d.names.ports || '^[a-z][a-z0-9._-]{0,15}$');
	const nameIn = h('input', { type: 'text', maxlength: 16, placeholder: 'lan3' });
	const slot = h('div', null);
	const msg = h('div', { class: 'error' });
	const next = () => {
		msg.replaceChildren();
		const name = nameIn.value.trim();
		if (!pattern.test(name)) {
			msg.replaceChildren('A port name is lower case, such as lan3 or eth1.');
			return;
		}
		slot.replaceChildren(portForm(ctx, d, here, nodeName, name, fields, () => box.replaceChildren(), edit));
	};
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, 'Set up another port'),
		h('div', { class: 'fieldform' },
			h('label', { class: 'field' }, h('span', { class: 'label' }, 'Port name'), nameIn),
			msg,
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button', onclick: next }, 'Next'),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel')),
			slot)));
}

// portsNow lists each AP's ports as it last reported them: whether each has
// a link and at what speed, beside what Aeolus sets for it there.
function portsNow(rows) {
	if (!rows.length) return null;
	const lines = rows.flatMap(({ ap, cfg }) => {
		const rep = cfg?.condition?.state;
		const ports = rep?.report?.ports || [];
		const apLink = link(`/aps/${encodeURIComponent(ap.id)}`, ap.name);
		if (!ports.length) return [h('tr', null, h('td', null, apLink), h('td', { colspan: 4, class: 'sub' }, cfg ? 'No ports reported yet; its agent may be older than this.' : 'You cannot see this AP.'))];
		return ports.map((p, i) => h('tr', null,
			h('td', null, i === 0 && apLink),
			h('td', { class: 'mono' }, p.name, p.uplink && h('span', { class: 'chip from' }, 'uplink')),
			h('td', null, linkState(p)),
			h('td', null, settings(cfg.location || {}, p.name)),
			h('td', null, i === 0 && ago(rep.at))));
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'Ports now', h('span', { class: 'note' }, 'as each AP last reported')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'Port', 'Link', 'Aeolus sets', 'Reported'].map((c) => h('th', null, c))),
			lines));
}

// linkState says whether a port has a link, and at what speed: "1000F" is
// 1 Gbit/s, full duplex.
function linkState(p) {
	if (!p.up) return h('span', { class: 'chip idle' }, 'off');
	if (!p.carrier) return h('span', { class: 'sub' }, 'no link');
	const m = /^(\d+)([FH])$/.exec(p.speed || '');
	if (!m) return 'link';
	const mbit = Number(m[1]);
	return `${mbit >= 1000 ? mbit / 1000 + ' Gbit/s' : mbit + ' Mbit/s'}${m[2] === 'H' ? ', half duplex' : ''}`;
}

// settings sums up what Aeolus sets for a port at an AP.
function settings(location, name) {
	const v = (k) => location[`ports.${name}.${k}`]?.value;
	const parts = [];
	if (v('enabled') === false) parts.push('off');
	if (v('mode') === 'access') parts.push(`access, VLAN ${v('untagged')}`);
	else if (v('mode') === 'trunk') {
		parts.push('trunk');
		if (v('untagged')) parts.push(`untagged ${v('untagged')}`);
		if (v('tagged')?.length) parts.push(`tagged ${v('tagged').join(', ')}`);
	} else if (v('mode') === 'tunnel') {
		// "VLAN 50 → arista · VNI 50" for each VNI the port carries (0058).
		const vnis = vnisOf(location, name);
		parts.push(vnis.length ? `tunnel: ${vnis.map((m) => `${onWire(m.vlan)} → ${carried(m)}`).join(', ')}` : 'tunnel, with no VNIs yet');
	} else if (v('mode') === 'lacp') parts.push('LACP (not applied yet)');
	return parts.length ? parts.join(', ') : h('span', { class: 'sealed' }, 'not set; the AP keeps its own');
}
