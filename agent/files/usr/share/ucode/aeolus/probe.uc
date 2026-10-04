// The prober's frames, and how it judges a tunnel (0059); the MAC an AP
// uses on each segment, and the DHCP it takes its address there with (0060).
//
// Pure: it builds and reads frames, and never touches a socket or a file, so
// the same code runs on the AP and in CI (agent/test/probe.uc).

'use strict';

// The loop guard's frames: a locally administered multicast address, IEEE's
// local experimental EtherType, and a mark of Aeolus's own.
const GUARD_DST = '03:ae:01:00:00:59';
const GUARD_TYPE = 0x88b5;
const GUARD_MARK = 'aeolus-loop';

// What an echo carries, so a reply is easy to tell apart.
const PAYLOAD = 'aeolus59';

// OVERLAY_FILTER keeps, of everything on a tunnel device, only what answers
// a probe: incoming ARP replies, incoming ICMPv6 echo replies, and incoming
// DHCP answers (IPv4 UDP to port 68, 0060). The rest of the tunnel's
// traffic, clients' included, never reaches the prober. Classic BPF,
// [code, jt, jf, k]; jumps count from the next instruction.
const OVERLAY_FILTER = [
	[0x20, 0, 0, 0xfffff004],   //  0  A = packet type
	[0x15, 14, 0, 4],           //  1  outgoing: drop
	[0x28, 0, 0, 12],           //  2  A = EtherType
	[0x15, 0, 2, 0x0806],       //  3  not ARP: to 6
	[0x28, 0, 0, 20],           //  4  A = ARP operation
	[0x15, 11, 10, 2],          //  5  a reply: keep; else drop
	[0x15, 0, 4, 0x86dd],       //  6  not IPv6: to 11
	[0x30, 0, 0, 20],           //  7  A = next header
	[0x15, 0, 7, 58],           //  8  not ICMPv6: drop
	[0x30, 0, 0, 54],           //  9  A = ICMPv6 type
	[0x15, 6, 5, 129],          // 10  an echo reply: keep; else drop
	[0x15, 0, 4, 0x0800],       // 11  not IPv4: drop
	[0x30, 0, 0, 23],           // 12  A = IP protocol
	[0x15, 0, 2, 17],           // 13  not UDP: drop
	[0x28, 0, 0, 36],           // 14  A = UDP destination port
	[0x15, 1, 0, 68],           // 15  DHCP's client port: keep
	[0x06, 0, 0, 0],            // 16  drop
	[0x06, 0, 0, 0x40000],      // 17  keep
];

// GUARD_FILTER keeps incoming loop guard frames, on any device.
const GUARD_FILTER = [
	[0x20, 0, 0, 0xfffff004],   //  0  A = packet type
	[0x15, 2, 0, 4],            //  1  outgoing: drop
	[0x28, 0, 0, 12],           //  2  A = EtherType
	[0x15, 1, 0, 0x88b5],       //  3  a guard frame: keep
	[0x06, 0, 0, 0],            //  4  drop
	[0x06, 0, 0, 0x40000],      //  5  keep
];

// WATCH_FILTER keeps, of what comes in on the uplink, what tells which VLANs
// reach the AP (0064): broadcasts and multicasts, which ARP, DHCP, OSPF,
// mDNS and IPv6 neighbour discovery use, and frames to the AP's own MACs on
// the VLANs (06:…, 0060), where the answers to a nudge come. Clients' unicast
// traffic is not read.
const WATCH_FILTER = [
	[0x20, 0, 0, 0xfffff004],   //  0  A = packet type
	[0x15, 4, 0, 4],            //  1  outgoing: drop
	[0x30, 0, 0, 0],            //  2  A = the first byte of the destination
	[0x45, 1, 0, 1],            //  3  a broadcast or multicast: keep
	[0x15, 0, 1, 6],            //  4  one of the AP's VLAN MACs: keep; else drop
	[0x06, 0, 0, 0x40000],      //  5  keep
	[0x06, 0, 0, 0],            //  6  drop
];

// LLDP_FILTER keeps incoming LLDP frames: what the switch says of itself and
// of the uplink's port (0064).
const LLDP_FILTER = [
	[0x20, 0, 0, 0xfffff004],   //  0  A = packet type
	[0x15, 2, 0, 4],            //  1  outgoing: drop
	[0x28, 0, 0, 12],           //  2  A = EtherType
	[0x15, 1, 0, 0x88cc],       //  3  LLDP: keep
	[0x06, 0, 0, 0],            //  4  drop
	[0x06, 0, 0, 0x40000],      //  5  keep
];

