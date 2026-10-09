// Rendering an AP's intent into UCI (0006, 0008, 0040).
//
// render() is pure: it takes the composed config from the manager, the AP's
// current UCI and a few local facts, and returns the new UCI. It never reads
// or writes files, so the same code runs on the AP and in CI, where its output
// is fed to the manager's render check.
//
// Aeolus only touches what it owns (0040): the radio options intent names,
// the bridge-vlan entries and enabled of the ports intent names (0053),
// usteer's band_steering_interval and ssid_list (0050), all of snmpd's
// config (0052), sections named aeolus_ (in the firewall, 0054, and in its
// own package, the prober's plan, 0059), and the time zone, NTP and syslog
// settings. It takes OpenWrt's internet pool out of the time servers, and
// adds to dnsmasq's rebind_domain the names it uses on the AP (0069). A
// section it did not create is never otherwise edited, or removed; nor are
// the key agent's wifi-station sections, which hold per-user keys (0070). A
// radio another service owns, such as airscan's scan radio, is left alone
// altogether (0081).

'use strict';

import { text } from 'aeolus.uciexport';
import { segment_mac } from 'aeolus.probe';

const PACKAGES = ['wireless', 'network', 'system', 'aeolus', 'usteer', 'snmpd', 'firewall', 'dhcp'];

// OpenWrt's default time servers, which are on the internet (0069).
const POOL = /\.openwrt\.pool\.ntp\.org$/;

// The MSS clamp's nftables file, which fw4 loads (0054).
const CLAMP = '/etc/aeolus/clamp.nft';

const ENCRYPTION = {
	'open': 'none', 'owe': 'owe', 'wpa2-psk': 'psk2', 'wpa3-sae': 'sae', 'wpa2-wpa3': 'sae-mixed',
};
const NEEDS_KEY = { 'wpa2-psk': true, 'wpa3-sae': true, 'wpa2-wpa3': true };

// The htmode families each band can use, best first.
const FAMILIES = { '2g': ['EHT', 'HE', 'HT'], '5g': ['EHT', 'HE', 'VHT', 'HT'], '6g': ['EHT', 'HE'] };

function clone(v) {
	return json(sprintf('%J', v));
}

function owned(name) {
	return substr(name ?? '', 0, 7) == 'aeolus_';
}

// iface_name is the wifi-iface for a network on a radio (0039).
function iface_name(network, radio) {
	return 'aeolus_' + replace(network, '-', '_') + '_' + radio;
}

function interface_name(network) {
	return 'aeolus_' + replace(network, '-', '_');
}

// net_hash names a network's own bridge and veth pairs (0061): FNV-1a of the
// network's name, as 8 hex digits, so the names fit Linux's 15 characters.
function net_hash(id) {
	let h = 0x811c9dc5;
	for (let i = 0; i < length(id); i++)
		h = ((h ^ ord(id, i)) * 0x01000193) & 0xffffffff;
	return sprintf('%08x', h);
}

// The VLAN end of a network's veth pair, which Aeolus puts in the uplink's
// bridge (0061): av, p or f for the slot, and the network's hash.
const VLAN_END = /^av[pf][0-9a-f]{8}$/;

function of_type(pkg, typ) {
	let out = filter(values(pkg), s => s['.type'] == typ);
	sort(out, (a, b) => a['.index'] - b['.index']);
	return out;
}

function list(v) {
	return v == null ? [] : (type(v) == 'array' ? v : [v]);
}

// put creates or replaces a section Aeolus owns, keeping its place.
function put(pkg, name, typ, opts) {
	let index = pkg[name]?.['.index'];
	if (index == null) {
		index = 0;
		for (let k in pkg)
			index = max(index, pkg[k]['.index'] + 1);
	}
	let s = { '.anonymous': false, '.type': typ, '.name': name, '.index': index };
	for (let k in opts)
		if (opts[k] != null)
			s[k] = type(opts[k]) == 'array' ? map(opts[k], v => '' + v) : '' + opts[k];
	pkg[name] = s;
}

function htmode(band, width, current, modes) {
	for (let fam in FAMILIES[band] ?? ['HT'])
		if (index(modes ?? [], fam + width) >= 0)
			return fam + width;
	// Capabilities unknown: keep the family the radio runs now.
	let m = match(current ?? '', /^(HT|VHT|HE|EHT)[0-9]/);
	return (m ? m[1] : 'HT') + width;
}

// BLOCKS5 are the 5 GHz channels joined at each width, by their lowest and
// highest 20 MHz channel, as internal/radio has them.
const BLOCKS5 = {
	'40': [[36, 40], [44, 48], [52, 56], [60, 64], [100, 104], [108, 112], [116, 120], [124, 128], [132, 136], [140, 144], [149, 153], [157, 161]],
	'80': [[36, 48], [52, 64], [100, 112], [116, 128], [132, 144], [149, 161]],
	'160': [[36, 64], [100, 128]],
};

// whole keeps, of the channels an automatic channel may be (0075), those a
// radio at width can use: on 5 GHz at 40 MHz or more, the blocks wholly in
// the set, as hostapd checks only a block's primary channel against its
// list. Sorted, as strings for UCI.
function whole(band, set, width) {
	let have = {};
	for (let c in set)
		have['' + c] = true;
	let out = [];
	if (band != '5g' || width <= 20)
		out = map(keys(have), c => +c);
	else
		for (let b in BLOCKS5['' + width] ?? []) {
			let all = true;
			for (let c = b[0]; c <= b[1]; c += 4)
				all = all && have['' + c];
			if (all)
				for (let c = b[0]; c <= b[1]; c += 4)
					push(out, c);
		}
	return map(sort(out, (a, b) => a - b), c => '' + c);
}

// reserved says another service on the AP owns a radio, and Aeolus leaves it
// alone (0081): no radio settings, no networks. airscan marks the dedicated
// scan radio it takes out of netifd's hands (option airscan '1').
function reserved(s) {
	return s.airscan == '1';
}

// scan_radio says the AP has a radio another service owns, a scan radio
// that serves no clients: there its serving radios never scan (0081).
function scan_radio(w) {
	return length(filter(of_type(w, 'wifi-device'), reserved)) > 0;
}

