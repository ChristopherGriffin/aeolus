// Words for fields, values and times.

import { h, icon } from './dom.js';

const BANDS = { '2g': '2.4 GHz', '5g': '5 GHz', '6g': '6 GHz' };

const SECURITY = {
	'open': 'Open', 'owe': 'OWE (encrypted open)', 'wpa2-psk': 'WPA2',
	'wpa3-sae': 'WPA3', 'wpa2-wpa3': 'WPA2/WPA3',
};

const SYSTEM = {
	'country': 'Country', 'tz': 'Time zone', 'ntp': 'NTP servers', 'poll': 'Poll interval',
	'syslog': 'Syslog', 'ssh_keys': 'SSH keys',
	'management.vlan': 'VLAN', 'management.addressing': 'Addressing', 'management.address': 'Address',
	'management.gateway': 'Gateway', 'management.dns': 'DNS',
};

const SNMP = {
	'enabled': 'Answer SNMP', 'community': 'v2c community', 'v3.user': 'v3 user',
	'v3.auth': 'v3 auth passphrase (SHA)', 'v3.privacy': 'v3 privacy passphrase (AES)',
	'location': 'Location (sysLocation)', 'contact': 'Contact (sysContact)',
};

const RADIO = { enabled: 'Radio on', channel: 'Channel', width: 'Width', power: 'Power' };

const PORT = { enabled: 'Port on', uplink: 'Uplink', mode: 'Mode', untagged: 'Untagged VLAN', tagged: 'Tagged VLANs', bond: 'LACP bond' };

const MODE = { access: 'Access', trunk: 'Trunk', tunnel: 'Tunnel', lacp: 'LACP' };

const NETWORK = {
	'ssid': 'SSID', 'security': 'Security', 'passphrase': 'Passphrase', 'hidden': 'Hidden',
	'bands': 'Bands', 'isolation': 'Client isolation', 'enabled': 'Broadcast',
	'multicast_to_unicast': 'Multicast to unicast', 'band_steering': 'Band steering',
	'roaming.ft': 'Fast roaming (11r)', 'roaming.rrm': 'Neighbor reports (11k)', 'roaming.btm': 'Steering (11v)',
	'rate_limit.down_kbps': 'Download limit', 'rate_limit.up_kbps': 'Upload limit',
	'transport.switching': 'Switching', 'transport.ha': 'HA mode', 'transport.failback': 'Failback', 'transport.holddown': 'Hold-down (s)',
};

const SLOT = { type: 'type', vlan: 'VLAN', concentrator: 'tunnel', vni: 'VNI', probe: 'probe address' };

const TUNNEL = { address: 'Far end (IP)', port: 'Port', mtu: 'MTU', probe_interval: 'Probe interval', underlay_vlan: 'Starts from' };

// A tunnel port's VNI's fields (0058, 0059).
const PORT_VNI = { tunnel: 'tunnel', vni: 'VNI', probe: 'probe address' };

// group says which panel a field belongs in, and its label there.
export function group(path) {
	const p = path.split('.');
	if (p[0] === 'radio') return { key: 'radio.' + p[1], title: (BANDS[p[1]] || p[1]) + ' radio', order: 1 + Object.keys(BANDS).indexOf(p[1]) / 10, label: RADIO[p[2]] || p[2] };
	if (p[0] === 'system' && p[1] === 'snmp') return { key: 'snmp', title: 'SNMP', order: 2.5, label: SNMP[p.slice(2).join('.')] || p.slice(2).join('.') };
	if (p[0] === 'system' && p[1] === 'management') return { key: 'management', title: 'Management', order: 3, label: SYSTEM[p.slice(1).join('.')] || p[2] };
	if (p[0] === 'system') return { key: 'system', title: 'System', order: 2, label: SYSTEM[p[1]] || p[1] };
	// A tunnel port's VNI, by how it is carried: "VLAN 50 tunnel" (0058).
	if (p[0] === 'ports' && p[2] === 'vxlan') return { key: 'ports.' + p[1], title: 'Port ' + p[1], order: 4, label: `${p[3] === 'untagged' ? 'Untagged' : 'VLAN ' + p[3]} ${PORT_VNI[p[4]] || p[4]}` };
	if (p[0] === 'ports') return { key: 'ports.' + p[1], title: 'Port ' + p[1], order: 4, label: PORT[p[2]] || p[2] };
	if (p[0] === 'concentrators') return { key: 'concentrators.' + p[1], title: 'Tunnel ' + p[1], order: 4.5, label: TUNNEL[p[2]] || p[2] };
	if (p[0] === 'services') return { key: 'services', title: 'Services', order: 5, label: 'Service folders' };
	if (p[0] === 'network') {
		const f = p.slice(2).join('.');
		let label = NETWORK[f];
		if (!label && p[2] === 'transport') label = (p[3] === 'primary' ? 'Primary ' : 'Fallback ') + (SLOT[p[4]] || p[4]);
		return { key: 'network.' + p[1], title: p[1], order: 10, label: label || f, network: p[1] };
	}
	return { key: p[0], title: p[0], order: 20, label: p.slice(1).join('.') || p[0] };
}

