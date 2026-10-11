// Prints radio resource management's pure parts (0073), for the manager's
// contract test: rrm.out is what it must print. The first three HMACs are
// RFC 4231's test cases 1, 2 and 6, and the fourth, with NUL bytes in its
// key and message, was worked out with Python's hmac, so they are checked
// apart from ucode.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/rrm.uc

'use strict';

import * as rrm from 'aeolus.rrm';

function rep(c, n) {
	let o = '';
	for (let i = 0; i < n; i++)
		o += c;
	return o;
}

printf('hmac 1 %s\n', rrm.hmac(rep('\x0b', 20), 'Hi There'));
printf('hmac 2 %s\n', rrm.hmac('Jefe', 'what do ya want for nothing?'));
printf('hmac 6 %s\n', rrm.hmac(rep('\xaa', 131), 'Test Using Larger Than Block-Size Key - Hash Key First'));
printf('hmac nul %s\n', rrm.hmac('\x00\x01\x00\x02', 'a\x00b\x00c'));

// The lab's element, as hostapd took it on 2026-10-05, and what an AP that
// hears it reads.
let ad = rrm.advert('ap-a0046021365e', '192.168.1.38', rrm.PORT);
printf('advert %s\n', ad);
printf('read %J\n', rrm.read_advert(rrm.unhex(substr(ad, 4))));
printf('read later version %J\n', rrm.read_advert(rrm.unhex(replace(substr(ad, 4), /^02ae010101/, '02ae010102') + 'ff')));
printf('read other OUI %J\n', rrm.read_advert(rrm.unhex('0050f2' + substr(ad, 10))));
printf('read short %J\n', rrm.read_advert(rrm.unhex(substr(ad, 4, 20))));
printf('advert bad ID %J\n', rrm.advert('office', '192.168.1.38', rrm.PORT));
printf('advert bad address %J\n', rrm.advert('ap-a0046021365e', '192.168.1.300', rrm.PORT));

// A hello, signed and read back; tampered with, under another key, or of
// another version, it is refused.
let key = rrm.unhex(rep('42', 32));
let hello = { v: 1, t: 'hello', ap: 'ap-2005b6018be0', at: 1000, seq: 7, sees: ['ap-a0046021365e'] };
let pkt = rrm.seal(key, hello);
printf('open %J\n', rrm.open(key, pkt));
printf('open tampered %J\n', rrm.open(key, replace(pkt, '"seq": 7', '"seq": 8')));
printf('open other key %J\n', rrm.open(rrm.unhex(rep('43', 32)), pkt));
printf('open version 2 %J\n', rrm.open(key, rrm.seal(key, { ...hello, v: 2 })));
printf('open not an AP %J\n', rrm.open(key, rrm.seal(key, { ...hello, ap: 'office' })));

// Hellos: new, too far off the clock, sent again, and later.
printf('fresh first %J\n', rrm.fresh(null, { at: 1000, seq: 1 }, 1010));
printf('fresh skewed %J\n', rrm.fresh(null, { at: 1000, seq: 1 }, 1100));
printf('fresh again %J\n', rrm.fresh({ at: 1000, seq: 1 }, { at: 1000, seq: 1 }, 1010));
printf('fresh next %J\n', rrm.fresh({ at: 1000, seq: 1 }, { at: 1000, seq: 2 }, 1010));
printf('fresh older %J\n', rrm.fresh({ at: 1000, seq: 5 }, { at: 999, seq: 9 }, 1010));

// The three heard most strongly on each band, by name where alike.
printf('choose %J\n', rrm.choose({
	'ap-000000000001': { '2g': -60, '5g': -70 }, 'ap-000000000002': { '2g': -50 }, 'ap-000000000003': { '2g': -80 },
	'ap-000000000004': { '2g': -55 }, 'ap-000000000005': { '5g': -40 }, 'ap-000000000006': { '2g': -55 },
}, rrm.NEIGHBOURS));
printf('smooth %J %J\n', rrm.smooth(null, -70), rrm.smooth(-70, -50));

