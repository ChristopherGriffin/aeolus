// The pure parts of a client test over the wire (0128): TestAgentWireTest
// runs this with the real ucode and compares what it prints with
// wiretest.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/wiretest.uc

'use strict';

import * as wt from 'aeolus.wiretest';
import * as ct from 'aeolus.clienttest';

function hexed(s) {
	let out = '';
	for (let i = 0; i < length(s); i++)
		out += sprintf('%02x', ord(s, i));
	return out;
}

function unhex(h) {
	let out = '';
	for (let i = 0; i + 1 < length(h); i += 2)
		out += chr(hex(substr(h, i, 2)));
	return out;
}

let b = (list) => join('', map(list, x => chr(x)));
let u16 = (n) => chr((n >> 8) & 255) + chr(n & 255);
let u32 = (n) => u16((n >> 16) & 0xffff) + u16(n & 0xffff);
let mac = (t) => b(map(split(t, ':'), x => hex(x)));
let ip4 = (t) => b(map(split(t, '.'), x => +x));

// The MAC a test is made from: the AP's, and the segment's number.
printf('mac %J %J %J %J %J %J\n', wt.test_mac('ap-30862d0317ff', 20), wt.test_mac('ap-30862d0317ff', 4094), wt.test_mac('ap-30862d0317ff', 10050),
	wt.test_mac('ap-30862d0317ff', null), wt.test_mac('office-ap', 20), wt.test_mac(null, 20));

// What the socket keeps: frames to the test's MAC, and of the broadcasts
// ARP and DHCP to a client.
const SRC = '0e:03:17:ff:00:20';
printf('filter %J\n', wt.bpf(SRC));

// What is asked: the wire is no band, and needs to be told the AP and the
// network; it has no BSSIDs and signs in as no one.
printf('wanted %J\n', ct.wanted({ ssid: 'Aeolus Lab', network: 'lab', band: 'wire', ap: 'ap-30862d0317ff', dns_name: 'openwrt.org' }));
for (let bad in [{ ssid: 'x', band: 'wire' }, { ssid: 'x', network: 'lab', band: 'wire' }, { ssid: 'x', network: 'lab', band: 'wire', ap: 'office-ap' },
	{ ssid: 'x', network: 'lab', band: 'wire', ap: 'ap-30862d0317ff', bssids: ['20:05:b6:01:8b:ec'] },
	{ ssid: 'x', network: 'lab', band: 'wire', ap: 'ap-30862d0317ff', eap: { method: 'peap', identity: 'u', password: 'p' } }])
	printf('wanted %J: %J\n', bad, ct.wanted(bad));

// DHCP, as the test sends it: a discover, the request that takes an offer
// up, and the release, which goes to its server by the way the test knows.
let d = wt.dhcp(SRC, 'discover', 0x1234abcd);
printf('discover %d bytes, to %s from %s, flags %s, options %s\n', length(d), wt.mac_text(d, 0), wt.mac_text(d, 6), hexed(substr(d, 52, 2)), hexed(substr(d, 282, 30)));
printf('tagged for VLAN 20: %s; untagged there: %s; no VLAN: %s\n', hexed(substr(wt.with_tag(d, 20, true), 12, 6)), hexed(substr(wt.with_tag(d, 20, false), 12, 2)),
	hexed(substr(wt.with_tag(d, null, true), 12, 2)));
let r = wt.dhcp(SRC, 'request', 0x1234abcd, { requested: '192.168.20.77', server: '192.168.20.1' });
printf('request options %s\n', hexed(substr(r, 282, 42)));
let rel = wt.dhcp(SRC, 'release', 7, { ciaddr: '192.168.20.77', server: '192.168.20.1', to: '192.168.20.1', dst: 'd4:01:c3:00:00:01' });
printf('release to %s, IP %s to %s, flags %s, options %s\n', wt.mac_text(rel, 0), join('.', map([26, 27, 28, 29], i => ord(rel, i))),
	join('.', map([30, 31, 32, 33], i => ord(rel, i))), hexed(substr(rel, 52, 2)), hexed(substr(rel, 282, 16)));

// A discover sent again says how long it has been asking; an address that
// is another's already is turned down; and every datagram has a checksum
// that adds up, the routed release among them.
let dec = wt.dhcp(SRC, 'decline', 0x1234abcd, { requested: '192.168.20.77', server: '192.168.20.1' });
printf('asked again after 4.7 s: secs %s; decline to %s, options %s\n', hexed(substr(wt.dhcp(SRC, 'discover', 1, { secs: 4.7 }), 50, 2)),
	wt.mac_text(dec, 0), hexed(substr(dec, 282, 22)));
