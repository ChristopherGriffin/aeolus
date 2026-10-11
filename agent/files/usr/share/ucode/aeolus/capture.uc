// A capture of a client's failed attempt to come online (0135): the frames
// of it the AP's radio heard and sent, and those that crossed its wire
// side, as a file Wireshark opens. No I/O here: aeolus-journey listens,
// keeps and writes.
//
// The radio's frames come from a monitor interface on the radio that
// serves the client, with no flags, so the radio hears what it heard
// before: management frames, but beacons, and the messages of the key
// handshake and of an 802.1X sign-in (EAPOL), each with its radiotap header.
// They are kept in a ring a radio, a few minutes back, and those of one
// client are taken from it when its attempt fails. The wire side's are the
// frames aeolus-journey already reads of a client it follows: DHCP, ARP,
// DNS and a first connection.

'use strict';

import { assemble } from 'aeolus.journey';

const SNAP = 2048;           // bytes kept of a frame: a management frame whole
const RING_SECONDS = 180;    // how far back a radio's frames are kept
const RING_BYTES = 2097152;  // and how much of them, at most
const RING_FRAMES = 8000;    // and how many: each costs the daemon more than its bytes
const BEFORE = 10;           // seconds before an attempt's start taken with it, as the probes that led to it
const AFTER = 2;             // and after its end
const MOST = 600;            // frames at most in one capture
const MOST_BYTES = 524288;   // and bytes of them: past that the earliest go
const WIRE = 96;             // frames kept from the wire side of one attempt
const LATE = 5000;           // ms at most a radio may hand a frame up after it heard it, and be put right
const SOON = 250;            // ms at most a radio may say it sent a message after the answer to it came, and be put right
const LINK_RADIO = 127;      // pcapng's link types: 802.11 with a radiotap header
const LINK_WIRE = 1;         // and Ethernet

// radio_filter is the kernel's filter for a monitor interface: management
// frames but beacons; and data frames that carry EAPOL, whose LLC header
// follows the 802.11 header, with a QoS field or none, and with the eight
// bytes of a CCMP header before it where the frame was encrypted and the
// radio left them in. Everything else is dropped in the kernel: on some
// radios a monitor interface is handed every frame on the channel.
function radio_filter() {
	return assemble([
		[0x30, 0, 0, 3],                  // A = the radiotap header's length: its high byte,
		[0x64, 0, 0, 8],                  // shifted up,
		[0x07, 0, 0, 0],                  // kept in X,
		[0x30, 0, 0, 2],                  // and its low byte
		[0x4c, 0, 0, 0],                  // with it
		[0x07, 0, 0, 0],                  // X = where the 802.11 frame begins
		[0x50, 0, 0, 0],                  // A = frame control's first byte
		[0x54, 0, 0, 0x0c],               // its type
		[0x15, 'management', 0, 0x00],
		[0x15, 'data', 'drop', 0x08],
		'management',
		[0x50, 0, 0, 0],
		[0x54, 0, 0, 0xf0],               // its subtype
		[0x15, 'drop', 'keep', 0x80],     // a beacon goes
		'data',
		[0x48, 0, 0, 30],                 // the LLC header's protocol, after a header of 24 bytes
		[0x15, 'keep', 0, 0x888e],
		[0x48, 0, 0, 32],                 // of 26, with a QoS field
		[0x15, 'keep', 0, 0x888e],
		[0x48, 0, 0, 38],                 // and each with a CCMP header before it
		[0x15, 'keep', 0, 0x888e],
		[0x48, 0, 0, 40],
		[0x15, 'keep', 'drop', 0x888e],
		'keep',
		[0x06, 0, 0, SNAP],
		'drop',
		[0x06, 0, 0, 0],
	]);
}

function mac_text(s, at) {
	return join(':', map([0, 1, 2, 3, 4, 5], i => sprintf('%02x', ord(s, at + i))));
}

function u32(s, at) {
	return ord(s, at) | (ord(s, at + 1) << 8) | (ord(s, at + 2) << 16) | (ord(s, at + 3) * 16777216);
}

