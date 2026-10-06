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

// Which transport carries a network with a fallback (0061), its hold-down
// 300 seconds, now at 1000.
let net = (o) => {
	let n = { mode: 'automatic', ha: false, failback: 'revertive', holddown: 300, want: 'primary',
		primary: 'up', fallback: 'up', fallback_vxlan: false, up_since: null };
	for (let k, v in o)
		n[k] = v;
	return n;
};
for (let c in [
	['report mode, primary down', { mode: 'report', primary: 'down' }],
	['primary up', {}],
	['primary down, fallback up', { primary: 'down' }],
	['primary down, fallback unverified', { primary: 'down', fallback: 'unverified' }],
	['primary down, fallback down', { primary: 'down', fallback: 'down' }],
	['primary unverified', { primary: 'unverified' }],
	['primary down, tunnel fallback stopped', { primary: 'down', fallback: 'off', fallback_vxlan: true }],
	['primary down, tunnel fallback started and up', { primary: 'down', fallback_vxlan: true }],
	['on the fallback, primary still down', { want: 'fallback', primary: 'down' }],
	['on the fallback, primary up 100 s', { want: 'fallback', up_since: 900 }],
	['on the fallback, primary up 300 s', { want: 'fallback', up_since: 700 }],
	['on the fallback, primary up 300 s, failback equal', { want: 'fallback', up_since: 700, failback: 'equal' }],
	['on the fallback, fallback down, primary up', { want: 'fallback', fallback: 'down', up_since: 990, failback: 'equal' }],
	['on the fallback, both down', { want: 'fallback', fallback: 'down', primary: 'down' }],
	['on the tunnel fallback, back to the primary', { want: 'fallback', up_since: 700, fallback_vxlan: true }],
	['on the tunnel fallback, back to the primary, HA', { want: 'fallback', up_since: 700, fallback_vxlan: true, ha: true }],
	['on the tunnel fallback, it stopped', { want: 'fallback', fallback: 'off', primary: 'down', fallback_vxlan: true }],
	['on the tunnel fallback, it stopped, primary up', { want: 'fallback', fallback: 'off', up_since: 990, fallback_vxlan: true }],
	['on the tunnel fallback, it is unknown, primary up', { want: 'fallback', fallback: 'unknown', up_since: 990, fallback_vxlan: true }],
	['primary up, tunnel fallback still running', { fallback_vxlan: true }],
	['primary unknown, tunnel fallback stopped', { primary: 'unknown', fallback: 'off', fallback_vxlan: true }],
	['HA, tunnel fallback stopped', { ha: true, fallback: 'off', fallback_vxlan: true }],
	['primary down, VLAN fallback off', { primary: 'down', fallback: 'off' }],
	['report mode, on the fallback', { mode: 'report', want: 'fallback' }],
	['report mode, tunnel fallback running', { mode: 'report', fallback_vxlan: true }],
])
	print('switch ', c[0], ': ', probe.switch_step(net(c[1]), 1000), '\n');

// The uplink's LLDP (0064), as the Arista sends it on OpenWrtnight's port,
// with two VLAN Name TLVs, which it doesn't send there yet.
let tlv = (t, v) => chr((t << 1) | (length(v) >> 8)) + chr(length(v) & 255) + v;
let lldp_head = bytes([0x01, 0x80, 0xc2, 0x00, 0x00, 0x0e, 0x28, 0xe7, 0x1d, 0xca, 0x29, 0x13, 0x88, 0xcc]) +
	tlv(1, bytes([4, 0x28, 0xe7, 0x1d, 0xca, 0x29, 0x13])) + tlv(2, chr(5) + 'Ethernet6') + tlv(3, bytes([0, 120])) +
	tlv(4, 'Pumphouse OpenWrt') + tlv(5, 'homelab.symtus.com') + tlv(127, bytes([0x00, 0x80, 0xc2, 1, 0, 1]));
