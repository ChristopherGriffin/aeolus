// The Interfaces tab of a Locations folder or AP (0053): the Ethernet ports
// of its APs, as each last reported them, and what Aeolus sets for each
// port, with an editor built from the schema. A port is set by name, so a
// folder's setting for lan2 reaches every AP below with a lan2. The uplink
// carries the AP's management, so it is shown but not offered for editing.
// Tunnels shows each AP's VXLAN tunnels live, beside what its networks ask
// for (0054).

import { h, link } from '../dom.js';
import { get, schema } from '../api.js';
import { group, value, origin, ago } from '../format.js';
import { tabBar, pick } from '../layout.js';
import { configs } from './sections.js';
import { fieldsForm, changedValues } from './edit.js';
import { ask, confirm } from './confirm.js';
import { followButton } from './follow.js';
import { availableAt, concentratorName } from './networks.js';

const INTERFACES = [['ethernet', 'Ethernet'], ['tunnels', 'Tunnels']];

// The port fields offered, in order. LACP and its bond are not applied yet,
// and the uplink is the agent's own setting.
const FIELDS = ['enabled', 'mode', 'untagged', 'tagged'];

const RELOAD = "Applying reloads each AP's network: wired clients on this port drop briefly. An AP that can no longer reach Aeolus puts its old settings back within 90 seconds.";

// interfacesTab draws the Interfaces tab of a Locations node whose page is
// at base. ap ({ap, cfg}) is the AP itself, on an AP's page.
export async function interfacesTab(ctx, base, id, page, sub, ap, edit) {
	sub = pick(INTERFACES, sub);
	const body = sub === 'tunnels' ? await tunnels(ctx, id, page, ap) : await ethernet(ctx, id, page, ap, edit);
	return [tabBar(`${base}/interfaces`, INTERFACES, sub, true), body];
}

// tunnels lists, for each AP here, the tunnels its networks ask for and the
// ones it reported (0054), joined by VNI, and below them the concentrators
// that may be used here.
async function tunnels(ctx, here, page, ap) {
	const [rows, library] = await Promise.all([ap ? [ap] : configs(page.hardware?.aps || []), get('/v1/library')]);
	const lib = library.concentrators || [];
	const lines = rows.flatMap(({ ap, cfg }) => tunnelRows(ap, cfg, lib));
	return [
		lines.length
			? h('section', { class: 'panel' },
				h('h2', null, 'Tunnels now', h('span', { class: 'note' }, 'as each AP last reported')),
				h('table', { class: 'list' },
					h('tr', null, ['AP', 'Network', 'As', 'Concentrator', 'VNI', 'MTU', 'State'].map((c) => h('th', null, c))),
					lines))
			: h('div', { class: 'banner info' }, 'No network here travels over VXLAN.'),
		concentratorsHere(availableAt(lib, page.ancestry || [here])),
	];
}

// tunnelRows joins what an AP's config asks for with what it reported.
function tunnelRows(ap, cfg, lib) {
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
		h('td', null, concentratorName(lib, w.t.concentrator), w.conc && h('span', { class: 'sub' }, ` ${w.conc.address}:${w.conc.port}`)),
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

// concentratorsHere lists the library's concentrators that may be used here,
// with their labeled VNIs (0023).
function concentratorsHere(lib) {
	return h('section', { class: 'panel' },
		h('h2', null, 'Concentrators available here', h('span', { class: 'note' }, 'from the library')),
		lib.length === 0
			? h('div', { class: 'sub' }, 'None yet. A concentrator is added to the library by an admin at the Services root.')
			: h('table', { class: 'list' },
				h('tr', null, ['Concentrator', 'Address', 'Port', 'MTU', 'VNIs'].map((c) => h('th', null, c))),
				lib.map((c) => h('tr', null,
					h('td', null, c.name),
					h('td', { class: 'mono' }, c.address),
					h('td', null, String(c.port)),
					h('td', null, String(c.mtu)),
					h('td', null, Object.entries(c.vnis || {}).sort((a, b) => a[0] - b[0]).map(([v, label]) => `${v} · ${label}`).join(', ') || '—')))));
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
			h('button', { type: 'button', class: 'button', onclick: () => addPort(ctx, d, here, page.node.name, fields, addBox) }, 'Set up another port')),
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
				h('button', { type: 'button', class: 'button small', onclick: () => body.replaceChildren(portForm(ctx, d, here, nodeName, name, fields, close)) }, 'Edit'))),
		uplinkOn.length > 0 && h('div', { class: 'sub' },
			`The uplink on ${uplinkOn.map((a) => a.name).join(', ')}: it carries the AP's management, so Aeolus leaves it alone there.`),
		body);
}

function view(ctx, here, nodeName, name, fields, edit) {
	const set = FIELDS.map((k) => `ports.${name}.${k}`).filter((p) => fields[p]);
	if (!set.length) return h('div', { class: 'sub' }, 'Not set: each AP leaves this port as it is.');
	const names = (id) => ctx.name('locations', id);
	return set.map((path) => {
		const r = fields[path];
		const box = h('div', { class: 'edit' });
		return [h('div', { class: 'row' },
			h('div', { class: 'label' }, group(path).label),
			h('div', { class: 'value' }, value(path, r.value)),
			origin('locations', here, r, names),
			edit && r.origin === 'self' && followButton(ctx, 'locations', here, edit.nodeName, edit.parentName, [path], box)),
		box];
	});
}

// portForm edits one port's settings on this node, in one change. Only what
// the mode uses is shown: an access port's one VLAN, or a trunk's untagged
// VLAN and tagged ones.
function portForm(ctx, d, here, nodeName, name, fields, close) {
	const prefix = `ports.${name}.`;
	const { body, inputs, rows } = fieldsForm(d, [[null, FIELDS.map((k) => prefix + k)]], fields, here);
	const mode = rows.get(prefix + 'mode')?.it.el;
	// LACP is in the schema, but not applied yet (0053).
	if (mode && mode.value !== 'lacp') mode.querySelector('option[value="lacp"]')?.remove();
	const sync = () => {
		const m = mode?.value;
		const show = (k, on) => {
			const r = rows.get(prefix + k)?.row;
			if (r) r.hidden = !on;
		};
		show('untagged', m === 'access' || m === 'trunk');
		show('tagged', m === 'trunk');
	};
	mode?.addEventListener('change', sync);
	sync();
	const out = h('div', { class: 'edit flush' });
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
		const now = (k) => (prefix + k in values ? values[prefix + k] : fields[prefix + k]?.value);
		// An access port carries no tagged VLANs, so a list in force goes.
		if (now('mode') === 'access' && now('tagged')?.length) values[prefix + 'tagged'] = [];
		if (now('mode') === 'access' && !now('untagged')) {
			msg.replaceChildren('An access port needs its untagged VLAN.');
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
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review changes'),
			h('button', { type: 'button', class: 'button', onclick: close }, 'Cancel')),
		out);
}

// addPort sets up a port no AP here has reported yet, by its name, such as
// one on APs that are still to be adopted.
function addPort(ctx, d, here, nodeName, fields, box) {
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
		slot.replaceChildren(portForm(ctx, d, here, nodeName, name, fields, () => box.replaceChildren()));
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
	} else if (v('mode') === 'lacp') parts.push('LACP (not applied yet)');
	return parts.length ? parts.join(', ') : h('span', { class: 'sealed' }, 'not set; the AP keeps its own');
}
