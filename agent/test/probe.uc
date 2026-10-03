// What the prober's frames are, and how it reads and judges them (0059):
// TestProbeFrames runs this with the real ucode and compares what it prints
// with probe.out. The checksums in probe.out were also worked out apart from
// ucode.

'use strict';

import * as probe from 'aeolus.probe';

function hexs(s) {
	let out = '';
	for (let i = 0; i < length(s); i++)
		out += sprintf('%02x', ord(s, i));
	return out;
}

function bytes(list) {
	return join('', map(list, b => chr(b)));
}

const SRC = 'aa:04:60:21:36:60';
const LL = 'fe80::ac04:60ff:fe21:3661';

// An ARP probe, and the Arista's answer to it.
let arp = probe.arp_probe(SRC, '192.168.50.1');
print('arp probe ', hexs(arp), '\n');
let reply = bytes([0xaa, 0x04, 0x60, 0x21, 0x36, 0x60, 0x28, 0xe7, 0x1d, 0xca, 0x29, 0x13, 0x08, 0x06,
	0, 1, 8, 0, 6, 4, 0, 2, 0x28, 0xe7, 0x1d, 0xca, 0x29, 0x13, 192, 168, 50, 1,
	0xaa, 0x04, 0x60, 0x21, 0x36, 0x60, 0, 0, 0, 0]);
print('arp answer ', probe.answer(reply, SRC, 0x1234), '\n');
print('arp answer to another AP ', probe.answer(reply, 'aa:04:60:21:36:61', 0x1234), '\n');
print('arp request is no answer ', probe.answer(arp, SRC, 0x1234), '\n');

// An echo to IPv6 all-nodes, and an answer to it.
let e6 = probe.echo6(SRC, LL, 0x1234, 1);
print('echo6 ', hexs(e6), '\n');
let r6 = bytes([0xaa, 0x04, 0x60, 0x21, 0x36, 0x60, 0x28, 0xe7, 0x1d, 0xca, 0x29, 0x13, 0x86, 0xdd,
	0x60, 0, 0, 0, 0, 16, 58, 64,
	0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0x2a, 0xe7, 0x1d, 0xff, 0xfe, 0xca, 0x29, 0x13,
	0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0xac, 0x04, 0x60, 0xff, 0xfe, 0x21, 0x36, 0x61,
	129, 0, 0, 0, 0x12, 0x34, 0, 1]) + 'aeolus59';
print('echo6 answer ', probe.answer(r6, SRC, 0x1234), '\n');
print('echo6 answer to another id ', probe.answer(r6, SRC, 0x4321), '\n');

// A ping to the concentrator, and its reply as a raw socket gets it.
let e4 = probe.echo4(0x1234, 7);
print('echo4 ', hexs(e4), '\n');
let r4 = bytes([0x45, 0, 0, 36, 0, 0, 0, 0, 64, 1, 0, 0, 1, 1, 1, 2, 192, 168, 1, 38,
	0, 0, 0, 0, 0x12, 0x34, 0, 7]) + 'aeolus59';
print('echo4 answer ', probe.echo4_answer(r4, 0x1234), '\n');
print('echo4 request is no answer ', probe.echo4_answer(bytes([0x45, 0, 0, 36, 0, 0, 0, 0, 64, 1, 0, 0, 1, 1, 1, 2, 192, 168, 1, 38]) + e4, 0x1234), '\n');
print('echo6 plain ', hexs(probe.echo6_plain(0x1234, 7)), '\n');
print('echo6 plain answer ', probe.echo6_plain_answer(bytes([129, 0, 0, 0, 0x12, 0x34, 0, 7]), 0x1234), '\n');

// The Internet checksum of RFC 1071's example.
print('checksum ', sprintf('%04x', probe.checksum(bytes([0x00, 0x01, 0xf2, 0x03, 0xf4, 0xf5, 0xf6, 0xf7]))), '\n');

// IPv6 addresses, there and back.
for (let a in ['2001:db8::2', 'fe80::ac04:60ff:fe21:3661', 'ff02::1', '::', '1:0:0:2:0:0:0:3', '2001:db8:0:1:1:1:1:1'])
	print('ip6 ', a, ' -> ', probe.ip6_text(probe.ip6(a), 0), '\n');

// The loop guard's frame, and whose it is.
let g = probe.guard_frame('aa:04:60:21:36:63', '00112233aabbccdd', 'lan3', 'lan3.30');
print('guard ', hexs(g), '\n');
print('guard seen ', probe.guard_seen(g, '00112233aabbccdd'), '\n');
print('guard of another run ', probe.guard_seen(g, 'ffeeddccbbaa9988'), '\n');
print('not a guard frame ', probe.guard_seen(arp, '00112233aabbccdd'), '\n');

// How a tunnel is judged, its interval 30 seconds and its probes started at 0.
for (let c in [
	['answered 10 s ago', { answered: 990 }, 1000],
	['answered 100 s ago', { answered: 900, underlay: 999 }, 1000],
	['just started', {}, 60],
	['never answered, concentrator does', { underlay: 990 }, 1000],
	['never answered, nor the concentrator', {}, 1000],
	['answered at 90, now 180', { answered: 90 }, 180],
	['answered at 90, now 181', { answered: 90 }, 181],
])
	print('verdict ', c[0], ': ', probe.verdict({ interval: 30, started: 0, ...c[1] }, c[2]), '\n');

// Every jump in the filters lands inside them.
for (let name, prog in { overlay: probe.OVERLAY_FILTER, guard: probe.GUARD_FILTER }) {
	let ok = true;
	for (let i = 0; i < length(prog); i++)
		if ((prog[i][0] & 0x07) == 0x05 && (i + 1 + prog[i][1] >= length(prog) || i + 1 + prog[i][2] >= length(prog)))
			ok = false;
	print('filter ', name, ': ', length(prog), ' instructions, jumps ', ok ? 'land inside' : 'LEAVE IT', ', ends ', prog[length(prog) - 1][0] == 0x06 ? 'returning' : 'NOT RETURNING', '\n');
}
