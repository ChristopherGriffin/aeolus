// What aeolus-journey keeps of a client's failed attempt as a capture
// (0135): which frames a radio's monitor interface is let through, by a
// small interpreter of the kernel's filter; what a radio's ring keeps and
// gives of one client; and the file made of them. internal/rendercheck runs
// this with the real ucode and compares what it prints with capture.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/capture.uc

'use strict';

import * as capture from 'aeolus.capture';

function bytes(list) {
	return join('', map(list, b => chr(b)));
}
function zeros(n) {
	let s = '';
	for (let i = 0; i < n; i++)
		s += chr(0);
	return s;
}
function mac(t) {
	return bytes(map(split(t, ':'), x => hex(x)));
}
function hexdump(s) {
	return join('', map(split(s, ''), c => sprintf('%02x', ord(c))));
}

// run is the kernel's filter, for the steps the radio's uses.
function run(prog, f) {
	let a = 0, x = 0;
	let byte = at => at < length(f) ? ord(f, at) : null;
	for (let pc = 0; pc < length(prog); pc++) {
		let s = prog[pc], k = s[3], v;
		switch (s[0]) {
		case 0x30: v = byte(k); break;
		case 0x50: v = byte(x + k); break;
		case 0x48: v = byte(x + k + 1) == null ? null : byte(x + k) << 8 | byte(x + k + 1); break;
		case 0x64: a = (a << k) & 0xffffffff; continue;
		case 0x54: a &= k; continue;
		case 0x4c: a |= x; continue;
		case 0x07: x = a; continue;
		case 0x15: pc += a == k ? s[1] : s[2]; continue;
		case 0x06: return k;
		default: return 'unknown step ' + s[0];
		}
		if (v == null)
			return 0;   // past the frame's end: the kernel drops it
		a = v;
	}
	return 'ran off the end';
}

const AP = '32:86:2d:03:17:c0', STA = '30:86:2d:03:17:f0', OTHER = '72:bb:66:12:2a:94', ALL = 'ff:ff:ff:ff:ff:ff';

// A radiotap header of n bytes: its version, its length, and what else a
// radio puts in it, which is not read.
let radiotap = (n) => bytes([0, 0, n & 255, n >> 8]) + zeros(n - 4);
// An 802.11 header: frame control's two bytes, a duration, three addresses
// and a sequence number; and for a QoS frame its QoS field.
let dot11 = (fc0, fc1, a1, a2, a3) => bytes([fc0, fc1, 0, 0]) + mac(a1) + mac(a2) + mac(a3) + bytes([0x10, 0]);
let llc = (type) => bytes([0xaa, 0xaa, 0x03, 0, 0, 0, type >> 8, type & 255]);
let key = zeros(99);   // an EAPOL-Key frame's body

let frames = {
	'authentication, from the client': radiotap(18) + dot11(0xb0, 0, AP, STA, AP) + zeros(6),
	'authentication, with a long radiotap header': radiotap(56) + dot11(0xb0, 0, STA, AP, AP) + zeros(6),
	'association request': radiotap(18) + dot11(0x00, 0, AP, STA, AP) + zeros(60),
	'probe request, to all': radiotap(18) + dot11(0x40, 0, ALL, STA, ALL) + zeros(40),
	'deauthentication': radiotap(18) + dot11(0xc0, 0, STA, AP, AP) + zeros(2),
	'action': radiotap(18) + dot11(0xd0, 0, AP, STA, AP) + zeros(9),
	'beacon': radiotap(18) + dot11(0x80, 0, ALL, AP, AP) + zeros(200),
	'an acknowledgement': radiotap(18) + bytes([0xd4, 0, 0, 0]) + mac(AP),
	'key message 1, to the client': radiotap(13) + dot11(0x08, 0x02, STA, AP, AP) + llc(0x888e) + key,
	'key message 2, with a QoS field': radiotap(18) + dot11(0x88, 0x01, AP, STA, AP) + bytes([0, 0]) + llc(0x888e) + key,
	'a key message once the keys are in, encrypted': radiotap(18) + dot11(0x88, 0x41, AP, STA, AP) + bytes([0, 0]) + zeros(8) + llc(0x888e) + key,
	'the same with no QoS field': radiotap(18) + dot11(0x08, 0x42, STA, AP, AP) + zeros(8) + llc(0x888e) + key,
	'data that is not EAPOL': radiotap(18) + dot11(0x88, 0x01, AP, STA, AP) + bytes([0, 0]) + llc(0x0800) + zeros(60),
	'a null data frame': radiotap(18) + dot11(0x48, 0x01, AP, STA, AP),
	'too short for a header': radiotap(18) + bytes([0xb0, 0]),
	'nothing but a radiotap header': radiotap(8),
};
let prog = capture.radio_filter();
printf('the radio\'s filter has %d steps\n', length(prog));
for (let name in sort(keys(frames))) {
	let f = frames[name], got = run(prog, f);
	printf('%s: %s; names %J\n', name, got == capture.SNAP ? 'kept' : got == 0 ? 'dropped' : got, capture.addresses(f));
}