function radios(w, intent, facts) {
	let country = intent.system?.country;
	for (let s in of_type(w, 'wifi-device')) {
		if (reserved(s))
			continue;
		let set = intent.radio?.[s.band] ?? {};
		if (country != null)
			s.country = country;
		if (set.enabled != null)
			s.disabled = set.enabled ? '0' : '1';
		if (set.width != null)
			s.htmode = htmode(s.band, set.width, s.htmode, facts.radios?.[s['.name']]?.htmodes);
		if (set.channel != null) {
			s.channel = '' + set.channel;
			// An automatic channel is one of the set's (0075), at the radio's
			// width; unset, on 2.4 GHz, one of 1, 6 and 11, the only ones that
			// do not overlap (0045).
			let width = int(match(s.htmode ?? '', /([0-9]+)$/)?.[1] ?? 20);
			let list = set.channel != 'auto' ? [] : set.channels ? whole(s.band, set.channels, width) : s.band == '2g' ? ['1', '6', '11'] : [];
			if (length(list))
				s.channels = list;
			else
				delete s.channels;
		}
		if (set.power == 'auto')
			delete s.txpower;
		else if (set.power != null)
			s.txpower = '' + set.power;
		// With DFS avoided, an automatic channel is picked outside the
		// channels shared with radar (0071). A set channel needs nothing:
		// the config check refuses one that is DFS.
		if (set.dfs != null) {
			if (set.dfs == 'avoid' && (s.channel ?? 'auto') == 'auto')
				s.acs_exclude_dfs = '1';
			else if (s.acs_exclude_dfs in ['1', 'yes', 'on', 'true', 'enabled'])
				delete s.acs_exclude_dfs;
		}
	}
}

// uplink_bridge finds the VLAN-filtering bridge the uplink port is in.
function uplink_bridge(n, uplink, errors) {
	if (!uplink) {
		push(errors, 'no uplink port is set (aeolus.agent.uplink)');
		return null;
	}
	for (let d in of_type(n, 'device')) {
		if (d.type != 'bridge' || index(list(d.ports), uplink) < 0)
			continue;
		if (!length(filter(of_type(n, 'bridge-vlan'), v => v.device == d.name))) {
			push(errors, `bridge ${d.name} does not filter VLANs; 802.1Q devices on the uplink are not rendered yet`);
			return null;
		}
		return d.name;
	}
	push(errors, `the uplink ${uplink} is in no bridge`);
	return null;
}

// ensure_vlan makes sure the bridge carries a VLAN on the uplink, reusing a
// bridge-vlan that already exists so a VLAN is never defined twice (0040),
// and returns that section.
function ensure_vlan(n, bridge, uplink, vlan, keep) {
	let name = 'aeolus_vlan' + vlan;
	for (let v in of_type(n, 'bridge-vlan'))
		if (v.device == bridge && v.vlan == '' + vlan && v['.name'] != name)
			return v;
	// Kept as it is, but for the uplink: ports() sets the ports' entries.
	let ports = filter(list(n[name]?.ports), e => split(e, ':')[0] != uplink);
	put(n, name, 'bridge-vlan', { device: bridge, vlan: vlan, ports: [uplink + ':t', ...ports] });
	keep[name] = true;
	return n[name];
}

function iface_options(net, radio, network, btm) {
	let o = {
		device: radio, mode: 'ap', network: network, ssid: net.ssid,
		encryption: ENCRYPTION[net.security] ?? 'none',
	};
	if (NEEDS_KEY[net.security])
		o.key = net.passphrase;
	if (net.hidden)
		o.hidden = '1';
	if (net.isolation)
		o.isolate = '1';
	if (net.roaming?.ft) {
		o.ieee80211r = '1';
		o.ft_over_ds = '0';
		o.ft_psk_generate_local = '1';
	}
	// Band steering works through 802.11k and 802.11v (0050).
	if (net.roaming?.rrm || net.band_steering)
		o.ieee80211k = '1';
	if (btm)
		o.bss_transition = '1';
	if (net.multicast_to_unicast != null)
		o.multicast_to_unicast = net.multicast_to_unicast ? '1' : '0';
	return o;
}

// The interfaces tunnels start from on a VLAN (0063): their routing tables
// are 1000 plus the VLAN, clear of the kernel's own, and they share a zone.
const START_TABLE = 1000;
const START_ZONE = 'aeolus_ul';

// start_on_vlan makes the interface that tunnels start from on a VLAN of the
// uplink (0063), and returns its name. The VLAN is tagged on the uplink, and
// the interface takes an address by DHCP. Its routes go in a table of their
// own, so only the tunnels use its gateway, and its DNS servers are not
// used. Its zone rejects what comes in but the tunnels' own rules, so a VLAN
// that also carries clients gives them no way into the AP.
function start_on_vlan(cfg, where, vlan, facts, errors, keep) {
	let n = cfg.network, fw = cfg.firewall;
	let bridge = uplink_bridge(n, facts.uplink, errors);
	if (!bridge)
		return null;
	let device = `${bridge}.${vlan}`;
	if (n[facts.management]?.device == device) {
		push(errors, `${where}: VLAN ${vlan} is the management VLAN here, which tunnels start from anyway; set it to the management VLAN`);
		return null;
	}
	ensure_vlan(n, bridge, facts.uplink, vlan, keep);
	let name = `aeolus_vlan${vlan}_tunnels`;
	put(n, name, 'interface', { proto: 'dhcp', device: device, ip4table: START_TABLE + vlan, peerdns: 0 });
	keep[name] = true;
	let nets = keep.aeolus_zone_ul ? list(fw.aeolus_zone_ul.network) : [];
	if (index(nets, name) < 0)
		push(nets, name);
	put(fw, 'aeolus_zone_ul', 'zone', { name: START_ZONE, input: 'REJECT', output: 'ACCEPT', forward: 'REJECT', network: nets });
	keep.aeolus_zone_ul = true;
	return name;
}