// Where a radio listens.
printf('visits 2g 11 %J\n', rrm.visits('2g', 11, false));
printf('visits 2g 3 %J\n', rrm.visits('2g', 3, false));
printf('visits 2g set %J\n', rrm.visits('2g', 6, false, [11, 1, 6, 3]));
printf('visits 5g 149 %J\n', rrm.visits('5g', 149, false));
printf('visits 5g 100 dfs %d\n', length(rrm.visits('5g', 100, true)));
printf('band %s %s %J\n', rrm.band_of(2462), rrm.band_of(5745), rrm.band_of(900));

// Channel ratings: a network's weight, a visit's cost, the ratings, and the
// channels a neighbour's radio takes up.
printf('weight %J %J %J %J\n', rrm.weight(-50), rrm.weight(-75), rrm.weight(-100), rrm.weight(null));
printf('cost quiet %J\n', rrm.cost(0, null, []));
printf('cost busy %J\n', rrm.cost(0.3, -92, [-60, -85]));
printf('cost no noise %J\n', rrm.cost(1.5, null, [-40]));
printf('blend %J %J %J\n', rrm.blend(null, 40, rrm.LASTING), rrm.blend(40, 0, rrm.LASTING), rrm.blend(40, 0, rrm.NOW));
// The pick: a free channel first; with all three of 2.4 GHz used, the one
// whose nearest user is heard most weakly; alike, the best rated.
let free = [{ channel: 1, cost: 60, blotted_by: [] }, { channel: 6, cost: 30, blotted_by: [] }, { channel: 11, cost: 5, blotted_by: ['ap-000000000001'] }];
let full = [{ channel: 1, cost: 60, blotted_by: ['ap-000000000002'] }, { channel: 6, cost: 30, blotted_by: ['ap-000000000001', 'ap-000000000003'] }, { channel: 11, cost: 5, blotted_by: ['ap-000000000001'] }];
let close = { 'ap-000000000001': -60, 'ap-000000000002': -82, 'ap-000000000003': -85 };
printf('pick free %J\n', rrm.pick(free, close));
printf('pick all used %J\n', rrm.pick(full, close));
printf('pick alike %J\n', rrm.pick([{ channel: 1, cost: 60, blotted_by: ['ap-000000000002'] }, { channel: 6, cost: 30, blotted_by: ['ap-000000000002'] }], close));
printf('pick unknown is near %J\n', rrm.pick([{ channel: 1, cost: 60, blotted_by: ['ap-00000000000f'] }, { channel: 6, cost: 30, blotted_by: ['ap-000000000001'] }], close));
printf('pick none %J\n', rrm.pick([], close));
printf('covers %J %J %J %J %J\n', rrm.covers('2g', 11, 20), rrm.covers('5g', 149, 40), rrm.covers('5g', 157, 80), rrm.covers('5g', 165, 40), rrm.covers('5g', 100, 160));

