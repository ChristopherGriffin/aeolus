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
// Where it holds too much, the oldest go; and of one client so many are
// given at the most, the latest.
let big = capture.ring();
for (let i = 0; i < 700; i++)
	capture.ring_add(big, 1000 + i / 100, 1791745000000 + i * 10, radiotap(18) + dot11(0x00, 0, AP, STA, AP) + zeros(2000));
printf('a full ring: %d frames, %d bytes, the oldest from %.2f\n', length(big.list), big.bytes, big.list[0].at);
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

// What is older than three minutes goes from a ring.
capture.ring_add(r, 290.000, 1791744690000, auth);
printf('three minutes on: %d frames, the oldest from %.3f\n', length(r.list), r.list[0].at);
