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
	// What it says at debug (0133), as the C-360 logged it on 2026-10-11:
	// the four messages of the key handshake, its giving up, and WPA3's
	// authentication are read; the rest of what it says there is not.
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: sending 1/4 msg of 4-Way Handshake',
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: received EAPOL-Key frame (2/4 Pairwise)',
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: sending 3/4 msg of 4-Way Handshake',
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: received EAPOL-Key frame (4/4 Pairwise)',
	'<31>Oct 11 12:40:59 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: invalid MIC in msg 2/4 of 4-Way Handshake',
	'<31>Oct 11 12:40:59 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: EAPOL-Key timeout',
	'<31>Oct 11 12:41:02 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: PTKSTART: Retry limit 4 reached',
	'<31>Oct 11 12:41:02 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: PTKINITNEGOTIATING: Retry limit 4 reached',
	'<31>Oct 11 12:45:02 hostapd: phy0-ap2: STA 30:86:2d:03:17:f0 IEEE 802.11: start SAE authentication (RX commit, status=126 (SAE_HASH_TO_ELEMENT))',
	'<31>Oct 11 12:45:02 hostapd: phy0-ap2: STA 30:86:2d:03:17:f0 IEEE 802.11: SAE authentication (RX confirm, status=0 (SUCCESS))',
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 IEEE 802.11: authentication OK (open system)',
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 MLME: MLME-AUTHENTICATE.indication(30:86:2d:03:17:f0, OPEN_SYSTEM)',
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 IEEE 802.11: association OK (aid 1)',
	"<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 IEEE 802.11: binding station to interface 'phy0-ap1'",
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: event 1 notification',
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: start authentication',
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 IEEE 802.1X: unauthorizing port',
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 IEEE 802.1X: authorizing port',
	'<31>Oct 11 12:43:13 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: sending 1/2 msg of Group Key Handshake',
	// The same of the messages without a priority, as a line handed over by
	// hand has none, is read too; what is not known is then kept as it reads.
	'hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: sending 1/4 msg of 4-Way Handshake',
	'hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: event 1 notification',
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
	// How it came, where anything is known of it (0136).
	if (r.how)
		printf('  how it came: %J\n', r.how);
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