// tunnel renders a VXLAN transport (0054): an interface named for the VNI,
// which is also its device, to the concentrator from the management
// interface, or from a VLAN of the uplink (0063); its own bridge, which the
// network's Wi-Fi joins; a firewall rule letting it in; and below an MTU of
// 1500, the MSS clamp. A standby tunnel, a network's fallback, is not
// started until the AP needs it (0061). It returns the bridge.
function tunnel(cfg, where, standby, t, conc, facts, errors, keep, bridged) {
	let n = cfg.network, fw = cfg.firewall;
	if (!facts.vxlan) {
		push(errors, `${where}: VXLAN needs the vxlan package, which is not installed on this AP (apk add vxlan)`);
		return null;
	}
	let address = replace(conc?.address ?? '', /^\[|\]$/g, '');
	let six = index(address, ':') >= 0;
	if (!six && !match(address, /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/)) {
		push(errors, `${where}: the concentrator's address "${address}" is not an IP address`);
		return null;
	}
	if (!facts.management) {
		push(errors, `${where}: the AP's management interface is not known`);
		return null;
	}
	// Where it starts: the management interface, or a VLAN (0063).
	let from = facts.management, zone = null;
	if (conc.underlay_vlan) {
		if (six) {
			push(errors, `${where}: a tunnel to an IPv6 concentrator starts from the management VLAN for now (0063)`);
			return null;
		}
		from = start_on_vlan(cfg, where, conc.underlay_vlan, facts, errors, keep);
		if (!from)
			return null;
		zone = START_ZONE;
	}
	let name = 'aeolus_' + t.vni;
	let o = { proto: six ? 'vxlan6' : 'vxlan', vid: t.vni, port: conc.port, mtu: conc.mtu, tunlink: from };
	o[six ? 'peer6addr' : 'peeraddr'] = address;
	if (standby)
		o.auto = 0;
	put(n, name, 'interface', o);
	keep[name] = true;
	// A tunnel of a network with a fallback is attached to the network's own
	// bridge by the prober (0061); others have a bridge of their own.
	let bridge = bridged === false ? name : 'br-vx' + t.vni;
	if (bridged !== false) {
		// Ports on the tunnel stay; ports() sets those it is told about
		// (0058). The bridge takes the AP's MAC for the segment, so what
		// answers the prober's DHCP and probes from it stays at the AP (0060).
		let members = filter(list(n[name + '_br']?.ports), e => e != name);
		put(n, name + '_br', 'device', {
			type: 'bridge', name: bridge, bridge_empty: 1, ports: [name, ...members],
			macaddr: segment_mac(facts.ap, 'vni', t.vni) || null,
		});
		keep[name + '_br'] = true;
	}
	zone ??= filter(of_type(fw, 'zone'), z => index(list(z.network), facts.management) >= 0)[0]?.name;
	if (zone)
		put(fw, 'aeolus_vxlan_' + t.vni, 'rule', {
			name: 'Aeolus VXLAN ' + t.vni, src: zone, proto: 'udp',
			src_ip: address, dest_port: conc.port, target: 'ACCEPT',
		});
	else
		push(errors, `${where}: no firewall zone holds the management interface ${facts.management}`);
	keep['aeolus_vxlan_' + t.vni] = true;
	if (conc.mtu < 1500) {
		if (facts.nft_bridge) {
			put(fw, 'aeolus_clamp', 'include', { type: 'nftables', path: CLAMP, position: 'ruleset-append' });
			keep.aeolus_clamp = true;
		} else
			push(errors, `${where}: an MTU below 1500 needs the MSS clamp, and so kmod-nft-bridge, which is not installed on this AP`);
	}
	return bridge;
}

// leave_uplink takes the VLAN ends of veth pairs Aeolus made out of every
// bridge and its VLANs; those still wanted join again (0061).
function leave_uplink(n) {
	for (let d in of_type(n, 'device'))
		if (d.type == 'bridge' && length(filter(list(d.ports), e => match(e, VLAN_END))))
			d.ports = filter(list(d.ports), e => !match(e, VLAN_END));
	for (let v in of_type(n, 'bridge-vlan'))
		if (length(filter(list(v.ports), e => match(split(e, ':')[0], VLAN_END))))
			v.ports = filter(list(v.ports), e => !match(split(e, ':')[0], VLAN_END));
}

// join_uplink puts a veth's VLAN end in the uplink's bridge, untagged in its
// VLAN, as a VLAN network's Wi-Fi is (0061).
function join_uplink(n, bridge, end, vlan) {
	let up = filter(of_type(n, 'device'), d => d.name == bridge)[0];
	let v = filter(of_type(n, 'bridge-vlan'), x => x.device == bridge && x.vlan == '' + vlan)[0];
	if (up && index(list(up.ports), end) < 0)
		up.ports = [...list(up.ports), end];
	if (v && index(list(v.ports), end + ':u*') < 0)
		v.ports = [...list(v.ports), end + ':u*'];
}

// switching renders a network with a fallback (0061): a bridge of its own,
// br-n and its hash, which its Wi-Fi joins and neither transport is
// configured in. The prober attaches the transport that carries the network
// at run time, and moves it when it switches. A VLAN transport reaches the
// bridge through a veth pair whose configured end is in the uplink's bridge,
// untagged in the VLAN; a VXLAN transport is its tunnel's device. The bridge
// takes the primary's segment MAC (0060). The prober's plan says which
// device is which, and how the network switches: in report mode never; in
// automatic mode, with HA mode, failback and hold-down (0022). A tunnel
// fallback is started only in HA mode; otherwise the prober starts it when
// the primary goes down. It returns the bridge.
function switching(cfg, id, net, intent, facts, errors, keep, uplink) {
	let n = cfg.network, a = cfg.aeolus;
	let h = net_hash(id), sect = 'aeolus_n' + h, br = 'br-n' + h;
	if (!facts.prober)
		push(errors, `network.${id}.transport: a network with a fallback needs the prober, and so ucode-mod-socket, which is not installed on this AP (0061)`);
	let tr = net.transport, auto = tr.switching == 'automatic';
	let plan = { network: id, bridge: br, mode: auto ? 'automatic' : 'report' }, mac = null;
	if (auto) {
		plan.ha = tr.ha ? 1 : 0;
		plan.failback = tr.failback ?? 'revertive';
		plan.holddown = tr.holddown ?? 300;
	}
	for (let slot in ['primary', 'fallback']) {
		let t = net.transport?.[slot], where = `network.${id}.transport.${slot}`;
		if (t?.type == 'vlan') {
			let bridge = uplink();
			if (!bridge)
				continue;
			if (!facts.veth)
				push(errors, `${where}: a VLAN transport of a network with a fallback needs kmod-veth, which is not installed on this AP (apk add kmod-veth)`);
			ensure_vlan(n, bridge, facts.uplink, t.vlan, keep);
			let s = substr(slot, 0, 1), name = `${sect}_${s}`;
			put(n, name, 'device', { type: 'veth', name: `av${s}${h}`, peer_name: `an${s}${h}` });
			keep[name] = true;
			join_uplink(n, bridge, `av${s}${h}`, t.vlan);
			plan[slot] = `an${s}${h}`;
			plan[slot + '_vlan'] = t.vlan;
			mac ??= segment_mac(facts.ap, 'vlan', t.vlan);
		} else if (t?.type == 'vxlan') {
			let dev = tunnel(cfg, where, slot == 'fallback' && !(auto && tr.ha), t, intent.concentrators?.[t.concentrator], facts, errors, keep, false);
			if (!dev)
				continue;
			plan[slot] = dev;
			plan[slot + '_vni'] = t.vni;
			mac ??= segment_mac(facts.ap, 'vni', t.vni);
		}
	}
	put(n, sect, 'device', { type: 'bridge', name: br, bridge_empty: 1, macaddr: mac || null });
	put(a, sect, 'switch', plan);
	keep[sect] = true;
	return br;
}

