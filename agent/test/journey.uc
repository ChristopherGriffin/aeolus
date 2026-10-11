// A client coming online, as the AP records it (0118): TestAgentJourney runs
// this with the real ucode and compares what it prints with journey.out.
// The frames are made here, byte by byte, and the packet filter is run by a
// small interpreter of the kernel's, so what it keeps is seen, not assumed.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/journey.uc

'use strict';

import * as journey from 'aeolus.journey';

function bytes(list) {
	return join('', map(list, b => chr(b)));
}

function u16(n) {
	return chr((n >> 8) & 255) + chr(n & 255);
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

function ip(t) {
	return bytes(map(split(t, '.'), x => +x));
}

function eth(dst, src, type, payload) {
	return mac(dst) + mac(src) + u16(type) + payload;
}

function ip4(src, dst, proto, payload) {
	return bytes([0x45, 0]) + u16(20 + length(payload)) + u16(1) + u16(0) + bytes([64, proto]) + u16(0) + ip(src) + ip(dst) + payload;
}

function ip6(src, dst, next, payload) {
	return bytes([0x60, 0, 0, 0]) + u16(length(payload)) + bytes([next, 64]) + src + dst + payload;
}

function udp(sport, dport, payload) {
	return u16(sport) + u16(dport) + u16(8 + length(payload)) + u16(0) + payload;
}

function tcp(sport, dport, flags) {
	return u16(sport) + u16(dport) + zeros(8) + bytes([0x50, flags]) + zeros(6);
}

function arp(op, smac, sip, tmac, tip) {
	return u16(1) + u16(0x0800) + bytes([6, 4]) + u16(op) + mac(smac) + ip(sip) + mac(tmac) + ip(tip);
}

function option(code, data) {
	return bytes([code, length(data)]) + data;
}

function bootp(op, client, given, options) {
	return bytes([op, 1, 6, 0]) + bytes([0x12, 0x34, 0x56, 0x78]) + zeros(8) + ip(given) + zeros(8) +
		mac(client) + zeros(10) + zeros(192) + bytes([0x63, 0x82, 0x53, 0x63]) + options + chr(255);
}

function dns(id, answer, rcode, name, qtype, records) {
	let q = '';
	for (let l in split(name, '.'))
		q += chr(length(l)) + l;
	return u16(id) + u16((answer ? 0x8180 : 0x0100) | rcode) + u16(1) + u16(records) + u16(0) + u16(0) + q + chr(0) + u16(qtype) + u16(1);
}

// run is the kernel's classic packet filter, as far as these filters use
// it: what it returns for a frame going out (type 4) or coming in (0).
function run(prog, f, out) {
	let a = 0, x = 0;
	let byte = at => at < length(f) ? ord(f, at) : null;
	for (let pc = 0; pc < length(prog); pc++) {
		let s = prog[pc], k = s[3], v;
		switch (s[0]) {
		case 0x28: v = byte(k + 1) == null ? null : byte(k) << 8 | byte(k + 1); break;
		case 0x30: v = byte(k); break;
		case 0x20: v = k == 0xfffff004 ? (out ? 4 : 0) : null; break;
		case 0x48: v = byte(x + k + 1) == null ? null : byte(x + k) << 8 | byte(x + k + 1); break;
		case 0x50: v = byte(x + k); break;
		case 0xb1:
			if (byte(k) == null)
				return 0;
			x = 4 * (byte(k) & 15);
			continue;
		case 0x15: pc += a == k ? s[1] : s[2]; continue;
		case 0x45: pc += a & k ? s[1] : s[2]; continue;
		case 0x06: return k;
		default: return 'unknown step ' + s[0];
		}
		if (v == null)
			return 0;   // past the frame's end: the kernel drops it
		a = v;
	}
	return 'ran off the end';
}

const AP = '20:05:b6:01:8b:e2', STA = '72:bb:66:12:2a:94', GW = 'd4:01:c3:00:00:01', ALL = 'ff:ff:ff:ff:ff:ff';
const V6A = bytes([0xfe, 0x80]) + zeros(13) + chr(1), V6B = bytes([0x20, 0x01, 0x0d, 0xb8]) + zeros(11) + chr(0x53);

let frames = {
	'arp ask, in': [eth(ALL, STA, 0x0806, arp(1, STA, '192.168.20.61', '00:00:00:00:00:00', '192.168.20.1')), false],
	'arp answer, out': [eth(STA, GW, 0x0806, arp(2, GW, '192.168.20.1', STA, '192.168.20.61')), true],
	'arp ask from another host, out': [eth(ALL, GW, 0x0806, arp(1, GW, '192.168.20.1', '00:00:00:00:00:00', '192.168.20.9')), true],
	'arp ask for another host, in': [eth(ALL, STA, 0x0806, arp(1, STA, '192.168.20.61', '00:00:00:00:00:00', '192.168.20.9')), false],
	'arp probe for a free address, in': [eth(ALL, STA, 0x0806, arp(1, STA, '0.0.0.0', '00:00:00:00:00:00', '192.168.20.61')), false],
	'dhcp discover, in': [eth(ALL, STA, 0x0800, ip4('0.0.0.0', '255.255.255.255', 17, udp(68, 67,
		bootp(1, STA, '0.0.0.0', option(53, chr(1)) + option(12, 'Griffs-iPhone'))))), false],
	'dhcp ack, out': [eth(STA, GW, 0x0800, ip4('192.168.20.1', '192.168.20.61', 17, udp(67, 68,
		bootp(2, STA, '192.168.20.61', option(53, chr(5)) + option(54, ip('192.168.20.1')) + option(3, ip('192.168.20.1')) +
			option(6, ip('192.168.20.1') + ip('9.9.9.9')))))), true],
	'dhcp nak, out': [eth(STA, GW, 0x0800, ip4('192.168.20.1', '255.255.255.255', 17, udp(67, 68,
		bootp(2, STA, '0.0.0.0', option(53, chr(6)) + option(54, ip('192.168.20.1')))))), true],
	'dns lookup, in': [eth(GW, STA, 0x0800, ip4('192.168.20.61', '192.168.20.1', 17, udp(50123, 53, dns(7, false, 0, 'captive.apple.com', 1, 0)))), false],
	'dns answer, out': [eth(STA, GW, 0x0800, ip4('192.168.20.1', '192.168.20.61', 17, udp(53, 50123, dns(7, true, 0, 'captive.apple.com', 1, 2)))), true],
	'dns lookup over IPv6, in': [eth(GW, STA, 0x86dd, ip6(V6A, V6B, 17, udp(50124, 53, dns(8, false, 0, 'Example.COM', 28, 0)))), false],
	'tcp open, in': [eth(GW, STA, 0x0800, ip4('192.168.20.61', '17.253.144.10', 6, tcp(50200, 80, 0x02))), false],
	'tcp answer, out': [eth(STA, GW, 0x0800, ip4('17.253.144.10', '192.168.20.61', 6, tcp(80, 50200, 0x12))), true],
	'tcp data, in': [eth(GW, STA, 0x0800, ip4('192.168.20.61', '17.253.144.10', 6, tcp(50200, 80, 0x18))), false],
	'tcp open over IPv6, in': [eth(GW, STA, 0x86dd, ip6(V6A, V6B, 6, tcp(50201, 443, 0x02))), false],
	'ntp, in': [eth(GW, STA, 0x0800, ip4('192.168.20.61', '17.253.4.125', 17, udp(123, 123, zeros(48)))), false],
	'a later fragment, in': [eth(GW, STA, 0x0800, substr(ip4('192.168.20.61', '192.168.20.1', 17, udp(50123, 53, zeros(40))), 0, 6) +
		u16(0x00b9) + substr(ip4('192.168.20.61', '192.168.20.1', 17, udp(50123, 53, zeros(40))), 8)), false],
	'too short': [substr(eth(ALL, STA, 0x0800, zeros(10)), 0, 20), false],
};

let narrow = journey.capture(false), wide = journey.capture(true);
printf('filter narrow %d steps, wide %d steps\n', length(narrow), length(wide));
for (let name in sort(keys(frames))) {
	let f = frames[name];
	let keep = p => run(p, f[0], f[1]) == 0x40000 ? 'keeps' : run(p, f[0], f[1]) == 0 ? 'drops' : run(p, f[0], f[1]);
	printf('%s: narrow %s, wide %s; reads %J\n', name, keep(narrow), keep(wide), journey.frame(f[0], f[1]));
}

// hostapd's own words, as the office AP logged them on 2026-10-10, and
// others it has.
for (let line in [
	'<30>Oct 10 21:19:15 hostapd: phy0-ap0: STA 72:bb:66:12:2a:94 IEEE 802.11: authenticated',
	'<30>Oct 10 21:19:15 hostapd: phy0-ap0: STA 72:bb:66:12:2a:94 IEEE 802.11: associated (aid 7)',
	'<29>Oct 10 21:19:15 hostapd: phy0-ap0: AP-STA-CONNECTED 72:bb:66:12:2a:94 auth_alg=open',
	'<30>Oct 10 21:19:15 hostapd: phy0-ap0: STA 72:bb:66:12:2a:94 RADIUS: starting accounting session DE7F14E5BF80113D',
	'<30>Oct 10 21:19:15 hostapd: phy0-ap0: STA 72:bb:66:12:2a:94 WPA: pairwise key handshake completed (RSN)',
	'<29>Oct 10 21:19:15 hostapd: phy0-ap0: EAPOL-4WAY-HS-COMPLETED 72:bb:66:12:2a:94',
	'<30>Oct 10 21:19:15 hostapd: phy0-ap0: STA 72:bb:66:12:2a:94 WPA: group key handshake completed (RSN)',
	'<29>Oct 10 21:20:01 hostapd: phy0-ap0: AP-STA-POSSIBLE-PSK-MISMATCH 72:bb:66:12:2a:94',
	'<30>Oct 10 21:20:02 hostapd: phy1-ap5: STA 72:bb:66:12:2a:94 IEEE 802.1X: authenticated - EAP type: 25 (PEAP)',
	'<29>Oct 10 21:20:02 hostapd: phy1-ap5: CTRL-EVENT-EAP-FAILURE2 72:bb:66:12:2a:94',
	'<30>Oct 10 20:22:28 hostapd: phy1-ap0: STA 6c:ac:c2:60:aa:3f IEEE 802.11: disassociated due to inactivity',
	'<30>Oct 10 20:22:29 hostapd: phy1-ap0: STA 6c:ac:c2:60:aa:3f IEEE 802.11: deauthenticated due to inactivity (timer DEAUTH/REMOVE)',
	'<29>Oct 10 21:20:05 hostapd: phy0-ap0: AP-STA-DISCONNECTED 72:bb:66:12:2a:94',
	'<30>Oct 10 21:20:06 hostapd: phy0-ap0: STA 72:bb:66:12:2a:94 IEEE 802.11: Station tried to associate before authentication (aid=-1 flags=0x0)',
	'<29>Oct 10 21:28:14 hostapd: Unexpected Link measurement report, token 1',
	'<14>Oct 10 20:22:28 usteer: station 6c:ac:c2:60:aa:3f disconnected from node hostapd.phy1-ap0',
	'<29>Oct 10 21:20:05 hostapd: phy0-ap0: AP-STA-POLL-OK 72:bb:66:12:2a:94',
])
	printf('log %J\n', journey.log_event(line));

// Whole attempts. Times are seconds on the AP's steady clock.
let who = { bss: 'phy0-ap0', mac: STA, network: 'sweet', ssid: 'Sweet Spot', band: '5g' };
let log = (a, at, rest) => journey.on_log(a, journey.log_event('<30>Oct 10 21:19:15 hostapd: phy0-ap0: ' + rest), at);
let got = (a, at, name) => journey.on_frame(a, journey.frame(frames[name][0], frames[name][1]), at);
let join_up = (a, at) => {
	log(a, at, `STA ${STA} IEEE 802.11: authenticated`);
	log(a, at + 0.004, `STA ${STA} IEEE 802.11: associated (aid 7)`);
	log(a, at + 0.031, `AP-STA-CONNECTED ${STA} auth_alg=open`);
	log(a, at + 0.032, `STA ${STA} WPA: pairwise key handshake completed (RSN)`);
	log(a, at + 0.032, `EAPOL-4WAY-HS-COMPLETED ${STA}`);
};
let show = (name, a, at) => {
	let r = journey.record(a, at);
	printf('%s: %s, stage %s, %d ms, reason %J, address %J, host %J\n', name, r.outcome, r.stage, r.took_ms, r.reason, r.address, r.host);
	for (let e in r.events)
		printf('  %J\n', e);
};

// Online: joined, an address by DHCP, its gateway answers, a lookup and a
// first connection.
let a = journey.begin(who, 1000, 1791684000000);
join_up(a, 1000);
got(a, 1000.210, 'dhcp discover, in');
got(a, 1000.230, 'dhcp ack, out');
printf('due before anything answers %J\n', journey.due(a, 1001));
got(a, 1000.300, 'arp ask, in');
got(a, 1000.302, 'arp answer, out');
got(a, 1000.350, 'dns lookup, in');
got(a, 1000.362, 'dns answer, out');
got(a, 1000.400, 'tcp open, in');
got(a, 1000.423, 'tcp answer, out');
printf('due at once %J, after the wait %J\n', journey.due(a, 1001), journey.due(a, 1005));
show('online', a, 1005);

// A wrong passphrase: associated, then hostapd's word for it, and gone.
a = journey.begin(who, 2000, 1791685000000);
log(a, 2000, `STA ${STA} IEEE 802.11: authenticated`);
log(a, 2000.005, `STA ${STA} IEEE 802.11: associated (aid 7)`);
log(a, 2001.1, `AP-STA-POSSIBLE-PSK-MISMATCH ${STA}`);
log(a, 2004.2, `STA ${STA} IEEE 802.11: deauthenticated due to local deauth request`);
printf('due once it left %J\n', journey.due(a, 2004.3));
show('wrong passphrase', a, 2004.3);

// Let on, and DHCP never answers.
a = journey.begin(who, 3000, 1791686000000);
join_up(a, 3000);
got(a, 3000.2, 'dhcp discover, in');
got(a, 3004.2, 'arp probe for a free address, in');
got(a, 3008.2, 'dhcp discover, in');
printf('due while watched %J, after %J\n', journey.due(a, 3020), journey.due(a, 3031));
show('no DHCP', a, 3031);

// An address, and the gateway does not answer.
a = journey.begin(who, 4000, 1791687000000);
join_up(a, 4000);
got(a, 4000.2, 'dhcp ack, out');
got(a, 4000.3, 'arp ask, in');
show('gateway silent', a, 4031);

// Authenticated, and nothing more.
a = journey.begin(who, 5000, 1791688000000);
log(a, 5000, `STA ${STA} IEEE 802.11: authenticated`);
printf('due after 10 s %J, after 21 s %J\n', journey.due(a, 5010), journey.due(a, 5021));
show('stopped after authenticating', a, 5021);

// A device that keeps its address and says nothing: let on, and that is all.
a = journey.begin(who, 6000, 1791689000000);
join_up(a, 6000);
show('quiet', a, 6031);

// One that roams in with its address, and looks something up.
a = journey.begin(who, 7000, 1791690000000);
join_up(a, 7000);
got(a, 7000.1, 'dns lookup, in');
got(a, 7000.12, 'dns answer, out');
show('roamed in', a, 7005);

// Sign-in refused on an Enterprise network.
a = journey.begin({ ...who, bss: 'phy1-ap5', network: 'enterprise', ssid: 'Aeolus-Enterprise' }, 8000, 1791691000000);
journey.on_log(a, journey.log_event(`hostapd: phy1-ap5: STA ${STA} IEEE 802.11: authenticated`), 8000);
journey.on_log(a, journey.log_event(`hostapd: phy1-ap5: STA ${STA} IEEE 802.11: associated (aid 2)`), 8000.01);
journey.on_log(a, journey.log_event(`hostapd: phy1-ap5: CTRL-EVENT-EAP-STARTED ${STA}`), 8000.02);
journey.on_log(a, journey.log_event(`hostapd: phy1-ap5: CTRL-EVENT-EAP-FAILURE2 ${STA}`), 8001.5);
journey.on_log(a, journey.log_event(`hostapd: phy1-ap5: STA ${STA} IEEE 802.1X: authentication failed - EAP type: 25 (PEAP)`), 8001.5);
journey.on_log(a, journey.log_event(`hostapd: phy1-ap5: AP-STA-DISCONNECTED ${STA}`), 8001.6);
show('sign-in refused', a, 8001.7);

// A client that goes of its own accord has failed nothing: one that
// leaves while associating, as one that picks another AP; and one that
// leaves right after asking for an address.
a = journey.begin(who, 10000, 1791693000000);
log(a, 10000, `STA ${STA} IEEE 802.11: authenticated`);
log(a, 10000.005, `STA ${STA} IEEE 802.11: associated (aid 7)`);
log(a, 10000.4, `STA ${STA} IEEE 802.11: disassociated`);
show('left while associating', a, 10000.5);
a = journey.begin(who, 11000, 1791694000000);
join_up(a, 11000);
got(a, 11000.2, 'dhcp discover, in');
log(a, 11001.5, `AP-STA-DISCONNECTED ${STA}`);
show('left right after asking for an address', a, 11001.6);

// One that waited for DHCP and then left did fail; and so one whose gateway
// never answered.
a = journey.begin(who, 12000, 1791695000000);
join_up(a, 12000);
got(a, 12000.2, 'dhcp discover, in');
got(a, 12004.2, 'dhcp discover, in');
log(a, 12012, `AP-STA-DISCONNECTED ${STA}`);
show('gave up on DHCP and left', a, 12012.1);
a = journey.begin(who, 13000, 1791696000000);
join_up(a, 13000);
got(a, 13000.2, 'dhcp ack, out');
got(a, 13000.3, 'arp ask, in');
log(a, 13006, `AP-STA-DISCONNECTED ${STA}`);
show('gave up on its gateway and left', a, 13006.1);

// Refused by the DHCP server, a client that then leaves at once did fail:
// it was told no, and waited for nothing.
a = journey.begin(who, 14000, 1791697000000);
join_up(a, 14000);
got(a, 14000.2, 'dhcp discover, in');
got(a, 14000.3, 'dhcp nak, out');
log(a, 14001.5, `AP-STA-DISCONNECTED ${STA}`);
show('refused by DHCP and left', a, 14001.6);

// How long it waited for its gateway is from when it first asked for it,
// not from an earlier question about another host.
a = journey.begin(who, 15000, 1791698000000);
join_up(a, 15000);
got(a, 15000.1, 'arp ask for another host, in');
got(a, 15005, 'dhcp ack, out');
got(a, 15005.1, 'arp ask, in');
log(a, 15006, `AP-STA-DISCONNECTED ${STA}`);
show('left a second after asking for its gateway', a, 15006.1);

// More steps than are kept.
a = journey.begin(who, 9000, 1791692000000);
for (let i = 0; i < 60; i++)
	journey.on_log(a, { bss: 'phy0-ap0', mac: STA, stage: null, what: 'line ' + i, ok: null }, 9000 + i / 100);
let r = journey.record(a, 9001);
printf('many steps: %d kept, %d more\n', length(r.events), r.more);
