// A tunnel's path, drawn: where its traffic starts on the AP, the VXLAN
// tunnel, the VNI, and the far end, in the order the traffic goes. Each leg
// is coloured by how the APs' probers find it (0059): green where it works,
// amber where it is unproven, red where it is broken, grey where nothing
// has said yet. A VLAN transport is one hop: the VLAN on the uplink. The
// path runs across where there is room, else down, like a rail.

import { h } from '../dom.js';

const RANK = { bad: 3, warn: 2, ok: 1 };
const VERDICT = { up: 'ok', down: 'bad', unverified: 'warn' };

// worst is the worst of the states the APs reported for a leg, or idle
// where none did.
function worst(states) {
	return states.filter(Boolean).sort((a, b) => (RANK[b] || 0) - (RANK[a] || 0))[0] || 'idle';
}

// probed is how the APs (rows, each with its config and condition) find
// the tunnels carrying vnis, leg by leg, the worst of them: whether each
// has the address it starts from (start), whether traffic gets across it
// (vni), and whether the far end answers pings (far).
export function probed(rows, vnis) {
	const start = [], mid = [], far = [];
	for (const { cfg } of rows)
		for (const t of cfg?.condition?.state?.report?.vxlan?.tunnels || []) {
			if (!vnis.includes(t.vni)) continue;
			start.push(t.from_vlan && !t.from_address ? 'bad' : 'ok');
			if (!t.standby) mid.push(VERDICT[t.probe?.verdict] || null);
			far.push(t.probe?.underlay === true ? 'ok' : t.probe?.underlay === false ? 'bad' : null);
		}
	return { start: worst(start), vni: worst(mid), far: worst(far) };
}

// from says where a tunnel starts on the AP (0063): its management VLAN,
// untagged on most uplinks, or another VLAN of the uplink.
function from(vlan) {
	return vlan ? `VLAN ${vlan}` : 'mgmt VLAN';
}

// pathOf draws hops ([{ top, main, sub, state, title }]) joined by legs,
// each leg, and the hop it leads to, coloured by that hop's state. label
// says it in words, for a screen reader.
export function pathOf(hops, label) {
	return h('div', { class: 'path', role: 'img', 'aria-label': label },
		hops.map((p, i) => [
			i > 0 && h('span', { class: `leg ${p.state || 'idle'}` }),
			h('div', { class: `hop ${i > 0 ? p.state || 'idle' : 'start'}`, title: p.title || null },
				h('span', { class: 'top' }, p.top),
				h('span', { class: 'main' }, p.main),
				p.sub && h('span', { class: 'sub' }, p.sub)),
		]));
}

// vxlanPath draws a VXLAN transport, or a tunnel and what it carries: from
// where it starts (start, a VLAN or 0), over tunnel (its name, port and
// MTU), as vnis ([{ vni, by }]), to the far end, address. state is
// probed()'s.
export function vxlanPath({ start, tunnel, port, mtu, vnis, address }, state = {}) {
	const list = vnis.map((v) => v.vni).join(' · ');
	const by = vnis.map((v) => v.by).filter(Boolean).join(', ');
	const says = { ok: 'works', warn: 'unproven', bad: 'broken', idle: 'not reported' };
	const say = (s) => says[s || 'idle'];
	return pathOf([
		{ top: 'From', main: from(start), title: start ? `Starts from VLAN ${start} on the uplink` : 'Starts from the AP\'s management VLAN' },
		{ top: 'VXLAN', main: tunnel, sub: [port && `:${port}`, mtu && `MTU ${mtu}`].filter(Boolean).join(' · ') || null, state: state.start,
			title: `Tunnel ${tunnel}: ${say(state.start)}` },
		{ top: vnis.length > 1 ? 'VNIs' : 'VNI', main: list || '—', sub: by || null, state: state.vni,
			title: `Traffic across: ${say(state.vni)}` },
		{ top: 'To', main: address || '?', state: state.far, title: `The far end, by ping: ${say(state.far)}` },
	], `From the ${start ? `VLAN ${start}` : 'management VLAN'} over VXLAN tunnel ${tunnel}, VNI ${list || 'none yet'}, to ${address || 'an unknown far end'}. `
		+ `The tunnel: ${say(state.start)}; traffic across: ${say(state.vni)}; the far end, by ping: ${say(state.far)}.`);
}

// vlanPath draws a VLAN transport: the VLAN, tagged on the uplink.
export function vlanPath(vlan) {
	return pathOf([{ top: 'VLAN', main: String(vlan), sub: 'tagged on the uplink' }], `VLAN ${vlan}, tagged on the uplink`);
}
