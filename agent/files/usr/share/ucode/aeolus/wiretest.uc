// A client test over the wire (0128): the frames a client's first seconds
// on a network are made of, DHCP, the ARP for its gateway and a DNS lookup,
// sent from the AP's own uplink on the network's VLAN, or into its tunnel,
// from a MAC of the test's own, and read back. The AP takes no address and
// makes no interface: the test is its frames and nothing else.
//
// Pure: it builds and reads frames, and never touches a socket or a file, so
// the same code runs on the AP and in CI (agent/test/wiretest.uc).

'use strict';

const BROADCAST = 'ff:ff:ff:ff:ff:ff';
const HOST = 'aeolus-test';

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

// checksum is the Internet checksum (RFC 1071).
function checksum(data) {
	let sum = 0;
	for (let i = 0; i < length(data); i += 2)
		sum += ord(data, i) << 8 | (i + 1 < length(data) ? ord(data, i + 1) : 0);
	while (sum >> 16)
		sum = (sum & 0xffff) + (sum >> 16);
	return ~sum & 0xffff;
}

// test_mac is the MAC an AP tests a segment from: 0e, the last three bytes
// of the AP's own MAC, read from its ID ("ap-a0046021365e"), and the
// segment's number, a VLAN's or a tunnel's, in four decimal digits, its
// last four where it has more; 0000 for the uplink with no VLAN. It is
// locally administered, and not one the prober uses on a segment (02, 06 and
// 0a, 0060): a test that gives its lease back gives back none of those.
// Null for an ID that is not an AP's.
function test_mac(ap, n) {
	let m = match(ap ?? '', /^ap-[0-9a-f]{6}([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/);
	if (!m)
		return null;
	n = int(n ?? 0);
	if (!(n >= 0))
		n = 0;
	let tail = sprintf('%04d', n % 10000);
	return sprintf('0e:%s:%s:%s:%s:%s', m[1], m[2], m[3], substr(tail, 0, 2), substr(tail, 2, 2));
}

// bpf keeps, of everything that comes in on the device the test is made
// on, only what is the test's: frames to its own MAC, which are the answers
// to it, and of the broadcasts ARP, which may ask for the address it was
// given, and DHCP to a client. The clients' own traffic, which crosses the
// same device, never reaches the test. Classic BPF, [code, jt, jf, k];
// jumps count from the next instruction.
function bpf(src) {
	let m = mac(src);
	return [
		[0x20, 0, 0, 0xfffff004],        //  0  A = packet type
		[0x15, 16, 0, 4],                //  1  outgoing: drop
		[0x20, 0, 0, 0],                 //  2  A = the destination's first four bytes
		[0x15, 0, 2, get32(m, 0)],       //  3  not the test's: to 6
		[0x28, 0, 0, 4],                 //  4  A = its last two
		[0x15, 13, 12, get16(m, 4)],     //  5  the test's MAC: keep; else drop
		[0x15, 0, 11, 0xffffffff],       //  6  not a broadcast: drop
		[0x28, 0, 0, 12],                //  7  A = EtherType
		[0x15, 10, 0, 0x0806],           //  8  ARP: keep
		[0x15, 0, 8, 0x0800],            //  9  not IPv4: drop
		[0x30, 0, 0, 23],                // 10  A = IP protocol
		[0x15, 0, 6, 17],                // 11  not UDP: drop
		[0x28, 0, 0, 20],                // 12  A = flags and fragment offset
		[0x45, 4, 0, 0x1fff],            // 13  a later fragment: drop
		[0xb1, 0, 0, 14],                // 14  X = the IP header's length
		[0x48, 0, 0, 16],                // 15  A = UDP destination port
		[0x15, 2, 0, 68],                // 16  DHCP's client port: keep
		[0x06, 0, 0, 0],                 // 17  drop
		[0x06, 0, 0, 0],                 // 18  drop
		[0x06, 0, 0, 0x40000],           // 19  keep
	];
}

// with_tag gives a frame a VLAN's tag, where the device carries the VLAN
// tagged.
function with_tag(frame, vlan, tagged) {
	if (!vlan || !tagged)
		return frame;
	return substr(frame, 0, 12) + chr(0x81, 0x00, (vlan >> 8) & 0x0f, vlan & 0xff) + substr(frame, 12);
}

// The DHCP messages the test sends.
const DHCP = { discover: 1, request: 3, decline: 4, release: 7 };

// dhcp builds a DHCP message from src as a whole frame: a discover, a
// request, a decline or a release, with xid, and o: the address held
// (ciaddr, when releasing), the offer a request takes up or a decline
// turns down (requested, server), how many seconds it has been asking
// (secs, which a relay may wait on), and where to send it (to, and dst,
// the MAC of where it goes next; broadcast by default). It asks, as a
// client does, for its mask, router, DNS servers, lease time and server,
// and to be answered by broadcast, as it has no address to be answered at.
// Its datagram has its checksum: one that is routed to its server, as a
// release is, may not be let by without.
function dhcp(src, type, xid, o) {
	let held = o?.ciaddr ?? '0.0.0.0';
	let msg = chr(1) + chr(1) + chr(6) + chr(0) + u32(xid) + u16(min(max(int(o?.secs ?? 0), 0), 0xffff)) + u16(o?.ciaddr ? 0 : 0x8000) +
		ip4(held) + ip4('0.0.0.0') + ip4('0.0.0.0') + ip4('0.0.0.0') + mac(src);
	while (length(msg) < 236)
		msg += chr(0);
	msg += bytes([99, 130, 83, 99, 53, 1, DHCP[type]]) + chr(61) + chr(7) + chr(1) + mac(src);
	if (o?.requested)
		msg += chr(50) + chr(4) + ip4(o.requested);
	if (o?.server)
		msg += chr(54) + chr(4) + ip4(o.server);
	if (type == 'discover' || type == 'request')
		msg += bytes([55, 5, 1, 3, 6, 51, 54]) + chr(12) + chr(length(HOST)) + HOST;
	msg += chr(255);
	while (length(msg) < 300)
		msg += chr(0);
	let to = o?.to ?? '255.255.255.255';
	let udp = u16(68) + u16(67) + u16(8 + length(msg)) + u16(0) + msg;
	let sum = checksum(ip4(held) + ip4(to) + chr(0) + chr(17) + u16(length(udp)) + udp);
	udp = substr(udp, 0, 6) + u16(sum || 0xffff) + substr(udp, 8);
	let ip = bytes([0x45, 0]) + u16(20 + length(udp)) + u16(0) + u16(0) + chr(64) + chr(17) + u16(0) +
		ip4(held) + ip4(to);
	ip = substr(ip, 0, 10) + u16(checksum(ip)) + substr(ip, 12);
	return mac(o?.dst ?? BROADCAST) + mac(src) + u16(0x0800) + ip + udp;
}

// bits is a mask's prefix length; null for what is no mask.
function bits(mask) {
	let n = 0, done = false;
	for (let b in map(split(mask ?? '', '.'), x => +x))
		for (let i = 7; i >= 0; i--) {
			if (b & (1 << i)) {
				if (done)
					return null;
				n++;
			} else
				done = true;
		}
	return length(split(mask ?? '', '.')) == 4 ? n : null;
}

// dhcp_reply reads a DHCP answer to xid for src: {type: offer, ack or nak,
// address, mask as a prefix length, router, dns, server, lease in seconds},
// as a lease is said (0121), or null.
function dhcp_reply(f, xid, src) {
	if (length(f) < 34 || get16(f, 12) != 0x0800 || ord(f, 23) != 17)
		return null;
	let u = 14 + (ord(f, 14) & 15) * 4, b = u + 8;
	if (length(f) < b + 240 || get16(f, u + 2) != 68 || ord(f, b) != 2 || get32(f, b + 4) != xid ||
	    get32(f, b + 236) != 0x63825363 || mac_text(f, b + 28) != src)
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
	let dns = [];
	for (let at = 0; at + 4 <= length(opts[6] ?? '') && length(dns) < 3; at += 4)
		push(dns, ip4_text(opts[6], at));
	return {
		type: type, address: ip4_text(f, b + 16), mask: bits(ip(1)) ?? 32, router: ip(3), dns: dns, server: ip(54),
		lease: length(opts[51] ?? '') >= 4 ? get32(opts[51], 0) : 0,
	};
}

// arp_request asks who has target, from the address the test was given.
function arp_request(src, from, target) {
	return pad(mac(BROADCAST) + mac(src) + u16(0x0806) +
		u16(1) + u16(0x0800) + chr(6) + chr(4) + u16(1) +
		mac(src) + ip4(from) + mac('00:00:00:00:00:00') + ip4(target));
}

// arp_probe asks whether anyone has an address already, before the test
// uses it: from no address, as a host asks of one it was just given (RFC
// 5227), so that nobody's table takes the test for its holder yet.
function arp_probe(src, target) {
	return arp_request(src, '0.0.0.0', target);
}

// arp_of reads an ARP frame: {op (1 a request, 2 a reply), mac and ip of
// who sent it, target the address it is about}; null for anything else.
function arp_of(f) {
	if (length(f) < 42 || get16(f, 12) != 0x0806 || get16(f, 14) != 1 || get16(f, 16) != 0x0800)
		return null;
	return { op: get16(f, 20), mac: mac_text(f, 22), ip: ip4_text(f, 28), target: ip4_text(f, 38) };
}

// arp_answer is the answer to a request for the address the test was
// given, so that what answers it can find it; null for any other frame.
function arp_answer(f, src, held) {
	let a = arp_of(f);
	if (!a || a.op != 1 || a.target != held || a.mac == src)
		return null;
	return pad(mac(a.mac) + mac(src) + u16(0x0806) +
		u16(1) + u16(0x0800) + chr(6) + chr(4) + u16(2) +
		mac(src) + ip4(held) + mac(a.mac) + ip4(a.ip));
}

// arp_conflict is the MAC of another that says an address is its own: any
// ARP sent from that address by a MAC that is not the test's. Null for
// anything else.
function arp_conflict(f, src, address) {
	let a = arp_of(f);
	return a && a.ip == address && a.mac != src ? a.mac : null;
}

// udp builds a UDP datagram as a whole frame, from the test's MAC and the
// address it was given to dst, the MAC of where it goes next. It has its
// checksum: IPv4 lets one be left out, and not everything on the way to a
// DNS server on the internet lets such a datagram by.
function udp(src, dst, from, to, sport, dport, data) {
	let u = u16(sport) + u16(dport) + u16(8 + length(data)) + u16(0) + data;
	let sum = checksum(ip4(from) + ip4(to) + chr(0) + chr(17) + u16(length(u)) + u);
	u = substr(u, 0, 6) + u16(sum || 0xffff) + substr(u, 8);
	let ip = bytes([0x45, 0]) + u16(20 + length(u)) + u16(sport) + u16(0x4000) + chr(64) + chr(17) + u16(0) + ip4(from) + ip4(to);
	ip = substr(ip, 0, 10) + u16(checksum(ip)) + substr(ip, 12);
	return pad(mac(dst) + mac(src) + u16(0x0800) + ip + u);
}

// udp_of reads a UDP datagram sent to the address and port the test asked
// from: {from, sport, data}; null for anything else, a fragment among them.
function udp_of(f, held, port) {
	if (length(f) < 42 || get16(f, 12) != 0x0800 || (ord(f, 14) >> 4) != 4 || ord(f, 23) != 17 || (get16(f, 20) & 0x3fff) != 0 ||
	    ip4_text(f, 30) != held)
		return null;
	let u = 14 + (ord(f, 14) & 15) * 4;
	if (length(f) < u + 8 || get16(f, u + 2) != port)
		return null;
	let len = get16(f, u + 4);
	if (len < 8 || u + len > length(f))
		return null;
	return { from: ip4_text(f, 26), sport: get16(f, u), data: substr(f, u + 8, len - 8) };
}

// on_link says whether an address is on the test's own subnet, by the
// lease's mask: such a one is asked for by ARP itself, another through the
// gateway.
function on_link(ip, lease) {
	let a = map(split(ip ?? '', '.'), x => +x), b = map(split(lease?.address ?? '', '.'), x => +x);
	if (length(a) != 4 || length(b) != 4 || !(lease.mask >= 0 && lease.mask <= 32))
		return false;
	for (let i = 0; i < 4; i++) {
		let keep = min(8, max(0, lease.mask - 8 * i));
		let m = (0xff00 >> keep) & 0xff;
		if ((a[i] & m) != (b[i] & m))
			return false;
	}
	return true;
}

// where is the wire a network's clients' frames take from this AP, from
// what its config says: dev, the device its Wi-Fi is bridged to (the
// interface aeolus_<network>'s device); sw, the switch section of a network
// with a fallback (0061), whose bridge dev then is, with members, the
// devices in that bridge now; watch, a function giving the watch section of
// a VLAN (0064), which says whether the uplink carries it tagged; and out,
// the uplink's port the AP's traffic takes now (0096).
//
// It is {device, vlan?, tagged?, vni?}: the uplink's port with the VLAN, or
// a tunnel's own device; or {why} where there is none to test on.
function where(dev, sw, members, watch, out) {
	let on_vlan = (v) => {
		let w = watch(v);
		if (!w)
			return { why: `this AP does not watch VLAN ${v} on its uplink, so it cannot say how it is carried` };
		if (!out)
			return { why: 'this AP has no uplink port to test from' };
		return { device: out, vlan: +v, tagged: w.tagged == '1' };
	};
	if (type(dev) != 'string' || !length(dev))
		return { why: 'this AP has no interface for that network' };
	if (sw) {
		for (let slot in ['primary', 'fallback']) {
			let d = sw[slot];
			if (!d || index(members ?? [], d) < 0)
				continue;
			if (sw[slot + '_vni'] != null)
				return { device: d, vni: +sw[slot + '_vni'] };
			if (sw[slot + '_vlan'] != null)
				return on_vlan(sw[slot + '_vlan']);
		}
		return { why: 'the network has no transport in its bridge now' };
	}
	let m = match(dev, /^[A-Za-z0-9_-]{1,10}\.([0-9]{1,4})$/);
	if (m)
		return on_vlan(m[1]);
	// A network on a tunnel alone (0054): the tunnel's device is in a
	// bridge named for its VNI, which the network's Wi-Fi joins.
	m = match(dev, /^br-vx([0-9]{1,8})$/);
	if (m)
		return { device: 'aeolus_' + m[1], vni: +m[1] };
	return { why: `the network is on ${dev}, which is no VLAN of the uplink's and no tunnel of Aeolus's` };
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export {
	BROADCAST, HOST, test_mac, bpf, with_tag, dhcp, dhcp_reply, bits, arp_request, arp_probe, arp_of, arp_answer, arp_conflict, udp, udp_of, on_link, where, mac_text
};
