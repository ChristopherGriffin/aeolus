// The Interfaces tab of a Locations folder or AP (0053, 0072): its APs'
// radios, Ethernet ports and tunnels. Radios has the band cards, the
// channel map, where the channels the APs may go to are picked (0075), and
// its radio neighbours and their ratings (0073). Ethernet is one table of
// its APs' ports, a row each: its link, as each AP last reported it, its
// mode and what it carries, with an editor built from the schema under its
// row. A port is set by name, so a folder's setting for lan2 reaches every
// AP below with a lan2. The uplink carries the AP's management, so it is
// shown but not offered for editing.
// A port in tunnel mode carries VNIs instead of VLANs, each untagged or on a
// VLAN of its own (0058).
// Tunnels is where tunnels are set, by name, for every AP below (0055), and
// shows each AP's VXLAN tunnels live, beside what its networks and ports ask
// for (0054), with what the AP's prober found (0059).

import { h, link } from '../dom.js';
import { schema } from '../api.js';
import { group, value, origin, ago, secondsAgo, probeOnly, PROBE_ONLY, uplinkJudgment, switchPort } from '../format.js';
import { tabBar, pick } from '../layout.js';
import { configs } from './sections.js';
import { bandsSection } from './bands.js';
import { neighboursSection, ratingsSection } from './neighbours.js';
import { fieldsForm, changedValues } from './edit.js';
import { ask, confirm } from './confirm.js';
import { followButton } from './follow.js';
import { inheritsBar } from './inherits.js';
import { tunnelsAt, tunnelName } from './networks.js';
import { vxlanPath, probed } from './path.js';

const INTERFACES = [['radios', 'Radios'], ['ethernet', 'Ethernet'], ['tunnels', 'Tunnels']];
// Radios' views: the band cards, the channel map (0075), the other Aeolus
// APs each hears, and how it rates each channel (0073).
// Bands and their channel maps are one view (Griff, 2026-10-09); an old
// link to Channels lands there.
const RADIOS = [['bands', 'Bands'], ['neighbours', 'Neighbours'], ['ratings', 'Ratings']];

// The port fields offered, in order. LACP and its bond are not applied yet,
// and the uplink is the agent's own setting.
const FIELDS = ['enabled', 'mode', 'untagged', 'tagged'];

const RELOAD = "Applying reloads each AP's network: wired clients on this port drop briefly. An AP that can no longer reach Aeolus puts its old settings back within 90 seconds.";

// A tunnel's fields, in order, and where a new one starts (0055). Its MTU,
// probe interval and where it starts on the AP are left to their defaults
// unless someone sets them (0056, 0059, 0063).
const TUNNEL = ['address', 'port', 'mtu', 'probe_interval', 'underlay_vlan'];
const TUNNEL_START = { port: 4789 };
const DEFAULTED = new Set(['mtu', 'probe_interval', 'underlay_vlan']);

// defaultMTU is a tunnel's MTU when none is set: what a 1500-byte uplink
// carries once VXLAN's headers are added (0056).
function defaultMTU(address) {
	return String(address ?? '').includes(':') ? 1430 : 1450;
}
const TUNNEL_RELOAD = "Applying reloads the network, and restarts the Wi-Fi, on each AP whose networks use this tunnel. An AP that can no longer reach Aeolus puts its old settings back within 90 seconds.";

// interfacesTab draws the Interfaces tab of a Locations node whose page is
// at base: the part sub, and within Radios, the view. ap ({ap, cfg}) is the
// AP itself, on an AP's page.
export async function interfacesTab(ctx, base, id, page, sub, view, ap, edit) {
	sub = pick(INTERFACES, sub);
	const bar = tabBar(`${base}/interfaces`, INTERFACES, sub, true);
	if (sub === 'radios') {
		view = pick(RADIOS, view);
		const rows = () => (ap ? [ap] : configs(page.hardware?.aps || []));
		let body;
		const at = { node: id, nodeName: page.node.name, page, canEdit: !!edit,
			parentName: page.node.parent ? ctx.name('locations', page.node.parent) : null };
		if (view === 'ratings') body = ratingsSection(ctx, await rows());
		else if (view === 'neighbours') body = neighboursSection(ctx, at, await rows());
		else body = bandsSection(ctx, id, page, ap?.cfg?.condition?.state, await rows());
		return [bar, tabBar(`${base}/interfaces/radios`, RADIOS, view, 'minor'), body];
	}
	const body = sub === 'tunnels' ? await tunnels(ctx, id, page, ap, edit) : await ethernet(ctx, id, page, ap, edit);
	return [bar, body];
}

// interfacesLive says whether a part of the Interfaces tab shows what the
// APs report, and so is drawn again every 30 seconds: all but the band cards.
export function interfacesLive(sub, view) {
	return pick(INTERFACES, sub) !== 'radios' || pick(RADIOS, view) !== 'bands';
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
		loopBanner(rows),
		set.length
			? h('div', { class: 'bands' }, set.map((t) => tunnelCard(ctx, d, here, page.node.name, t.id, fields, edit, rows, !!ap)))
			: h('div', { class: 'banner info' }, 'No tunnel is set here. A tunnel set on a folder reaches every AP below it, and a network picks one in its VXLAN transport.'),
		edit && h('div', { class: 'below' },
			h('button', { type: 'button', class: 'button', onclick: () => addTunnel(ctx, d, here, page.node.name, fields, addBox) }, 'Add a tunnel')),
		addBox,
		lines.length
			? h('section', { class: 'panel' },
				h('h2', null, 'Tunnels now', h('span', { class: 'note' }, 'as each AP last reported')),
				h('table', { class: 'list' },
					h('tr', null, ['AP', 'Used by', 'As', 'Tunnel', 'VNI', 'MTU', 'State', 'Probes'].map((c) => h('th', null, c))),
					lines))
			: h('div', { class: 'banner info' }, 'No network or port here travels over VXLAN.'),
	];
}

// usesOf is what the APs here (rows) carry over the tunnel name: each VNI,
// with every network and tunnel port that carries it: a network and a port
// may share one.
function usesOf(rows, name) {
	const out = new Map();
	const add = (vni, by) => out.set(vni, (out.get(vni) || new Set()).add(by));
	for (const { cfg } of rows) {
		const doc = cfg?.document || {};
		for (const [id, n] of Object.entries(doc.network || {})) {
			if (n.enabled === false) continue;
			for (const slot of ['primary', 'fallback']) {
				const t = n.transport?.[slot];
				if (t?.type === 'vxlan' && t.concentrator === name) add(t.vni, n.ssid || id);
			}
		}
		for (const [p, set] of Object.entries(doc.ports || {}))
			if (set.mode === 'tunnel')
				for (const [vlan, m] of Object.entries(set.vxlan || {}))
					if (m.tunnel === name) add(m.vni, `${p} ${onWire(vlan)}`);
	}
	return [...out].sort((a, b) => a[0] - b[0]).map(([vni, by]) => ({ vni, by: [...by].join(' + ') }));
}

