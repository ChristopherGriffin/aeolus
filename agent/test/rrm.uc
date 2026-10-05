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
printf('visits 5g 149 %J\n', rrm.visits('5g', 149, false));
printf('visits 5g 100 dfs %d\n', length(rrm.visits('5g', 100, true)));
printf('band %s %s %J\n', rrm.band_of(2462), rrm.band_of(5745), rrm.band_of(900));
printf('radar %J %J\n', rrm.radar(5500), rrm.radar(5745));