function u32be(s, at) {
	return ord(s, at) * 16777216 + ((ord(s, at + 1) << 16) | (ord(s, at + 2) << 8) | ord(s, at + 3));
}

// stamp is when the kernel says a frame came, in ms on the calendar's
// clock, from what a socket hands with the frame where it was asked for
// the time in nanoseconds; null where it does not say, or says a time
// that is not within five seconds before near (ms), as one read wrongly
// is not. The kernel's stamp is seconds and the part of one, eight bytes
// each or four, in the processor's own order; ucode hands those bytes as
// they are. Read into numbers, as a later ucode may, a stamp with no part
// of a second at all is one half read, and is not believed: so ucode reads
// the stamp in microseconds on a 32-bit processor, where every frame
// would be put at the start of its second.
function stamp(data, near) {
	let tries = [];
	if (type(data) == 'object' && type(data.sec) == 'int') {
		let part = type(data.nsec) == 'int' ? data.nsec / 1e6 : type(data.usec) == 'int' ? data.usec / 1e3 : 0;
		if (part > 0)
			push(tries, data.sec * 1000 + part);
	}
	else if (type(data) == 'string' && length(data) == 16) {
		push(tries, u32(data, 0) * 1000 + u32(data, 8) / 1e6);
		push(tries, u32be(data, 4) * 1000 + u32be(data, 12) / 1e6);
	}
	else if (type(data) == 'string' && length(data) == 8) {
		push(tries, u32(data, 0) * 1000 + u32(data, 4) / 1e6);
		push(tries, u32be(data, 0) * 1000 + u32be(data, 4) / 1e6);
	}
	for (let ms in tries)
		if (near - ms >= 0 && near - ms < 5000)
			return ms;
	return null;
}

// header reads a frame's radiotap header: rt, where the 802.11 frame
// begins; sum, whether it says the frame ends in its check sequence; and
// heard, the radio's own time of a frame it heard, in microseconds on a
// clock of its own, where it gives one. Null for what has no header.
function header(m) {
	if (length(m) < 8)
		return null;
	let rt = ord(m, 2) | (ord(m, 3) << 8);
	if (rt < 8 || rt > length(m))
		return null;
	// What it has is said in a word, or several; then a time of eight
	// bytes, on a boundary of eight, and its flags.
	let at = 4, word = 0x80000000;
	while (word >= 0x80000000 && at + 4 <= rt) {
		word = u32(m, at);
		at += 4;
	}
	let has = ord(m, 4), heard = null;
	if (has & 1) {
		at = (at + 7) & ~7;
		if (at + 8 <= rt)
			heard = u32(m, at) + u32(m, at + 4) * 4294967296;
		at += 8;
	}
	return { rt: rt, heard: heard, sum: (has & 2) != 0 && at < rt && (ord(m, at) & 0x10) != 0 };
}

// addresses are the stations a frame from a monitor interface names: the
// one it is for, the one that sent it, and its third, the network's or the
// far end's. Null for a frame too short to have the first two.
function addresses(m) {
	if (length(m) < 8)
		return null;
	let rt = ord(m, 2) | (ord(m, 3) << 8);
	if (rt < 8 || length(m) < rt + 16)
		return null;
	return [mac_text(m, rt + 4), mac_text(m, rt + 10), length(m) >= rt + 22 ? mac_text(m, rt + 16) : null];
}

// exchange is what ties a message of the AP's to the client's answer to
// it, where a frame is one of a key handshake or of a sign-in, in the
// clear: a key message's replay counter, which the answer repeats, or an
// EAP request's identifier, which its response does. Null for any other.
function exchange(m) {
	let h = header(m), rt = h?.rt;
	if (!h || length(m) < rt + 24)
		return null;
	let fc0 = ord(m, rt), fc1 = ord(m, rt + 1);
	// A data frame, not encrypted: with its QoS field where it has one,
	// and a fourth address where it goes from one AP to another.
	if ((fc0 & 0x0c) != 0x08 || (fc1 & 0x40))
		return null;
	let at = rt + 24 + ((fc0 & 0x80) ? 2 : 0) + ((fc1 & 3) == 3 ? 6 : 0);
	if (length(m) < at + 12 || substr(m, at, 8) != chr(0xaa, 0xaa, 0x03, 0, 0, 0, 0x88, 0x8e))
		return null;
	at += 8;
	let kind = ord(m, at + 1);
	if (kind == 3 && length(m) >= at + 17)
		return 'key ' + join('', map([9, 10, 11, 12, 13, 14, 15, 16], i => sprintf('%02x', ord(m, at + i))));
	if (kind == 0 && length(m) >= at + 6 && (ord(m, at + 4) == 1 || ord(m, at + 4) == 2))
		return 'eap ' + ord(m, at + 5);
	return null;
}