// What a radio left behind a management frame goes: the C-360's hands one
// up with its check sequence and four bytes more, as this authentication
// came on 2026-10-11, and its header does not say so. One with its check
// sequence alone loses that; one whose header says it has one, one with
// neither, and a data frame are as they came.
let heard = bytes([0, 0, 0x1a, 0, 0x2f, 0x48, 0, 0, 0xf5, 0xdf, 0x02, 0x0b, 0x2c, 0, 0, 0, 0, 0x0c, 0x64, 0x14, 0x40, 0x01, 0xf7, 0, 0, 0]);
let body = dot11(0xb0, 0, AP, STA, AP);
body = substr(body, 0, 2) + bytes([0x3c, 0]) + substr(body, 4, 18) + bytes([0, 0]) + bytes([0, 0, 1, 0, 0, 0]);
let sum = bytes([0x1a, 0x71, 0xb1, 0x0b]), more = bytes([0, 0, 0x8e, 0xfd]);
printf('its check sequence: %s, worked out %s\n', hexdump(sum), hexdump(capture.fcs(body, 0, length(body))));
let flagged = substr(heard, 0, 16) + chr(0x10) + substr(heard, 17);
for (let name, f in {
	'with its check sequence and four bytes more': heard + body + sum + more,
	'with its check sequence alone': heard + body + sum,
	'with neither': heard + body,
	'its header saying it has one': flagged + body + sum,
	'a short header, and both': radiotap(8) + body + sum + more,
	'a data frame with the same behind it': heard + chr(0x08) + substr(body, 1) + sum + more,
})
	printf('%s: %d bytes of frame, %d kept\n', name, length(f) - (ord(f, 2) | (ord(f, 3) << 8)), length(capture.tidy(f)) - (ord(f, 2) | (ord(f, 3) << 8)));

// A radio's ring: frames by when they came. One client's are taken from it
// by a span of time; what is older than three minutes goes.
let r = capture.ring();
let auth = frames['authentication, from the client'], answer = frames['authentication, with a long radiotap header'];
let other = radiotap(18) + dot11(0xb0, 0, AP, OTHER, AP) + zeros(6);
capture.ring_add(r, 100.000, 1791744500000, frames['probe request, to all']);
capture.ring_add(r, 105.010, 1791744505010, auth);
capture.ring_add(r, 105.012, 1791744505012, answer);
capture.ring_add(r, 105.500, 1791744505500, other);
capture.ring_add(r, 106.000, 1791744506000, frames['key message 1, to the client']);
capture.ring_add(r, 106.100, 1791744506100, frames['nothing but a radiotap header']);
printf('ring: %d frames, %d bytes\n', length(r.list), r.bytes);
printf('the client\'s, from 105 to 107: %d; from 90: %d; another\'s: %d; no one\'s: %d\n',
	length(capture.ring_of(r, STA, 105, 107)), length(capture.ring_of(r, STA, 90, 107)), length(capture.ring_of(r, OTHER, 90, 107)),
	length(capture.ring_of(r, '02:00:00:00:00:01', 90, 107)));
// Where it holds too much, the oldest go: when it is a quarter past what
// it may hold, back to that, so that the list is made anew now and then
// and not at each frame. And of one client so many are given at the most,
// the latest.
let big = capture.ring(), trims = 0, most_held = 0;
for (let i = 0; i < 2600; i++) {
	let before = length(big.list);
	capture.ring_add(big, 1000 + i / 100, 1791745000000 + i * 10, radiotap(18) + dot11(0x00, 0, AP, STA, AP) + zeros(2000));
	if (length(big.list) <= before)
		trims++;
	most_held = max(most_held, big.bytes);
}
printf('a full ring: %d frames, %d bytes, never more than %d, made anew %d times in 2600 frames, the oldest from %.2f\n', length(big.list), big.bytes, most_held, trims, big.list[0].at);
let small = capture.ring();
for (let i = 0; i < 10100; i++)
	capture.ring_add(small, 1000 + i / 1000, 1791745000000 + i, auth);
