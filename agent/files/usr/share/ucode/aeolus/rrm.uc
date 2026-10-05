// Radio resource management's pure parts (0073), so they are tested
// anywhere: the element an AP's beacons carry to say it is an Aeolus AP and
// where to reach it; HMAC-SHA256, which signs the hellos neighbours
// exchange; the hellos themselves; which APs become neighbours; and the
// channels a radio listens on for them.

'use strict';

import { sha256 } from 'digest';

// The element's OUI, from the locally assigned range, which no vendor's OUI
// or CID is in, so it is never taken for another's.
const OUI = '\x02\xae\x01';
const ADVERT = 1;             // the element's type: an AP's advert
const VERSION = 1;            // of the advert and the hellos
const PORT = 16730;           // where neighbours send hellos, on their management addresses
const NEIGHBOURS = 3;         // on each band, the APs heard most strongly
const HELLO_EVERY = 10;       // seconds between hellos
const DEAD = 40;              // seconds without a hello after which a neighbour is down
const SKEW = 60;              // seconds a hello's time may be off from this AP's clock

function hexstr(s) {
	let o = '';
	for (let i = 0; i < length(s); i++)
		o += sprintf('%02x', ord(s, i));
	return o;
}

// unhex reads hex digits as bytes, or returns null.
function unhex(h) {
	if (type(h) != 'string' || length(h) % 2 || !match(h, /^[0-9a-fA-F]*$/))
		return null;
	let o = '';
	for (let i = 0; i < length(h); i += 2)
		o += chr(hex(substr(h, i, 2)));
	return o;
}

// hmac is HMAC-SHA256 (RFC 2104) of msg under key, both byte strings, in
// hex.
function hmac(key, msg) {
	if (length(key) > 64)
		key = unhex(sha256(key));
	let ipad = '', opad = '';
	for (let i = 0; i < 64; i++) {
		let b = i < length(key) ? ord(key, i) : 0;
		ipad += chr(b ^ 0x36);
		opad += chr(b ^ 0x5c);
	}
	return sha256(opad + unhex(sha256(ipad + msg)));
}

// same compares two strings in a time that doesn't depend on where they
// differ, as a signature must be.
function same(a, b) {
	if (type(a) != 'string' || type(b) != 'string' || length(a) != length(b))
		return false;
	let d = 0;
	for (let i = 0; i < length(a); i++)
		d |= ord(a, i) ^ ord(b, i);
	return d == 0;
}

// advert is the vendor element, in hex as hostapd's vendor_elements takes
// it, that says this is Aeolus AP ap (ap-<12 hex digits>), reached at its
// IPv4 management address on port. Null if any of them is not one.
function advert(ap, address, port) {
	let id = substr(ap ?? '', 0, 3) == 'ap-' ? unhex(substr(ap, 3)) : null;
	let ip = match(address ?? '', /^([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})$/);
	if (id == null || length(id) != 6 || !ip || +ip[1] > 255 || +ip[2] > 255 || +ip[3] > 255 || +ip[4] > 255 ||
	    type(port) != 'int' || port < 1 || port > 65535)
		return null;
	let data = OUI + chr(ADVERT, VERSION) + id + chr(+ip[1], +ip[2], +ip[3], +ip[4], port >> 8, port & 0xff);
	return 'dd' + sprintf('%02x', length(data)) + hexstr(data);
}

// read_advert reads an element's body, as nl80211 gives it (its OUI first),
// as another AP's advert: { ap, address, port }, or null if it is not one.
// A later version may add to its end.
function read_advert(data) {
	if (type(data) != 'string' || length(data) < 17 || substr(data, 0, 3) != OUI || ord(data, 3) != ADVERT || ord(data, 4) < VERSION)
		return null;
	return {
		ap: 'ap-' + hexstr(substr(data, 5, 6)),
		address: sprintf('%d.%d.%d.%d', ord(data, 11), ord(data, 12), ord(data, 13), ord(data, 14)),
		port: ord(data, 15) << 8 | ord(data, 16),
	};
}

// seal signs a message for neighbours: the HMAC of its JSON, then the JSON.
function seal(key, msg) {
	let body = sprintf('%J', msg);
	return hmac(key, body) + body;
}

// open checks a packet's signature and reads it: a message of this version
// from an AP, or null.
function open(key, pkt) {
	if (type(pkt) != 'string' || length(pkt) < 66 || !same(hmac(key, substr(pkt, 64)), substr(pkt, 0, 64)))
		return null;
	let m = null;
	try {
		m = json(substr(pkt, 64));
	} catch (e) {
		return null;
	}
	return type(m) == 'object' && m.v == VERSION && type(m.ap) == 'string' && match(m.ap, /^ap-[0-9a-f]{12}$/) ? m : null;
}

// fresh says whether a hello is new: its time near this AP's clock, and
// after the last one from the same AP, so one sent again is refused.
function fresh(last, msg, wall) {
	if (type(msg?.at) != 'int' || type(msg.seq) != 'int' || msg.at < wall - SKEW || msg.at > wall + SKEW)
		return false;
	return !last || msg.at > last.at || (msg.at == last.at && msg.seq > last.seq);
}

// choose picks each band's neighbours: the n APs heard most strongly there,
// by name where they are heard alike. heard is { <ap>: { <band>: signal } }.
function choose(heard, n) {
	let by = {};
	for (let ap, bands in heard)
		for (let band, signal in bands)
			push(by[band] ??= [], [ap, signal]);
	let out = {};
	for (let band, list in by)
		out[band] = map(slice(sort(list, (a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : 1)), 0, n), x => x[0]);
	return out;
}

// smooth folds a new signal reading into the running one: a few readings
// move it, one odd one barely does.
function smooth(old, sample) {
	return old == null ? sample : old * 0.75 + sample * 0.25;
}

function freq(band, ch) {
	if (band == '2g')
		return ch == 14 ? 2484 : 2407 + 5 * ch;
	if (band == '5g')
		return 5000 + 5 * ch;
	return band == '6g' ? 5950 + 5 * ch : 0;
}

function band_of(f) {
	if (f >= 2412 && f <= 2484)
		return '2g';
	if (f >= 5160 && f <= 5885)
		return '5g';
	return f >= 5955 && f <= 7115 ? '6g' : null;
}

// visits are the frequencies a radio listens on for other APs: on 2.4 GHz,
// channels 1, 6 and 11, where automatic channels go (0045); on 5 GHz,
// every 20 MHz channel, those shared with radar (DFS) only where the radio
// may use them (0071); and its own channel, wherever it is.
function visits(band, own, dfs) {
	let ch = [];
	if (band == '2g')
		ch = [1, 6, 11];
	else if (band == '5g')
		ch = [36, 40, 44, 48, ...(dfs ? [52, 56, 60, 64, 100, 104, 108, 112, 116, 120, 124, 128, 132, 136, 140, 144] : []), 149, 153, 157, 161, 165];
	let out = map(ch, c => freq(band, c));
	let mine = freq(band, own);
	if (own && index(out, mine) < 0)
		push(out, mine);
	return out;
}

// radar says whether a 5 GHz frequency is a DFS channel's, where a radio may
// only listen.
function radar(f) {
	return f >= 5260 && f <= 5720;
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export {
	OUI, PORT, NEIGHBOURS, HELLO_EVERY, DEAD, SKEW,
	hexstr, unhex, hmac, same, advert, read_advert, seal, open, fresh, choose, smooth, freq, band_of, visits, radar
};
