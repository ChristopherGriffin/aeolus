// An AP's own Wi-Fi networks (0137): those set on the AP itself and not by
// Aeolus, which leaves them as they are (0040). What there is to say of each
// without its secrets, for the manager to show beside its own; which section
// one is, where it is asked for; and that section as text, to be kept before
// it is taken out.
//
// Pure: the packages are what uci's get_all() returns, so the same code
// runs on the AP and in CI.

'use strict';

import { file } from 'aeolus.uciexport';

const MOST = 32;   // as many are said of an AP

// mine says a section is Aeolus's (0039).
function mine(name) {
	return substr(name ?? '', 0, 7) == 'aeolus_';
}

// names is an option that names several, as a list or with spaces between.
function names(v) {
	return filter(type(v) == 'array' ? v : split(v ?? '', /[ \t]+/), x => length(x) > 0);
}

// vlan is the VLAN a network of the AP's is on, where its device says: a
// VLAN device with its number, or one named for it, as br-lan.20. Nothing
// where it does not, as on the AP's own LAN.
function vlan(network, name) {
	let dev = network?.[name]?.device ?? network?.[name]?.ifname;
	if (type(dev) != 'string')
		return null;
	let id = null;
	for (let k, d in network)
		if (d['.type'] == 'device' && d.name == dev && d.type in ['8021q', '8021ad'] && match(d.vid ?? '', /^[0-9]+$/))
			id = int(d.vid);
	if (id == null) {
		let m = match(dev, /^[^.]+\.([0-9]+)$/);
		if (m)
			id = int(m[1]);
	}
	return id != null && id >= 1 && id <= 4094 ? id : null;
}

// theirs says a section is a Wi-Fi network of the AP's own: an access
// point's, and not Aeolus's. A station's, a mesh's or a monitor's is not a
// network clients join, and may be what the AP itself is reached by.
function theirs(s) {
	return type(s) == 'object' && s['.type'] == 'wifi-iface' && !mine(s['.name']) && (s.mode ?? 'ap') == 'ap';
}

// list is the AP's own networks, a radio of each at a time, in the order
// its config has them: {section, radio, band, ssid, security, clients,
// vlan?, hidden?, off?, ft?}. security is the encryption as OpenWrt names
// it. clients(section, radio) says how many clients one has now. Nothing of
// a key, and nothing on a radio another service owns (0081).
function list(wireless, network, clients) {
	let out = [];
	let secs = sort(values(wireless ?? {}), (a, b) => (a['.index'] ?? 0) - (b['.index'] ?? 0));
	for (let s in secs) {
		if (!theirs(s))
			continue;
		for (let r in names(s.device)) {
			let d = wireless[r];
			if (d?.['.type'] != 'wifi-device' || d.airscan == '1')
				continue;
			let e = { section: s['.name'], radio: r, band: d.band ?? '', ssid: s.ssid ?? '', security: s.encryption ?? 'none', clients: 0 };
			let id = vlan(network, names(s.network)[0]);
			if (id != null)
				e.vlan = id;
			if (s.hidden == '1')
				e.hidden = true;
			if (s.disabled == '1' || d.disabled == '1')
				e.off = true;
			else if (clients)
				e.clients = clients(s['.name'], r) ?? 0;
			if (s.ieee80211r == '1')
				e.ft = true;
			push(out, e);
			if (length(out) >= MOST)
				return out;
		}
	}
	return out;
}

// find is the section of one of the AP's own networks, by its name and the
// SSID it is to have: asked for by both, as a section with no name of its
// own is named for where it stands, and another may stand there since.
function find(wireless, section, ssid) {
	let s = wireless?.[section];
	return theirs(s) && (s.ssid ?? '') == (ssid ?? '') ? s : null;
}

// kept is a section as text, as it stands in /etc/config/wireless, under a
// line that says what it is: to be kept before the section is taken out,
// and put back by hand. It has the network's key in it.
function kept(s, when, why) {
	return sprintf('# %s, %s. To have it back, add what follows to /etc/config/wireless, and run: wifi reload\n', why, when) +
		file({ [s['.name']]: s });
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export { MOST, list, find, kept };