// tunnelCard shows one tunnel by name: drawn as a path, from where it
// starts, with its port and MTU, through the VNIs the APs here carry over
// it, to its far end; then its other fields, and where each comes from.
// Someone who may change this node can edit it, or have it follow the
// folder above again; one set only here is deleted that way.
function tunnelCard(ctx, d, here, nodeName, name, fields, edit, rows = [], isAP = false) {
	const paths = TUNNEL.map((k) => `concentrators.${name}.${k}`).filter((p) => fields[p]);
	const own = paths.filter((p) => fields[p].origin === 'self');
	const onlyHere = own.length === paths.length;
	const body = h('div', null);
	const box = h('div', { class: 'edit' });
	const names = (id) => ctx.name('locations', id);
	const mtuPath = `concentrators.${name}.mtu`;
	const ivPath = `concentrators.${name}.probe_interval`;
	const v = (k) => fields[`concentrators.${name}.${k}`]?.value;
	const uses = usesOf(rows, name);
	// Where the fields the path shows come from: one origin for all, or each
	// field's, where they differ or one set here may follow the folder above
	// again by itself, as the other fields' rows may.
	const drawn = ['address', 'port', 'underlay_vlan'].map((k) => `concentrators.${name}.${k}`).filter((p) => fields[p]);
	const whence = new Map(drawn.map((p) => [`${fields[p].origin} ${fields[p].from}`, p]));
	const alone = (p) => edit && fields[p].origin === 'self' && !onlyHere;
	const pathBox = h('div', { class: 'edit' });
	const close = () => body.replaceChildren(h('div', null,
		h('div', { class: 'pathbox' },
			vxlanPath({ start: v('underlay_vlan'), tunnel: name, port: v('port'), mtu: v('mtu') ?? defaultMTU(v('address')), vnis: uses, address: v('address') },
				probed(rows, uses.map((u) => u.vni), isAP)),
			!uses.length && h('div', { class: 'sub' }, 'No network or port here travels over it yet.'),
			drawn.length > 0 && h('div', { class: 'origins' }, whence.size === 1 && !drawn.some(alone)
				? origin('locations', here, fields[drawn[0]], names)
				: drawn.map((p) => h('span', { class: 'origin' },
					h('span', { class: 'sub' }, group(p).label), origin('locations', here, fields[p], names),
					alone(p) && followButton(ctx, 'locations', here, edit.nodeName, edit.parentName, [p], pathBox)))),
			pathBox),
		paths.filter((p) => !drawn.includes(p)).map((path) => {
			const r = fields[path];
			const rowBox = h('div', { class: 'edit' });
			return [h('div', { class: 'row' },
				h('div', { class: 'label' }, group(path).label),
				h('div', { class: 'value' }, value(path, r.value)),
				origin('locations', here, r, names),
				edit && r.origin === 'self' && !onlyHere && followButton(ctx, 'locations', here, edit.nodeName, edit.parentName, [path], rowBox),
				edit && (path === mtuPath || path === ivPath) && r.origin === 'self' && onlyHere && followButton(ctx, 'locations', here, edit.nodeName, edit.parentName, [path], rowBox,
					'Use the default', `Tunnel ${name} on ${nodeName} goes back to the default ${path === mtuPath ? 'MTU' : 'probe interval'}`)),
			rowBox];
		}),
		!fields[mtuPath] && h('div', { class: 'row' },
			h('div', { class: 'label' }, 'MTU'),
			h('div', { class: 'value' }, `default, ${defaultMTU(fields[`concentrators.${name}.address`]?.value)}`)),
		!fields[ivPath] && h('div', { class: 'row' },
			h('div', { class: 'label' }, 'Probe interval'),
			h('div', { class: 'value' }, 'default, 30 s')),
	));
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
	startChoice(prefix + 'underlay_vlan', fields, inputs, rows);
	const out = h('div', { class: 'edit flush' });
	const msg = h('div', { class: 'error' });
	const review = async () => {
		msg.replaceChildren();
		let values;
		try {
			values = changedValues(inputs, rows);
			// A new tunnel is set whole, but for what is left to its default.
			if (fresh)
				for (const k of TUNNEL) {
					if (DEFAULTED.has(k)) continue;
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
		], [h('div', { class: 'sub warn' }, probeOnly(paths) ? PROBE_ONLY : TUNNEL_RELOAD)]);
	};
	return h('div', { class: 'fieldform', 'data-editing': true },
		body,
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review changes'),
			h('button', { type: 'button', class: 'button', onclick: close }, 'Cancel')),
		out);
}

// startChoice makes where a tunnel starts on the AP a choice (0063): the
// management VLAN, the default, which is 0; or a VLAN of the uplink, by its
// number. It stands in for the plain number box of the field at path.
function startChoice(path, fields, inputs, rows) {
	const r = rows.get(path);
	if (!r || !inputs.has(path)) return;   // locked above: the box stays, shut
	const cur = fields[path]?.value || 0;
	const box = r.it.el;
	const radios = 'start-' + path;
	const mgmt = h('input', { type: 'radio', name: radios, checked: !cur });
	const onVLAN = h('input', { type: 'radio', name: radios, checked: !!cur });
	box.value = cur ? String(cur) : '';
	box.placeholder = '1 to 4094';
	box.disabled = !cur;
	mgmt.addEventListener('change', () => { box.disabled = true; });
	onVLAN.addEventListener('change', () => { box.disabled = false; box.focus(); });
	const row = h('div', { class: 'field start' },
		h('span', { class: 'label' }, 'Starts from'),
		h('label', { class: 'choice' }, mgmt, ' the management VLAN'),
		h('label', { class: 'choice' }, onVLAN, ' VLAN ', box));
	r.row.replaceWith(row);
	rows.set(path, { it: r.it, row });
	inputs.set(path, {
		el: box,
		changed: () => (mgmt.checked ? cur !== 0 : true),
		read: () => {
			if (mgmt.checked) return 0;
			const v = Number(box.value);
			if (!Number.isInteger(v) || v < 1 || v > 4094) throw new Error('give a VLAN from 1 to 4094, or pick the management VLAN');
			return v === cur ? undefined : v;
		},
	});
}

// tunnelRows joins what an AP's config asks for, by its networks and its
// tunnel ports (0058), with what it reported, and what its prober found.
function tunnelRows(ap, cfg) {
	if (!cfg) return [];
	const doc = cfg.document || {};
	const report = cfg.condition?.state?.report;
	const at = cfg.condition?.state?.at;
	const reported = new Map((report?.vxlan?.tunnels || []).map((t) => [t.vni, t]));
	const want = [];
	for (const [id, n] of Object.entries(doc.network || {}).sort()) {
		if (n.enabled === false) continue;
		for (const slot of ['primary', 'fallback']) {
			const t = n.transport?.[slot];
			if (t?.type === 'vxlan') want.push({ by: n.ssid || id, as: slot, t, conc: doc.concentrators?.[t.concentrator] });
		}
	}
	for (const [p, set] of Object.entries(doc.ports || {}).sort()) {
		if (set.mode !== 'tunnel') continue;
		for (const [vlan, m] of Object.entries(set.vxlan || {}))
			want.push({ by: `${p}, ${onWire(vlan)}`, as: 'port', t: { vni: m.vni, concentrator: m.tunnel }, conc: doc.concentrators?.[m.tunnel] });
	}
	const apLink = link(`/aps/${encodeURIComponent(ap.id)}`, ap.name);
	const lines = want.map((w, i) => h('tr', null,
		h('td', null, i === 0 && apLink),
		h('td', null, w.by),
		h('td', null, w.as),
		h('td', null, w.t.concentrator, w.conc && h('span', { class: 'sub' }, ` ${w.conc.address}:${w.conc.port}`)),
		h('td', { class: 'mono' }, String(w.t.vni)),
		h('td', null, w.conc ? String(w.conc.mtu) : '—'),
		h('td', null, tunnelState(report, reported.get(w.t.vni))),
		h('td', null, probeState(report, reported.get(w.t.vni), at))));
	// What the AP runs that nothing asks for any more, until it applies.
	const asked = new Set(want.map((w) => w.t.vni));
	for (const t of reported.values())
		if (!asked.has(t.vni))
			lines.push(h('tr', null,
				h('td', null, lines.length === 0 && apLink),
				h('td', { class: 'sub', colspan: 3 }, `to ${t.peer}:${t.port}; nothing asks for it`),
				h('td', { class: 'mono' }, String(t.vni)),
				h('td', null, String(t.mtu || '—')),
				h('td', null, t.up ? h('span', { class: 'chip ok' }, 'up') : h('span', { class: 'chip idle' }, 'down')),
				h('td', null, probeState(report, t, at))));
	return lines;
}