function adds_up(f) {
	let u = substr(f, 14 + (ord(f, 14) & 15) * 4), data = substr(f, 26, 8) + chr(0) + chr(17) + substr(u, 4, 2) + u, sum = 0;
	for (let i = 0; i < length(data); i += 2)
		sum += ord(data, i) << 8 | (i + 1 < length(data) ? ord(data, i + 1) : 0);
	while (sum >> 16)
		sum = (sum & 0xffff) + (sum >> 16);
	return sum == 0xffff;
}
printf('checksums add up: discover %J, request %J, decline %J, release %J\n', adds_up(d), adds_up(r), adds_up(dec), adds_up(rel));

// A server's answer: an offer and an acknowledgement, with a mask, a
// router, two DNS servers and a lease time; one to another client, one to
// another transaction, and a refusal.
function answer(kind, xid, client, yours, opts) {
	let msg = chr(2) + chr(1) + chr(6) + chr(0) + u32(xid) + u16(0) + u16(0x8000) +
		ip4('0.0.0.0') + ip4(yours) + ip4('192.168.20.1') + ip4('0.0.0.0') + mac(client);
	while (length(msg) < 236)
		msg += chr(0);
	msg += b([99, 130, 83, 99, 53, 1, kind]) + opts + chr(255);
	let udp = u16(67) + u16(68) + u16(8 + length(msg)) + u16(0) + msg;
	let ip = b([0x45, 0]) + u16(20 + length(udp)) + u16(0) + u16(0) + chr(64) + chr(17) + u16(0) + ip4('192.168.20.1') + ip4('255.255.255.255');
	return mac('ff:ff:ff:ff:ff:ff') + mac('d4:01:c3:00:00:01') + u16(0x0800) + ip + udp;
}
let opts = b([54, 4]) + ip4('192.168.20.1') + b([51, 4]) + u32(43200) + b([1, 4]) + ip4('255.255.255.0') + b([3, 4]) + ip4('192.168.20.1') +
	b([6, 8]) + ip4('192.168.20.1') + ip4('9.9.9.9');
printf('offer %J\n', wt.dhcp_reply(answer(2, 0x1234abcd, SRC, '192.168.20.77', opts), 0x1234abcd, SRC));
printf('ack %J\n', wt.dhcp_reply(answer(5, 0x1234abcd, SRC, '192.168.20.77', opts), 0x1234abcd, SRC));
printf('nak %J\n', wt.dhcp_reply(answer(6, 0x1234abcd, SRC, '0.0.0.0', b([54, 4]) + ip4('192.168.20.1')), 0x1234abcd, SRC)?.type);
printf('another client %J, another transaction %J, no DHCP %J\n', wt.dhcp_reply(answer(2, 0x1234abcd, '0e:03:17:ff:00:50', '192.168.20.77', opts), 0x1234abcd, SRC),
	wt.dhcp_reply(answer(2, 0x1111, SRC, '192.168.20.77', opts), 0x1234abcd, SRC), wt.dhcp_reply(d, 0x1234abcd, SRC));
printf('masks %J %J %J %J %J\n', wt.bits('255.255.255.0'), wt.bits('255.255.252.0'), wt.bits('0.0.0.0'), wt.bits('255.0.255.0'), wt.bits('garbage'));

// ARP: the test asks for its gateway from the address it was given, reads
// the answer, and answers who asks for its own address.
let ask = wt.arp_request(SRC, '192.168.20.77', '192.168.20.1');
printf('arp request %d bytes: %J\n', length(ask), wt.arp_of(ask));
let reply = mac(SRC) + mac('d4:01:c3:00:00:01') + u16(0x0806) + u16(1) + u16(0x0800) + chr(6) + chr(4) + u16(2) +
	mac('d4:01:c3:00:00:01') + ip4('192.168.20.1') + mac(SRC) + ip4('192.168.20.77');
printf('arp reply %J\n', wt.arp_of(reply));
let who = mac('ff:ff:ff:ff:ff:ff') + mac('d4:01:c3:00:00:01') + u16(0x0806) + u16(1) + u16(0x0800) + chr(6) + chr(4) + u16(1) +
	mac('d4:01:c3:00:00:01') + ip4('192.168.20.1') + mac('00:00:00:00:00:00') + ip4('192.168.20.77');
let mine = wt.arp_answer(who, SRC, '192.168.20.77');
printf('asked for its address, it answers %J to %s\n', wt.arp_of(mine), wt.mac_text(mine, 0));
printf('asked for another %J, its own request %J, no ARP %J\n', wt.arp_answer(who, SRC, '192.168.20.78'), wt.arp_answer(ask, SRC, '192.168.20.1'), wt.arp_answer(d, SRC, '192.168.20.77'));