// ring is a radio's frames, the latest last.
function ring() {
	return { list: [], bytes: 0 };
}

// ring_add keeps a frame a radio's monitor interface gave, with when it
// came: by the steady clock in seconds and by the calendar's in ms. What
// is older than RING_SECONDS goes, and the oldest where the ring holds
// more than RING_BYTES or RING_FRAMES: not at each frame, as making the
// list anew costs as much as it holds, but when it is a quarter past one
// of them, back to it. A frame that names no station is not kept.
function ring_add(r, at, wall, m) {
	if (length(m) < 8)
		return;
	let rt = ord(m, 2) | (ord(m, 3) << 8);
	if (rt < 8 || length(m) < rt + 16)
		return;
	push(r.list, { at: at, wall: wall, m: m });
	r.bytes += length(m);
	if (r.bytes <= RING_BYTES * 1.25 && length(r.list) <= RING_FRAMES * 1.25 && at - r.list[0].at <= RING_SECONDS * 1.25)
		return;
	let gone = 0;
	while (gone < length(r.list) - 1 && (at - r.list[gone].at > RING_SECONDS || r.bytes > RING_BYTES || length(r.list) - gone > RING_FRAMES))
		r.bytes -= length(r.list[gone++].m);
	if (gone)
		r.list = slice(r.list, gone);
}

// mac_bytes is a MAC as it is in a frame.
function mac_bytes(mac) {
	return join('', map(split(mac, ':'), x => chr(hex(x))));
}

// ring_of is a client's frames in a ring, from a time to a time by the
// steady clock: those it sent, and those sent to it. MOST at the most, the
// latest of them.
function ring_of(r, mac, from, to) {
	let raw = mac_bytes(mac), out = [];
	for (let f in r.list) {
		if (f.at < from || f.at > to)
			continue;
		let rt = ord(f.m, 2) | (ord(f.m, 3) << 8);
		if (substr(f.m, rt + 4, 6) == raw || substr(f.m, rt + 10, 6) == raw)
			push(out, f);
	}
	return length(out) > MOST ? slice(out, -MOST) : out;
}

function le16(n) {
	return chr(n & 255, (n >> 8) & 255);
}

function le32(n) {
	return chr(n & 255, (n >> 8) & 255, (n >> 16) & 255, (n >> 24) & 255);
}

// padded is bytes made up to a multiple of four, as pcapng has them.
function padded(s) {
	let more = (4 - length(s) % 4) % 4;
	for (let i = 0; i < more; i++)
		s += chr(0);
	return s;
}

function block(type, body) {
	let n = 12 + length(body);
	return le32(type) + le32(n) + body + le32(n);
}

// An option: its code, its length and its value, padded.
function option(code, value) {
	return le16(code) + le16(length(value)) + padded(value);
}

// pcapng is a capture as a file: frames, each { wall, m, link }, link 0 for
// one from the radio and 1 for one from the wire side, in the order given;
// and what the two are named in it. Times are the calendar's, wall in ms,
// written in microseconds, which a file's times are unless it says. A
// frame with a note has it as its comment, which Wireshark shows with it.
function pcapng(radio, wire, frames) {
	let out = block(0x0a0d0d0a, le32(0x1a2b3c4d) + le16(1) + le16(0) + le32(0xffffffff) + le32(0xffffffff) +
		option(4, 'Aeolus') + option(0, ''));
	let iface = (link, name) => block(1, le16(link) + le16(0) + le32(SNAP) + option(2, name) + option(0, ''));
	out += iface(LINK_RADIO, radio) + iface(LINK_WIRE, wire);
	for (let f in frames) {
		let us = int(f.wall * 1000 + 0.5), hi = int(us / 4294967296), lo = us % 4294967296;
		out += block(6, le32(f.link) + le32(hi) + le32(lo) + le32(length(f.m)) + le32(length(f.m)) + padded(f.m) +
			(f.note ? option(1, f.note) + option(0, '') : ''));
	}
	return out;
}