// probeState says what the AP's prober found for a tunnel (0059): whether
// traffic gets across, and when it does not, which half is broken.
function probeState(report, t, at) {
	if (!report?.vxlan || !t) return null;
	if (report.vxlan.prober === false) return h('span', { class: 'chip warn' }, 'not probed: the prober needs ucode-mod-socket');
	const p = t.probe;
	if (!p) return h('span', { class: 'sub' }, t.standby ? 'not probed while standing by' : 'no results; its agent may be older than this');
	const under = p.underlay === true ? `${t.peer} answers pings${p.underlay_ms != null ? ` in ${p.underlay_ms} ms` : ''}`
		: p.underlay === false ? `${t.peer} does not answer pings` : `${t.peer}`;
	const last = p.from ? `${p.from} answered${p.rtt_ms != null ? ` in ${p.rtt_ms} ms` : ''}${p.answered_ago != null ? `, ${secondsAgo(p.answered_ago, at)}` : ''}` : '';
	// The address the AP leased on the segment, which its probes come from (0060).
	const lease = p.lease ? `leased ${p.lease.address}${p.lease.server ? ` from ${p.lease.server}` : ''}${p.lease.router ? `, gateway ${p.lease.router}` : ''}, ${leaseLeft(p.lease.expires_in, at)}` : '';
	const show = (label, cls, ...notes) => [h('span', { class: 'chip ' + cls }, label), notes.filter(Boolean).map((n) => h('div', { class: 'sub' }, n))];
	switch (p.verdict) {
	case 'up':
		return show('answering', 'ok', last, lease);
	case 'down':
		if (p.underlay === false) return show('down', 'bad', `cannot reach ${t.peer}`);
		return show('down', 'bad', p.from ? `stopped answering: ${last}` : `${under}, but nothing on VNI ${t.vni} answers: is the VNI mapped there?`, lease);
	case 'unverified':
		return show('unverified', 'warn', `${under}, but nothing asked on VNI ${t.vni} (${(p.asks || []).join(', ')}) has answered`,
			(p.asks || []).includes('ff02::1')
				? 'No DHCP server answered on the segment either: set a probe address, normally its gateway.'
				: 'Is the probe address on the segment, and does it answer ARP?', lease);
	case 'unknown':
		return show('starting', 'idle', `probing every ${p.interval} s`);
	}
	return show('not running', 'idle');
}

// leaseLeft says how long a lease has left now, given what it had left when
// the AP reported at.
function leaseLeft(s, at) {
	const left = Math.max(0, s - Math.round((Date.now() - new Date(at ?? Date.now()).getTime()) / 1000));
	if (left === 0) return 'ran out';
	if (left < 60) return left + ' s left';
	if (left < 3600) return Math.round(left / 60) + ' min left';
	if (left < 86400) return Math.round(left / 3600) + ' h left';
	return Math.round(left / 86400) + ' d left';
}

// loopBanner names the tunnel ports the loop guard took off their tunnels on
// the APs here (0059), and what puts one back.
function loopBanner(rows) {
	const loops = rows.flatMap(({ ap, cfg }) => (cfg?.condition?.state?.report?.vxlan?.loops || []).map((l) => ({ ap, l, at: cfg.condition.state.at })));
	if (!loops.length) return null;
	return h('div', { class: 'banner problems' },
		h('strong', null, 'The loop guard took these ports off their tunnels'),
		h('ul', null, loops.map(({ ap, l, at }) => h('li', null,
			`${ap.name}: ${l.port}, ${secondsAgo(l.ago, at)}. A frame it sent on ${l.device} came back in on ${l.came_in || 'the AP'}, so VNI ${l.vni ?? '?'} loops. `,
			'Find the second path to that segment; changing the port\'s settings, or restarting the agent, puts the port back.'))));
}

// tunnelState says what the AP reported for a tunnel its config asks for.
function tunnelState(report, t) {
	if (!report) return h('span', { class: 'sub' }, 'no report yet');
	if (!report.vxlan) return h('span', { class: 'sub' }, 'its agent does not report tunnels; update it');
	if (report.vxlan.installed === false) return h('span', { class: 'chip warn' }, 'vxlan is not installed (apk add vxlan)');
	if (report.vxlan.loaded === false) return h('span', { class: 'chip warn' }, 'vxlan is installed, but netifd has not loaded it: restart the network');
	if (!t) return h('span', { class: 'sub' }, 'not on the AP yet');
	// Where it starts, when that is a VLAN (0063): the AP's address there,
	// or that DHCP has not given one, which keeps the tunnel down; and
	// another interface sharing that address, whose restarts drop the
	// tunnel's routes until the prober puts them back.
	const from = t.from_vlan
		? h('div', { class: 'sub' }, t.from_address ? `from VLAN ${t.from_vlan}, ${t.from_address}` : `no address on VLAN ${t.from_vlan}: does DHCP answer there?`,
			t.from_shared?.length ? [' ', h('span', { class: 'chip warn', title: `${t.from_shared.join(', ')} also lease${t.from_shared.length > 1 ? '' : 's'} on VLAN ${t.from_vlan}, sharing the address: a restart of either drops the tunnel's routes until the prober puts them back. Remove ${t.from_shared.length > 1 ? 'them' : 'it'} from the AP.` }, `shared with ${t.from_shared.join(', ')}`)] : null)
		: null;
	if (t.up) return [h('span', { class: 'chip ok' }, 'up'), from];
	if (t.standby) return [h('span', { class: 'chip idle' }, 'standing by'), from];
	return [h('span', { class: 'chip warn' }, 'down'), from];
}

// ethernet is Interfaces › Ethernet (Griff, 2026-10-09; 0092). The bar
// says where the node's ports come from, as on Bands. Below it, a line for
// each kind of AP here, by board: its name, how many there are, and its
// ports; its arrow opens a card for each port: its link, as each AP of the
// kind last reported it; its mode; what it carries, and where that is set
// when not here; and Edit, whose form opens under the cards. On a folder,
// a kind's ports are set as that kind's own (boards.<board>.ports...), so
// one kind's settings never reach another's ports of the same name; on an
// AP's page, as the AP's own. The uplink carries the AP's management, so
// Aeolus leaves it alone: its card has Info, all the AP knows of it, in
// place of Edit. A node that sets no port itself shows them greyed out,
// without Edit, until Customize is pressed.
async function ethernet(ctx, here, page, ap, edit) {
	const [d, rows] = await Promise.all([schema(), ap ? [ap] : configs(page.hardware?.aps || [])]);
	const fields = page.fields || {};
	const kinds = kindsOf(rows, fields, !ap);
	const report = ap?.cfg?.condition?.state;
	const box = h('div', { class: 'edit flush' });
	const panel = h('div', { class: 'ports' });
	const draw = (open) => {
		panel.classList.toggle('inherited', !open);
		panel.replaceChildren(...(kinds.length
			? kinds.map((k) => kindPanel(ctx, d, here, page, k, fields, !!edit && open, edit, !ap, kinds.length === 1))
			: [h('div', { class: 'banner info' }, 'No AP here has reported its ports yet, and no port is set here.')]));
	};
	const bar = inheritsBar(ctx, here, page, { prefix: 'ports.', also: 'boards.', tab: 'interfaces/ethernet', box, onToggle: (open) => {
		// Customizing pauses the page's redraw, as an open form does.
		if (open) panel.setAttribute('data-editing', 'true');
		else panel.removeAttribute('data-editing');
		draw(open);
	} });
	draw(bar.open);
	return [
		bar.bar,
		box,
		ap && h('div', { class: 'sub lead' }, report ? `Reported ${ago(report.at)}.` : 'No report yet.'),
		loopBanner(rows),
		panel,
	];
}

