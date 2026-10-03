// The prober's frames, and how it judges a tunnel (0059).
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
// a probe: incoming ARP replies, and incoming ICMPv6 echo replies. The rest
// of the tunnel's traffic, clients' included, never reaches the prober.
// Classic BPF, [code, jt, jf, k]; jumps count from the next instruction.
const OVERLAY_FILTER = [
	[0x20, 0, 0, 0xfffff004],   //  0  A = packet type
	[0x15, 9, 0, 4],            //  1  outgoing: drop
	[0x28, 0, 0, 12],           //  2  A = EtherType
	[0x15, 0, 2, 0x0806],       //  3  not ARP: to 6
	[0x28, 0, 0, 20],           //  4  A = ARP operation
	[0x15, 6, 5, 2],            //  5  a reply: keep; else drop
	[0x15, 0, 4, 0x86dd],       //  6  not IPv6: drop
	[0x30, 0, 0, 20],           //  7  A = next header
	[0x15, 0, 2, 58],           //  8  not ICMPv6: drop
	[0x30, 0, 0, 54],           //  9  A = ICMPv6 type
	[0x15, 1, 0, 129],          // 10  an echo reply: keep
	[0x06, 0, 0, 0],            // 11  drop
	[0x06, 0, 0, 0x40000],      // 12  keep
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

function bytes(list) {
	return join('', map(list, b => chr(b)));
}

function u16(n) {
	return chr((n >> 8) & 255) + chr(n & 255);
}

function get16(s, at) {
	return ord(s, at) << 8 | ord(s, at + 1);
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

// checksum is the Internet checksum (RFC 1071).
function checksum(data) {
	let sum = 0;
	for (let i = 0; i < length(data); i += 2)
		sum += ord(data, i) << 8 | (i + 1 < length(data) ? ord(data, i + 1) : 0);
	while (sum >> 16)
		sum = (sum & 0xffff) + (sum >> 16);
	return ~sum & 0xffff;
}

// arp_probe asks who has target, from 0.0.0.0, as a host checks an address
// before it uses one (RFC 5227), so the AP needs no address on the segment.
function arp_probe(src, target) {
	return pad(mac('ff:ff:ff:ff:ff:ff') + mac(src) + u16(0x0806) +
		u16(1) + u16(0x0800) + chr(6) + chr(4) + u16(1) +
		mac(src) + ip4('0.0.0.0') + mac('00:00:00:00:00:00') + ip4(target));
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

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export {
	GUARD_DST, GUARD_TYPE, OVERLAY_FILTER, GUARD_FILTER,
	mac_text, ip6, ip6_text, checksum, arp_probe, echo6, answer,
	echo4, echo4_answer, echo6_plain, echo6_plain_answer,
	guard_frame, guard_seen, verdict
};