// Before it uses an address it asks whether another has it, from no
// address; and another that answers for it, or asks from it, has.
let probe = wt.arp_probe(SRC, '192.168.20.77');
printf('probe %J\n', wt.arp_of(probe));
let taken = mac('ff:ff:ff:ff:ff:ff') + mac('7e:2a:ea:9b:2b:8f') + u16(0x0806) + u16(1) + u16(0x0800) + chr(6) + chr(4) + u16(2) +
	mac('7e:2a:ea:9b:2b:8f') + ip4('192.168.20.77') + mac(SRC) + ip4('0.0.0.0');
printf('another has it %J; the gateway asking for it %J; the test itself %J; no ARP %J\n', wt.arp_conflict(taken, SRC, '192.168.20.77'),
	wt.arp_conflict(who, SRC, '192.168.20.77'), wt.arp_conflict(mine, SRC, '192.168.20.77'), wt.arp_conflict(d, SRC, '192.168.20.77'));

// A DNS lookup as a frame, and its answer read back.
let q = wt.udp(SRC, 'd4:01:c3:00:00:01', '192.168.20.77', '9.9.9.9', 40001, 53, ct.dns_query(0x4165, 'openwrt.org'));
printf('query %d bytes to %s: IP %s, its checksum adds up %J\n', length(q), wt.mac_text(q, 0), hexed(substr(q, 14, 20)), adds_up(q));
let back = wt.udp('d4:01:c3:00:00:01', SRC, '9.9.9.9', '192.168.20.77', 53, 40001, unhex('41658180000100010000000000000100010000010001000000e10004404482a4'));
let got = wt.udp_of(back, '192.168.20.77', 40001);
printf('answer from %s port %d: %J\n', got.from, got.sport, ct.dns_answer(got.data, 0x4165));
printf('to another port %J, to another address %J, no UDP %J\n', wt.udp_of(back, '192.168.20.77', 40002), wt.udp_of(back, '192.168.20.78', 40001), wt.udp_of(ask, '192.168.20.77', 40001));

// Which of a lease's addresses are its own subnet's.
let lease = { address: '192.168.20.77', mask: 24 };
printf('on link %J %J %J %J\n', wt.on_link('192.168.20.1', lease), wt.on_link('192.168.21.1', lease), wt.on_link('9.9.9.9', lease),
	wt.on_link('192.168.21.1', { address: '192.168.20.77', mask: 22 }));

// The wire a network's frames take: a VLAN of the uplink's, tagged or not
// as the watch says; a tunnel, or its fallback's VLAN, whichever is in the
// network's bridge now; and what cannot be tested.
let watch = (v) => ({ '20': { vlan: '20', device: 'eth0', tagged: '1' }, '50': { vlan: '50', device: 'eth0', tagged: '1' }, '7': { vlan: '7', device: 'eth0', tagged: '0' } })['' + v];
let sw = { bridge: 'br-n351c496e', primary: 'aeolus_50', primary_vni: '50', fallback: 'anf351c496e', fallback_vlan: '50' };
printf('a VLAN %J\n', wt.where('br-lan.20', null, null, watch, 'eth1'));
printf('a VLAN carried untagged %J\n', wt.where('br-lan.7', null, null, watch, 'eth0'));
printf('a VLAN not watched %J\n', wt.where('br-lan.99', null, null, watch, 'eth0'));
printf('on its tunnel %J\n', wt.where('br-n351c496e', sw, ['aeolus_50', 'phy0-ap1'], watch, 'eth0'));
printf('on its fallback %J\n', wt.where('br-n351c496e', sw, ['anf351c496e', 'phy0-ap1'], watch, 'eth0'));
printf('on neither %J\n', wt.where('br-n351c496e', sw, ['phy0-ap1'], watch, 'eth0'));
printf('on a tunnel alone %J\n', wt.where('br-vx50', null, null, watch, 'eth0'));
printf('no uplink %J, no VLAN %J, no interface %J\n', wt.where('br-lan.20', null, null, watch, null), wt.where('br-lan', null, null, watch, 'eth0'), wt.where(null, null, null, watch, 'eth0'));

// What a test over the wire comes to, in a line.
let res = { v: 1, band: 'wire', dhcp: { ms: 212, address: '192.168.20.77' }, gateway: { address: '192.168.20.1', sent: 0, got: 0, arp: true, mac: 'd4:01:c3:00:00:01', arp_ms: 0.8 },
	dns: { server: '192.168.20.1', name: 'openwrt.org', rcode: 0, answers: 1, ms: 14 } };
ct.verdict(res);
printf('passed: %s\n', ct.summary(res));
let lost = { v: 1, band: 'wire', dhcp: { ms: 212, address: '192.168.20.77' }, gateway: { address: '192.168.20.1', sent: 0, got: 0, arp: false } };
ct.verdict(lost);
printf('no gateway: %s\n', ct.summary(lost));
let none = { v: 1, band: 'wire', stopped: { stage: 'dhcp', why: 'no DHCP server gave it an address' } };
ct.verdict(none);
printf('no address: %s\n', ct.summary(none));