// kindsOf groups the APs here by board (0092), each kind with its name, its
// APs, and its ports: those its APs reported, and those set for it. On an
// AP's page, its one kind is set as the AP's own; elsewhere a kind is set
// under boards.<board>., and APs that never said their board as plain ports.
function kindsOf(rows, fields, folder) {
	const out = new Map();
	const kind = (board, name) => {
		const key = board || '';
		if (!out.has(key)) out.set(key, { board: key, name: name || board || 'APs that said no board', base: folder && key ? `boards.${key}.` : '', aps: [], ports: new Map() });
		return out.get(key);
	};
	for (const r of rows) {
		const t = r.cfg?.template;
		const k = kind(t?.board, t?.model || (t?.id ? t.name : ''));
		k.aps.push(r);
		for (const p of r.cfg?.condition?.state?.report?.ports || []) {
			if (!k.ports.has(p.name)) k.ports.set(p.name, []);
			k.ports.get(p.name).push({ ap: r.ap, cfg: r.cfg, p });
		}
	}
	// Kinds set here or above with no AP here yet, and ports set for a kind
	// that none of its APs reported.
	for (const path of Object.keys(fields)) {
		const m = /^boards\.([^.]+)\.ports\.([^.]+)\./.exec(path);
		if (!m || !folder) continue;
		const k = kind(m[1]);
		if (!k.ports.has(m[2])) k.ports.set(m[2], []);
	}
	if (!folder && out.size === 0) kind('', 'This AP');
	return [...out.values()].sort((a, b) => (a.board === '') - (b.board === '') || a.name.localeCompare(b.name));
}

// kindFields is what applies to a kind's ports here, by the paths its form
// sets: its own settings, or the plain ones where those are set closer, or
// locked above, as the manager folds them for its APs (0092). A plain one
// shown under the kind's path is marked plain: nothing is set at that path
// to unset.
function kindFields(fields, kind, ancestry) {
	if (!kind.base) return fields;
	const at = (id) => ancestry.indexOf(id);
	const out = { ...fields };
	const pick = (plain, own) => {
		if (!own) return plain;
		if (!plain) return own;
		if (plain.origin === 'locked' && plain.from !== own.from) return plain;
		return at(plain.from) > at(own.from) ? plain : own;
	};
	const paths = new Set();
	for (const p of Object.keys(fields)) {
		if (p.startsWith('ports.')) paths.add(p);
		else if (p.startsWith(kind.base + 'ports.')) paths.add(p.slice(kind.base.length));
	}
	for (const p of paths) {
		const r = pick(fields[p], fields[kind.base + p]);
		out[kind.base + p] = r && r === fields[p] ? { ...r, plain: true } : r;
	}
	return out;
}

// byPort orders port names as people count: lan2 before lan10.
function byPort(a, b) {
	return a.localeCompare(b, undefined, { numeric: true });
}

// text is a cell's words, whether a string or an element.
const text = (x) => (typeof x === 'string' ? x : x?.textContent ?? '');

// linkCell says a port's link: on an AP, its own; on a folder, how many of
// the APs that have it are up, and at what speed if they agree, with each
// AP's on hover.
function linkCell(reps, folder) {
	if (!reps.length) return h('span', { class: 'sub' }, 'not reported');
	if (!folder || reps.length === 1) return linkState(reps[0].p);
	const up = reps.filter((r) => r.p.up && r.p.carrier);
	const speeds = [...new Set(up.map((r) => text(linkState(r.p))))];
	return h('span', { title: reps.map((r) => `${r.ap.name}: ${text(linkState(r.p))}`).join('\n') },
		`${up.length} of ${reps.length} up${speeds.length === 1 ? ` · ${speeds[0]}` : ''}`);
}

// carries says what a port carries, by its fields in force under base: an
// access port's VLAN; a trunk's untagged and tagged VLANs; a tunnel port's
// VNIs.
function carries(fields, name, base = '') {
	const v = (k) => fields[`${base}ports.${name}.${k}`]?.value;
	switch (v('mode')) {
	case 'access':
		return `VLAN ${v('untagged')}, untagged`;
	case 'trunk':
		return [v('untagged') ? `untagged ${v('untagged')}` : 'nothing untagged', v('tagged')?.length && `tagged ${v('tagged').join(', ')}`].filter(Boolean).join(' · ');
	case 'tunnel': {
		const vnis = vnisOf(fields, name, base);
		return vnis.length ? vnis.map((m) => `${onWire(m.vlan, true)} → ${carried(m)}`).join('; ') : 'no VNIs yet';
	}
	case 'lacp':
		return 'LACP, not applied yet';
	}
	return null;
}

const MODES = { access: 'Access', trunk: 'Trunk', tunnel: 'Tunnel', lacp: 'LACP' };

// plainOf is a field's plain path, which the labels know: a kind of AP's
// own (boards.<board>.ports...) is shown as its port's (0092).
const plainOf = (path) => path.replace(/^boards\.[^.]+\./, '');

// The kinds whose ports are open, by folder and board, so a redraw keeps
// them open.
const opened = new Set();

// jackIcon is an RJ45 jack as people know it from the front: its body, the
// latch's slot below, and its eight pins (Griff, 2026-10-09; 0093).
function jackIcon() {
	const ns = 'http://www.w3.org/2000/svg';
	const el = (tag, attrs) => {
		const e = document.createElementNS(ns, tag);
		for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
		return e;
	};
	const svg = el('svg', { viewBox: '0 0 44 36', 'aria-hidden': 'true' });
	svg.append(el('polygon', { class: 'body', points: '2,2 42,2 42,26 32,26 32,34 12,34 12,26 2,26' }),
		...[7.5, 11.5, 15.5, 19.5, 23.5, 27.5, 31.5, 35.5].map((x) => el('line', { class: 'pin', x1: x, y1: 6, x2: x, y2: 13 })));
	return h('span', { class: 'icon' }, svg);
}

// jackState is how a port is drawn (0093). On a folder: white where it is
// set, the uplink and a bond's members included; grey where it is not, or
// is off. On an AP: green where it has a link, white where it is set but has
// none, grey where it is not set, or is off.
function jackState(folder, { off, set, carrier }) {
	if (off) return 'off';
	if (!folder && carrier) return 'on';
	return set ? 'set' : 'unset';
}

// jack is one port drawn as a jack, with its name and a word under it; it
// is a button that selects it.
function jack(name, state, under, title, onclick, marks = []) {
	return h('button', { type: 'button', class: `jack ${state}`, title, onclick },
		jackIcon(), h('span', { class: 'jname mono' }, name, marks), under && h('span', { class: 'jsub' }, under));
}