// DHCP_FILTER keeps, of what crosses a Wi-Fi interface, what tells how its
// clients do with DHCP (0065): DHCP itself, both ways (IPv4 UDP from or to
// port 67 or 68, but a later fragment), and ARP coming in from a client,
// which shows the address a client uses.
const DHCP_FILTER = [
	[0x28, 0, 0, 12],           //  0  A = EtherType
	[0x15, 0, 2, 0x0806],       //  1  not ARP: to 4
	[0x20, 0, 0, 0xfffff004],   //  2  A = packet type
	[0x15, 13, 12, 4],          //  3  outgoing: drop; else keep
	[0x15, 0, 12, 0x0800],      //  4  not IPv4: drop
	[0x30, 0, 0, 23],           //  5  A = IP protocol
	[0x15, 0, 10, 17],          //  6  not UDP: drop
	[0x28, 0, 0, 20],           //  7  A = flags and fragment offset
	[0x45, 8, 0, 0x1fff],       //  8  a later fragment: drop
	[0xb1, 0, 0, 14],           //  9  X = the IP header's length
	[0x48, 0, 0, 14],           // 10  A = UDP source port
	[0x15, 4, 0, 67],           // 11  from a server's port: keep
	[0x15, 3, 0, 68],           // 12  from a client's port: keep
	[0x48, 0, 0, 16],           // 13  A = UDP destination port
	[0x15, 1, 0, 67],           // 14  to a server's port: keep
	[0x15, 0, 1, 68],           // 15  to a client's port: keep; else drop
	[0x06, 0, 0, 0x40000],      // 16  keep
	[0x06, 0, 0, 0],            // 17  drop
];

// How long, in seconds, the DHCP watch waits and remembers (0065): an answer
// to a request; a client that joined, before it is judged not to use DHCP;
// the window unanswered requests are counted in; and how long servers and
// duplicates are remembered.
const DHCP_WAIT = 10;
const DHCP_QUIET = 60;
const DHCP_WINDOW = 600;
const DHCP_MEMORY = 3600;

// WATCH_SPAN is how long a watched VLAN may go unheard before it is silent,
// in seconds: three of the prober's spells of listening (0064).
const WATCH_SPAN = 180;

function bytes(list) {
	return join('', map(list, b => chr(b)));
}

function u16(n) {
	return chr((n >> 8) & 255) + chr(n & 255);
}

function get16(s, at) {
	return ord(s, at) << 8 | ord(s, at + 1);
}

function u32(n) {
	return u16((n >> 16) & 0xffff) + u16(n & 0xffff);
}

function get32(s, at) {
	return get16(s, at) << 16 | get16(s, at + 2);
}

function pad(f) {
	while (length(f) < 60)
		f += chr(0);
	return f;
}

function mac(text) {
	return bytes(map(split(text, ':'), x => hex(x)));
}

function mac_text(s, at) {
	return join(':', map([0, 1, 2, 3, 4, 5], i => sprintf('%02x', ord(s, at + i))));
}

function ip4(text) {
	return bytes(map(split(text, '.'), x => +x));
}

function ip4_text(s, at) {
	return join('.', map([0, 1, 2, 3], i => ord(s, at + i)));
}

// ip6 is an IPv6 address's 16 bytes, from its text.
function ip6(text) {
	let halves = split(text, '::');
	let head = halves[0] != '' ? split(halves[0], ':') : [];
	let tail = length(halves) > 1 && halves[1] != '' ? split(halves[1], ':') : [];
	let groups = [...head];
	for (let i = length(head) + length(tail); i < 8; i++)
		push(groups, '0');
	for (let g in tail)
		push(groups, g);
	return join('', map(groups, g => u16(hex(g))));
}

// ip6_text writes the IPv6 address at s[at] as RFC 5952 has it: the longest
// run of two or more zero groups shortened to "::".
function ip6_text(s, at) {
	let g = map([0, 1, 2, 3, 4, 5, 6, 7], i => get16(s, at + 2 * i));
	let best = -1, len = 0;
	for (let i = 0; i < 8; i++) {
		let j = i;
		while (j < 8 && g[j] == 0)
			j++;
		if (j - i > 1 && j - i > len) {
			best = i;
			len = j - i;
		}
		if (j > i)
			i = j - 1;
	}
	let hx = map(g, x => sprintf('%x', x));
	if (best < 0)
		return join(':', hx);
	return join(':', slice(hx, 0, best)) + '::' + join(':', slice(hx, best + len));
}

// link_local is the IPv6 link-local address a MAC makes (EUI-64), which a
// VLAN's nudge asks IPv6 all-nodes from (0064).
function link_local(m) {
	let b = mac(m);
	return ip6_text(bytes([0xfe, 0x80, 0, 0, 0, 0, 0, 0, ord(b, 0) ^ 2, ord(b, 1), ord(b, 2), 0xff, 0xfe,
		ord(b, 3), ord(b, 4), ord(b, 5)]), 0);
}

