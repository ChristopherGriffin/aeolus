// What the prober's RADIUS Status-Server request is, how its answers read,
// how hostapd's MIB is read, and the verdict (0114): TestAgentRadius runs
// this with the real ucode and compares what it prints with radius.out. The
// HMAC-MD5 values are RFC 2202's test cases 1 and 2.

'use strict';

import * as radius from 'aeolus.radius';
import { md5 } from 'digest';

function hexs(s) {
	let out = '';
	for (let i = 0; i < length(s); i++)
		out += sprintf('%02x', ord(s, i));
	return out;
}

function bytes(list) {
	return join('', map(list, b => chr(b)));
}

function bin(h) {
	let out = '';
	for (let i = 0; i + 1 < length(h); i += 2)
		out += chr(hex(substr(h, i, 2)));
	return out;
}

printf('able %J\n', radius.able());

let key1 = '';
for (let i = 0; i < 16; i++)
	key1 += chr(0x0b);
printf('hmac 1 %s\n', hexs(radius.hmac_md5(key1, 'Hi There')));
printf('hmac 2 %s\n', hexs(radius.hmac_md5('Jefe', 'what do ya want for nothing?')));

// A request with a known authenticator.
let auth = bytes([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]);
let req = radius.status_request(42, auth, 'fixture-radius-secret');
printf('request %s\n', hexs(req));

// The server's Access-Accept to it: code 2, the id, length 20, and the
// Response Authenticator, MD5 of all that, the request's authenticator and
// the secret.
function answer(code, id, secret) {
	let head = chr(code) + chr(id) + chr(0) + chr(20);
	return head + bin(md5(head + auth + secret));
}
printf('accept %J\n', radius.status_answer(answer(2, 42, 'fixture-radius-secret'), 42, auth, 'fixture-radius-secret'));
printf('accounting response %J\n', radius.status_answer(answer(5, 42, 'fixture-radius-secret'), 42, auth, 'fixture-radius-secret'));
printf('reject %J\n', radius.status_answer(answer(3, 42, 'fixture-radius-secret'), 42, auth, 'fixture-radius-secret'));
printf('another secret %J\n', radius.status_answer(answer(2, 42, 'another-secret'), 42, auth, 'fixture-radius-secret'));
printf('another id %J\n', radius.status_answer(answer(2, 41, 'fixture-radius-secret'), 42, auth, 'fixture-radius-secret'));
printf('too short %J\n', radius.status_answer('abc', 42, auth, 'fixture-radius-secret'));

// hostapd's MIB on the C-360's Aeolus-Enterprise, 2026-10-10, cut to the
// lines that matter and two that don't.
let text = 'dot11RSNAOptionImplemented=TRUE\n' +
	'radiusAuthServerIndex=70\nradiusAuthServerAddress=192.168.20.106\nradiusAuthClientServerPortNumber=1812\n' +
	'radiusAuthClientRoundTripTime=0\nradiusAuthClientAccessRequests=10\nradiusAuthClientAccessRetransmissions=0\n' +
	'radiusAuthClientAccessAccepts=1\nradiusAuthClientAccessRejects=0\nradiusAuthClientAccessChallenges=9\n' +
	'radiusAuthClientMalformedAccessResponses=0\nradiusAuthClientBadAuthenticators=0\nradiusAuthClientPendingRequests=0\n' +
	'radiusAuthClientTimeouts=0\nradiusAuthClientUnknownTypes=0\nradiusAuthClientPacketsDropped=0\n' +
	'radiusAccServerIndex=71\nradiusAccServerAddress=192.168.20.106\nradiusAccClientRequests=3\n';
printf('mib %J\n', radius.mib(text));
printf('mib of a passphrase network %J\n', radius.mib('dot11RSNAOptionImplemented=TRUE\n'));

// The verdict, at 1000 seconds, asked every 30, silent after 3 misses, a
// timed-out sign-in held for 600.
const C = { every: 30, misses: 3, hold: 600 };
function says(what, s) {
	printf('%s: %s\n', what, radius.verdict(1000, s, C));
}
says('not asked yet', { misses: 0, ever: false });
says('asked twice, no answer', { misses: 2, ever: false });
says('answered 10 s ago', { answered: 990, misses: 0, ever: true });
says('answered 100 s ago, two misses since', { answered: 900, misses: 2, ever: true });
says('answered 130 s ago, three misses since', { answered: 870, misses: 3, ever: true });
says('never answers status, no sign-ins', { misses: 9, ever: false });
says('never answers status, a sign-in answered 20 s ago', { signed: 980, misses: 9, ever: false });
says('never answers status, a sign-in answered 200 s ago', { signed: 800, misses: 9, ever: false });
says('never answers status, a sign-in timed out 30 s ago, the last answered 200 s ago', { signed: 800, timed_out: 970, misses: 9, ever: false });
says('never answers status, a sign-in timed out 700 s ago', { signed: 100, timed_out: 300, misses: 9, ever: false });
says('never answers status, a sign-in timed out and was answered in the same read, 200 s ago', { signed: 800, timed_out: 800, misses: 9, ever: false });
says('answers status, and a sign-in timed out 30 s ago', { answered: 990, timed_out: 970, misses: 0, ever: true });
says('was answering, silent, and a sign-in is answered now', { answered: 500, signed: 995, misses: 12, ever: true });