printf('a ring of small frames: %d at the most, %d now\n', 10000, length(small.list));
let many = capture.ring();
for (let i = 0; i < 700; i++)
	capture.ring_add(many, 1000 + i / 100, 1791745000000 + i * 10, auth);
let most = capture.ring_of(many, STA, 0, 2000);
printf('of %d frames of one client, %d are given, from %.2f\n', length(many.list), length(most), most[0].at);

// The file of an attempt from 105.0 to 106.0: the client's frames of the
// ten seconds before it and the two after, with those from the wire side
// in among them by when they came. Read back a block at a time.
let wire = [{ at: 105.300, wall: 1791744505300, m: mac(ALL) + mac(STA) + bytes([0x08, 0x00]) + zeros(300) }];
let c = capture.capture(r, STA, 105.0, 106.0, wire, 'phy0', 'phy0-ap1');
printf('capture: %d frames, %d from the radio, %d from the wire side, %d bytes\n', c.frames, c.radio, c.wire, length(c.file));
let u32 = (s, at) => ord(s, at) | (ord(s, at + 1) << 8) | (ord(s, at + 2) << 16) | (ord(s, at + 3) * 16777216);
for (let at = 0; at < length(c.file); ) {
	let type = u32(c.file, at), n = u32(c.file, at + 4);
	if (n < 12 || n % 4 || at + n > length(c.file) || u32(c.file, at + n - 4) != n) {
		printf('a block that does not close at %d\n', at);
		break;
	}
	if (type == 0x0a0d0d0a)
		printf('  section: byte order %08x, version %d.%d\n', u32(c.file, at + 8), ord(c.file, at + 12), ord(c.file, at + 14));
	else if (type == 1)
		printf('  interface: link type %d, at most %d bytes a frame\n', ord(c.file, at + 8) | (ord(c.file, at + 9) << 8), u32(c.file, at + 12));
	else if (type == 6)
		printf('  frame: interface %d, at %d us, %d bytes, begins %s\n', u32(c.file, at + 8), u32(c.file, at + 12) * 4294967296 + u32(c.file, at + 16),
			u32(c.file, at + 20), hexdump(substr(c.file, at + 28, 4)));
	else
		printf('  a block of type %d\n', type);
	at += n;
}
printf('nothing kept of a client: %J\n', capture.capture(r, '02:00:00:00:00:01', 105, 106, [], 'phy0', 'phy0-ap1'));
printf('no ring, and frames from the wire side alone: %d\n', capture.capture(null, STA, 105, 106, wire, 'phy0', 'phy0-ap1').frames);
// A frame the radio heard is put at the radio's own time of it. This one
// hands the client's key messages up a second late, as the C-360's does:
// its authentication came 2 ms after it was heard, and its answer to the
// first key message, heard 30 ms after that, came a second on, the AP's
// next try already sent.
let at_radio = (us) => bytes([0, 0, 16, 0, 0x01, 0, 0, 0]) + bytes(map([0, 8, 16, 24, 32, 40, 48, 56], (b) => (us / (2 ** b)) & 255));
let late = capture.ring();
capture.ring_add(late, 500.002, 1791745000002, at_radio(77000000) + dot11(0xb0, 0, AP, STA, AP) + zeros(6));
capture.ring_add(late, 500.020, 1791745000020, radiotap(13) + dot11(0x08, 0x02, STA, AP, AP) + llc(0x888e) + key);
capture.ring_add(late, 501.020, 1791745001020, radiotap(13) + dot11(0x08, 0x02, STA, AP, AP) + llc(0x888e) + key);
capture.ring_add(late, 501.021, 1791745001021, at_radio(77030000) + dot11(0x88, 0x01, AP, STA, AP) + bytes([0, 0]) + llc(0x888e) + key);
printf('a header\'s time: %J\n', capture.header(late.list[0].m));
let c2 = capture.capture(late, STA, 500, 502, [], 'phy0', 'phy0-ap1');
for (let at = 0, n = 0; at < length(c2.file); at += u32(c2.file, at + 4))
	if (u32(c2.file, at) == 6)
		printf('  frame %d: at %d us, begins %s\n', ++n, (u32(c2.file, at + 12) * 4294967296 + u32(c2.file, at + 16)) % 10000000, hexdump(substr(c2.file, at + 28, 4)));