// The table a frame check sequence is worked out with: made when first
// needed, as a capture is made seldom.
let crc_table = null;

// crc_run takes a CRC-32 on over bytes from to; crc_bytes is it as a frame
// check sequence is sent: low byte first.
function crc_run(c, s, from, to) {
	if (!crc_table) {
		crc_table = [];
		for (let n = 0; n < 256; n++) {
			let c = n;
			for (let k = 0; k < 8; k++)
				c = (c & 1) ? (0xedb88320 ^ (c >> 1)) : (c >> 1);
			push(crc_table, c);
		}
	}
	for (let i = from; i < to; i++)
		c = crc_table[(c ^ ord(s, i)) & 255] ^ (c >> 8);
	return c;
}

function crc_bytes(c) {
	c = (c ^ 0xffffffff) & 0xffffffff;
	return chr(c & 255, (c >> 8) & 255, (c >> 16) & 255, (c >> 24) & 255);
}

// fcs is a frame check sequence, the CRC-32 of bytes from to.
function fcs(s, from, to) {
	return crc_bytes(crc_run(0xffffffff, s, from, to));
}

// tidy takes from a management frame what a radio left after it. Some
// hand a frame up with its check sequence, and four bytes more, behind it,
// and a radiotap header that does not say so (ath11k's monitor interface,
// the C-360, 2026-10-11): Wireshark then reads them as part of the frame,
// and calls it malformed. Where the bytes before the last four, or the last
// eight, end in the check sequence of all that is before them, they go; a
// frame whose header says it has a check sequence is as it says, and one
// that is no management frame is left be, as its check sequence is not
// always the frame's own. The frame is gone over once for both.
function tidy(m) {
	let h = header(m), rt = h?.rt, n = length(m);
	if (!h || h.sum || n < rt + 24 + 4 || (ord(m, rt) & 0x0c) != 0)
		return m;
	let c = 0xffffffff, at = rt;
	if (n - 8 >= rt + 24) {
		c = crc_run(c, m, rt, n - 8);
		at = n - 8;
		if (substr(m, n - 8, 4) == crc_bytes(c))
			return substr(m, 0, n - 8);
	}
	c = crc_run(c, m, at, n - 4);
	return substr(m, n - 4, 4) == crc_bytes(c) ? substr(m, 0, n - 4) : m;
}

// loses_offload says whether a radio's clients would lose what the radio
// does for them in hardware were a monitor interface made on it, from the
// radio's own list of what it can do (mac80211's hwflags, a line each).
// One that builds and takes apart each frame's 802.11 header itself does
// so for no client while a monitor interface is there, unless it can hand
// frames up both ways at once: every frame of every client then goes the
// slow way through the AP's processor, and cannot be handed straight to
// the wire (mt76: the OpenWrt One, 2026-10-11). True where it is not
// known: what a radio is not known to keep, it is not risked.
function loses_offload(hwflags) {
	if (type(hwflags) != 'string')
		return true;
	let has = (f) => match(hwflags, regexp('(^|\n)' + f + '(\n|$)')) != null;
	return has('SUPPORTS_TX_ENCAP_OFFLOAD') || (has('SUPPORTS_RX_DECAP_OFFLOAD') && !has('SUPPORTS_CONC_MON_RX_DECAP'));
}