function networks(cfg, intent, facts, errors, keep) {
	let w = cfg.wireless, n = cfg.network;
	let bridge = null, looked = false;
	let uplink = () => {
		if (!looked)
			bridge = uplink_bridge(n, facts.uplink, errors);
		looked = true;
		return bridge;
	};
	let nets = intent.network ?? {};
	leave_uplink(n);
	for (let id in sort(keys(nets))) {
		let net = nets[id];
		if (net.enabled === false)
			continue;
		let iface = interface_name(id);
		// The device the network's interface is on: its primary's, or with a
		// fallback, its own bridge (0061).
		let path = net.transport?.fallback?.type ? switching(cfg, id, net, intent, facts, errors, keep, uplink) : null;
		for (let slot in (path ? [] : ['primary', 'fallback'])) {
			let t = net.transport?.[slot];
			let where = `network.${id}.transport.${slot}`;
			let device = null;
			if (t?.type == 'vlan') {
				if (!uplink())
					continue;
				ensure_vlan(n, bridge, facts.uplink, t.vlan, keep);
				device = `${bridge}.${t.vlan}`;
			} else if (t?.type == 'vxlan')
				device = tunnel(cfg, where, slot != 'primary', t, intent.concentrators?.[t.concentrator], facts, errors, keep);
			if (slot == 'primary')
				path = device;
		}
		if (path != null) {
			put(n, iface, 'interface', { proto: 'none', device: path });
			keep[iface] = true;
		}
		// BSS transition (802.11v), which band steering uses too, is left
		// out where hostapd lacks it: there, the line makes hostapd refuse
		// its whole config, so every network on the AP goes down (0057).
		// The render check then refuses the config.
		let btm = !!(net.roaming?.btm || net.band_steering);
		if (btm && facts.bss_transition === false) {
			push(errors, `network.${id}: band steering and BSS transition need 802.11v, which this AP's hostapd lacks (install wpad-mbedtls)`);
			btm = false;
		}
		let names = [], no_ap_vlan = [];
		for (let d in of_type(w, 'wifi-device')) {
			if (reserved(d) || (net.bands && index(net.bands, d.band) < 0))
				continue;
			let name = iface_name(id, d['.name']);
			put(w, name, 'wifi-iface', iface_options(net, d['.name'], iface, btm));
			keep[name] = true;
			push(names, name);
			if (facts.radios?.[d['.name']]?.ap_vlan === false)
				push(no_ap_vlan, d['.name']);
		}
		// A key's VLAN is an AP/VLAN interface hostapd makes for its clients.
		// On a radio whose driver makes none (ath11k), hostapd fails every
		// network on the radio, and the apply is reverted (0082): refused here
		// instead, before anything is applied.
		if (length(net.keys?.vlans ?? []) && length(no_ap_vlan))
			push(errors, `network.${id}.keys.vlans: ${join(', ', no_ap_vlan)} cannot put clients in VLANs of their own (the driver has no AP/VLAN interfaces); offer the network on other bands, or give its keys no VLANs`);
		// The VLANs the network's per-user keys may put clients in (0070):
		// each tagged on the uplink, an interface on it, and a wifi-vlan on
		// the network's Wi-Fi. hostapd makes the VLAN's Wi-Fi interface,
		// <bss>-k<vlan>, and netifd puts it on that interface.
		for (let v in net.keys?.vlans ?? []) {
			if (!uplink())
				break;
			ensure_vlan(n, bridge, facts.uplink, v, keep);
			let vi = `${iface}_k${v}`, wv = `${iface}_kv${v}`;
			put(n, vi, 'interface', { proto: 'none', device: `${bridge}.${v}` });
			put(w, wv, 'wifi-vlan', { iface: names, name: `k${v}`, vid: '' + v, network: [vi] });
			keep[vi] = keep[wv] = true;
		}
	}
}

// leave_tunnels takes a port off every tunnel's bridge Aeolus made: the port
// itself, and its 802.1Q devices (0058).
function leave_tunnels(n, p) {
	for (let d in of_type(n, 'device'))
		if (owned(d['.name']) && d.type == 'bridge')
			d.ports = filter(list(d.ports), e => e != p && substr(e, 0, length(p) + 1) != p + '.');
}

// tunnel_port puts a port on tunnels (0058): out of the uplink's bridge and
// its VLANs, and each VNI it carries onto its tunnel's bridge, the port
// itself for the untagged one and an 802.1Q device <port>.<vlan> for each
// tagged one. The tunnels are the ones networks' transports make (0054).
function tunnel_port(cfg, p, set, bridge, up, intent, facts, errors, keep) {
	let n = cfg.network;
	up.ports = filter(list(up.ports), e => e != p);
	for (let v in of_type(n, 'bridge-vlan'))
		if (v.device == bridge)
			v.ports = filter(list(v.ports), e => split(e, ':')[0] != p);
	leave_tunnels(n, p);
	let safe = replace(p, /[^a-z0-9_]/g, '_');
	for (let vlan in sort(keys(set.vxlan ?? {}))) {
		let m = set.vxlan[vlan];
		let br = tunnel(cfg, `ports.${p}.vxlan.${vlan}`, false, { vni: m.vni, concentrator: m.tunnel },
			intent.concentrators?.[m.tunnel], facts, errors, keep);
		if (!br)
			continue;
		let member = p;
		if (vlan != 'untagged') {
			member = `${p}.${vlan}`;
			let name = `aeolus_port_${safe}_${vlan}`;
			put(n, name, 'device', { type: '8021q', ifname: p, vid: vlan, name: member });
			keep[name] = true;
		}
		let b = n[`aeolus_${m.vni}_br`];
		b.ports = [...list(b.ports), member];
	}
}