print('lldp ', probe.lldp(lldp_head + tlv(0, '')), '\n');
print('lldp with vlans ', probe.lldp(lldp_head + tlv(127, bytes([0x00, 0x80, 0xc2, 3, 0, 20, 4]) + 'Core') +
	tlv(127, bytes([0x00, 0x80, 0xc2, 3, 0, 10, 3]) + 'IoT') + tlv(127, bytes([0x00, 0x80, 0xc2, 3, 0, 20, 4]) + 'Core') + tlv(0, '')), '\n');
print('lldp of an ARP ', probe.lldp(arp), '\n');
// Every TLV the reader knows, as a switch might send them all, and two it
// doesn't: an Arista one, and a type 802.1AB reserves.
let ieee1 = (sub, list, tail) => tlv(127, bytes([0x00, 0x80, 0xc2, sub, ...list]) + (tail ?? ''));
let ieee3 = (sub, list, tail) => tlv(127, bytes([0x00, 0x12, 0x0f, sub, ...list]) + (tail ?? ''));
let med = (sub, list, tail) => tlv(127, bytes([0x00, 0x12, 0xbb, sub, ...list]) + (tail ?? ''));
print('lldp of everything ', probe.lldp(lldp_head +
	tlv(6, 'Arista Networks EOS version 4.30.1F running on an Arista CCS-720DP-24ZS-2') +
	tlv(7, bytes([0, 0x14, 0, 0x14])) +
	tlv(8, bytes([5, 1, 172, 16, 0, 4, 2, 0x00, 0x4c, 0x4b, 0x40, 0])) +
	ieee1(2, [0x06, 0, 30]) +
	ieee1(3, [0, 20, 4], 'Core') + ieee1(3, [0x03, 0xf2, 14], 'PTP_Tower_Link') +
	ieee1(4, [2, 0x88, 0xcc]) +
	ieee3(1, [0x03, 0x6c, 0x01, 0x00, 30]) +
	ieee3(2, [0x07, 1, 5, 0x51, 0x00, 0xff, 0x00, 0xff]) +
	ieee3(3, [0x01, 0, 0, 0, 0]) +
	ieee3(4, [0x24, 0xc8]) +
	med(1, [0x00, 0x33, 4]) +
	med(2, [0x01, 0x40, 0x3d, 0x6e]) +
	med(7, [], 'EOS-4.30.1F') + med(10, [], 'CCS-720DP-24ZS-2') +
	tlv(127, bytes([0x00, 0x1c, 0x73, 1, 0xde, 0xad])) +
	tlv(9, 'xyz') + tlv(0, '')), '\n');

// The link-local address a VLAN's nudge asks IPv6 all-nodes from (0064).
print('link-local of 06:21:36:5e:00:50: ', probe.link_local('06:21:36:5e:00:50'), '\n');
print('link-local of ', SRC, ': ', probe.link_local(SRC), '\n');

// Whether a watched VLAN reaches the AP, watched since 0 (0064).
for (let c in [
	['heard 50 s ago', { started: 0, heard: 950 }, 1000],
	['heard 180 s ago', { started: 0, heard: 820 }, 1000],
	['heard 200 s ago', { started: 0, heard: 800 }, 1000],
	['never heard, watched 100 s', { started: 900 }, 1000],
	['never heard, watched 200 s', { started: 800 }, 1000],
])
	print('watch ', c[0], ': ', probe.watch_verdict(c[1], c[2]), '\n');

// The AP's MACs on the segments it probes (0060).
for (let c in [['vni', 50], ['vni', 1234], ['vni', 10000], ['vni', 70000], ['vlan', 20]])
	print('segment mac ', c[0], ' ', c[1], ': ', probe.segment_mac('ap-a0046021365e', c[0], c[1]), '\n');
print('segment mac of no AP: ', probe.segment_mac('office-ap', 'vni', 50), '\n');

// An ARP probe from the address the AP leased.
print('arp probe from a lease ', hexs(probe.arp_probe('02:21:36:5e:00:50', '192.168.50.1', '192.168.50.6')), '\n');