// What is older than three minutes goes from a ring, once it is a quarter
// past that; and is given of no client before then, by its time.
capture.ring_add(r, 290.000, 1791744690000, auth);
printf('three minutes on: %d frames held, %d of the client given from then\n', length(r.list), length(capture.ring_of(r, STA, 290 - 180, 290)));
capture.ring_add(r, 340.000, 1791744740000, auth);
printf('four minutes on: %d frames, the oldest from %.3f\n', length(r.list), r.list[0].at);

// A file holds so many bytes of frames at the most: the latest.
let fat = capture.ring();
for (let i = 0; i < 400; i++)
	capture.ring_add(fat, 700 + i / 100.0, 1791745700000 + i * 10, radiotap(13) + dot11(0xb0, 0, AP, STA, AP) + zeros(1960));
let c3 = capture.capture(fat, STA, 700, 704, [], 'phy0', 'phy0-ap1');
printf('of %d large frames of one client, %d are in its file: %s, as it may hold %d bytes of them\n', length(fat.list), c3.frames,
	c3.frames * length(fat.list[0].m) <= capture.MOST_BYTES && (c3.frames + 1) * length(fat.list[0].m) > capture.MOST_BYTES ? 'as many as fit' : 'not as many as fit', capture.MOST_BYTES);

// When the kernel says a frame came: its stamp as a socket hands it, the
// bytes as they are, in either order and of either size; or read into
// numbers. One that is not near now, or has no part of a second at all, as
// ucode reads the stamp on a 32-bit processor, is not believed.
let near = 1791750185400;
let raw = (list) => bytes(list);
printf('a stamp of 16 bytes: %.3f; the other way round: %.3f; of 8 bytes: %.3f\n',
	capture.stamp(raw([0x29, 0xf0, 0xcb, 0x6a, 0, 0, 0, 0, 0x5d, 0x7f, 0x27, 0x15, 0, 0, 0, 0]), near),
	capture.stamp(raw([0, 0, 0, 0, 0x6a, 0xcb, 0xf0, 0x29, 0, 0, 0, 0, 0x15, 0x27, 0x7f, 0x5d]), near),
	capture.stamp(raw([0x29, 0xf0, 0xcb, 0x6a, 0x5d, 0x7f, 0x27, 0x15]), near));
printf('read into numbers: %.3f, %.3f; with no part of a second: %J; from another day: %J; later than now: %J; nothing: %J\n',
	capture.stamp({ sec: 1791750185, usec: 354910 }, near), capture.stamp({ sec: 1791750185, nsec: 354910045 }, near),
	capture.stamp({ sec: 1791750185, usec: 0 }, near), capture.stamp({ sec: 1791663785, usec: 354910 }, near),
	capture.stamp({ sec: 1791750186, usec: 354910 }, near), capture.stamp(null, near));

// A message of the AP's is put before the client's answer to it, whatever
// time the radio gave it. This radio says what it sent some milliseconds
// after it did, as the OpenWrt One's does: the client's answer to the
// first key message, heard 4 ms after the association's answer went, is
// in hand before the radio says the message was sent; so with the second.
// The association's answer, said after the client's answer too, stays
// before the message that followed it. A message said a second after an
// answer with its number is another's, and is left.
let keymsg = (n) => bytes([2, 3, 0, 95, 2, 0, 0x8a, 0, 16, 0, 0, 0, 0, 0, 0, 0, n]) + zeros(82);
let from_ap = (body) => radiotap(13) + dot11(0x08, 0x02, STA, AP, AP) + llc(0x888e) + body;
let from_sta = (us, body) => at_radio(us) + dot11(0x88, 0x01, AP, STA, AP) + bytes([0, 0]) + llc(0x888e) + body;
printf('what ties a key message to its answer: %J and %J; an EAP request to its response: %J and %J; a frame of neither: %J\n',
	capture.exchange(from_ap(keymsg(1))), capture.exchange(from_sta(1, keymsg(1))),
	capture.exchange(from_ap(bytes([2, 0, 0, 5, 1, 7, 0, 5, 1]))), capture.exchange(from_sta(1, bytes([2, 0, 0, 5, 2, 7, 0, 5, 1]))),
	capture.exchange(from_ap(key)));