// hold_bridges puts an interface on each started tunnel's bridge that no
// network's interface is on: a VNI that only ports carry (0058). netifd makes
// a bridge only for an interface on it, so without one the bridge is never
// made, and its ports are on nothing. The interface takes no address. A
// fallback's bridge waits, as its tunnel does.
function hold_bridges(n, keep) {
	let used = {};
	for (let s in of_type(n, 'interface'))
		if (keep[s['.name']] || !owned(s['.name']))
			used[s.device] = true;
	for (let d in of_type(n, 'device')) {
		let vni = match(d['.name'], /^aeolus_([0-9]+)_br$/)?.[1];
		let t = vni ? n['aeolus_' + vni] : null;
		if (!t || !keep[d['.name']] || t.auto == '0' || used[d.name])
			continue;
		let name = `aeolus_${vni}_ports`;
		put(n, name, 'interface', { proto: 'none', device: d.name });
		keep[name] = true;
	}
}

// ports applies the intent's Ethernet port settings (0053): a port's VLANs,
// as its entries in the bridge's bridge-vlan sections, and whether it is on.
// Only ports in the uplink's bridge count; the uplink itself is the AP's
// management and is left alone. Each VLAN a port uses is tagged on the
// uplink as well. A port in tunnel mode carries VNIs over tunnels instead,
// and leaves the uplink's bridge; set back to access or trunk, it returns
// (0058). LACP is not applied yet; the manager holds it.
//
// A setting left unset leaves the port as it is, as with a radio, so a VLAN
// Aeolus added stays while a port is still on it, and so does a tunnel.
function ports(cfg, intent, facts, errors, keep) {
	let n = cfg.network;
	let want = intent.ports ?? {};
	let bridge = length(keys(want)) ? uplink_bridge(n, facts.uplink, errors) : null;
	let up = filter(of_type(n, 'device'), d => d.name == bridge)[0];
	// A port is on this AP if it is in the uplink's bridge, or on a tunnel's
	// bridge or an 802.1Q device Aeolus made (0058).
	let on_ap = p => index(list(up?.ports), p) >= 0 ||
		length(filter(of_type(n, 'device'), d => owned(d['.name']) &&
			(index(list(d.ports), p) >= 0 || (d.type == '8021q' && d.ifname == p)))) > 0;
	for (let p in (bridge ? sort(keys(want)) : [])) {
		let set = want[p];
		if (!on_ap(p))
			continue;   // not on this AP
		if (p == facts.uplink) {
			push(errors, `ports.${p}: the uplink carries the AP's management; Aeolus leaves it alone`);
			continue;
		}
		// On or off goes on the port's own device section, or on one Aeolus
		// adds to turn it off; a port with neither is on.
		if (set.enabled != null) {
			let own = 'aeolus_port_' + replace(p, /[^a-z0-9_]/g, '_');
			let d = filter(of_type(n, 'device'), x => x.name == p && x.type == null && x['.name'] != own)[0];
			if (d)
				d.enabled = set.enabled ? '1' : '0';
			if (d || set.enabled)
				delete n[own];
			else
				put(n, own, 'device', { name: p, enabled: 0 });
		}
		if (set.mode == 'tunnel') {
			tunnel_port(cfg, p, set, bridge, up, intent, facts, errors, keep);
			continue;
		}
		if (set.mode != 'access' && set.mode != 'trunk')
			continue;
		// Back from tunnels, if it was on them, into the uplink's bridge.
		leave_tunnels(n, p);
		if (index(list(up.ports), p) < 0)
			up.ports = [...list(up.ports), p];
		for (let v in of_type(n, 'bridge-vlan'))
			if (v.device == bridge)
				v.ports = filter(list(v.ports), e => split(e, ':')[0] != p);
		let untagged = set.untagged ?? 0;
		if (untagged) {
			let v = ensure_vlan(n, bridge, facts.uplink, untagged, keep);
			v.ports = [...list(v.ports), p + ':u*'];
		}
		for (let vlan in (set.mode == 'trunk' ? set.tagged ?? [] : [])) {
			let v = ensure_vlan(n, bridge, facts.uplink, vlan, keep);
			v.ports = [...list(v.ports), p + ':t'];
		}
	}
	// What Aeolus added that a port is still on stays, and so does a port
	// Aeolus turned off.
	for (let v in of_type(n, 'bridge-vlan'))
		if (owned(v['.name']) && length(filter(list(v.ports), e => split(e, ':')[0] != facts.uplink)))
			keep[v['.name']] = true;
	for (let d in of_type(n, 'device'))
		if (substr(d['.name'], 0, 12) == 'aeolus_port_' && d.type != '8021q')
			keep[d['.name']] = true;
	// So does a tunnel a port is still on: its bridge, the tunnel, the rule
	// that lets it in, the clamp, and the port's 802.1Q devices on it (0058).
	for (let b in of_type(n, 'device')) {
		let m = match(b['.name'], /^aeolus_([0-9]+)_br$/);
		let others = m ? filter(list(b.ports), e => e != 'aeolus_' + m[1]) : [];
		if (!length(others))
			continue;
		keep[b['.name']] = keep['aeolus_' + m[1]] = keep['aeolus_vxlan_' + m[1]] = true;
		if (int(n['aeolus_' + m[1]]?.mtu ?? '1500') < 1500 && cfg.firewall?.aeolus_clamp)
			keep.aeolus_clamp = true;
		for (let d in of_type(n, 'device'))
			if (owned(d['.name']) && d.type == '8021q' && index(others, d.name) >= 0)
				keep[d['.name']] = true;
	}
}