// checksum is the Internet checksum (RFC 1071).
function checksum(data) {
	let sum = 0;
	for (let i = 0; i < length(data); i += 2)
		sum += ord(data, i) << 8 | (i + 1 < length(data) ? ord(data, i + 1) : 0);
	while (sum >> 16)
		sum = (sum & 0xffff) + (sum >> 16);
	return ~sum & 0xffff;
}

// segment_mac is the MAC an AP uses on one segment it probes (0060): 02 for
// a VNI, its number in decimal digits; 06 for a VLAN, the same; 0a for a VNI
// above 9999, its last 16 bits in hex. Between them, the last three bytes of
// the AP's own MAC, read from its ID ("ap-a0046021365e"). Null for an ID
// that is not an AP's.
function segment_mac(ap, kind, n) {
	let m = match(ap ?? '', /^ap-[0-9a-f]{6}([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/);
	if (!m || n == null)
		return null;
	n = int(n);
	if (n < 0)
		return null;
	let tail = kind == 'vlan' || n <= 9999 ? sprintf('%04d', n) : sprintf('%04x', n & 0xffff);
	return sprintf('%s:%s:%s:%s:%s:%s', kind == 'vlan' ? '06' : n <= 9999 ? '02' : '0a',
		m[1], m[2], m[3], substr(tail, 0, 2), substr(tail, 2, 2));
}

// arp_probe asks who has target. Without an address of its own, the AP asks
// from 0.0.0.0, as a host checks an address before it uses one (RFC 5227);
// with a lease (0060), from the address it holds.
function arp_probe(src, target, from) {
	return pad(mac('ff:ff:ff:ff:ff:ff') + mac(src) + u16(0x0806) +
		u16(1) + u16(0x0800) + chr(6) + chr(4) + u16(1) +
		mac(src) + ip4(from ?? '0.0.0.0') + mac('00:00:00:00:00:00') + ip4(target));
}

// The DHCP messages the prober sends (0060).
const DHCP = { discover: 1, request: 3, release: 7 };

// dhcp builds a DHCP message from src, as a whole frame sent straight on a
// segment: a discover, a request or a release, with xid, and o: the address
// held (ciaddr, when renewing or releasing), the offer a request takes up
// (requested, server), where to send it (to, broadcast by default) and the
// host name it asks with (host).
function dhcp(src, type, xid, o) {
	let held = o?.ciaddr ?? '0.0.0.0';
	let msg = chr(1) + chr(1) + chr(6) + chr(0) + u32(xid) + u16(0) + u16(o?.ciaddr ? 0 : 0x8000) +
		ip4(held) + ip4('0.0.0.0') + ip4('0.0.0.0') + ip4('0.0.0.0') + mac(src);
	while (length(msg) < 236)
		msg += chr(0);
	msg += bytes([99, 130, 83, 99, 53, 1, DHCP[type]]) + chr(61) + chr(7) + chr(1) + mac(src);
	if (o?.requested)
		msg += chr(50) + chr(4) + ip4(o.requested);
	if (o?.server)
		msg += chr(54) + chr(4) + ip4(o.server);
	if (type != 'release')
		msg += bytes([55, 4, 1, 3, 51, 54]);   // the mask, router, lease time and server
	if (o?.host)
		msg += chr(12) + chr(length(o.host)) + o.host;
	msg += chr(255);
	while (length(msg) < 300)
		msg += chr(0);
	let udp = u16(68) + u16(67) + u16(8 + length(msg)) + u16(0) + msg;
	let ip = bytes([0x45, 0]) + u16(20 + length(udp)) + u16(0) + u16(0) + chr(64) + chr(17) + u16(0) +
		ip4(held) + ip4(o?.to ?? '255.255.255.255');
	ip = substr(ip, 0, 10) + u16(checksum(ip)) + substr(ip, 12);
	return mac('ff:ff:ff:ff:ff:ff') + mac(src) + u16(0x0800) + ip + udp;
}

// dhcp_reply reads a DHCP answer to xid: {type: offer, ack or nak, address,
// server, router, mask, lease in seconds}, or null.
function dhcp_reply(f, xid) {
	if (length(f) < 34 || get16(f, 12) != 0x0800 || ord(f, 23) != 17)
		return null;
	let u = 14 + (ord(f, 14) & 15) * 4, b = u + 8;
	if (length(f) < b + 240 || get16(f, u + 2) != 68 || ord(f, b) != 2 || get32(f, b + 4) != xid || get32(f, b + 236) != 0x63825363)
		return null;
	let opts = {};
	for (let at = b + 240; at + 1 < length(f) && ord(f, at) != 255; ) {
		if (ord(f, at) == 0) {
			at++;
			continue;
		}
		opts[ord(f, at)] = substr(f, at + 2, ord(f, at + 1));
		at += 2 + ord(f, at + 1);
	}
	let type = { '2': 'offer', '5': 'ack', '6': 'nak' }[opts[53] != null ? '' + ord(opts[53], 0) : ''];
	if (!type)
		return null;
	let ip = (k) => length(opts[k] ?? '') >= 4 ? ip4_text(opts[k], 0) : null;
	return {
		type: type, address: ip4_text(f, b + 16), server: ip(54), router: ip(3), mask: ip(1),
		lease: length(opts[51] ?? '') >= 4 ? get32(opts[51], 0) : null,
	};
}

// dhcp_seen reads any DHCP message in a frame (0065): op (1 a client's
// request, 2 a server's answer); its kind; the transaction ID; the client's
// MAC; the address the client holds (ciaddr) and the one it is given
// (yiaddr); the server's ID (option 54) and the address asked for (50); the
// host name a client gives (option 12, else 81's FQDN, 0066); its vendor
// class (60) and the options it asks for (55, 0067); and the
// frame's source MAC and IP address. Null for anything else.
function dhcp_seen(f) {
	if (length(f) < 34 || get16(f, 12) != 0x0800 || ord(f, 23) != 17)
		return null;
	let u = 14 + (ord(f, 14) & 15) * 4, b = u + 8;
	if (length(f) < b + 240 || get32(f, b + 236) != 0x63825363 || !(ord(f, b) in [1, 2]) || ord(f, b + 2) != 6)
		return null;
	let opts = {};
	for (let at = b + 240; at + 1 < length(f) && ord(f, at) != 255; ) {
		if (ord(f, at) == 0) {
			at++;
			continue;
		}
		opts[ord(f, at)] = substr(f, at + 2, ord(f, at + 1));
		at += 2 + ord(f, at + 1);
	}
	let kinds = [null, 'discover', 'offer', 'request', 'decline', 'ack', 'nak', 'release', 'inform'];
	let type = opts[53] != null ? kinds[ord(opts[53], 0)] : null;
	if (!type)
		return null;
	let ip = (k) => length(opts[k] ?? '') >= 4 ? ip4_text(opts[k], 0) : null;
	// Option 81 is flags, two codes, then the name: as DNS's labels when the
	// E flag is set, else as text.
	let name = (v) => substr(replace(v ?? '', /[^ -~]/g, ''), 0, 64) || null;
	let host = name(opts[12]);
	if (!host && length(opts[81] ?? '') > 3) {
		let o = opts[81], text = substr(o, 3);
		if (ord(o, 0) & 4) {
			let labels = [];
			for (let at = 3; at < length(o) && ord(o, at) > 0; at += 1 + ord(o, at))
				push(labels, substr(o, at + 1, ord(o, at)));
			text = join('.', labels);
		}
		host = name(text);
	}
	// What a client says of itself (0067): its vendor class (option 60),
	// and the options it asks for, in order (55), its fingerprint.
	let params = opts[55] != null ? join(',', map(split(substr(opts[55], 0, 64), ''), c => ord(c))) : null;
	return {
		op: ord(f, b), type: type, xid: get32(f, b + 4), client: mac_text(f, b + 28),
		ciaddr: ip4_text(f, b + 12), yiaddr: ip4_text(f, b + 16), server: ip(54), requested: ip(50),
		host: host, vendor_class: name(opts[60]), params: params || null,
		src_mac: mac_text(f, 6), src_ip: ip4_text(f, 26),
	};
}

// arp_seen reads who an ARP frame comes from (0065): the sender's MAC and the
// address it uses. Null for anything else, and for a probe from 0.0.0.0,
// which uses no address yet.
function arp_seen(f) {
	if (length(f) < 42 || get16(f, 12) != 0x0806 || get16(f, 16) != 0x0800 || ord(f, 18) != 6 || ord(f, 19) != 4)
		return null;
	let address = ip4_text(f, 28);
	return address == '0.0.0.0' ? null : { mac: mac_text(f, 22), address: address };
}

// What LLDP's TLVs name, for lldp (IEEE 802.1AB, 802.1Q, 802.3, ANSI/TIA-1057):
// the kinds of chassis and port ID, by subtype; the capabilities, by bit; a
// few MAU types (RFC 4836); LLDP-MED's applications and inventory.
const LLDP_CHASSIS = [null, 'chassis component', 'interface alias', 'port component', 'mac', 'network address', 'interface name', 'local'];
const LLDP_PORT = [null, 'interface alias', 'port component', 'mac', 'network address', 'interface name', 'agent circuit id', 'local'];
const LLDP_CAPS = ['other', 'repeater', 'bridge', 'wlan-ap', 'router', 'telephone', 'docsis', 'station', 'c-vlan', 's-vlan', 'tpmr'];
const LLDP_MAU = {
	'10': '10BASE-T, half duplex', '11': '10BASE-T, full duplex', '15': '100BASE-TX, half duplex', '16': '100BASE-TX, full duplex',
	'29': '1000BASE-T, half duplex', '30': '1000BASE-T, full duplex',
};
const LLDP_MED_APPS = [null, 'voice', 'voice signaling', 'guest voice', 'guest voice signaling', 'softphone voice',
	'video conferencing', 'streaming video', 'video signaling'];
const LLDP_INVENTORY = { '5': 'hardware', '6': 'firmware', '7': 'software', '8': 'serial', '9': 'manufacturer', '10': 'model', '11': 'asset' };

// lldp reads an LLDP frame (0064): all a switch says of itself and of the
// port it sends from. Its chassis and port IDs, each as a MAC, an address or
// printable text, with what kind of ID it is; how long it holds (ttl); its
// name and description; its capabilities, and those enabled; its management
// addresses; the port's description, native VLAN, the VLANs it names (with
// their names), protocol VLANs and protocols (802.1); link aggregation,
// maximum frame size, MAC/PHY and power (802.3); and LLDP-MED's class,
// network policies, inventory, location and power. What it doesn't know is
// kept in other, as hex. Null for any other frame.
function lldp(f) {
	if (length(f) < 16 || get16(f, 12) != 0x88cc)
		return null;
	let printable = (v) => substr(replace(v, /[^ -~]/g, ''), 0, 255);
	let hexs = (v) => join('', map(split(substr(v, 0, 64), ''), c => sprintf('%02x', ord(c))));
	let caps = (n) => {
		let out = [];
		for (let i = 0; i < length(LLDP_CAPS); i++)
			if ((n >> i) & 1)
				push(out, LLDP_CAPS[i]);
		return out;
	};
	// An address by its IANA family: IPv4, IPv6, or a MAC (all 802).
	let address = (fam, a) => fam == 1 && length(a) == 4 ? ip4_text(a, 0)
		: fam == 2 && length(a) == 16 ? ip6_text(a, 0)
		: fam == 6 && length(a) == 6 ? mac_text(a, 0) : hexs(a);
	// A chassis or port ID, and its kind.
	let id = (v, kinds, mac_kind, address_kind) => {
		let k = ord(v, 0), rest = substr(v, 1);
		let text = k == mac_kind && length(rest) == 6 ? mac_text(rest, 0)
			: k == address_kind && length(rest) >= 2 ? address(ord(rest, 0), substr(rest, 1)) : printable(rest);
		return [text, kinds[k] ?? 'unknown'];
	};
	let out = {
		chassis: null, chassis_kind: null, system: null, system_description: null,
		port: null, port_kind: null, port_description: null, ttl: null,
		capabilities: null, enabled_capabilities: null, management: [],
		native_vlan: null, vlans: [], vlan_names: {}, protocol_vlans: [], protocols: [],
		aggregation: null, max_frame: null, mac_phy: null, power: null, med: null, other: [],
	};
	let other = (t, v) => {
		if (length(out.other) < 16)
			push(out.other, t == 127 && length(v) >= 4
				? { type: t, oui: hexs(substr(v, 0, 3)), subtype: ord(v, 3), data: hexs(substr(v, 4)) }
				: { type: t, data: hexs(v) });
	};
	const IEEE_8021 = bytes([0x00, 0x80, 0xc2]), IEEE_8023 = bytes([0x00, 0x12, 0x0f]), TIA_MED = bytes([0x00, 0x12, 0xbb]);
	for (let at = 14; at + 2 <= length(f); ) {
		let h = get16(f, at), t = h >> 9, l = h & 0x1ff, v = substr(f, at + 2, l);
		if (t == 0 || length(v) < l)
			break;
		at += 2 + l;
		if (t == 1 && l >= 2) {
			let r = id(v, LLDP_CHASSIS, 4, 5);
			out.chassis = r[0];
			out.chassis_kind = r[1];
		} else if (t == 2 && l >= 2) {
			let r = id(v, LLDP_PORT, 3, 4);
			out.port = r[0];
			out.port_kind = r[1];
		} else if (t == 3 && l >= 2)
			out.ttl = get16(v, 0);
		else if (t == 4)
			out.port_description = printable(v);
		else if (t == 5)
			out.system = printable(v);
		else if (t == 6)
			out.system_description = printable(v);
		else if (t == 7 && l >= 4) {
			out.capabilities = caps(get16(v, 0));
			out.enabled_capabilities = caps(get16(v, 2));
		} else if (t == 8 && l >= 8 && l >= ord(v, 0) + 6) {
			// The address's length, with its family; the address; how the
			// interface is numbered, and its number; then an OID, unread.
			let n = ord(v, 0);
			if (length(out.management) < 4)
				push(out.management, {
					address: address(ord(v, 1), substr(v, 2, n - 1)), interface: get32(v, 2 + n),
					interface_kind: [null, 'unknown', 'ifindex', 'port number'][ord(v, 1 + n)] ?? 'unknown',
				});
		} else if (t == 127 && l >= 4) {
			let oui = substr(v, 0, 3), sub = ord(v, 3), d = substr(v, 4);
			if (oui == IEEE_8021) {
				if (sub == 1 && length(d) >= 2)
					out.native_vlan = (get16(d, 0) & 0xfff) || null;   // 0: the port has none
				else if (sub == 2 && length(d) >= 3) {
					if ((ord(d, 0) & 2) && length(out.protocol_vlans) < 64)   // enabled
						push(out.protocol_vlans, get16(d, 1) & 0xfff);
				} else if (sub == 3 && length(d) >= 3) {
					let vid = get16(d, 0) & 0xfff;
					if (vid && index(out.vlans, vid) < 0) {
						push(out.vlans, vid);
						out.vlan_names['' + vid] = substr(printable(substr(d, 3, ord(d, 2))), 0, 32);
					}
				} else if (sub == 4 && length(d) >= 1) {
					if (length(out.protocols) < 16)
						push(out.protocols, hexs(substr(d, 1, ord(d, 0))));
				} else if (sub == 7 && length(d) >= 5)
					out.aggregation = { capable: !!(ord(d, 0) & 1), enabled: !!(ord(d, 0) & 2), port: get32(d, 1) };
				else
					other(t, v);
			} else if (oui == IEEE_8023) {
				if (sub == 1 && length(d) >= 5)
					out.mac_phy = {
						autoneg_supported: !!(ord(d, 0) & 1), autoneg_enabled: !!(ord(d, 0) & 2),
						advertised: sprintf('%04x', get16(d, 1)), mau: get16(d, 3), mau_name: LLDP_MAU['' + get16(d, 3)] ?? null,
					};
				else if (sub == 2 && length(d) >= 3) {
					let p = ord(d, 0);
					out.power = {
						pse: !!(p & 1), supported: !!(p & 2), enabled: !!(p & 4),
						pair: ord(d, 1) == 1 ? 'signal' : ord(d, 1) == 2 ? 'spare' : null, class: ord(d, 2) ? ord(d, 2) - 1 : null,
						requested_w: length(d) >= 8 ? get16(d, 4) / 10.0 : null, allocated_w: length(d) >= 8 ? get16(d, 6) / 10.0 : null,
					};
				} else if (sub == 3 && length(d) >= 5)
					out.aggregation = { capable: !!(ord(d, 0) & 1), enabled: !!(ord(d, 0) & 2), port: get32(d, 1) };
				else if (sub == 4 && length(d) >= 2)
					out.max_frame = get16(d, 0);
				else
					other(t, v);
			} else if (oui == TIA_MED) {
				out.med ??= { capabilities: null, class: null, policies: [], inventory: {}, location: null, power_w: null };
				if (sub == 1 && length(d) >= 3) {
					out.med.capabilities = sprintf('%04x', get16(d, 0));
					out.med.class = ord(d, 2);
				} else if (sub == 2 && length(d) >= 4) {
					// The application, then unknown, tagged and a reserved bit,
					// the VLAN, its priority and the DSCP.
					let x = get32(d, 0);
					if (length(out.med.policies) < 8)
						push(out.med.policies, {
							application: LLDP_MED_APPS[x >> 24] ?? sprintf('type %d', x >> 24), unknown: !!((x >> 23) & 1),
							tagged: !!((x >> 22) & 1), vlan: (x >> 9) & 0xfff, priority: (x >> 6) & 7, dscp: x & 0x3f,
						});
				} else if (sub == 3)
					out.med.location = hexs(d);
				else if (sub == 4 && length(d) >= 3)
					out.med.power_w = get16(d, 1) / 10.0;
				else if (LLDP_INVENTORY['' + sub])
					out.med.inventory[LLDP_INVENTORY['' + sub]] = printable(d);
				else
					other(t, v);
			} else
				other(t, v);
		} else
			other(t, v);
	}
	out.vlans = sort(out.vlans, (a, b) => a - b);
	return out;
}

// echo6 asks every IPv6 host on the segment (ff02::1) to answer, from the
// link-local address src_ip, which the AP's bridge on the segment holds.
function echo6(src, src_ip, id, seq) {
	let from = ip6(src_ip), to = ip6('ff02::1');
	let icmp = chr(128) + chr(0) + u16(0) + u16(id) + u16(seq) + PAYLOAD;
	let sum = checksum(from + to + u16(0) + u16(length(icmp)) + bytes([0, 0, 0, 58]) + icmp);
	icmp = substr(icmp, 0, 2) + u16(sum) + substr(icmp, 4);
	return mac('33:33:00:00:00:01') + mac(src) + u16(0x86dd) +
		bytes([0x60, 0, 0, 0]) + u16(length(icmp)) + chr(58) + chr(255) + from + to + icmp;
}

// answer reads a frame from a tunnel device: if it answers a probe sent from
// src, or an echo carrying id, who answered and which probe it answers
// ({from, key}); otherwise null.
function answer(f, src, id) {
	let type = length(f) >= 14 ? get16(f, 12) : 0;
	if (type == 0x0806 && length(f) >= 42 && get16(f, 20) == 2 && mac_text(f, 32) == lc(src ?? '')) {
		let from = ip4_text(f, 28);
		return { from: from, key: 'arp:' + from };
	}
	if (type == 0x86dd && length(f) >= 62 && ord(f, 20) == 58 && ord(f, 54) == 129 && get16(f, 58) == id)
		return { from: ip6_text(f, 22), key: 'echo:' + get16(f, 60) };
	return null;
}

// echo4 is an ICMP echo request, for a raw socket: the kernel adds the IP
// header. It pings the concentrator over the underlay.
function echo4(id, seq) {
	let icmp = chr(8) + chr(0) + u16(0) + u16(id) + u16(seq) + PAYLOAD;
	return substr(icmp, 0, 2) + u16(checksum(icmp)) + substr(icmp, 4);
}

// echo4_answer reads what a raw ICMP socket got, IP header first: a reply to
// id, as {from, seq}, or null.
function echo4_answer(pkt, id) {
	let ihl = (ord(pkt, 0) & 15) * 4;
	if (length(pkt) < ihl + 8 || ord(pkt, ihl) != 0 || get16(pkt, ihl + 4) != id)
		return null;
	return { from: ip4_text(pkt, 12), seq: get16(pkt, ihl + 6) };
}

// echo6_plain is an ICMPv6 echo request for a raw ICMPv6 socket, which fills
// in the checksum itself, and echo6_plain_answer reads a reply to id, which
// comes without its IP header.
function echo6_plain(id, seq) {
	return chr(128) + chr(0) + u16(0) + u16(id) + u16(seq) + PAYLOAD;
}

function echo6_plain_answer(pkt, id) {
	if (length(pkt) < 8 || ord(pkt, 0) != 129 || get16(pkt, 4) != id)
		return null;
	return { seq: get16(pkt, 6) };
}

// guard_frame is what the loop guard sends on a tunnel port's device: the
// mark, this run's nonce, the port and the device.
function guard_frame(src, nonce, port, device) {
	return pad(mac(GUARD_DST) + mac(src) + u16(GUARD_TYPE) + GUARD_MARK +
		chr(length(nonce)) + nonce + chr(length(port)) + port + chr(length(device)) + device);
}

// guard_seen reads a frame: if it is one this run's loop guard sent, the
// port and device it was sent on; otherwise null. Another AP's frames, or
// an earlier run's, carry another nonce.
function guard_seen(f, nonce) {
	if (length(f) < 14 || get16(f, 12) != GUARD_TYPE || substr(f, 14, length(GUARD_MARK)) != GUARD_MARK)
		return null;
	let at = 14 + length(GUARD_MARK);
	let field = () => {
		let n = ord(f, at);
		let v = n == null ? null : substr(f, at + 1, n);
		at += 1 + (n ?? 0);
		return v;
	};
	if (field() != nonce)
		return null;
	let port = field(), device = field();
	return port && device ? { port: port, device: device } : null;
}

// verdict says how a tunnel is doing (0059), from t: its interval, when the
// prober started on it, when its segment last answered, and when its
// concentrator last answered a ping, in seconds. Up while the segment
// answered within the last three intervals; unknown for the first three
// intervals; down once a segment that answered has stopped, or while
// neither the segment nor the concentrator has ever answered; unverified
// while the concentrator answers but nothing on the segment ever has.
function verdict(t, now) {
	let span = 3 * t.interval;
	if (t.answered != null && now - t.answered <= span)
		return 'up';
	if (now - t.started < span)
		return 'unknown';
	if (t.answered != null)
		return 'down';
	return t.underlay != null && now - t.underlay <= span ? 'unverified' : 'down';
}

// switch_step says which transport should carry a network with a fallback
// (0061), from where it is and how each transport is doing. n holds the
// network's settings: mode (report or automatic), ha, failback (revertive
// or equal) and holddown; want, the transport that carries it now (primary
// or fallback); each transport's verdict, primary and fallback;
// fallback_vxlan, whether the fallback is a tunnel; and up_since, since when
// the primary has been up. It returns want; whether to start or stop the
// fallback's tunnel; why the network moves; and why a move that is due
// cannot be made.
//
// In report mode the primary carries the network. In automatic mode the
// network moves to the fallback when the primary is down, and only to a
// fallback that is up. It moves back when the primary is up and the fallback
// is down or stopped, or, with revertive failback, once the primary has been
// up for the hold-down. A tunnel fallback without HA mode runs only while it
// is needed: it is started when the primary goes down, and used once it
// answers.
function switch_step(n, now) {
	let r = { want: 'primary', start: false, stop: false, why: null, cannot: null };
	if (n.mode != 'automatic')
		return r;
	r.want = n.want ?? 'primary';
	// A stopped tunnel fallback is started wherever a switch waits on it.
	let fallback = n.fallback == 'off' && n.fallback_vxlan ? 'starting' : n.fallback;
	if (r.want == 'primary') {
		if (n.primary == 'down' && n.fallback == 'up') {
			r.want = 'fallback';
			r.why = 'the primary is down';
		} else if (n.primary == 'down')
			r.cannot = `the primary is down, and the fallback is ${fallback}`;
	} else if (n.primary == 'up' && n.fallback in { down: 1, off: 1 }) {
		r.want = 'primary';
		r.why = `the fallback is ${n.fallback}`;
	} else if (n.primary == 'up' && n.failback != 'equal' && n.up_since != null && now - n.up_since >= n.holddown) {
		r.want = 'primary';
		r.why = `the primary has been up for ${n.holddown} s`;
	} else if (n.fallback != 'up' && n.primary != 'up')
		r.cannot = `the fallback is ${fallback}, and the primary is ${n.primary}`;
	if (n.fallback_vxlan) {
		let needed = n.ha || r.want == 'fallback' || n.primary == 'down';
		r.start = needed && n.fallback == 'off';
		r.stop = !needed && n.fallback != 'off';
	}
	return r;
}

// watch_verdict says whether a watched VLAN reaches the AP (0064), from w:
// when the watch started, and when a frame last came in on the VLAN. It is
// present if one came in within WATCH_SPAN; silent if none has for that
// long; unknown while it hasn't been watched that long.
function watch_verdict(w, now) {
	if (w.heard != null && now - w.heard <= WATCH_SPAN)
		return 'present';
	return now - (w.heard ?? w.started) >= WATCH_SPAN ? 'silent' : 'unknown';
}

// request_state says how a client's DHCP request stands (0065), from r:
// when it was made, and the answers to it, by server. Answered once any
// came; waiting for DHCP_WAIT seconds; unanswered after that.
function request_state(r, now) {
	if (length(keys(r.answers ?? {})))
		return 'answered';
	return now - r.at < DHCP_WAIT ? 'waiting' : 'unanswered';
}

// client_dhcp says how a Wi-Fi client does with DHCP (0065), from c: when it
// joined; whether the prober watched since then (watched); when it last
// asked for DHCP; and the address it shows. ok once it asked since it
// joined; static when it shows an address but asked nothing within
// DHCP_QUIET seconds of joining; none when it shows no address either;
// unknown before then, or for a client that joined before the watch began.
function client_dhcp(c, now) {
	if (c.asked != null && c.asked >= c.joined - 2)   // nl80211 gives the join to the second
		return 'ok';
	if (!c.watched || now - c.joined < DHCP_QUIET)
		return 'unknown';
	return c.address ? 'static' : 'none';
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export {
	GUARD_DST, GUARD_TYPE, OVERLAY_FILTER, GUARD_FILTER, WATCH_FILTER, LLDP_FILTER, WATCH_SPAN,
	DHCP_FILTER, DHCP_WAIT, DHCP_QUIET, DHCP_WINDOW, DHCP_MEMORY,
	mac_text, ip6, ip6_text, checksum, segment_mac, arp_probe, dhcp, dhcp_reply, echo6, answer,
	echo4, echo4_answer, echo6_plain, echo6_plain_answer,
	guard_frame, guard_seen, verdict, switch_step, link_local, lldp, watch_verdict,
	dhcp_seen, arp_seen, request_state, client_dhcp
};