// capture is the file of one attempt, or null where nothing was kept of
// it: the client's frames from its radio's ring, around the attempt, and
// those kept from the wire side, each { at, wall, m }, in the order they
// came. from and to are the attempt's start and end by the steady clock.
// 512 kB of frames at the most, the latest: how it ended says more of a
// failure than how it began.
//
// A frame the radio heard is put at the radio's own time of it, where it
// gives one. Some hand a frame up late: ath11k's monitor interface gave the
// client's key messages a second after it heard them, the next message
// already sent (the C-360, 2026-10-11), so by when they came the client
// seems to answer a second late. The radio's clock is set against the
// calendar's by the frame of those minutes that came soonest after it was
// heard: one that came later than that by up to LATE ms is put back by as
// much.
//
// A frame the AP sent has no such time: it is put at when the radio said
// it had sent it, which some say late (mt76's monitor interface gave a key
// message 1 to 12 ms after the client's answer to it had come: the
// OpenWrt One, 2026-10-11), so the answer seems to come first. A message
// the client answered was sent before the answer, whatever the radio
// said: where it is said up to SOON ms after, it is put just before the
// answer, with a note of how much later it was said; and what the AP had
// sent before it stays before it. Only what this radio sent is so moved:
// what it heard, of whoever, has the radio's own time.
function capture(r, mac, from, to, wire, radio_name, wire_name) {
	let least = null;
	for (let f in r?.list ?? []) {
		if (f.at < from - BEFORE - 30 || f.at > to + AFTER + 30)
			continue;
		let heard = header(f.m)?.heard;
		if (heard == null)
			continue;
		let after = f.wall - heard / 1000.0;
		if (least == null || after < least)
			least = after;
	}
	let got = r ? ring_of(r, mac, from - BEFORE, to + AFTER) : [];
	let first = length(got), size = 0;
	for (let f in wire ?? [])
		size += length(f.m);
	for (; first > 0 && size + length(got[first - 1].m) <= MOST_BYTES; first--)
		size += length(got[first - 1].m);
	got = slice(got, first);
	let raw = mac_bytes(mac), frames = [], answers = {};
	for (let f in got) {
		let h = header(f.m), heard = h?.heard;
		let late = heard != null && least != null ? f.wall - heard / 1000.0 - least : 0;
		if (late <= 0 || late >= LATE)
			late = 0;
		let its = h != null && substr(f.m, h.rt + 10, 6) == raw;
		let e = { wall: f.wall - late, m: tidy(f.m), link: 0, n: length(frames), sent: heard == null && !its, x: exchange(f.m) };
		push(frames, e);
		if (its && e.x) {
			if (!answers[e.x])
				answers[e.x] = [];
			push(answers[e.x], e);
		}
	}
	// The AP's messages, the last first, so that one put back takes with
	// it those the AP had sent before it.
	let next = null;
	for (let i = length(frames) - 1; i >= 0; i--) {
		let f = frames[i], answer = null;
		if (!f.sent)
			continue;
		for (let g in (f.x ? answers[f.x] ?? [] : []))
			if (g.wall < f.wall && f.wall - g.wall <= SOON && (!answer || g.wall < answer.wall))
				answer = g;
		if (answer) {
			f.note = sprintf('Sent before the answer to it, and put there: the radio said it had sent this %.1f ms after the answer came.', f.wall - answer.wall);
			f.wall = answer.wall - 0.001;
		}
		if (next != null && f.wall >= next) {
			f.note = f.note ?? 'Put before the next message the AP sent, which is put before the answer to it: the radio said it had sent this after that answer came.';
			f.wall = next - 0.001;
		}
		if (f.note)
			next = f.wall;
	}
	for (let f in wire ?? [])
		push(frames, { wall: f.wall, m: f.m, link: 1, n: length(frames) });
	if (!length(frames))
		return null;
	// By when they came, as the file says it; two at once as they were kept.
	frames = sort(frames, (a, b) => a.wall < b.wall ? -1 : a.wall > b.wall ? 1 : a.n - b.n);
	let radio = length(filter(frames, f => f.link == 0));
	return { frames: length(frames), radio: radio, wire: length(frames) - radio, file: pcapng(radio_name, wire_name, frames) };
}

export { SNAP, WIRE, AFTER, MOST_BYTES, stamp, exchange, radio_filter, header, addresses, ring, ring_add, ring_of, fcs, tidy, loses_offload, pcapng, capture };