// probes renders the prober's plan (0059) into the agent's own package: for
// each tunnel Aeolus made, a probe section named for it, aeolus_<VNI>, with
// its tunnel's probe interval, the addresses on its segment to ask, from
// every network and port that uses the VNI, and the MAC the AP uses there,
// for its lease (0060); and for each port on a tunnel,
// a guard section, aeolus_guard_<port>, naming the devices the loop guard
// sends on: the port itself, and its 802.1Q devices. Both come from the
// network config as rendered, so a port left on a tunnel is guarded too.
function probes(cfg, intent, facts, keep) {
	let n = cfg.network, a = cfg.aeolus;
	let uses = {};
	let use = (vni, conc, address) => {
		let u = uses['' + vni] ??= { interval: null, address: [] };
		u.interval ??= conc?.probe_interval;
		if (address != null && index(u.address, '' + address) < 0)
			push(u.address, '' + address);
	};
	for (let id in sort(keys(intent.network ?? {}))) {
		let net = intent.network[id];
		if (net.enabled === false)
			continue;
		for (let slot in ['primary', 'fallback']) {
			let t = net.transport?.[slot];
			if (t?.type == 'vxlan')
				use(t.vni, intent.concentrators?.[t.concentrator], t.probe);
		}
	}
	for (let p in sort(keys(intent.ports ?? {}))) {
		let set = intent.ports[p];
		if (set.mode != 'tunnel')
			continue;
		for (let vlan in sort(keys(set.vxlan ?? {})))
			use(set.vxlan[vlan].vni, intent.concentrators?.[set.vxlan[vlan].tunnel], set.vxlan[vlan].probe);
	}
	for (let s in of_type(n, 'interface')) {
		let vni = match(s['.name'], /^aeolus_([0-9]+)$/)?.[1];
		if (!vni || !(s.proto in { vxlan: 1, vxlan6: 1 }))
			continue;
		let u = uses[vni] ?? { address: [] };
		put(a, s['.name'], 'probe', {
			vni: vni, interval: u.interval ?? 30, address: length(u.address) ? sort(u.address) : null,
			mac: segment_mac(facts.ap, 'vni', vni),
		});
		keep[s['.name']] = true;
	}
	// The VLAN transports of networks with a fallback (0061): probed from
	// the VLAN's own MAC, on the uplink, tagged as the uplink carries it.
	let on = {};
	for (let id in sort(keys(intent.network ?? {}))) {
		let net = intent.network[id];
		if (net.enabled === false || !net.transport?.fallback?.type)
			continue;
		for (let slot in ['primary', 'fallback']) {
			let t = net.transport[slot];
			if (t?.type != 'vlan')
				continue;
			on['' + t.vlan] ??= [];
			if (t.probe != null && index(on['' + t.vlan], '' + t.probe) < 0)
				push(on['' + t.vlan], '' + t.probe);
		}
	}
	for (let vlan in sort(keys(on))) {
		let entry = null;
		for (let v in of_type(n, 'bridge-vlan'))
			if (v.vlan == vlan)
				for (let e in list(v.ports))
					if (split(e, ':')[0] == facts.uplink)
						entry = split(e, ':')[1] ?? '';
		if (entry == null)
			continue;   // the uplink does not carry it: the render says so
		put(a, 'aeolus_vlan' + vlan, 'probe', {
			vlan: vlan, device: facts.uplink, tagged: index(entry, 't') >= 0 ? 1 : 0, interval: 30,
			address: length(on[vlan]) ? sort(on[vlan]) : null, mac: segment_mac(facts.ap, 'vlan', +vlan) || null,
		});
		keep['aeolus_vlan' + vlan] = true;
	}
	let vlans = {};
	for (let d in of_type(n, 'device'))
		if (owned(d['.name']) && d.type == '8021q' && d.name && d.ifname)
			vlans[d.name] = d.ifname;
	let guards = {};
	for (let b in of_type(n, 'device')) {
		if (!match(b['.name'], /^aeolus_[0-9]+_br$/))
			continue;
		for (let e in list(b.ports)) {
			if (owned(e))
				continue;   // the tunnel itself
			let p = vlans[e] ?? e;
			guards[p] ??= [];
			if (index(guards[p], e) < 0)
				push(guards[p], e);
		}
	}
	for (let p in sort(keys(guards))) {
		let name = 'aeolus_guard_' + replace(p, /[^a-z0-9_]/g, '_');
		put(a, name, 'guard', { port: p, device: sort(guards[p]) });
		keep[name] = true;
	}
}

// watches renders the VLANs the prober watches on the uplink (0064): each one
// the intent needs there, for a network's VLAN transport, a port's VLANs on
// this AP, or the start of a tunnel (0063), as the uplink carries it. A
// section aeolus_watch<N> says whether it is tagged there, and the AP's MAC
// on it, which nudges come from (0060). A VLAN the uplink doesn't carry is
// left out: the render says so elsewhere.
function watches(cfg, intent, facts, keep) {
	let n = cfg.network, a = cfg.aeolus;
	let up = filter(of_type(n, 'device'), d => d.type == 'bridge' && facts.uplink && index(list(d.ports), facts.uplink) >= 0)[0];
	if (!up)
		return;
	let need = {};
	for (let id in keys(intent.network ?? {})) {
		let net = intent.network[id];
		if (net.enabled === false)
			continue;
		for (let slot in ['primary', 'fallback'])
			if (net.transport?.[slot]?.type == 'vlan')
				need['' + net.transport[slot].vlan] = true;
	}
	for (let p in keys(intent.ports ?? {})) {
		let set = intent.ports[p];
		if (!(set.mode in { access: 1, trunk: 1 }) || p == facts.uplink || index(list(up.ports), p) < 0)
			continue;   // not on this AP, or not on VLANs
		if (set.untagged)
			need['' + set.untagged] = true;
		for (let v in (set.mode == 'trunk' ? set.tagged ?? [] : []))
			need['' + v] = true;
	}
	for (let s in of_type(n, 'interface')) {
		let m = match(s['.name'], /^aeolus_vlan([0-9]+)_tunnels$/);
		if (m)
			need[m[1]] = true;
	}
	for (let vlan in sort(keys(need), (x, y) => +x - +y)) {
		let entry = null;
		for (let v in of_type(n, 'bridge-vlan'))
			if (v.device == up.name && v.vlan == vlan)
				for (let e in list(v.ports))
					if (split(e, ':')[0] == facts.uplink)
						entry = split(e, ':')[1] ?? '';
		if (entry == null)
			continue;
		put(a, 'aeolus_watch' + vlan, 'watch', {
			vlan: vlan, device: facts.uplink, tagged: index(entry, 't') >= 0 ? 1 : 0,
			mac: segment_mac(facts.ap, 'vlan', +vlan) || null,
		});
		keep['aeolus_watch' + vlan] = true;
	}
}