// DHCP as it goes out: a discover, the request for an offer, a release.
const SEG = '02:21:36:5e:00:50';
print('dhcp discover ', hexs(probe.dhcp(SEG, 'discover', 0x12345678, { host: 'OpenWrtnight-vni50' })), '\n');
print('dhcp request ', hexs(probe.dhcp(SEG, 'request', 0x12345678, { requested: '192.168.50.6', server: '192.168.50.254', host: 'OpenWrtnight-vni50' })), '\n');
print('dhcp renew ', hexs(probe.dhcp(SEG, 'request', 0x0abcdef0, { ciaddr: '192.168.50.6', host: 'OpenWrtnight-vni50' })), '\n');
print('dhcp release ', hexs(probe.dhcp(SEG, 'release', 0x0abcdef0, { ciaddr: '192.168.50.6', server: '192.168.50.254', to: '192.168.50.254' })), '\n');

// An offer, as the lab's DHCP server sends one.
let bootp = bytes([2, 1, 6, 0, 0x12, 0x34, 0x56, 0x78, 0, 0, 0x80, 0, 0, 0, 0, 0, 192, 168, 50, 6, 0, 0, 0, 0, 0, 0, 0, 0,
	0x02, 0x21, 0x36, 0x5e, 0x00, 0x50]);
while (length(bootp) < 236)
	bootp += chr(0);
bootp += bytes([99, 130, 83, 99, 53, 1, 2, 54, 4, 192, 168, 50, 254, 51, 4, 0, 1, 0x51, 0x80,
	1, 4, 255, 255, 255, 0, 3, 4, 192, 168, 50, 1, 255]);
let udp = bytes([0, 67, 0, 68]) + bytes([(8 + length(bootp)) >> 8, (8 + length(bootp)) & 255, 0, 0]) + bootp;
let offer = bytes([0x02, 0x21, 0x36, 0x5e, 0x00, 0x50, 0x28, 0xe7, 0x1d, 0xca, 0x29, 0x13, 0x08, 0x00,
	0x45, 0, (20 + length(udp)) >> 8, (20 + length(udp)) & 255, 0, 0, 0, 0, 64, 17, 0, 0,
	192, 168, 50, 254, 192, 168, 50, 6]) + udp;
print('dhcp offer ', probe.dhcp_reply(offer, 0x12345678), '\n');
print('dhcp offer to another xid ', probe.dhcp_reply(offer, 0x12345679), '\n');
print('an arp reply is no dhcp ', probe.dhcp_reply(reply, 0x12345678), '\n');

// A client's DHCP, as the watch on a Wi-Fi interface reads it (0065): a
// discover and a renewal from a client, the server's offer, and ARP.
print('dhcp seen: a discover ', probe.dhcp_seen(probe.dhcp(SEG, 'discover', 0x12345678, { host: 'phone' })), '\n');
print('dhcp seen: a renewal ', probe.dhcp_seen(probe.dhcp(SEG, 'request', 0x0abcdef0, { ciaddr: '192.168.50.6' })), '\n');
print('dhcp seen: an offer ', probe.dhcp_seen(offer), '\n');
// A client that gives its name as an FQDN (option 81), in DNS's labels (0066).
let d81 = probe.dhcp(SEG, 'discover', 0x12345678, {});
let end81 = index(substr(d81, 282), chr(255)) + 282;
let fq = chr(4, 0, 0, 5) + 'phone' + chr(4) + 'home' + chr(0);
d81 = substr(d81, 0, end81) + chr(81, length(fq)) + fq + substr(d81, end81);
print('dhcp seen: a host name as an FQDN ', probe.dhcp_seen(d81)?.host, '\n');
// A client that names its DHCP software (option 60), with the options it
// asks for (55) (0067).
let d60 = probe.dhcp(SEG, 'discover', 0x12345678, {});
let end60 = index(substr(d60, 282), chr(255)) + 282;
d60 = substr(d60, 0, end60) + chr(60, 15) + 'android-dhcp-14' + substr(d60, end60);
let s60 = probe.dhcp_seen(d60);
print('dhcp seen: a vendor class and parameter list ', s60?.vendor_class, ' ', s60?.params, '\n');
print('dhcp seen: an arp reply ', probe.dhcp_seen(reply), '\n');
print('arp seen: a reply ', probe.arp_seen(reply), '\n');
print('arp seen: a probe from 0.0.0.0 ', probe.arp_seen(arp), '\n');
print('arp seen: an offer ', probe.arp_seen(offer), '\n');