let slow = capture.ring();
capture.ring_add(slow, 600.000, 1791745100000, at_radio(88000000) + dot11(0x00, 0, AP, STA, AP) + zeros(6));        // it asks to associate
capture.ring_add(slow, 600.006, 1791745100006, from_sta(88006000, keymsg(1)));                                      // its answer to the first message
capture.ring_add(slow, 600.0065, 1791745100006.5, radiotap(13) + dot11(0x10, 0, STA, AP, AP) + zeros(6));           // the association's answer, said late
capture.ring_add(slow, 600.007, 1791745100007, from_ap(keymsg(1)));                                                 // the first message, later still
capture.ring_add(slow, 601.004, 1791745101004, from_sta(89004000, keymsg(2)));
capture.ring_add(slow, 601.016, 1791745101016, from_ap(keymsg(2)));
capture.ring_add(slow, 602.300, 1791745102300, from_ap(keymsg(1)));                                                 // of another handshake
let c4 = capture.capture(slow, STA, 600, 603, [], 'phy1', 'phy1-ap3');
for (let at = 0, n = 0; at < length(c4.file); at += u32(c4.file, at + 4)) {
	if (u32(c4.file, at) != 6)
		continue;
	let size = u32(c4.file, at + 20), after = at + 28 + size + (4 - size % 4) % 4, noted = after < at + u32(c4.file, at + 4) - 4;
	printf('  frame %d: at %d us, %s%s\n', ++n, (u32(c4.file, at + 12) * 4294967296 + u32(c4.file, at + 16)) % 10000000,
		ord(c4.file, at + 28 + 2) == 16 ? 'from the client' : 'from the AP',
		noted ? sprintf(', noted: %s', substr(c4.file, after + 4, u32(c4.file, after) >> 16)) : '');
}

// A frame the radio heard from another, to the client, is not the AP's
// own, and is not moved for an answer: a neighbour's probe response among
// the client's key messages stays where the radio heard it.
let third = capture.ring();
capture.ring_add(third, 800.000, 1791745300000, at_radio(99000000) + dot11(0x00, 0, AP, STA, AP) + zeros(6));
capture.ring_add(third, 800.006, 1791745300006, from_sta(99006000, keymsg(1)));
capture.ring_add(third, 800.007, 1791745300007, at_radio(99006500) + dot11(0x50, 0, STA, OTHER, OTHER) + zeros(12));
capture.ring_add(third, 800.008, 1791745300008, from_ap(keymsg(1)));
let c5 = capture.capture(third, STA, 800, 801, [], 'phy1', 'phy1-ap3');
for (let at = 0, n = 0; at < length(c5.file); at += u32(c5.file, at + 4)) {
	if (u32(c5.file, at) != 6)
		continue;
	let size = u32(c5.file, at + 20), after = at + 28 + size + (4 - size % 4) % 4;
	printf('  frame %d: at %d us%s\n', ++n, (u32(c5.file, at + 12) * 4294967296 + u32(c5.file, at + 16)) % 10000000, after < at + u32(c5.file, at + 4) - 4 ? ', noted' : '');
}

// Whether a radio's clients would lose what it does for them in hardware
// to a monitor interface, by what the radio says it can do.
for (let name, flags in {
	'a radio that leaves the 802.11 header to the AP (ath11k, ath10k)': 'HAS_RATE_CONTROL\nSIGNAL_DBM\nSUPPORTS_FAST_FRAG\n',
	'one that makes and unmakes it itself (mt76)': 'SIGNAL_DBM\nSUPPORTS_TX_ENCAP_OFFLOAD\nSUPPORTS_RX_DECAP_OFFLOAD\n',
	'one that unmakes it, and can hand frames up both ways': 'SUPPORTS_RX_DECAP_OFFLOAD\nSUPPORTS_CONC_MON_RX_DECAP\n',
	'one that says nothing': '',
})
	printf('%s: %s\n', name, capture.loses_offload(flags) ? 'it would' : 'it would not');
printf('a radio whose list cannot be read: %s\n', capture.loses_offload(null) ? 'it would' : 'it would not');