// rrm turns radio resource management on (0073): the agent's daemon then
// advertises this AP in its beacons, keeps neighbours with the others, and
// moves its radios as the policy says, defaults written out. Off, there is
// no section, and the daemon stays idle. It stays off on an AP with a scan
// radio (0081): its scans are the serving radios' own, which there never scan.
function rrm(a, w, intent, facts, keep) {
	let r = intent.rrm;
	if (r?.enabled != true || scan_radio(w))
		return;
	// Power control (0077) runs in the same daemon, with RRM's neighbours.
	let p = intent.apc?.enabled == true ? intent.apc : null;
	put(a, 'aeolus_rrm', 'rrm', {
		enabled: 1, ap: facts.ap || null, moves: r.moves == false ? 0 : 1,
		window: r.window ?? '02:00-05:00', margin: r.margin ?? 20,
		apc: p ? 1 : null, apc_neighbours: p ? p.neighbours ?? 3 : null, apc_target: p ? p.target ?? -70 : null,
	});
	keep['aeolus_rrm'] = true;
}

// host_port splits "host", "host:port", "[v6]" or "[v6]:port".
function host_port(s) {
	let m = match(s, /^\[([^\]]+)\](:([0-9]+))?$/);
	if (m)
		return [m[1], m[3]];
	m = match(s, /^([^:]+):([0-9]+)$/);
	return m ? [m[1], m[2]] : [s, null];
}

function system(pkg, intent, facts) {
	let want = intent.system ?? {};
	let s = of_type(pkg, 'system')[0];
	// The AP's name in Aeolus is its hostname (0076).
	if (s && intent.ap?.hostname)
		s.hostname = intent.ap.hostname;
	if (s && want.tz != null) {
		s.zonename = want.tz;
		if (facts.timezone)
			s.timezone = facts.timezone;
	}
	if (s && want.syslog != null) {
		let hp = host_port(want.syslog);
		s.log_ip = hp[0];
		if (hp[1])
			s.log_port = hp[1];
		else
			delete s.log_port;
	}
	if (want.ntp != null) {
		if (pkg.ntp?.['.type'] != 'timeserver')
			put(pkg, 'ntp', 'timeserver', {});
		pkg.ntp.server = map(want.ntp, v => '' + v);
	}
	else if (pkg.ntp?.['.type'] == 'timeserver' && pkg.ntp.server != null) {
		// Never OpenWrt's internet pool (0069): without time servers of
		// its own, the AP takes those its DHCP gives it.
		let left = filter(type(pkg.ntp.server) == 'array' ? pkg.ntp.server : [pkg.ntp.server], v => !match(v, POOL));
		if (length(left))
			pkg.ntp.server = left;
		else
			delete pkg.ntp.server;
	}
}

// rebind lets the AP resolve to a private address each name Aeolus uses on
// it (0069): the manager's host, the time servers and the syslog host.
// dnsmasq's rebind protection drops a private answer to any other name. The
// names it adds are kept in its own package, so one no longer used is taken
// out again; the rest of the list is the site's, and left alone.
function rebind(cfg, intent) {
	let want = [];
	// The manager's host, from the agent's URL: scheme://host[:port]/...
	let url = cfg.aeolus.agent?.url ?? '', at = index(url, '://');
	if (at >= 0) {
		let host = substr(url, at + 3);
		for (let sep in ['/', ':', '?'])
			if (index(host, sep) >= 0)
				host = substr(host, 0, index(host, sep));
		push(want, host);
	}
	for (let v in intent.system?.ntp ?? [])
		push(want, '' + v);
	if (intent.system?.syslog != null)
		push(want, host_port(intent.system.syslog)[0]);
	want = uniq(filter(want, n => match(n, /^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*$/) && !match(n, /^[0-9.]+$/)));
	let d = of_type(cfg.dhcp, 'dnsmasq')[0];
	let agent = cfg.aeolus.agent;
	let had = agent?.rebind ?? [];
	if (type(had) != 'array')
		had = [had];
	if (!d) {
		if (agent)
			delete agent.rebind;
		return;
	}
	let list = d.rebind_domain ?? [];
	if (type(list) != 'array')
		list = [list];
	list = filter(list, n => !(n in had) || n in want);
	for (let n in want)
		if (!(n in list))
			push(list, n);
	if (length(list))
		d.rebind_domain = list;
	else
		delete d.rebind_domain;
	if (agent) {
		if (length(want))
			agent.rebind = want;
		else
			delete agent.rebind;
	}
}

// agent renders the settings meant for the agent itself (0040).
function agent(pkg, intent) {
	let poll = intent.system?.poll;
	if (poll == null)
		return;
	if (pkg.agent?.['.type'] != 'agent')
		put(pkg, 'agent', 'agent', {});
	pkg.agent.poll = '' + poll;
}

// steering turns band steering on for the networks that ask for it, through
// usteer, which the agent's installer adds (0050). Aeolus owns two of its
// options: band_steering_interval, 0 while no network asks, so usteer
// steers nothing on its own, and ssid_list.
function steering(u, intent, errors) {
	let ssids = [];
	for (let id in sort(keys(intent.network ?? {}))) {
		let net = intent.network[id];
		if (net.band_steering && net.enabled !== false)
			push(ssids, net.ssid);
	}
	let s = of_type(u, 'usteer')[0];
	if (!s) {
		if (length(ssids))
			push(errors, 'band steering needs usteer, which is not installed on this AP');
		return;
	}
	s.band_steering_interval = length(ssids) ? '30000' : '0';
	if (length(ssids))
		s.ssid_list = uniq(sort(ssids));
	else
		delete s.ssid_list;
}