// mbit reads a link's speed ("2500F") in Mbit/s.
const mbit = (speed) => Number(/^(\d+)/.exec(speed || '')?.[1] || 0);
const gbit = (m) => (m >= 1000 ? `${m / 1000} Gbit/s` : `${m} Mbit/s`);

// short writes a speed the short way, 2.5G or 100M; gbe a port's, 10GbE.
const short = (m) => (m >= 1000 ? `${m / 1000}G` : `${m}M`);
const gbe = (m) => (m >= 1000 ? `${m / 1000}GbE` : `${m}M`);

// sync says a link as its port and the speed it synced at, "10GbE
// port/2.5G sync" (Griff, 2026-10-09; 0093), with what the far end offers
// on hover; a port that doesn't say how fast it can go, its speed alone.
function sync(p) {
	if (!p.carrier) return 'no link';
	const now = mbit(p.speed);
	if (!p.max) return gbit(now);
	return h('span', { title: p.partner_max ? `the far end offers up to ${gbit(p.partner_max)}` : '' }, `${gbe(p.max)} port/${short(now)} sync`);
}

// poe says what the switch's LLDP says of the power it gives the AP, or
// null where it says nothing (0064, 0093).
function poe(report) {
	const n = report?.uplink_neighbor;
	const x = n?.power;
	if (!x) return n?.med?.power_w != null ? `${n.med.power_w} W, by LLDP-MED` : null;
	if (!x.pse) return 'the AP powers the switch';
	return [`powered by ${n.system || 'the switch'}${n.port ? ` ${n.port}` : ''}`, x.supported ? (x.enabled ? null : 'power off') : 'not supported',
		x.class != null && `class ${x.class}`, x.allocated_w != null && `${x.allocated_w} W allocated`, x.requested_w != null && `${x.requested_w} W asked`,
		x.pair && `${x.pair} pair`].filter(Boolean).join(' · ');
}

// kindPanel is one kind of AP: its line, with its arrow, and its ports
// drawn as jacks, a bond's members together. A jack selects its port, whose
// details, Edit and Info open under the jacks.
function kindPanel(ctx, d, here, page, kind, fields, canEdit, edit, folder, only) {
	const kf = kindFields(fields, kind, page.ancestry || []);
	const names = [...kind.ports.keys()];
	const uplink = (n) => (kind.ports.get(n) || []).some((r) => r.p.uplink);
	names.sort((a, b) => Number(uplink(b)) - Number(uplink(a)) || byPort(a, b));
	const key = `${here}|${kind.board}`;
	const isOpen = () => opened.has(key) || ((only || !folder) && !opened.has('!' + key));
	const box = h('div', { class: 'edit flush' });
	const strip = h('div', { class: 'jacks' });
	const body = h('div', { class: 'kindbody' }, strip, box);
	const arrow = h('span', { class: 'arrow', 'aria-hidden': 'true' });
	const head = h('button', { type: 'button', class: 'kindhead', 'aria-expanded': 'false' }, arrow,
		h('strong', null, kind.name),
		h('span', { class: 'sub' }, [`${kind.aps.length} AP${kind.aps.length === 1 ? '' : 's'}`, summary(names, uplink, kf, kind.base)].filter(Boolean).join(' · ')),
		h('span', { class: 'gap' }),
		kind.board && folder && h('span', { class: 'mono sub' }, kind.board));
	const set = (open) => {
		body.hidden = !open;
		arrow.textContent = open ? '▾' : '▸';
		head.setAttribute('aria-expanded', String(open));
		if (open) {
			opened.add(key);
			opened.delete('!' + key);
		} else {
			opened.delete(key);
			opened.add('!' + key);
		}
	};
	head.addEventListener('click', () => set(body.hidden));
	let chosen = null;
	const pick = (what, make) => {
		for (const b of strip.querySelectorAll('.jack.chosen, .bond.chosen')) b.classList.remove('chosen');
		if (chosen === what) {
			chosen = null;
			box.replaceChildren();
			return;
		}
		chosen = what;
		box.replaceChildren(make());
	};
	const ctxPort = { ctx, d, here, nodeName: page.node.name, kf, kind, canEdit, edit, folder, box };
	strip.replaceChildren(
		...names.map((n) => portJack(ctxPort, n, kind.ports.get(n) || [], pick)),
		canEdit && h('button', { type: 'button', class: 'jack add', title: `A port no ${folder && kind.board ? kind.name : 'AP'} here has reported yet`,
			onclick: () => addPort(ctx, d, here, page.node.name, kf, box, edit, kind) }, h('span', { class: 'icon plus' }, '+'), h('span', { class: 'jname' }, 'Add a port')));
	set(isOpen());
	return h('section', { class: 'panel kind' }, head, body);
}

// summary is a kind's ports in a few words: the uplink first, then the
// others, and how many of them something sets.
function summary(names, uplink, kf, base) {
	if (!names.length) return 'no ports reported';
	const up = names.filter(uplink);
	const rest = names.filter((n) => !uplink(n));
	const set = rest.filter((n) => carries(kf, n, base) || kf[`${base}ports.${n}.enabled`]?.value === false);
	return [up.length && `${up.join(', ')} uplink`, rest.length && rest.join(', '), set.length && `${set.length} set`].filter(Boolean).join(' · ');
}

