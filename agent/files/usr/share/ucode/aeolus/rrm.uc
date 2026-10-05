// Radio resource management's pure parts (0073), so they are tested
// anywhere: the element an AP's beacons carry to say it is an Aeolus AP and
// where to reach it; HMAC-SHA256, which signs the hellos neighbours
// exchange; the hellos themselves; which APs become neighbours; the
// channels a radio listens on for them; how each channel is rated; and
// when and where a radio moves.

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
const LASTING = 0.1;          // how much one visit moves a channel's lasting rating
const NOW = 0.5;              // and its rating now
const SUDDEN = 0.5;           // a radio's own channel kept busier than this by others is interference
const NEAR = 6;               // dB further away a shared channel's nearest user must be, to move to it
const WINDOW = '02:00-05:00'; // when planned moves may happen, unless the policy says
const MARGIN = 20;            // rating points a channel must beat the radio's own by, unless the policy says

// The 5 GHz channels taken up together at each width, by the lowest and
// highest 20 MHz channel, as internal/radio has them.
const BLOCKS5 = {
	'40': [[36, 40], [44, 48], [52, 56], [60, 64], [100, 104], [108, 112], [116, 120], [124, 128], [132, 136], [140, 144], [149, 153], [157, 161]],
	'80': [[36, 48], [52, 64], [100, 112], [116, 128], [132, 144], [149, 161]],
	'160': [[36, 64], [100, 128]],
};

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
// the channels automatic channels go to, its set (0075) or else 1, 6 and 11
// (0045); on 5 GHz, every 20 MHz channel, those shared with radar (DFS) only
// where the radio may use them (0071); and its own channel, wherever it is.
function visits(band, own, dfs, set) {
	let ch = [];
	if (band == '2g')
		ch = length(set ?? []) ? sort([...set], (a, b) => a - b) : [1, 6, 11];
	else if (band == '5g')
		ch = [36, 40, 44, 48, ...(dfs ? [52, 56, 60, 64, 100, 104, 108, 112, 116, 120, 124, 128, 132, 136, 140, 144] : []), 149, 153, 157, 161, 165];
	let out = map(ch, c => freq(band, c));
	let mine = freq(band, own);
	if (own && index(out, mine) < 0)
		push(out, mine);
	return out;
}

// weight is how much another network counts against the channel it is
// heard on: fully at -55 dBm or stronger, not at all at -95 or weaker.
// (ucode divides whole numbers to a whole number, so 40.0.)
function weight(signal) {
	if (!(type(signal) in ['int', 'double']))
		return 0;
	return min(1, max(0, (signal + 95) / 40.0));
}

// cost rates one visit to a channel: lower is better. busy is the share of
// the visit others kept it busy, 0 to 1; noise its noise floor in dBm, where
// the driver says; networks the signals of the other networks heard there.
// Busy counts most: a channel others use all the time costs 100. Each dB of
// noise above -95 adds 2, and each strong network 10.
function cost(busy, noise, networks) {
	let c = 100 * min(1, max(0, busy ?? 0));
	if (type(noise) in ['int', 'double'])
		c += 2 * max(0, noise + 95);
	for (let s in networks ?? [])
		c += 10 * weight(s);
	return c;
}

// blend folds a visit's cost into a rating at rate a: LASTING for the rating
// a channel earns over many visits, NOW for what it is like now.
function blend(old, sample, a) {
	return old == null ? sample : old * (1 - a) + sample * a;
}

// covers lists the 20 MHz channels a radio on channel takes up at width.
function covers(band, channel, width) {
	if (band != '5g' || !width || width <= 20)
		return [channel];
	for (let b in BLOCKS5['' + width] ?? [])
		if (channel >= b[0] && channel <= b[1]) {
			let out = [];
			for (let c = b[0]; c <= b[1]; c += 4)
				push(out, c);
			return out;
		}
	return [channel];
}

// nearest is how strongly a channel's nearest user is heard: of those
// blotting it out, by signal (close is { <ap>: dBm }, an AP not in it
// counted as near), or -200 where no one uses it.
function nearest(r, close) {
	let near = -200;
	for (let ap in r?.blotted_by ?? [])
		near = max(near, close?.[ap] ?? 0);
	return near;
}

// best_of chooses a band's channel from its ratings ([{ channel, cost,
// blotted_by }]): the best rated no neighbour uses; or, where neighbours
// use them all, as they can 2.4 GHz's three, the one whose nearest user is
// furthest away. Of those alike, the best rated.
function best_of(rates, close) {
	let best = null, key = null;
	for (let r in rates ?? []) {
		let k = [length(r.blotted_by ?? []) ? 1 : 0, nearest(r, close), r.cost];
		if (!best || k[0] < key[0] || (k[0] == key[0] && (k[1] < key[1] || (k[1] == key[1] && k[2] < key[2])))) {
			best = r;
			key = k;
		}
	}
	return best;
}

// pick is the channel best_of chooses.
function pick(rates, close) {
	return best_of(rates, close)?.channel;
}

// radar says whether a 5 GHz channel is one shared with radar (DFS).
function radar(band, channel) {
	return band == '5g' && channel >= 52 && channel <= 144;
}