// Moves (0073): what a radio at a width may move to, why it would, the
// tie-break, the window, and hostapd's switch arguments.
let five = [
	{ channel: 36, cost: 10, now: 12, blotted_by: [] }, { channel: 40, cost: 30, now: 20, blotted_by: [] },
	{ channel: 44, cost: 5, now: 5, blotted_by: ['ap-000000000001'] }, { channel: 48, cost: 8, now: 8, blotted_by: [] },
	{ channel: 149, cost: 50, now: 60, blotted_by: [] },
];
printf('blocks 20 %J\n', map(rrm.blocks(five, '5g', 20), x => x.channel));
printf('blocks 40 %J\n', rrm.blocks(five, '5g', 40));
printf('blocks 2g %J\n', map(rrm.blocks(free, '2g', 40), x => x.members));
printf('radar %J %J %J %J\n', rrm.radar('5g', 52), rrm.radar('5g', 48), rrm.radar('5g', 144), rrm.radar('2g', 1));
close['ap-000000000004'] = -64;
let b = (ch, cost, by) => ({ channel: ch, cost: cost, blotted_by: by, members: [ch] });
printf('reason shared %J\n', rrm.reason(b(11, 5, ['ap-000000000001']), b(6, 30, []), close, 20, null, false));
printf('reason all used, further %J\n', rrm.reason(b(11, 5, ['ap-000000000001']), b(1, 60, ['ap-000000000002']), close, 20, null, false));
printf('reason all used, not far enough %J\n', rrm.reason(b(11, 5, ['ap-000000000001']), b(1, 60, ['ap-000000000004']), close, 20, null, false));
printf('reason better %J\n', rrm.reason(b(1, 60, []), b(6, 30, []), close, 20, null, false));
printf('reason not by the margin %J\n', rrm.reason(b(1, 45, []), b(6, 30, []), close, 20, null, false));
printf('reason start %J\n', rrm.reason(b(1, 45, []), b(6, 30, []), close, 20, null, true));
printf('reason interference %J\n', rrm.reason(b(6, 10, []), b(1, 20, []), close, 20, 0.7, false));
printf('reason busy, nowhere better %J\n', rrm.reason(b(6, 10, []), b(1, 40, []), close, 20, 0.55, false));
printf('reason free to shared %J\n', rrm.reason(b(6, 60, []), b(1, 5, ['ap-000000000002']), close, 20, null, false));
printf('reason same block %J\n', rrm.reason({ channel: 36, cost: 50, blotted_by: [], members: [36, 40] }, b(40, 5, []), close, 20, null, true));
printf('first worse %J\n', rrm.first({ ap: 'ap-000000000002', cost: 40 }, { ap: 'ap-000000000001', cost: 30 }));
printf('first alike %J %J\n', rrm.first({ ap: 'ap-000000000002', cost: 30 }, { ap: 'ap-000000000001', cost: 30 }),
	rrm.first({ ap: 'ap-000000000001', cost: 30 }, { ap: 'ap-000000000002', cost: 30 }));
printf('window %J %J %J %J\n', rrm.window('02:00-05:00'), rrm.window('22:30-04:15'), rrm.window('08:09-09:00'), rrm.window('25:00-01:00'));
printf('in window %J %J %J %J %J %J %J\n', rrm.in_window([120, 300], 150), rrm.in_window([120, 300], 300),
	rrm.in_window([1350, 255], 1400), rrm.in_window([1350, 255], 100), rrm.in_window([1350, 255], 600),
	rrm.in_window([60, 60], 900), rrm.in_window(null, 150));
printf('switch 2g 6 %J\n', rrm.switch_args('2g', 6, 20, 'HT20', 10));
printf('switch 2g 11 he %J\n', rrm.switch_args('2g', 11, 20, 'HE20', 10));
printf('switch 2g 11 40 %J\n', rrm.switch_args('2g', 11, 40, 'HT40', 10));
printf('switch 5g 36 he40 %J\n', rrm.switch_args('5g', 36, 40, 'HE40', 10));
printf('switch 5g 40 vht40 %J\n', rrm.switch_args('5g', 40, 40, 'VHT40', 10));
printf('switch 5g 157 vht80 %J\n', rrm.switch_args('5g', 157, 80, 'VHT80', 10));
printf('switch 5g 161 vht80 %J\n', rrm.switch_args('5g', 161, 80, 'VHT80', 10));
printf('switch 5g 165 40 %J\n', rrm.switch_args('5g', 165, 40, 'HE40', 10));
printf('switch 5g 36 legacy %J\n', rrm.switch_args('5g', 36, 40, 'NOHT', 10));

