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
// config (0052), sections named aeolus_, and the time zone, NTP and syslog
// settings. A section it did not create is never otherwise edited, or
// removed.

'use strict';

import { text } from 'aeolus.uciexport';

const PACKAGES = ['wireless', 'network', 'system', 'aeolus', 'usteer', 'snmpd'];

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

function radios(w, intent, facts) {
	let country = intent.system?.country;
	for (let s in of_type(w, 'wifi-device')) {
		let set = intent.radio?.[s.band] ?? {};
		if (country != null)
			s.country = country;
		if (set.enabled != null)
			s.disabled = set.enabled ? '0' : '1';
		if (set.channel != null) {
			s.channel = '' + set.channel;
			// An automatic 2.4 GHz channel is one of 1, 6 and 11, the only
			// ones that do not overlap (0045).
			if (set.channel == 'auto' && s.band == '2g')
				s.channels = ['1', '6', '11'];
			else
				delete s.channels;
		}
		if (set.width != null)
			s.htmode = htmode(s.band, set.width, s.htmode, facts.radios?.[s['.name']]?.htmodes);
		if (set.power == 'auto')
			delete s.txpower;
		else if (set.power != null)
			s.txpower = '' + set.power;
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

function iface_options(net, radio, network) {
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
	if (net.roaming?.btm || net.band_steering)
		o.bss_transition = '1';
	if (net.multicast_to_unicast != null)
		o.multicast_to_unicast = net.multicast_to_unicast ? '1' : '0';
	return o;
}

function networks(cfg, intent, facts, errors, keep) {
	let w = cfg.wireless, n = cfg.network;
	let bridge = null;
	let nets = intent.network ?? {};
	for (let id in sort(keys(nets))) {
		let net = nets[id];
		if (net.enabled === false)
			continue;
		let iface = interface_name(id);
		let vlan = null;
		for (let slot in ['primary', 'fallback']) {
			let t = net.transport?.[slot];
			if (!t)
				continue;
			if (t.type != 'vlan') {
				push(errors, `network.${id}.transport.${slot}: ${t.type} transports come in M5 part 2`);
				continue;
			}
			bridge ??= uplink_bridge(n, facts.uplink, errors);
			if (!bridge)
				continue;
			ensure_vlan(n, bridge, facts.uplink, t.vlan, keep);
			vlan ??= t.vlan;
		}
		if (vlan != null) {
			put(n, iface, 'interface', { proto: 'none', device: `${bridge}.${vlan}` });
			keep[iface] = true;
		}
		for (let d in of_type(w, 'wifi-device')) {
			if (net.bands && index(net.bands, d.band) < 0)
				continue;
			let name = iface_name(id, d['.name']);
			put(w, name, 'wifi-iface', iface_options(net, d['.name'], iface));
			keep[name] = true;
		}
	}
}

// ports applies the intent's Ethernet port settings (0053): a port's VLANs,
// as its entries in the bridge's bridge-vlan sections, and whether it is on.
// Only ports in the uplink's bridge count; the uplink itself is the AP's
// management and is left alone. Each VLAN a port uses is tagged on the
// uplink as well. LACP is not applied yet; the manager holds it.
//
// A setting left unset leaves the port as it is, as with a radio, so a VLAN
// Aeolus added stays while a port is still on it.
function ports(n, intent, facts, errors, keep) {
	let want = intent.ports ?? {};
	let bridge = length(keys(want)) ? uplink_bridge(n, facts.uplink, errors) : null;
	let members = list(filter(of_type(n, 'device'), d => d.name == bridge)[0]?.ports);
	for (let p in (bridge ? sort(keys(want)) : [])) {
		let set = want[p];
		if (index(members, p) < 0)
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
		if (set.mode != 'access' && set.mode != 'trunk')
			continue;
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
		if (substr(d['.name'], 0, 12) == 'aeolus_port_')
			keep[d['.name']] = true;
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

// render returns the new packages, the names of those that changed, and
// what it could not render. facts: { uplink, radios: { <radio>: { htmodes } },
// timezone: the POSIX string for intent's time zone }.
function render(intent, current, facts) {
	let cfg = {};
	for (let p in PACKAGES)
		cfg[p] = clone(current[p] ?? {});
	let errors = [];
	let keep = {};
	radios(cfg.wireless, intent, facts ?? {});
	networks(cfg, intent, facts ?? {}, errors, keep);
	ports(cfg.network, intent, facts ?? {}, errors, keep);
	// What Aeolus made earlier and no longer needs goes.
	for (let pkg in [cfg.wireless, cfg.network])
		for (let k in keys(pkg))
			if (owned(k) && !keep[k])
				delete pkg[k];
	system(cfg.system, intent, facts ?? {});
	agent(cfg.aeolus, intent);
	steering(cfg.usteer, intent, errors);
	snmp(cfg, intent, facts ?? {}, errors);
	let changed = filter(PACKAGES, p => text(p, cfg[p]) != text(p, current[p] ?? {}));
	return { config: cfg, changed: changed, errors: errors };
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export { PACKAGES, iface_name, render };