// blocks are what a radio at width may move to, from its band's ratings
// ([{ channel, cost, now, blotted_by }]): on 2.4 GHz, or at 20 MHz, each
// channel; on 5 GHz at 40 MHz or more, each block whose channels are all
// rated, rated as its worst, blotted out by whoever uses any of them, and
// entered on its best-rated channel. Each lists its channels as members.
function blocks(list, band, width) {
	if (band != '5g' || !width || width <= 20)
		return map(list ?? [], x => ({ channel: x.channel, cost: x.cost, now: x.now ?? x.cost, blotted_by: x.blotted_by ?? [], members: [x.channel] }));
	let by = {};
	for (let x in list ?? [])
		by['' + x.channel] = x;
	let out = [];
	for (let b in BLOCKS5['' + width] ?? []) {
		let members = [], all = true;
		for (let c = b[0]; c <= b[1]; c += 4) {
			push(members, c);
			if (!by['' + c])
				all = false;
		}
		if (!all)
			continue;
		let entry = null, cost = 0, now = 0, blot = [];
		for (let c in members) {
			let x = by['' + c];
			cost = max(cost, x.cost);
			now = max(now, x.now ?? x.cost);
			for (let ap in x.blotted_by ?? [])
				push(blot, ap);
			if (!entry || x.cost < entry.cost)
				entry = x;
		}
		push(out, { channel: entry.channel, cost: cost, now: now, blotted_by: uniq(sort(blot)), members: members });
	}
	return out;
}

// reason says why a radio on block cur should move to block best, both as
// blocks gives them, or null:
//   interference: others kept cur busier than SUDDEN for a while (hot, that
//     share, else null), and best rates better than that by margin;
//   shared: a neighbour uses cur, and best is free, or its nearest user is
//     at least NEAR dB further away;
//   better: best rates better than cur by margin, and is no more shared;
//   start: the radio just started (start true), and best_of ranks best
//     above cur.
function reason(cur, best, close, margin, hot, start) {
	if (!cur || !best || index(cur.members ?? [cur.channel], best.channel) >= 0)
		return null;
	let used = length(cur.blotted_by ?? []) > 0, free = !length(best.blotted_by ?? []);
	if (type(hot) in ['int', 'double'] && hot > SUDDEN && best.cost + margin <= 100 * hot)
		return 'interference';
	if (used && (free || nearest(best, close) + NEAR <= nearest(cur, close)))
		return 'shared';
	if (best.cost + margin <= cur.cost && (free || (used && nearest(best, close) <= nearest(cur, close))))
		return 'better';
	return start && best_of([cur, best], close) == best ? 'start' : null;
}

// first says whether claim a ({ ap, cost }) goes before claim b, made at
// the same time on one band: the AP whose own channel rates worse moves
// first; where they rate alike, the one with the lower AP ID.
function first(a, b) {
	return a.cost > b.cost || (a.cost == b.cost && a.ap < b.ap);
}

// window reads a window such as '02:00-05:00' as its start and end, in
// minutes of the day, or null.
function window(s) {
	let m = match(s ?? '', /^([01][0-9]|2[0-3]):([0-5][0-9])-([01][0-9]|2[0-3]):([0-5][0-9])$/);
	return m ? [int(m[1]) * 60 + int(m[2]), int(m[3]) * 60 + int(m[4])] : null;
}

// in_window says whether minute t of the day is in window w. It may run
// past midnight; one that starts where it ends is the whole day.
function in_window(w, t) {
	if (!w)
		return false;
	if (w[0] == w[1])
		return true;
	return w[0] < w[1] ? t >= w[0] && t < w[1] : t >= w[0] || t < w[1];
}

// switch_args are hostapd's switch_chan arguments to move a radio running
// mode (its htmode, such as HE40) to channel at width, announced for count
// beacons; or null where the channel can't carry the width.
function switch_args(band, channel, width, mode, count) {
	let f = freq(band, channel);
	if (!(band in ['2g', '5g']) || !f)
		return null;
	let fam = match(mode ?? '', /^(NOHT|HT|VHT|HE|EHT)/)?.[1] ?? 'HT';
	let w = fam == 'NOHT' ? 20 : (width ?? 20);
	let a = {
		freq: f, center_freq1: f, bandwidth: w, sec_channel_offset: 0, ht: fam != 'NOHT',
		vht: band == '5g' && fam in ['VHT', 'HE', 'EHT'], he: fam in ['HE', 'EHT'], bcn_count: count,
	};
	if (w <= 20)
		return a;
	if (band == '2g') {
		if (w != 40 || channel < 1 || channel > 13)
			return null;
		a.sec_channel_offset = channel <= 7 ? 1 : -1;
		a.center_freq1 = f + 10 * a.sec_channel_offset;
		return a;
	}
	for (let b in BLOCKS5['' + w] ?? [])
		if (channel >= b[0] && channel <= b[1]) {
			a.center_freq1 = int((freq(band, b[0]) + freq(band, b[1])) / 2);
			a.sec_channel_offset = int((channel - b[0]) / 4) % 2 ? -1 : 1;
			return a;
		}
	return null;
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export {
	OUI, PORT, NEIGHBOURS, HELLO_EVERY, DEAD, SKEW, LASTING, NOW, SUDDEN, NEAR, WINDOW, MARGIN,
	hexstr, unhex, hmac, same, advert, read_advert, seal, open, fresh, choose, smooth, freq, band_of, visits,
	weight, cost, blend, covers, nearest, best_of, pick, radar, blocks, reason, first, window, in_window, switch_args
};