// How a request stands, made at 990 (0065).
for (let c in [
	['no answer, at 995', { at: 990, answers: {} }, 995],
	['no answer, at 1000', { at: 990, answers: {} }, 1000],
	['an offer', { at: 990, answers: { '192.168.50.254': { type: 'offer' } } }, 1000],
])
	print('request ', c[0], ': ', probe.request_state(c[1], c[2]), '\n');

// How a client does with DHCP, at 1000 (0065).
for (let c in [
	['asked after joining', { joined: 900, watched: true, asked: 902 }],
	['asked in the second nl80211 rounds the join into', { joined: 900, watched: true, asked: 899.4, address: '192.168.50.7' }],
	['asked before joining again', { joined: 900, watched: true, asked: 800, address: '192.168.50.7' }],
	['joined 30 s ago, nothing yet', { joined: 970, watched: true }],
	['joined 100 s ago, uses an address', { joined: 900, watched: true, address: '192.168.50.7' }],
	['joined 100 s ago, no address', { joined: 900, watched: true }],
	['joined before the watch', { joined: 100, watched: false }],
])
	print('client ', c[0], ': ', probe.client_dhcp(c[1], 1000), '\n');

// Every jump in the filters lands inside them.
for (let name, prog in { overlay: probe.OVERLAY_FILTER, guard: probe.GUARD_FILTER, watch: probe.WATCH_FILTER, lldp: probe.LLDP_FILTER, dhcp: probe.DHCP_FILTER }) {
	let ok = true;
	for (let i = 0; i < length(prog); i++)
		if ((prog[i][0] & 0x07) == 0x05 && (i + 1 + prog[i][1] >= length(prog) || i + 1 + prog[i][2] >= length(prog)))
			ok = false;
	print('filter ', name, ': ', length(prog), ' instructions, jumps ', ok ? 'land inside' : 'LEAVE IT', ', ends ', prog[length(prog) - 1][0] == 0x06 ? 'returning' : 'NOT RETURNING', '\n');
}

// The routes a tunnel's start on a VLAN needs (0063), and those the kernel
// dropped: the office AP on 2026-10-06, its table 1020 empty while netifd
// still held them, after another interface leasing the same address there
// restarted.
let office = {
	up: true, ip4table: 1020, l3_device: 'br-lan.20',
	'ipv4-address': [{ address: '192.168.20.88', mask: 24 }],
	route: [{ target: '0.0.0.0', mask: 0, nexthop: '192.168.20.1', source: '192.168.20.88/32' }],
};
let want = probe.start_routes(office);
print('start routes: ', want, '\n');
print('missing from an empty table: ', probe.missing_routes(want, []), '\n');
print('missing from a whole table: ', probe.missing_routes(want, [{ dst: '0.0.0.0/0', via: '192.168.20.1' }, { dst: '192.168.20.0/24', via: null }]), '\n');
print('missing beside another gateway: ', probe.missing_routes(want, [{ dst: '0.0.0.0/0', via: '192.168.20.254' }, { dst: '192.168.20.0/24' }]), '\n');
for (let r in want)
	print('ip route replace ', probe.route_args(r, 'br-lan.20', 1020), '\n');
print('a device that is no device: ', probe.route_args(want[0], 'br-lan.20; reboot', 1020), '\n');
print('a gateway that is no address: ', probe.route_args({ dst: '0.0.0.0/0', via: '$(reboot)' }, 'br-lan.20', 1020), '\n');
print('no status: ', probe.start_routes(null), '\n');
for (let c in [['192.168.20.88', 24], ['10.1.2.3', 8], ['10.1.2.3', 32], ['10.1.2.3', 0], ['172.16.200.9', 13], ['300.1.1.1', 24], ['10.1.1.1', 33]])
	print('prefix ', c[0], '/', c[1], ': ', probe.prefix4(c[0], c[1]), '\n');