// value writes a field's value for people. Secrets are never readable
// (0027); the API sends them as {sealed: true}.
export function value(path, v, names) {
	if (v && typeof v === 'object' && v.sealed === true) return h('span', { class: 'sealed' }, 'sealed');
	if (typeof v === 'boolean') return v ? 'on' : 'off';
	const last = path.split('.').pop();
	if (path === 'services' && Array.isArray(v)) return v.map((id) => names(id)).join(' + ') || 'none';
	if (Array.isArray(v)) return v.map((x) => (last === 'bands' ? BANDS[x] || x : String(x))).join(', ');
	if (last === 'security') return SECURITY[v] || v;
	if (last === 'width' && typeof v === 'number') return v + ' MHz';
	if (last === 'channel' && v === 'auto') return path.includes('.2g.') ? 'automatic (1, 6, 11)' : 'automatic';
	if (last === 'power' && typeof v === 'number') return v + ' dBm';
	if (last === 'poll' || last === 'holddown' || last === 'probe_interval') return v + ' s';
	if (last.endsWith('_kbps')) return v === 0 ? 'no limit' : v + ' kbps';
	if (last === 'type' && path.includes('.transport.')) return v === 'vxlan' ? 'VXLAN' : 'VLAN';
	if (path.startsWith('ports.') && last === 'mode') return MODE[v] || v;
	if (path.startsWith('ports.') && last === 'untagged' && v === 0) return 'none';
	// Where a tunnel starts on the AP (0063): 0 is the management VLAN.
	if (last === 'underlay_vlan' && typeof v === 'number') return v ? 'VLAN ' + v : 'the management VLAN';
	if (v == null) return '—';
	return String(v);
}

export function security(v) {
	return SECURITY[v] || v;
}

// origin shows where a value comes from (0012), as in the mockup.
export function origin(tree, here, r, names) {
	const from = r.from;
	const to = (label, cls) => h('a', { class: 'chip ' + cls, href: `#/${tree}/${encodeURIComponent(from)}` }, label);
	switch (r.origin) {
	case 'self':
		return h('span', { class: 'chip here' }, 'Set here');
	case 'inherited':
		return to('From ' + names(from), 'from');
	case 'locked':
		return from === here
			? h('span', { class: 'chip lockhere' }, icon('lock'), 'Locked here')
			: to('Locked by ' + names(from), 'locked');
	case 'baseline':
		return to('Branch baseline · ' + names(from), 'baseline');
	}
	return null;
}

export function when(t) {
	if (!t) return 'never';
	return new Date(t).toLocaleString();
}

// What changes only what the prober asks, and how often, reloads nothing: it is
// the agent's own config, which the prober reads again within 10 seconds (0059).
export const PROBE_ONLY = 'Nothing reloads on the APs: each one\'s prober picks this up within 10 seconds.';

// probeOnly says whether paths change only what the prober asks and how
// often.
export function probeOnly(paths) {
	return paths.length > 0 && paths.every((p) => /\.(probe|probe_interval)$/.test(p));
}

// secondsAgo writes as ago does the time s seconds before at, such as an
// AP's report counting from when it was made.
export function secondsAgo(s, at) {
	return ago(new Date(at ?? Date.now()).getTime() - s * 1000);
}

export function ago(t) {
	if (!t) return 'never';
	const s = Math.round((Date.now() - new Date(t).getTime()) / 1000);
	if (s < 10) return 'just now';
	if (s < 60) return s + ' s ago';
	if (s < 3600) return Math.round(s / 60) + ' min ago';
	if (s < 86400) return Math.round(s / 3600) + ' h ago';
	return Math.round(s / 86400) + ' d ago';
}

export function bandName(b) {
	return BANDS[b] || b;
}

