// Rendering an AP's intent into UCI (0006, 0008, 0040).
//
// render() is pure: it takes the composed config from the manager, the AP's
// current UCI and a few local facts, and returns the new UCI. It never reads
// or writes files, so the same code runs on the AP and in CI, where its output
// is fed to the manager's render check.
//
// Aeolus only touches what it owns (0040): the radio options intent names,
// sections named aeolus_, and the time zone, NTP and syslog settings. A
// section it did not create is never edited or removed.

'use strict';

import { text } from 'aeolus.uciexport';

const PACKAGES = ['wireless', 'network', 'system', 'aeolus'];

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
// bridge-vlan that already exists so a VLAN is never defined twice (0040).
function ensure_vlan(n, bridge, uplink, vlan, keep) {
	let name = 'aeolus_vlan' + vlan;
	for (let v in of_type(n, 'bridge-vlan'))
		if (v.device == bridge && v.vlan == '' + vlan && v['.name'] != name)
			return;
	put(n, name, 'bridge-vlan', { device: bridge, vlan: vlan, ports: [uplink + ':t'] });
	keep[name] = true;
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
	if (net.roaming?.rrm)
		o.ieee80211k = '1';
	if (net.roaming?.btm)
		o.bss_transition = '1';
	return o;
}

function networks(cfg, intent, facts, errors) {
	let w = cfg.wireless, n = cfg.network;
	let keep = {};
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
	for (let pkg in [w, n])
		for (let k in keys(pkg))
			if (owned(k) && !keep[k])
				delete pkg[k];
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

// render returns the new packages, the names of those that changed, and
// what it could not render. facts: { uplink, radios: { <radio>: { htmodes } },
// timezone: the POSIX string for intent's time zone }.
function render(intent, current, facts) {
	let cfg = {};
	for (let p in PACKAGES)
		cfg[p] = clone(current[p] ?? {});
	let errors = [];
	radios(cfg.wireless, intent, facts ?? {});
	networks(cfg, intent, facts ?? {}, errors);
	system(cfg.system, intent, facts ?? {});
	agent(cfg.aeolus, intent);
	let changed = filter(PACKAGES, p => text(p, cfg[p]) != text(p, current[p] ?? {}));
	return { config: cfg, changed: changed, errors: errors };
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export { PACKAGES, iface_name, render };