// portJack is one port as a jack, or a bond as its members' jacks together,
// with what selecting it shows.
function portJack(c, name, reps, pick) {
	const { ctx, d, here, nodeName, kf, kind, canEdit, edit, folder, box } = c;
	const base = kind.base;
	const f = (k) => kf[`${base}ports.${name}.${k}`];
	const uplinkOn = reps.filter((r) => r.p.uplink);
	const allUplink = uplinkOn.length > 0 && uplinkOn.length === reps.length;
	const off = f('enabled')?.value === false;
	const names = (id) => ctx.name('locations', id);
	const set = Object.entries(kf).filter(([p, r]) => p.startsWith(`${base}ports.${name}.`) && r);
	const from = [...new Set(set.filter(([, r]) => r.from !== here).map(([, r]) => `${r.origin === 'locked' ? 'locked by' : 'from'} ${names(r.from)}`))];
	const mine = set.filter(([, r]) => r.from === here && r.origin === 'self' && !r.plain).map(([p]) => p);
	const mode = allUplink ? 'Uplink' : off ? 'Off' : MODES[f('mode')?.value];
	const report = uplinkOn[0]?.cfg?.condition?.state?.report;
	const bond = reps.find((r) => r.p.bond?.members?.length)?.p.bond;
	// What the port can go, as the kind's APs say: the same for every AP of
	// one kind.
	const capOf = reps.map((r) => r.p.max).find(Boolean);
	const power = allUplink && !folder ? poe(report) : null;
	const what = allUplink
		? [h('span', { class: 'sub' }, `The AP's management${bond ? `, over a bond of ${bond.members.length}` : ''}; Aeolus leaves it alone.`),
			!folder && hasUplinkNews(report) && uplinkCell(report)]
		: carries(kf, name, base);
	const row = (label, v) => v != null && v !== '' && h('div', { class: 'row' }, h('div', { class: 'label' }, label), h('div', { class: 'value' }, v));
	const close = () => box.replaceChildren();
	const editForm = () => h('div', null,
		portForm(ctx, d, here, nodeName, name, kf, close, edit, kind),
		mine.length > 0 && h('div', { class: 'below' }, followButton(ctx, 'locations', here, nodeName, edit?.parentName, mine, box, `Follow ${edit?.parentName ?? 'above'} for ${name}`)));
	const info = () => h('div', null, uplinkOn.map((r) => [folder && h('h3', null, r.ap.name), uplinkInfo(r.cfg.condition.state.report)]));
	// What selecting it shows: the port's details, and Edit or Info.
	const detail = () => {
		const panel = h('section', { class: 'panel portdetail', 'data-editing': true });
		const show = (make) => panel.replaceChildren(head(), make());
		const head = () => h('h2', null, name, bond && h('span', { class: 'note' }, `${bond.mode === '802.3ad' ? 'LACP' : bond.mode} bond`),
			folder && kind.board && h('span', { class: 'note' }, `every ${kind.name} here`),
			h('span', { class: 'controls' },
				allUplink ? h('button', { type: 'button', class: 'button small', onclick: () => show(info) }, 'Info')
					: canEdit && h('button', { type: 'button', class: 'button small', onclick: () => show(editForm) }, 'Edit'),
				h('button', { type: 'button', class: 'button small', onclick: () => pick(name, () => null) }, 'Close')));
		// On a folder, the kind's ports as they are for every AP of it: what
		// each can go, not one AP's link (Griff, 2026-10-09).
		panel.append(...[head(),
			folder ? row('Port', capOf && gbe(capOf)) : row('Link', reps[0]?.p.max ? sync(reps[0].p) : linkCell(reps, folder)),
			bond && row('Members', bond.members.map((m) => h('div', null, h('span', { class: 'mono' }, m.name), ' ', folder ? (m.max ? gbe(m.max) : '') : sync(m),
				!folder && m.aggregator != null && bond.aggregator != null && m.aggregator !== bond.aggregator && h('span', { class: 'chip warn' }, 'outside the aggregate')))),
			row('Mode', mode ?? h('span', { class: 'sub' }, 'not set')),
			row('Carries', [what ?? h('span', { class: 'sub' }, 'not set'), from.length > 0 && h('span', { class: 'whence' }, from.join(', '))]),
			row('Power', power)].filter(Boolean));
		return panel;
	};
	const marks = [uplinkOn.length > 0 && h('span', { class: 'up', title: allUplink ? 'the uplink' : `the uplink on ${uplinkOn.map((r) => r.ap.name).join(', ')}` }, '↑'),
		power && h('span', { class: 'poe', title: power }, '⚡'),
		reps.some((r) => r.cfg?.condition?.state?.report?.vxlan?.loops?.some((l) => l.port === name)) && h('span', { class: 'loop', title: 'off its tunnels: a loop' }, '!')];
	const choose = (e) => {
		pick(name, detail);
		e.currentTarget.classList.toggle('chosen', box.childElementCount > 0);
	};
	const configured = allUplink || Boolean(mode && mode !== 'Off');
	const lacp = bond && (bond.mode === '802.3ad' ? 'LACP' : bond.mode);
	if (bond) {
		// A bond: its members' jacks, together, under its name; on a folder,
		// each member by what it can go, on an AP by its link.
		const group = h('button', { type: 'button', class: 'bond', title: `${name}: ${bond.members.map((m) => m.name).join(' + ')}`, onclick: choose },
			h('span', { class: 'bondname mono' }, name, marks, h('span', { class: 'sub' }, ` ${lacp}${!folder && reps[0]?.p.carrier ? ` · ${gbit(mbit(reps[0].p.speed))}` : ''}`)),
			h('span', { class: 'members' }, bond.members.map((m) => h('span', { class: `jack ${jackState(folder, { off, set: true, carrier: m.carrier })}` },
				jackIcon(), h('span', { class: 'jname mono' }, m.name),
				h('span', { class: 'jsub' }, folder ? (m.max ? gbe(m.max) : '') : m.carrier ? gbit(mbit(m.speed)) : 'no link')))));
		return group;
	}
	const p0 = reps[0]?.p;
	const under = folder ? [capOf && gbe(capOf), mode].filter(Boolean).join(' · ') : p0 ? (p0.carrier ? gbit(mbit(p0.speed)) : p0.up ? 'no link' : 'down') : '';
	return jack(name, jackState(folder, { off, set: configured, carrier: !!p0?.carrier }), under,
		[name, mode, what && text(what)].filter(Boolean).join(' · '), choose, marks);
}

// vniPath is where a tunnel port sets one of a VNI's fields (0058, 0059).
const vniPath = (name, vlan, k, base = '') => `${base}ports.${name}.vxlan.${vlan}.${k}`;
const VNI_FIELDS = ['tunnel', 'vni', 'probe'];

