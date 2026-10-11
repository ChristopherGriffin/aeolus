// RADIUS, as the prober asks it (0114): a Status-Server request (RFC 5997)
// and how its answer reads; and hostapd's own count of what the server
// answered its clients' sign-ins with. No I/O here: the prober sends and
// receives.

'use strict';

// MD5 is from ucode's digest module, which the agent needs too. Loaded this
// way, a prober on an AP without it still starts: able() says there is
// none, and the prober asks no server.
let md5 = null;
try {
	md5 = require('digest').md5;
} catch (e) {
}

function able() {
	return md5 != null;
}

function bin(h) {
	let out = '';
	for (let i = 0; i + 1 < length(h); i += 2)
		out += chr(hex(substr(h, i, 2)));
	return out;
}

function md5bin(s) {
	return bin(md5(s));
}

// hmac_md5 is HMAC-MD5 (RFC 2104), which a RADIUS Message-Authenticator is.
function hmac_md5(key, msg) {
	if (length(key) > 64)
		key = md5bin(key);
	let inner = '', outer = '';
	for (let i = 0; i < 64; i++) {
		let c = i < length(key) ? ord(key, i) : 0;
		inner += chr(c ^ 0x36);
		outer += chr(c ^ 0x5c);
	}
	return md5bin(outer + md5bin(inner + msg));
}

// status_request is a Status-Server packet: its identifier, 0 to 255, the
// 16 bytes of its Request Authenticator, and a Message-Authenticator made
// with the secret the AP shares with the server, without which a server
// drops it.
function status_request(id, authenticator, secret) {
	let head = chr(12) + chr(id & 255) + chr(0) + chr(38) + authenticator;
	let attr = chr(80) + chr(18);
	let zero = '';
	for (let i = 0; i < 16; i++)
		zero += chr(0);
	return head + attr + hmac_md5(secret, head + attr + zero);
}

// status_answer says what a packet is to a Status-Server request: 'ok', an
// Access-Accept or Accounting-Response to it whose Response Authenticator
// fits the secret; 'refused', another answer that fits, as a server that
// knows the AP but turns Status-Server down gives; 'bad', an answer that
// does not fit the secret; null, not an answer to it.
function status_answer(reply, id, authenticator, secret) {
	if (length(reply ?? '') < 20 || ord(reply, 1) != (id & 255))
		return null;
	let len = ord(reply, 2) * 256 + ord(reply, 3);
	if (len < 20 || len > length(reply))
		return null;
	if (substr(reply, 4, 16) != md5bin(substr(reply, 0, 4) + authenticator + substr(reply, 20, len - 20) + secret))
		return 'bad';
	return ord(reply, 0) in [2, 5] ? 'ok' : 'refused';
}

// mib reads hostapd's MIB, as its control socket gives it, for what its
// sign-in server answered: the server and port, and its counts since the
// network was started. hostapd lists one sign-in server it uses now; where
// there is none, null.
function mib(text) {
	let v = {};
	for (let line in split(text ?? '', '\n')) {
		let m = match(line, /^(radiusAuth[A-Za-z]+)=(.*)$/);
		// The first of each: a second server listed is a fallback's.
		if (m && !(m[1] in v))
			v[m[1]] = m[2];
	}
	if (!v.radiusAuthServerAddress)
		return null;
	let n = (k) => int(v[k] ?? '0') || 0;
	return {
		server: v.radiusAuthServerAddress, port: n('radiusAuthClientServerPortNumber'),
		requests: n('radiusAuthClientAccessRequests'), retransmissions: n('radiusAuthClientAccessRetransmissions'),
		accepts: n('radiusAuthClientAccessAccepts'), rejects: n('radiusAuthClientAccessRejects'),
		challenges: n('radiusAuthClientAccessChallenges'), timeouts: n('radiusAuthClientTimeouts'),
		bad: n('radiusAuthClientBadAuthenticators') + n('radiusAuthClientMalformedAccessResponses'),
	};
}

// verdict says how a server is doing for one network, at a time, from what
// is known of it, s: answered, when it last answered a status request;
// signed, when hostapd's count of its answers to sign-ins last grew;
// timed_out, when hostapd's count of timeouts last did (each null where
// never); misses, the status requests in a row that went unanswered; ever,
// whether it ever answered one. And from c: every, how often it is asked;
// misses, how many in a row make it silent; hold, how long a sign-in that
// timed out, with none answered since, keeps silent a server that takes no
// status requests.
//
// up: it answered a status request or a sign-in within the time of c.misses
// asks, or it answers status requests and has not missed that many. silent:
// it missed that many, and either it used to answer, or a sign-in timed out
// within c.hold and none was answered since. unverified: it never answered
// a status request, and no sign-in says anything. unknown: not asked that
// many times yet.
function verdict(at, s, c) {
	let answered = max(s.answered ?? -1, s.signed ?? -1);
	if (answered >= 0 && at - answered <= c.every * c.misses + 5)
		return 'up';
	let missed = s.misses >= c.misses;
	let timed_out = s.timed_out != null && s.timed_out > answered && at - s.timed_out <= c.hold;
	if (missed && (s.ever || timed_out))
		return 'silent';
	return s.ever ? 'up' : missed ? 'unverified' : 'unknown';
}

export { able, hmac_md5, status_request, status_answer, mib, verdict };