// The key handshake a message at a time (0133), where hostapd logs at
// debug, with the times the C-360 gave on 2026-10-11. said takes a line of
// any level, as the daemon does: one that says nothing is passed over.
let said = (a, at, pri, rest) => {
	let e = journey.log_event(`<${pri}>Oct 11 12:43:13 hostapd: phy0-ap0: ${rest}`);
	if (e)
		journey.on_log(a, e, at);
};
// associate is the association and the AP's first message, as hostapd
// says them; handshake, those after open authentication, of which it says
// "authenticated" twice over, once at debug: one step.
let associate = (a, at) => {
	said(a, at + 0.011, 31, `STA ${STA} IEEE 802.11: association OK (aid 1)`);
	said(a, at + 0.016, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
	said(a, at + 0.016, 31, `STA ${STA} MLME: MLME-ASSOCIATE.indication(${STA})`);
	said(a, at + 0.016, 31, `STA ${STA} IEEE 802.11: binding station to interface 'phy0-ap0'`);
	said(a, at + 0.018, 31, `STA ${STA} WPA: event 1 notification`);
	said(a, at + 0.018, 31, `STA ${STA} WPA: start authentication`);
	said(a, at + 0.018, 31, `STA ${STA} IEEE 802.1X: unauthorizing port`);
	said(a, at + 0.018, 31, `STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`);
};
let handshake = (a, at) => {
	said(a, at, 31, `STA ${STA} IEEE 802.11: authentication OK (open system)`);
	said(a, at, 31, `STA ${STA} MLME: MLME-AUTHENTICATE.indication(${STA}, OPEN_SYSTEM)`);
	said(a, at, 31, `STA ${STA} MLME: MLME-DELETEKEYS.request(${STA})`);
	said(a, at + 0.003, 30, `STA ${STA} IEEE 802.11: authenticated`);
	associate(a, at);
};

// The right passphrase: four messages, and it is let on.
a = journey.begin(who, 16000, 1791699000000);
handshake(a, 16000);
said(a, 16000.032, 31, `STA ${STA} WPA: received EAPOL-Key frame (2/4 Pairwise)`);
said(a, 16000.032, 31, `STA ${STA} WPA: sending 3/4 msg of 4-Way Handshake`);
said(a, 16000.036, 31, `STA ${STA} WPA: received EAPOL-Key frame (4/4 Pairwise)`);
said(a, 16000.099, 29, `AP-STA-CONNECTED ${STA} auth_alg=open`);
said(a, 16000.099, 31, `STA ${STA} IEEE 802.1X: authorizing port`);
said(a, 16000.100, 30, `STA ${STA} RADIUS: starting accounting session AA4211A5289F5F05`);
said(a, 16000.100, 30, `STA ${STA} WPA: pairwise key handshake completed (RSN)`);
said(a, 16000.100, 29, `EAPOL-4WAY-HS-COMPLETED ${STA}`);
show('the four messages', a, 16031);

// A wrong passphrase: its answer to the first message does not check out,
// four times over, and the AP gives it up.
a = journey.begin(who, 17000, 1791700000000);
handshake(a, 17000);
for (let i = 0; i < 4; i++) {
	if (i)
		said(a, 17000.018 + i, 31, `STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`);
	said(a, 17000.033 + i, 31, `STA ${STA} WPA: received EAPOL-Key frame (2/4 Pairwise)`);
	said(a, 17000.033 + i, 31, `STA ${STA} WPA: invalid MIC in msg 2/4 of 4-Way Handshake`);
	said(a, 17000.033 + i, 29, `AP-STA-POSSIBLE-PSK-MISMATCH ${STA}`);
	said(a, 17001.018 + i, 31, `STA ${STA} WPA: EAPOL-Key timeout`);
}
said(a, 17004.018, 31, `STA ${STA} WPA: PTKSTART: Retry limit 4 reached`);
said(a, 17004.018, 31, `STA ${STA} WPA: event 3 notification`);
said(a, 17009.018, 30, `STA ${STA} IEEE 802.11: deauthenticated due to local deauth request`);
show('a wrong passphrase, a message at a time', a, 17009.1);

// A client that answers nothing: the first message four times, and the AP
// gives it up. It failed nothing that is known: it may have gone.
a = journey.begin(who, 18000, 1791701000000);
handshake(a, 18000);
for (let i = 1; i < 4; i++) {
	said(a, 18000.018 + i, 31, `STA ${STA} WPA: EAPOL-Key timeout`);
	said(a, 18000.018 + i, 31, `STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`);
}
said(a, 18004.018, 31, `STA ${STA} WPA: EAPOL-Key timeout`);
said(a, 18004.018, 31, `STA ${STA} WPA: PTKSTART: Retry limit 4 reached`);
said(a, 18009.018, 30, `STA ${STA} IEEE 802.11: deauthenticated due to local deauth request`);
show('no answer to the first message', a, 18009.1);

// And one that answers the first and not the third: the AP gives it up, and
// sends it off five seconds on, as it always does. That failed.
a = journey.begin(who, 19000, 1791702000000);
handshake(a, 19000);
said(a, 19000.032, 31, `STA ${STA} WPA: received EAPOL-Key frame (2/4 Pairwise)`);
said(a, 19000.032, 31, `STA ${STA} WPA: sending 3/4 msg of 4-Way Handshake`);
for (let i = 1; i < 4; i++)
	said(a, 19000.032 + i, 31, `STA ${STA} WPA: sending 3/4 msg of 4-Way Handshake`);
said(a, 19004.032, 31, `STA ${STA} WPA: PTKINITNEGOTIATING: Retry limit 4 reached`);
said(a, 19009.032, 30, `STA ${STA} IEEE 802.11: deauthenticated due to local deauth request`);
show('no answer to the third message', a, 19009.1);

// A client that goes of its own accord in the middle of it says so, which
// hostapd passes on at debug only: it left, and failed nothing.
a = journey.begin(who, 22000, 1791705000000);
handshake(a, 22000);
said(a, 22000.5, 31, `STA ${STA} WPA: event 3 notification`);
said(a, 22000.5, 31, `STA ${STA} IEEE 802.1X: unauthorizing port`);
said(a, 22000.5, 31, `STA ${STA} IEEE 802.11: deauthenticated`);
said(a, 22000.5, 31, `STA ${STA} MLME: MLME-DEAUTHENTICATE.indication(${STA}, 3)`);
printf('due once it said it left %J\n', journey.due(a, 22000.6));
show('left in the middle of the handshake', a, 22000.6);

// A wrong passphrase, and then the right one within the same attempt, as
// a client that tries each key it has: let on, the first failed nothing.
a = journey.begin(who, 23000, 1791706000000);
handshake(a, 23000);
said(a, 23000.033, 31, `STA ${STA} WPA: received EAPOL-Key frame (2/4 Pairwise)`);
said(a, 23000.033, 29, `AP-STA-POSSIBLE-PSK-MISMATCH ${STA}`);
said(a, 23001.018, 31, `STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`);
said(a, 23001.030, 31, `STA ${STA} WPA: received EAPOL-Key frame (2/4 Pairwise)`);
said(a, 23001.031, 31, `STA ${STA} WPA: sending 3/4 msg of 4-Way Handshake`);
said(a, 23001.036, 31, `STA ${STA} WPA: received EAPOL-Key frame (4/4 Pairwise)`);
said(a, 23001.1, 29, `AP-STA-CONNECTED ${STA} auth_alg=open`);
said(a, 23001.1, 30, `STA ${STA} WPA: pairwise key handshake completed (RSN)`);
show('wrong, and then right', a, 23032);

// The AP gives a client up, and it joins again before it is sent off: the
// tries of the second handshake are counted from one.
a = journey.begin(who, 24000, 1791707000000);
handshake(a, 24000);
for (let i = 1; i < 4; i++)
	said(a, 24000.018 + i, 31, `STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`);
said(a, 24004.018, 31, `STA ${STA} WPA: PTKSTART: Retry limit 4 reached`);
handshake(a, 24004.5);
said(a, 24005.518, 31, `STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`);
show('given up, and joined again', a, 24030);

// An association the client does not acknowledge: said at debug, and read.
a = journey.begin(who, 25000, 1791708000000);
said(a, 25000, 30, `STA ${STA} IEEE 802.11: authenticated`);
said(a, 25000.2, 31, `STA ${STA} IEEE 802.11: did not acknowledge association response`);
show('association not acknowledged', a, 25021);

// WPA3 with a wrong passphrase: hostapd says the commit and the confirm,
// at debug, and nothing more. Without those lines there is no attempt.
a = journey.begin(who, 20000, 1791703000000);
said(a, 20000, 31, `STA ${STA} IEEE 802.11: start SAE authentication (RX commit, status=126 (SAE_HASH_TO_ELEMENT))`);
said(a, 20000.032, 31, `STA ${STA} IEEE 802.11: SAE authentication (RX confirm, status=0 (SUCCESS))`);
printf('due after 10 s %J, after 21 s %J\n', journey.due(a, 20010), journey.due(a, 20021));
show('WPA3, a wrong passphrase', a, 20021);

// WPA3 with the right one, as the C-360 logged it on 2026-10-11: the same
// two, and then that it is authenticated, which hostapd says of WPA3 at
// debug alone; it never says "authenticated" of it.
let sae = (a, at) => {
	said(a, at, 31, `STA ${STA} IEEE 802.11: start SAE authentication (RX commit, status=126 (SAE_HASH_TO_ELEMENT))`);
	said(a, at + 0.017, 31, `STA ${STA} IEEE 802.11: SAE authentication (RX confirm, status=0 (SUCCESS))`);
	said(a, at + 0.017, 31, `STA ${STA} MLME: MLME-AUTHENTICATE.indication(${STA}, unknown)`);
	said(a, at + 0.017, 31, `STA ${STA} MLME: MLME-DELETEKEYS.request(${STA})`);
};
a = journey.begin(who, 21000, 1791704000000);
sae(a, 21000);
associate(a, 21000.021);
said(a, 21000.051, 31, `STA ${STA} WPA: received EAPOL-Key frame (2/4 Pairwise)`);
said(a, 21000.051, 31, `STA ${STA} WPA: sending 3/4 msg of 4-Way Handshake`);
said(a, 21000.054, 31, `STA ${STA} WPA: received EAPOL-Key frame (4/4 Pairwise)`);
said(a, 21000.055, 29, `AP-STA-CONNECTED ${STA} auth_alg=sae`);
said(a, 21000.055, 31, `STA ${STA} IEEE 802.1X: authorizing port`);
said(a, 21000.055, 30, `STA ${STA} WPA: pairwise key handshake completed (RSN)`);
said(a, 21000.055, 29, `EAPOL-4WAY-HS-COMPLETED ${STA}`);
show('WPA3, the right passphrase', a, 21031);

// WPA3 with the right one, and then no association: it stopped there, and
// its passphrase was not wrong.
a = journey.begin(who, 26000, 1791709000000);
sae(a, 26000);
show('WPA3, authenticated and no more', a, 26021);

// And as hostapd says a WPA3 join where it does not log at debug: nothing
// of the authentication at all. Associated, it is taken as authenticated.
a = journey.begin(who, 27000, 1791710000000);
said(a, 27000, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 27000.021, 29, `AP-STA-CONNECTED ${STA} auth_alg=sae`);
said(a, 27000.021, 30, `STA ${STA} WPA: pairwise key handshake completed (RSN)`);
show('WPA3, with no debug lines', a, 27031);

// A client that comes back to an AP that still holds it as on, as one that
// went to another AP without a word and returned within minutes (the
// C-360, 2026-10-11, a network with protected management frames). The AP
// asks the client it remembers to prove itself, five times in a second,
// has no answer, and takes the newcomer; of the handshake that follows it
// says that it completed, and not that the client is connected, which to
// hostapd it never ceased to be. Its keys in place, it is let on.
a = journey.begin(who, 28000, 1791711000000);
said(a, 28000.000, 31, `STA ${STA} IEEE 802.11: authentication OK (open system)`);
said(a, 28000.000, 31, `STA ${STA} MLME: MLME-AUTHENTICATE.indication(${STA}, OPEN_SYSTEM)`);
said(a, 28000.001, 30, `STA ${STA} IEEE 802.11: authenticated`);
for (let i = 0; i < 5; i++)
	said(a, 28000.013 + i * 0.206, 31, `STA ${STA} IEEE 802.11: association SA Query attempt ${i + 1}`);
said(a, 28001.043, 31, `STA ${STA} IEEE 802.11: association SA Query timed out`);
said(a, 28001.077, 31, `STA ${STA} WPA: PMKID found from PMKSA cache eap_type=0 vlan=0`);
said(a, 28001.077, 31, `STA ${STA} IEEE 802.11: association OK (aid 1)`);
said(a, 28001.177, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 28001.177, 31, `STA ${STA} MLME: MLME-REASSOCIATE.indication(${STA})`);
said(a, 28001.178, 31, `STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`);
said(a, 28001.193, 31, `STA ${STA} WPA: received EAPOL-Key frame (2/4 Pairwise)`);
said(a, 28001.193, 31, `STA ${STA} WPA: sending 3/4 msg of 4-Way Handshake`);
said(a, 28001.197, 31, `STA ${STA} WPA: received EAPOL-Key frame (4/4 Pairwise)`);
said(a, 28001.198, 30, `STA ${STA} WPA: pairwise key handshake completed (RSN)`);
said(a, 28001.198, 29, `EAPOL-4WAY-HS-COMPLETED ${STA}`);
show('back at an AP that still held it', a, 28032);
// And so where hostapd does not log at debug, and says the one line.
a = journey.begin(who, 29000, 1791712000000);
said(a, 29000.000, 30, `STA ${STA} IEEE 802.11: authenticated`);
said(a, 29000.012, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 29000.030, 29, `EAPOL-4WAY-HS-COMPLETED ${STA}`);
show('back, with no debug lines', a, 29031);

// And where it says the other line alone.
a = journey.begin(who, 29100, 1791712100000);
said(a, 29100.000, 30, `STA ${STA} IEEE 802.11: authenticated`);
said(a, 29100.012, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 29100.030, 30, `STA ${STA} WPA: pairwise key handshake completed (RSN)`);
show('back, with its keys said in words alone', a, 29131);

// What begins an attempt, of a client that is not being followed: its
// authentication or its association, and nothing else. A client long on
// whose keys are made again, or that signs in again as 802.1X has it do
// every hour, comes nowhere.
for (let line in [
	`STA ${STA} IEEE 802.11: authenticated`,
	`STA ${STA} MLME: MLME-AUTHENTICATE.indication(${STA}, OPEN_SYSTEM)`,
	`STA ${STA} IEEE 802.11: start SAE authentication (RX commit, status=0 (SUCCESS))`,
	`STA ${STA} IEEE 802.11: associated (aid 1)`,
	`STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`,
	`STA ${STA} WPA: received EAPOL-Key frame (2/4 Pairwise)`,
	`STA ${STA} WPA: pairwise key handshake completed (RSN)`,
	`EAPOL-4WAY-HS-COMPLETED ${STA}`,
	`AP-STA-CONNECTED ${STA} auth_alg=open`,
	`CTRL-EVENT-EAP-STARTED ${STA}`,
	`STA ${STA} IEEE 802.1X: authenticated - EAP type: 25 (PEAP)`,
	`STA ${STA} IEEE 802.11: deauthenticated due to local deauth request`,
	`AP-STA-DISCONNECTED ${STA}`,
	// What is all hostapd says of a device it refuses at authentication
	// for the RADIUS server's answer; what it says on every answer.
	`STA ${STA} RADIUS: Invalid VLAN 30 received from RADIUS server`,
	`STA ${STA} RADIUS: invalid hex string (65 chars) in Tunnel-Password`,
	`STA ${STA} RADIUS: VLAN ID 30`,
	`CTRL-EVENT-SAE-UNKNOWN-PASSWORD-IDENTIFIER ${STA}`,
	// What hostapd says beside an attempt, of how the client came (0136).
	`STA ${STA} MLME: MLME-REASSOCIATE.indication(${STA})`,
	`STA ${STA} IEEE 802.11: association SA Query attempt 1`,
	`STA ${STA} IEEE 802.11: association SA Query timed out`,
	`STA ${STA} WPA: OKC match for PMKID`,
	`STA ${STA} WPA: PMKID found from PMKSA cache eap_type=0 vlan=0`,
	`STA ${STA} WPA: FT authentication already completed - do not start 4-way handshake`,
]) {
	// At info, as hostapd says each of these but the key messages.
	let e = journey.log_event(`<${match(line, /msg of 4-Way|EAPOL-Key frame|MLME|SA Query|PMKID|FT authentication/) ? 31 : 30}>Oct 11 12:43:13 hostapd: phy0-ap0: ${line}`);
	printf('%s: %s\n', e && journey.begins(e) ? 'begins an attempt' : 'begins nothing', replace(line, STA + ' ', ''));
}
// A device refused for the server's answer has that line for its record.
a = journey.begin(who, 34000, 1791717000000);
said(a, 34000, 30, `STA ${STA} RADIUS: Invalid VLAN 30 received from RADIUS server`);
show('refused at authentication for the VLAN the server gave', a, 34021);

// How long an attempt not yet let on is waited for with nothing said: 20 s,
// and 40 s of one that is signing in, as hostapd goes over to a second
// RADIUS server only after 21 s without the first's answer. Then the
// sign-in completes, and the attempt is the client's, let on.
a = journey.begin(who, 35000, 1791718000000);
said(a, 35000.000, 30, `STA ${STA} IEEE 802.11: authenticated`);
printf('authenticated, and silent: given up after 20 s %J, after 21 s %J\n', journey.due(a, 35020), journey.due(a, 35021.1));
said(a, 35000.010, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 35000.020, 29, `CTRL-EVENT-EAP-STARTED ${STA}`);
printf('signing in, and silent: given up after 21 s %J, after 40 s %J, after 41 s %J\n', journey.due(a, 35021.1), journey.due(a, 35040), journey.due(a, 35041.1));
said(a, 35021.500, 29, `CTRL-EVENT-EAP-SUCCESS2 ${STA}`);
said(a, 35021.520, 30, `STA ${STA} IEEE 802.1X: authenticated - EAP type: 25 (PEAP)`);
said(a, 35021.560, 29, `AP-STA-CONNECTED ${STA} auth_alg=open`);
said(a, 35021.560, 30, `STA ${STA} WPA: pairwise key handshake completed (RSN)`);
show('a sign-in that waited 21 s for a second RADIUS server', a, 35052);
// How a client came (0136), as hostapd said it on the C-360, 2026-10-11, of
// its own scan radio roaming to it from the office AP. By a fast transition
// (802.11r): authenticated as one, a reassociation, and its keys with it,
// 17 ms from its first frame.
a = journey.begin(who, 30000, 1791713000000);
said(a, 30000.000, 31, `STA ${STA} IEEE 802.11: binding station to interface 'phy0-ap2'`);
said(a, 30000.000, 31, `STA ${STA} IEEE 802.11: authentication OK (FT)`);
said(a, 30000.000, 31, `STA ${STA} MLME: MLME-AUTHENTICATE.indication(${STA}, FT)`);
said(a, 30000.011, 31, `STA ${STA} IEEE 802.11: association OK (aid 1)`);
said(a, 30000.016, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 30000.017, 29, `AP-STA-CONNECTED ${STA} auth_alg=ft`);
said(a, 30000.017, 31, `STA ${STA} MLME: MLME-REASSOCIATE.indication(${STA})`);
said(a, 30000.017, 31, `STA ${STA} IEEE 802.11: binding station to interface 'phy0-ap2'`);
said(a, 30000.018, 31, `STA ${STA} WPA: event 6 notification`);
said(a, 30000.018, 31, `STA ${STA} WPA: FT authentication already completed - do not start 4-way handshake`);
show('a roam by fast transition', a, 30031);
// Back by a fast transition at an AP that still held it as on: hostapd
// says it authenticated and associated, and no more, having no handshake to
// complete and no change to report. Associated, it is on.
a = journey.begin(who, 30500, 1791713500000);
said(a, 30500.000, 31, `STA ${STA} IEEE 802.11: authentication OK (FT)`);
said(a, 30500.000, 31, `STA ${STA} MLME: MLME-AUTHENTICATE.indication(${STA}, FT)`);
said(a, 30500.011, 31, `STA ${STA} IEEE 802.11: association OK (aid 1)`);
said(a, 30500.016, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 30500.017, 31, `STA ${STA} MLME: MLME-REASSOCIATE.indication(${STA})`);
show('back by a fast transition, to an AP that still held it', a, 30531);
// The same where hostapd does not log at debug, as on a network that signs
// clients in: its word for the client let on says how.
// The attempt begins at its association, the first line there is of it: its
// time to be let on is from then, and says so.
a = journey.begin(who, 31000.016, 1791714000016);
said(a, 31000.016, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 31000.017, 29, `AP-STA-CONNECTED ${STA} auth_alg=ft`);
show('a roam by fast transition, with no debug lines', a, 31031);
// And back that way to an AP that still held it, where hostapd does not
// log at debug: associated, and not a word more, for there is no handshake
// and no change to say. It is on: a client that is to have keys has a
// handshake that completes, or is sent off, and each is said.
a = journey.begin(who, 31500, 1791714500000);
said(a, 31500.000, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
show('associated, and nothing said after', a, 31521);
// Not so where its keys were begun and nothing came of them.
a = journey.begin(who, 31600, 1791714600000);
said(a, 31600.000, 30, `STA ${STA} IEEE 802.11: authenticated`);
said(a, 31600.004, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 31600.005, 31, `STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`);
show('associated, its keys begun, and nothing after', a, 31621);
// A fast transition that came to nothing, and then a join the usual way:
// what was known of the first try is not said of the second.
a = journey.begin(who, 31700, 1791714700000);
said(a, 31700.000, 31, `STA ${STA} MLME: MLME-AUTHENTICATE.indication(${STA}, FT)`);
said(a, 31705.000, 31, `STA ${STA} MLME: MLME-AUTHENTICATE.indication(${STA}, OPEN_SYSTEM)`);
said(a, 31705.002, 30, `STA ${STA} IEEE 802.11: authenticated`);
said(a, 31705.013, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 31705.014, 31, `STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`);
said(a, 31709.100, 30, `STA ${STA} IEEE 802.11: deauthenticated due to local deauth request`);
show('a fast transition given up, then the usual way, given up too', a, 31710);
// WPA3's exchange again at an AP that still held the client: hostapd does
// not say it connected, and its authentication is known all the same.
a = journey.begin(who, 31800, 1791714800000);
said(a, 31800.000, 31, `STA ${STA} IEEE 802.11: start SAE authentication (RX commit, status=0 (SUCCESS))`);
said(a, 31800.040, 31, `STA ${STA} IEEE 802.11: SAE authentication (RX confirm, status=0 (SUCCESS))`);
said(a, 31800.041, 31, `STA ${STA} MLME: MLME-AUTHENTICATE.indication(${STA}, unknown)`);
said(a, 31800.060, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 31800.060, 31, `STA ${STA} MLME: MLME-REASSOCIATE.indication(${STA})`);
said(a, 31800.061, 31, `STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`);
said(a, 31800.080, 30, `STA ${STA} WPA: pairwise key handshake completed (RSN)`);
said(a, 31800.080, 29, `EAPOL-4WAY-HS-COMPLETED ${STA}`);
show("WPA3's exchange again, at an AP that still held it", a, 31831);
// With a passphrase and no fast transition: open authentication, a
// reassociation, and the four messages again.
a = journey.begin(who, 32000, 1791715000000);
said(a, 32000.000, 31, `STA ${STA} IEEE 802.11: authentication OK (open system)`);
said(a, 32000.000, 31, `STA ${STA} MLME: MLME-AUTHENTICATE.indication(${STA}, OPEN_SYSTEM)`);
said(a, 32000.002, 30, `STA ${STA} IEEE 802.11: authenticated`);
said(a, 32000.009, 31, `STA ${STA} IEEE 802.11: association OK (aid 1)`);
said(a, 32000.013, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 32000.013, 31, `STA ${STA} MLME: MLME-REASSOCIATE.indication(${STA})`);
said(a, 32000.013, 31, `STA ${STA} WPA: sending 1/4 msg of 4-Way Handshake`);
said(a, 32000.029, 31, `STA ${STA} WPA: received EAPOL-Key frame (2/4 Pairwise)`);
said(a, 32000.029, 31, `STA ${STA} WPA: sending 3/4 msg of 4-Way Handshake`);
said(a, 32000.033, 31, `STA ${STA} WPA: received EAPOL-Key frame (4/4 Pairwise)`);
said(a, 32000.280, 29, `AP-STA-CONNECTED ${STA} auth_alg=open`);
said(a, 32000.281, 30, `STA ${STA} WPA: pairwise key handshake completed (RSN)`);
said(a, 32000.281, 29, `EAPOL-4WAY-HS-COMPLETED ${STA}`);
show('a roam with a passphrase', a, 32031);
// A key made from its sign-in at another AP (OKC), as hostapd's source has
// it say: that, and then that the key was found, as of one of its own.
a = journey.begin(who, 33000, 1791716000000);
said(a, 33000.000, 30, `STA ${STA} IEEE 802.11: authenticated`);
said(a, 33000.010, 31, `STA ${STA} WPA: OKC match for PMKID`);
said(a, 33000.010, 31, `STA ${STA} WPA: PMKID found from PMKSA cache eap_type=25 vlan=0`);
said(a, 33000.012, 30, `STA ${STA} IEEE 802.11: associated (aid 1)`);
said(a, 33000.012, 31, `STA ${STA} MLME: MLME-REASSOCIATE.indication(${STA})`);
said(a, 33000.030, 29, `AP-STA-CONNECTED ${STA} auth_alg=open`);
show('a roam with a key from another AP', a, 33031);

// When the log took a line in, on the daemon's clocks (logread -t): a line
// logged 45 ms before the daemon took it up, one 6 ms before, one that
// came in 3 ms after, one that says no time, and one whose time cannot be
// right. The daemon took them up at 5000.5 s by its steady clock.
for (let line in [
	'<31>Oct 11 13:11:03 [1791742263.455] hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: sending 1/4 msg of 4-Way Handshake',
	'<31>Oct 11 13:11:03 [1791742263.494] hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: received EAPOL-Key frame (2/4 Pairwise)',
	'<31>Oct 11 13:11:03 [1791742263.503] hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: sending 3/4 msg of 4-Way Handshake',
	'<31>Oct 11 13:11:03 hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: sending 3/4 msg of 4-Way Handshake',
	'<31>Oct 11 13:10:53 [1791742253.500] hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: sending 3/4 msg of 4-Way Handshake',
	'<31>Oct 11 13:11:03 [1791742263.008] hostapd: phy0-ap1: STA 30:86:2d:03:17:f0 WPA: sending 3/4 msg of 4-Way Handshake',
]) {
	let t = journey.log_time(line, 5000.5, 1791742263500);
	printf('log time %s: %.3f s, %d\n', match(line, /\[[0-9.]+\]/)?.[0] ?? 'none said', t[0], t[1]);
}
