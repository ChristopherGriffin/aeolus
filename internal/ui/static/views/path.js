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
// the tunnels carrying vnis, hop by hop, the worst of them: where each
// starts (from): on a VLAN, whether the AP has its address there and the
// gateway there answers, or the far end does, which it reaches through it;
// on the management VLAN, fine, as the report came that way; whether the
// tunnel is up (start); whether traffic gets across it (vni); and whether
// the far end answers pings (far). addresses are the AP's addresses where
// the tunnels start, each with its gateway, on an AP's own page (one); a
// folder's shows the VLAN only (Griff, 2026-10-06). shared names the APs'
// other interfaces with an address there, which share the start's: a
// restart of either drops the tunnels' routes, until the prober puts them
// back (putBack, seconds since it last did, the latest of the APs').
export function probed(rows, vnis, one = false) {
	const from = [], start = [], mid = [], far = [], addresses = [], shared = new Set();
	let putBack = null;
	for (const { cfg } of rows)
		for (const t of cfg?.condition?.state?.report?.vxlan?.tunnels || []) {
			if (!vnis.includes(t.vni)) continue;
			if (!t.from_vlan) from.push('ok');
			else if (!t.from_address) from.push('bad');
			else from.push(t.from_gateway_answers === true || t.probe?.underlay === true ? 'ok' : t.from_gateway_answers === false ? 'warn' : null);
			if (one && t.from_address && !addresses.some((a) => a.address === t.from_address)) addresses.push({ address: t.from_address, gateway: t.from_gateway });
			for (const s of t.from_shared || []) shared.add(s);
			if (t.from_shared?.length) from.push('warn');
			if (t.from_put_back_ago != null && (putBack == null || t.from_put_back_ago < putBack)) putBack = t.from_put_back_ago;
			start.push(t.up ? 'ok' : t.standby ? null : 'bad');
			if (!t.standby) mid.push(VERDICT[t.probe?.verdict] || null);
			far.push(t.probe?.underlay === true ? 'ok' : t.probe?.underlay === false ? 'bad' : null);
		}
	return { from: worst(from), start: worst(start), vni: worst(mid), far: worst(far), addresses, shared: [...shared].sort(), putBack };
}

// from says where a tunnel starts on the AP (0063): its management VLAN,
// untagged on most uplinks, or another VLAN of the uplink.
function from(vlan) {
	return vlan ? `VLAN ${vlan}` : 'mgmt VLAN';
}

// pathOf draws hops ([{ top, main, sub, state, title }]) joined by legs,
// each leg, and the hop it leads to, coloured by that hop's state; the
// first hop, where nothing leads, by its own, else plain. label says it in
// words, for a screen reader.
export function pathOf(hops, label) {
	return h('div', { class: 'path', role: 'img', 'aria-label': label },
		hops.map((p, i) => [
			i > 0 && h('span', { class: `leg ${p.state || 'idle'}` }),
			h('div', { class: `hop ${p.state || (i > 0 ? 'idle' : 'start')}`, title: p.title || null },
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
	// Where it starts: the VLAN, and the APs' addresses there (Griff,
	// 2026-10-06), with the gateway each reaches the far end through.
	const addrs = start ? state.addresses || [] : [];
	const gws = [...new Set(addrs.map((a) => a.gateway).filter(Boolean))];
	const shared = start ? state.shared || [] : [];
	const ago = state.putBack == null ? '' : state.putBack < 120 ? `${state.putBack} s` : state.putBack < 7200 ? `${Math.round(state.putBack / 60)} min` : `${Math.round(state.putBack / 3600)} h`;
	const caveat = (shared.length
		? ` Its address there is shared with ${shared.join(', ')}, which lease${shared.length > 1 ? '' : 's'} on the same VLAN: a restart of either drops the tunnels' routes, until the prober puts them back. Remove ${shared.length > 1 ? 'them' : 'it'} from the AP, or move ${shared.length > 1 ? 'them' : 'it'} off that VLAN.`
		: '') + (ago ? ` The prober last put back lost routes ${ago} ago.` : '');
	// A shared start that works is at risk, not unproven.
	const fromSays = shared.length && state.from === 'warn' ? 'works, at risk' : say(state.from);
	const sub = [addrs.length ? addrs.slice(0, 3).map((a) => a.address).join(', ') + (addrs.length > 3 ? ` +${addrs.length - 3}` : '') : null,
		shared.length ? `shared with ${shared.join(', ')}` : null].filter(Boolean).join(' · ') || null;
	return pathOf([
		{ top: 'From', main: from(start), sub,
			state: start ? state.from : null,
			title: start
				? `Starts from VLAN ${start} on the uplink${gws.length ? `, through gateway ${gws.join(', ')}` : ''}: ${start && state.from === 'bad' ? 'no address there: does DHCP answer?' : fromSays}.${caveat}`
				: 'Starts from the AP\'s management VLAN' },
		{ top: 'VXLAN', main: tunnel, sub: [port && `:${port}`, mtu && `MTU ${mtu}`].filter(Boolean).join(' · ') || null, state: state.start,
			title: `Tunnel ${tunnel}: ${say(state.start)}` },
		{ top: vnis.length > 1 ? 'VNIs' : 'VNI', main: list || '—', sub: by || null, state: state.vni,
			title: `Traffic across: ${say(state.vni)}` },
		{ top: 'To', main: address || '?', state: state.far, title: `The far end, by ping: ${say(state.far)}` },
	], `From the ${start ? `VLAN ${start}${addrs.length ? `, ${addrs.map((a) => a.address).join(', ')}` : ''}` : 'management VLAN'} over VXLAN tunnel ${tunnel}, VNI ${list || 'none yet'}, to ${address || 'an unknown far end'}. `
		+ `${start ? `The start, by its gateway: ${fromSays}; ` : ''}the tunnel: ${say(state.start)}; traffic across: ${say(state.vni)}; the far end, by ping: ${say(state.far)}.`);
}

// vlanPath draws a VLAN transport: the VLAN, tagged on the uplink.
export function vlanPath(vlan) {
	return pathOf([{ top: 'VLAN', main: String(vlan), sub: 'tagged on the uplink' }], `VLAN ${vlan}, tagged on the uplink`);
}