// Power control (0077): looking for three neighbours, the weakest at -70 dBm.
printf('power none heard %J\n', rrm.power_step([], 3, -70, 20, 26));
printf('power two of three %J\n', rrm.power_step([-60, -65], 3, -70, 20, 26));
printf('power weakest low %J\n', rrm.power_step([-60, -65, -74, -90], 3, -70, 20, 26));
printf('power at target %J\n', rrm.power_step([-60, -65, -68], 3, -70, 20, 26));
printf('power well above %J\n', rrm.power_step([-50, -55, -62], 3, -70, 20, 26));
printf('power near the ceiling %J\n', rrm.power_step([-80], 1, -70, 25, 26));
printf('power at the ceiling %J\n', rrm.power_step([-80], 1, -70, 26, 26));
printf('power at the floor %J\n', rrm.power_step([-40], 1, -70, 8, 26));
printf('power near the floor %J\n', rrm.power_step([-40], 1, -70, 10, 26));
printf('power ignores junk %J\n', rrm.power_step([-60, null, 'x', -66], 2, -70, 20, 26));

// What a scan radio heard (0116), as airscan lists it: this AP's own
// network, two of a neighbour's 2.4 GHz radio's networks and one of its 5 GHz
// radio's, by the BSSIDs its hellos carry, and others: named, hidden, heard
// louder than a signal is said, and ill formed.
let theirs = {
	'05:b6:01:8b:e2': { ap: 'ap-2005b6018be0', band: '2g' },
	'05:b6:01:8b:e8': { ap: 'ap-2005b6018be0', band: '5g' },
};
let look = rrm.swept([
	{ bssid: '36:86:2d:03:17:d0', ssid: 'test', band: '2g', channel: 6, signal: 4, own: true },
	{ bssid: '20:05:B6:01:8B:E2', ssid: 'Aeolus Lab', band: '2g', channel: 1, signal: -31 },
	{ bssid: '26:05:b6:01:8b:e2', ssid: 'Sweet Spot', band: '2g', channel: 1, signal: -30 },
	{ bssid: '22:05:b6:01:8b:e8', ssid: 'Aeolus Lab', band: '5g', channel: 132, signal: -44 },
	{ bssid: '22:05:b6:01:8b:e2', ssid: 'a 5 GHz network with the 2.4 GHz radio\'s tail', band: '5g', channel: 36, signal: -70 },
	{ bssid: '5c:83:6c:77:10:c0', ssid: 'Sweet Spot', band: '2g', channel: 1, signal: -43 },
	{ bssid: '5c:83:6c:77:10:c1', ssid: '', band: '2g', channel: 3, signal: -60, hidden: true },
	{ bssid: '5c:83:6c:77:10:c2', ssid: 'caf\xc3\xa9', band: '2g', channel: 8, signal: 3 },
	{ bssid: 'not a bssid', ssid: 'x', band: '2g', channel: 1, signal: -50 },
	{ bssid: '5c:83:6c:77:10:c3', ssid: 'x', band: '60g', channel: 1, signal: -50 },
	{ bssid: '5c:83:6c:77:10:c4', ssid: 'x', band: '2g', channel: 0, signal: -50 },
	{ bssid: '5c:83:6c:77:10:c5', ssid: 'x', band: '2g', channel: 1, signal: null },
	'junk',
], theirs);
printf('swept aps %J\n', look.aps);
printf('swept tails %J\n', look.tails);
for (let o in look.others)
	printf('swept other %J\n', o);
printf('swept networks %J\n', look.networks);
printf('around 2g 1 %J\n', rrm.around(look.networks, '2g', 1));
printf('around 2g 6 %J\n', rrm.around(look.networks, '2g', 6));
printf('around 2g 11 %J\n', rrm.around(look.networks, '2g', 11));
printf('around 5g 36 %J\n', rrm.around(look.networks, '5g', 36));
printf('around 5g 40 %J\n', rrm.around(look.networks, '5g', 40));
printf('swept nothing %J\n', rrm.swept(null, null));
printf('printable %J %J %J %J\n', rrm.printable('Sweet Spot'), rrm.printable(''), rrm.printable('\x00\x00'), rrm.printable('???'));