// vnisOf reads a tunnel port's VNIs from the values in force (by path, each
// {value, from, origin}) as [{vlan, tunnel, vni}], untagged first, then by
// VLAN.
function vnisOf(fields, name, base = '') {
	const prefix = `${base}ports.${name}.vxlan.`;
	const out = new Map();
	for (const [path, r] of Object.entries(fields || {})) {
		if (!path.startsWith(prefix)) continue;
		const [vlan, k] = path.slice(prefix.length).split('.');
		if (!VNI_FIELDS.includes(k)) continue;
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

// carried says where a VNI goes, and what the prober asks there: "arista ·
// VNI 50 · asks 192.168.50.1".
function carried(m) {
	return `${m.tunnel?.value ?? '(no tunnel)'} · VNI ${m.vni?.value ?? '(none)'}${m.probe?.value ? ` · asks ${m.probe.value}` : ''}`;
}

// ipv4 says whether text is an IPv4 address, as a probe address must be.
function ipv4(text) {
	const parts = text.split('.');
	return parts.length === 4 && parts.every((p) => /^[0-9]{1,3}$/.test(p) && Number(p) <= 255);
}

// portForm edits one port's settings on this node, in one change. Only what
// the mode uses is shown: an access port's one VLAN, a trunk's untagged VLAN
// and tagged ones, or a tunnel port's VNIs (0058).
function portForm(ctx, d, here, nodeName, name, fields, close, edit, kind = { base: '' }) {
	const base = kind.base;
	const prefix = `${base}ports.${name}.`;
	const { body, inputs, rows } = fieldsForm(d, [[null, FIELDS.map((k) => prefix + k)]], fields, here);
	const mode = rows.get(prefix + 'mode')?.it.el;
	// LACP is in the schema, but not applied yet (0053).
	if (mode && mode.value !== 'lacp') mode.querySelector('option[value="lacp"]')?.remove();
	const out = h('div', { class: 'edit flush' });
	const vnis = vniTable(ctx, here, nodeName, name, fields, edit, out, base);
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
		let unsets = [];
		try {
			values = changedValues(inputs, rows);
			if (!vnis.el.hidden) {
				const r = vnis.read();
				Object.assign(values, r.values);
				unsets = r.unsets;
			}
		} catch (e) {
			msg.replaceChildren(e.message);
			return;
		}
		// Clearing a probe address unsets it, a change of its own (0059).
		if (unsets.length) {
			if (Object.keys(values).length) {
				msg.replaceChildren('Clearing a probe address is a change of its own: apply the other changes first, or put the address back.');
				return;
			}
			const op = unsets.length === 1
				? { kind: 'unset', tree: 'locations', node: here, path: unsets[0] }
				: { kind: 'unset', tree: 'locations', node: here, paths: unsets };
			const p = await ask(out, op);
			if (!p) return;
			confirm(ctx, out, op, p, [
				h('div', null, h('strong', null, `Port ${name} on ${nodeName}`)),
				h('ul', { class: 'becomes' }, unsets.map((path) => h('li', null, `${group(plainOf(path)).label}: ${fields[path].value} → `,
					p.resolved?.[path] ? `${p.resolved[path].value} (from ${ctx.name('locations', p.resolved[path].from)})` : 'none: the AP asks any IPv6 host there'))),
			], [h('div', { class: 'sub warn' }, PROBE_ONLY)]);
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
			h('div', null, h('strong', null, `Port ${name} on ${nodeName}${base ? `, every ${kind.name}` : ''}`)),
			h('ul', { class: 'becomes' }, paths.map((path) => h('li', null,
				`${group(plainOf(path)).label}: `,
				fields[path] ? [value(plainOf(path), fields[path].value), ' → '] : '', value(plainOf(path), values[path])))),
			h('div', { class: 'sub' }, base
				? `Every ${kind.name} here uses it for its ${name}, and no other kind of AP; folders below and APs that set their own keep theirs.`
				: `Every AP here with a port named ${name} uses it; APs below that set their own keep theirs.`),
		], [h('div', { class: 'sub warn' }, probeOnly(paths) ? PROBE_ONLY : RELOAD)]);
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
// can take another tunnel, VNI or probe address (0059), and one more row can
// be added, both with the rest of the port's change; a row set here can be
// removed, which is a change of its own, previewed in out. read() returns
// {values, unsets}: {path: value} to set, and the probe addresses cleared,
// to unset; or throws with what is wrong. count() says how many VNIs the
// port would carry.
function vniTable(ctx, here, nodeName, name, fields, edit, out, base = '') {
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
	const address = (cur) => h('input', { type: 'text', maxlength: 15, value: cur ?? '', placeholder: 'probe address' });
	const have = vnisOf(fields, name, base);
	const lines = have.map((m) => {
		const tunnel = picker(m.tunnel?.value);
		const vni = number(m.vni?.value);
		const probe = address(m.probe?.value);
		tunnel.disabled = vni.disabled = probe.disabled = [m.tunnel, m.vni, m.probe].some((r) => r?.origin === 'locked' && r.from !== here);
		const own = VNI_FIELDS.map((k) => vniPath(name, m.vlan, k, base)).filter((p) => fields[p]?.origin === 'self' && fields[p]?.from === here && !fields[p]?.plain);
		const heading = `Port ${name} on ${nodeName}: ${onWire(m.vlan)} leaves VNI ${m.vni?.value}`;
		return {
			m, tunnel, vni, probe,
			row: h('div', { class: 'field' },
				h('span', { class: 'label' }, onWire(m.vlan, true)),
				tunnel, vni, probe,
				origin('locations', here, m.tunnel ?? m.vni, names),
				own.length > 0 && followButton(ctx, 'locations', here, nodeName, edit?.parentName, own, out, 'Remove', heading)),
		};
	});
	const vlanIn = h('input', { type: 'text', maxlength: 8, placeholder: 'untagged, or a VLAN' });
	const newTunnel = picker(undefined);
	const newVni = number(undefined);
	const newProbe = address(undefined);
	const probeIn = (el, vlan) => {
		const v = el.value.trim();
		if (v && !ipv4(v)) throw new Error(`${onWire(vlan, true)}: a probe address is an IPv4 address on the segment, such as its gateway`);
		return v || undefined;
	};
	const read = () => {
		const values = {};
		const unsets = [];
		for (const l of lines) {
			if (l.tunnel.disabled) continue;
			const pr = probeIn(l.probe, l.m.vlan);
			if (pr !== undefined && pr !== l.m.probe?.value) values[vniPath(name, l.m.vlan, 'probe', base)] = pr;
			if (pr === undefined && l.m.probe) {
				if (l.m.probe.origin !== 'self') throw new Error(`${onWire(l.m.vlan, true)}: its probe address is set above; set another here, or leave it`);
				unsets.push(vniPath(name, l.m.vlan, 'probe', base));
			}
			const t = l.tunnel.value || undefined;
			const v = vniIn(l.vni, l.m.vlan);
			if (t !== l.m.tunnel?.value) {
				if (!t) throw new Error(`${onWire(l.m.vlan, true)}: pick its tunnel, or remove the row`);
				values[vniPath(name, l.m.vlan, 'tunnel', base)] = t;
			}
			if (v !== l.m.vni?.value) {
				if (v === undefined) throw new Error(`${onWire(l.m.vlan, true)}: set its VNI, or remove the row`);
				values[vniPath(name, l.m.vlan, 'vni', base)] = v;
			}
		}
		const vlan = vlanIn.value.trim().toLowerCase();
		if (!vlan && !newTunnel.value && newVni.value === '' && !newProbe.value.trim()) return { values, unsets };
		if (!/^(untagged|[1-9][0-9]{0,3})$/.test(vlan) || (vlan !== 'untagged' && Number(vlan) > 4094))
			throw new Error('The new VNI is carried untagged, or on a VLAN from 1 to 4094.');
		if (have.some((m) => m.vlan === vlan)) throw new Error(`${onWire(vlan, true)} carries a VNI already; change that row instead.`);
		const v = vniIn(newVni, vlan);
		if (!newTunnel.value) throw new Error(`${onWire(vlan, true)}: pick its tunnel.`);
		if (v === undefined) throw new Error(`${onWire(vlan, true)}: set its VNI.`);
		values[vniPath(name, vlan, 'tunnel', base)] = newTunnel.value;
		values[vniPath(name, vlan, 'vni', base)] = v;
		const pr = probeIn(newProbe, vlan);
		if (pr !== undefined) values[vniPath(name, vlan, 'probe', base)] = pr;
		return { values, unsets };
	};
	const el = h('div', { class: 'vnis' },
		h('h3', null, 'VNIs'),
		h('div', { class: 'sub' }, 'Each VNI is carried untagged, or tagged with a VLAN of its own, over a tunnel set here. The port leaves the uplink\'s bridge. A probe address, normally the segment\'s gateway, is what the AP asks to check the VNI works; without one it asks any IPv6 host there.'),
		h('div', { class: 'fields' },
			lines.map((l) => l.row),
			h('div', { class: 'field' }, h('span', { class: 'label' }, have.length ? 'Add another' : 'Add one'), vlanIn, newTunnel, newVni, newProbe)));
	return { el, read, count: () => have.length + (vlanIn.value.trim() ? 1 : 0) };
}

// addPort sets up a port no AP here has reported yet, by its name, such as
// one on APs that are still to be adopted.
function addPort(ctx, d, here, nodeName, fields, box, edit, kind = { base: '' }) {
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
		slot.replaceChildren(portForm(ctx, d, here, nodeName, name, fields, () => box.replaceChildren(), edit, kind));
	};
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, kind.base ? `Add a port to every ${kind.name} here` : 'Add a port'),
		h('div', { class: 'fieldform' },
			h('label', { class: 'field' }, h('span', { class: 'label' }, 'Port name'), nameIn),
			msg,
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button', onclick: next }, 'Next'),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel')),
			slot)));
}

// hasUplinkNews says whether an AP knows anything of its uplink's switch
// port or VLANs to show beside it.
const hasUplinkNews = (report) => Boolean(report?.uplink_neighbor || report?.uplink_vlans?.length);

// uplinkCell says what reaches the AP on its uplink (0064): the switch port
// it is on, by LLDP, and each VLAN it carries for Aeolus, with whether that
// VLAN comes in from the switch.
function uplinkCell(report) {
	const n = report.uplink_neighbor;
	const vlans = report.uplink_vlans || [];
	return [
		n && h('div', { class: 'sub' }, `on ${switchPort(n)}${n.vlans?.length ? `, which carries VLANs ${n.vlans.join(' ')}` : ''}`),
		vlans.map((v) => {
			const [cls, word, says] = uplinkJudgment(v, n);
			return [h('span', { class: 'chip ' + cls, title: says || '' }, `VLAN ${v.vlan} ${word}`), ' '];
		}),
		!n && !vlans.length && h('span', { class: 'sub' }, '—'),
	];
}