// snmp renders snmpd's config (0052). Aeolus owns all of it, as the agent's
// installer added snmpd: it answers read-only, with the intent's v2c
// community, its v3 user (SHA and AES), or both, and while SNMP is off the
// config is only that it is off, so none of OpenWrt's default communities
// is left behind.
function snmp(cfg, intent, facts, errors) {
	let want = intent.system?.snmp ?? {};
	if (!length(keys(cfg.snmpd ?? {}))) {
		if (want.enabled)
			push(errors, 'SNMP needs snmpd, which is not installed on this AP');
		return;
	}
	let pkg = {};
	if (want.enabled) {
		put(pkg, 'agent', 'agent', { agentaddress: 'UDP:161,UDP6:161' });
		if (want.community != null) {
			put(pkg, 'aeolus_v2c', 'com2sec', { secname: 'ro', source: 'default', community: want.community });
			put(pkg, 'aeolus_v2c6', 'com2sec6', { secname: 'ro', source: 'default', community: want.community });
			put(pkg, 'aeolus_ro_v2c', 'group', { group: 'ro', version: 'v2c', secname: 'ro' });
			put(pkg, 'all', 'view', { viewname: 'all', type: 'included', oid: '.1' });
			put(pkg, 'aeolus_ro', 'access', {
				group: 'ro', context: 'none', version: 'any', level: 'noauth', prefix: 'exact',
				read: 'all', write: 'none', notify: 'none',
			});
		}
		put(pkg, 'system', 'system', { sysLocation: want.location, sysContact: want.contact });
		if (facts.uplink)
			put(pkg, 'engineid', 'engineid', { engineidtype: 3, engineidnic: facts.uplink });
		if (want.v3?.user != null)
			put(pkg, 'aeolus_v3', 'v3', {
				username: want.v3.user, auth_type: 'SHA', auth_pass: want.v3.auth,
				privacy_type: 'AES', privacy_pass: want.v3.privacy, allow_write: 0,
			});
	}
	let versions = want.community != null ? (want.v3?.user != null ? 'v1/v2c/v3' : 'v1/v2c') : 'v3';
	put(pkg, 'general', 'snmpd', { enabled: want.enabled ? 1 : 0, snmp_version: want.enabled ? versions : null });
	cfg.snmpd = pkg;
}

// clamp makes the MSS clamp's nftables file from the rendered network
// config (0054): on each tunnel's bridge whose MTU is below 1500, TCP's MSS
// is held to the MTU less 40 for IPv4 and less 60 for IPv6. It replaces the
// whole table each time fw4 loads it. With no such tunnel, the table is
// empty.
function clamp(network, aeolus) {
	let rules = [];
	let rule = (bridge, mtu) => {
		for (let fam in [['ip', 40], ['ip6', 60]])
			push(rules, sprintf('\t\tmeta ibrname "%s" ether type %s tcp flags & (syn | rst) == syn tcp option maxseg size > %d tcp option maxseg size set %d',
				bridge, fam[0], mtu - fam[1], mtu - fam[1]));
	};
	let mtu_of = {};
	for (let s in of_type(network ?? {}, 'interface')) {
		let vni = match(s['.name'], /^aeolus_([0-9]+)$/)?.[1];
		let mtu = int(s.mtu ?? '1500');
		if (!vni || !(s.proto in { vxlan: 1, vxlan6: 1 }) || mtu >= 1500)
			continue;
		mtu_of[vni] = mtu;
		if (network['aeolus_' + vni + '_br'])
			rule('br-vx' + vni, mtu);
	}
	// A network with a fallback carries its tunnel in its own bridge (0061).
	for (let sw in of_type(aeolus ?? {}, 'switch'))
		for (let slot in ['primary', 'fallback'])
			if (mtu_of[sw[slot + '_vni']])
				rule(sw.bridge, mtu_of[sw[slot + '_vni']]);
	return join('\n', [
		'# Made by the Aeolus agent from its VXLAN tunnels (0054); fw4 loads it.',
		'table bridge aeolus',
		'delete table bridge aeolus',
		'table bridge aeolus {',
		'\tchain forward {',
		'\t\ttype filter hook forward priority filter; policy accept;',
		...rules,
		'\t}',
		'}',
		'',
	]);
}

// render returns the new packages, the names of those that changed, and
// what it could not render. facts: { uplink, management: the interface the
// AP reaches the manager through, radios: { <radio>: { htmodes } },
// timezone: the POSIX string for intent's time zone, vxlan: whether netifd
// has loaded the vxlan package, nft_bridge: whether kmod-nft-bridge is
// installed, bss_transition: whether hostapd has 802.11v, null if not known,
// ap: the AP's ID, which its segment MACs are made from (0060), prober:
// whether the prober can run, which a network with a fallback needs, and
// veth: whether kmod-veth is installed, for its VLAN transports (0061) }.
function render(intent, current, facts) {
	let cfg = {};
	for (let p in PACKAGES)
		cfg[p] = clone(current[p] ?? {});
	let errors = [];
	let keep = {};
	radios(cfg.wireless, intent, facts ?? {});
	networks(cfg, intent, facts ?? {}, errors, keep);
	ports(cfg, intent, facts ?? {}, errors, keep);
	hold_bridges(cfg.network, keep);
	// What Aeolus made earlier and no longer needs goes. The prober's plan
	// follows what is left, and then goes the same way.
	for (let pkg in [cfg.wireless, cfg.network, cfg.firewall])
		for (let k in keys(pkg))
			if (owned(k) && !keep[k] && pkg[k]['.type'] != 'wifi-station')
				delete pkg[k];
	probes(cfg, intent, facts ?? {}, keep);
	watches(cfg, intent, facts ?? {}, keep);
	rrm(cfg.aeolus, cfg.wireless, intent, facts ?? {}, keep);
	for (let k in keys(cfg.aeolus))
		if (owned(k) && !keep[k])
			delete cfg.aeolus[k];
	system(cfg.system, intent, facts ?? {});
	agent(cfg.aeolus, intent);
	rebind(cfg, intent);
	steering(cfg.usteer, intent, errors);
	snmp(cfg, intent, facts ?? {}, errors);
	let changed = filter(PACKAGES, p => text(p, cfg[p]) != text(p, current[p] ?? {}));
	return { config: cfg, changed: changed, errors: errors };
}

// without_keys is a wireless package without the key agent's wifi-station
// sections (0070): keys are not config, and never go to the render check.
function without_keys(pkg) {
	let out = {};
	for (let k, v in pkg ?? {})
		if (v['.type'] != 'wifi-station')
			out[k] = v;
	return out;
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export { PACKAGES, CLAMP, VLAN_END, iface_name, render, clamp, without_keys };