// uplinkInfo shows all the AP knows of its uplink (0064), from its last
// report: the port itself, its traffic and errors; the VLANs it carries, and
// for each, whether it reaches the AP, or whether the switch carries it; the
// AP's addresses on them; and all the switch's LLDP says of itself and of
// the port.
function uplinkInfo(r) {
	const p = (r.ports || []).find((x) => x.uplink) || {};
	const link = linkState(p);
	const linkText = typeof link === 'string' ? link : link.textContent;
	const u = r.uplink_port;
	const n = r.uplink_neighbor;
	const row = (label, v) => v != null && v !== '' && v !== false && v !== 0 && (!Array.isArray(v) || v.length > 0) &&
		h('div', { class: 'row' }, h('div', { class: 'label' }, label), h('div', { class: 'value' }, v));
	const dot = (xs) => xs.filter(Boolean).join(' · ');
	const watched = new Map((r.uplink_vlans || []).map((v) => [v.vlan, v]));
	const names = n?.vlan_names || {};
	// Each VLAN on the uplink, and what is known of it: whether it reaches
	// the AP, if watched; else whether the switch carries it, by LLDP.
	const carried = (u?.vlans || []).map((v) => {
		const w = watched.get(v.vlan);
		let chip = null;
		if (w) {
			const [cls, word, says] = uplinkJudgment(w, n);
			chip = h('span', { class: 'chip ' + cls, title: says || '' }, word);
		} else if (v.tagged && n?.vlans?.length && !n.vlans.includes(v.vlan)) {
			chip = h('span', { class: 'chip bad' }, 'not on the switch port');
		} else if (!v.tagged && n?.native_vlan && n.native_vlan !== v.vlan) {
			chip = h('span', { class: 'chip warn' }, `the switch's native VLAN is ${n.native_vlan}`);
		}
		return h('div', null, `VLAN ${v.vlan} ${v.tagged ? 'tagged' : 'untagged'}${v.vlan === u.management_vlan ? ', management' : ''} `, chip);
	});
	// The AP's own addresses there: where tunnels start (0063), and the
	// leases of the VLAN transports' probes (0061).
	const addresses = [];
	for (const t of r.vxlan?.tunnels || [])
		if (t.from_vlan && t.from_address && !addresses.some((a) => a.startsWith(`VLAN ${t.from_vlan}: ${t.from_address}`)))
			addresses.push(`VLAN ${t.from_vlan}: ${t.from_address}, where tunnels start`);
	for (const v of r.vlan_probes || [])
		if (v.probe?.lease)
			addresses.push(`VLAN ${v.vlan}: ${v.probe.lease.address}, the probe's lease${v.probe.lease.router ? `, gateway ${v.probe.lease.router}` : ''}`);
	const power = (x) => dot([x.pse ? 'the switch powers it' : 'it powers the switch', x.supported ? (x.enabled ? 'on' : 'off') : 'not supported',
		x.class != null && `class ${x.class}`, x.pair && `${x.pair} pair`, x.allocated_w != null && `${x.allocated_w} W allocated`, x.requested_w != null && `${x.requested_w} W asked`]);
	return h('div', { class: 'info' },
		h('h3', null, 'This AP'),
		row('Port', dot([u?.name || p.name, linkText, u?.mtu && `MTU ${u.mtu}`, u?.mac])),
		row('Link changes', u && `${u.carrier_changes} since it came up`),
		row('Traffic', u && dot([`in ${amount(u.rx_bytes)}B, ${amount(u.rx_packets)} packets`, `out ${amount(u.tx_bytes)}B, ${amount(u.tx_packets)} packets`].map((x) => x.replace('  ', ' ')))),
		row('Errors', u && dot([`in ${u.rx_errors} errors, ${u.rx_dropped} dropped`, `out ${u.tx_errors} errors, ${u.tx_dropped} dropped`])),
		row('VLANs', carried),
		row('Its addresses', addresses.map((a) => h('div', null, a))),
		!u && h('div', { class: 'sub' }, 'Its agent reports no more of the port; it may be older than this.'),
		h('h3', null, 'The switch', n && h('span', { class: 'note' }, `by LLDP, ${n.ago} s ago${n.ttl != null ? `; it holds for ${n.ttl} s` : ''}`)),
		n ? [
			row('Name', n.system),
			row('Description', n.system_description),
			row('Port', dot([n.port && `${n.port}${n.port_kind ? ` (${n.port_kind})` : ''}`, n.port_description && `"${n.port_description}"`])),
			row('Chassis', n.chassis && `${n.chassis}${n.chassis_kind ? ` (${n.chassis_kind})` : ''}`),
			row('Management', (n.management || []).map((m) => `${m.address}${m.interface ? `, ${m.interface_kind} ${m.interface}` : ''}`).join(' · ')),
			row('Capabilities', n.capabilities && `${n.capabilities.join(', ') || 'none'}; enabled: ${(n.enabled_capabilities || []).join(', ') || 'none'}`),
			row('Native VLAN', n.native_vlan),
			row('VLANs', (n.vlans || []).map((v) => `${v}${names[v] ? ` (${names[v]})` : ''}`).join(' · ')),
			row('Protocol VLANs', (n.protocol_vlans || []).join(' · ')),
			row('Protocols', (n.protocols || []).join(' · ')),
			row('Max frame', n.max_frame && `${n.max_frame} bytes`),
			row('Aggregation', n.aggregation && (n.aggregation.enabled ? `in an aggregate, port ${n.aggregation.port}` : n.aggregation.capable ? 'capable, not in use' : 'not capable')),
			row('MAC/PHY', n.mac_phy && dot([n.mac_phy.mau_name || `MAU type ${n.mac_phy.mau}`,
				`autonegotiation ${n.mac_phy.autoneg_enabled ? 'on' : n.mac_phy.autoneg_supported ? 'off' : 'not supported'}`, `advertises ${n.mac_phy.advertised}`])),
			row('Power', n.power && power(n.power)),
			n.med && [
				row('LLDP-MED', dot([n.med.class && `class ${n.med.class}`, n.med.capabilities && `capabilities ${n.med.capabilities}`, n.med.power_w != null && `${n.med.power_w} W`])),
				row('Policies', (n.med.policies || []).map((x) => h('div', null,
					dot([x.application, x.unknown ? 'unknown' : `VLAN ${x.vlan}${x.tagged ? ' tagged' : ''}`, `priority ${x.priority}`, `DSCP ${x.dscp}`])))),
				row('Inventory', Object.entries(n.med.inventory || {}).map(([k, v]) => `${k} ${v}`).join(' · ')),
				row('Location', n.med.location),
			],
			row('Other TLVs', (n.other || []).map((o) => h('div', null, `type ${o.type}${o.oui ? `, ${o.oui} subtype ${o.subtype}` : ''}: ${o.data}`))),
		] : h('div', { class: 'sub' }, 'It sends no LLDP, or the AP\'s agent is older than this.'));
}

// amount writes a count the short way: 1.2 k, 3.4 M, 5.6 G.
function amount(x) {
	for (const [d, unit] of [[1e12, ' T'], [1e9, ' G'], [1e6, ' M'], [1e3, ' k']])
		if (x >= d) return `${(x / d).toFixed(1)}${unit}`;
	return `${x} `;
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
