// aeolus.steer: what Aeolus's steering asks of a Wi-Fi client, and what
// came of it (0138). It is the daemon's judgement and nothing else: no
// ubus, no files, no clock. aeolus-steer hears hostapd and looks at its
// clients, hands what it found here, and does what comes back; a test
// hands it the same.
//
// Three ways, each a switch of the network, so of one SSID, and each on its
// own:
//
//   suggest   a client that can use a higher band this AP has the network
//             on, and is near enough for it, is asked to move there (an
//             802.11v BSS transition request). It may say no.
//   hold      a client known to use a higher band is kept off 2.4 GHz for a
//             moment as it comes to join, so that it finds the higher band
//             first. Then 2.4 GHz is open to it again.
//   sticky    a client whose signal stays weak while the network has a
//             better place for it is asked to go, and put off for a moment
//             if it stays.
//
// Nothing here keeps a client off for longer than a few seconds, and
// nothing is asked of hostapd that it cannot undo by itself: what keeps a
// client off is hostapd's own ban, which hostapd lifts by its own timer,
// the daemon there or not. hostapd never waits for the daemon.
//
// Each asking is followed to its end and kept as one record: what was
// asked, what the client answered, and where it then was. Those are what
// Aeolus shows of whether steering works.

'use strict';

const RANK = { '2g': 0, '5g': 1, '6g': 2 };

// The signal a client must have where it is, in dBm, to be asked onto a
// higher band: that band carries less far, and 6 GHz less far still.
const NEAR = { '2g': { '5g': -65, '6g': -55 }, '5g': { '6g': -60 } };

const SUGGEST_AFTER = 10;          // seconds a client is left alone once it has joined
const SUGGEST_AGAIN = [120, 600];  // seconds before it is asked a second time, and a third; then no more
const ANSWER_WAIT = 20;            // seconds it has to move
const HOLD_MS = 2500;              // how long 2.4 GHz is kept from a client
const HOLD_EVERY = 30;             // seconds before a client is held again: its next try gets in
const HOLD_WAIT = 12;              // seconds in which it joins somewhere
const WEAK_LOOKS = 3;              // looks in a row a signal is weak before the client is asked to go
const STICKY_EVERY = 300;          // seconds before a client is asked to go again
const KICK_AFTER = 8;              // seconds it then has
// How long it is kept off the network it was put off: long enough that its
// first tries go elsewhere, and no longer. A client refused three times
// running, with nowhere else to go, gives the network up for ten seconds
// (wpa_supplicant, 2026-10-11): at five seconds it was; at two and a half
// its third try gets in.
const KICK_BAN_MS = 2500;
const RECOVERED = 3;               // dB over weak at which a client is left where it is
const BACK_WAIT = 60;              // seconds in which it may come back to where it was
const FELL_BACK = 300;             // seconds in which a client that moved may come back down
const REST = 1800;                 // seconds a client that came back is left alone
const KNOWN_FOR = 30 * 86400;      // seconds a client's bands are remembered
const KNOWN_MOST = 4096;
const EVENTS_KEPT = 200;
const EVENTS_SAID = 50;

function fresh() {
	return {
		known: {},    // each client's bands, and whether it takes a transition request
		on: {},       // where each client is on this AP now
		tries: {},    // what is asked of a client and not yet at its end, one a client
		held: {},     // when each was last held off 2.4 GHz
		asked: {},    // when each was last asked to go for a weak signal
		moved: {},    // where each last moved from, for whether it comes back
		rest: {},     // until when each is left alone
		down: {},     // until when each, sent down for a weak signal, is not asked or held up
		events: [],
		counts: {},
		wall: 0,
	};
}

// bands_of reads the bands a client says it can use from the elements of
// its association request: its Supported Operating Classes (element 59),
// by the global classes. A client need not say; one that does not is known
// only by where it has been.
function bands_of(ies) {
	let out = {}, i = 0, n = length(ies ?? '');
	while (i + 2 <= n) {
		let id = ord(ies, i), len = ord(ies, i + 1);
		if (i + 2 + len > n)
			break;
		if (id == 59)
			for (let j = 0; j < len; j++) {
				let c = ord(ies, i + 2 + j);
				if (c >= 81 && c <= 84)
					out['2g'] = true;
				else if (c >= 115 && c <= 130)
					out['5g'] = true;
				else if (c >= 131 && c <= 137)
					out['6g'] = true;
			}
		i += 2 + len;
	}
	return sort(keys(out), (a, b) => RANK[a] - RANK[b]);
}

// btm_of says whether a client takes a BSS transition request: bit 19 of
// its extended capabilities, as hostapd lists them a byte at a time.
function btm_of(ext) {
	return type(ext) == 'array' && length(ext) > 2 && (ext[2] & 0x08) != 0;
}

function siblings(view, ifname) {
	let b = view.bss?.[ifname], out = [];
	if (!b)
		return out;
	for (let name in sort(keys(view.bss)))
		if (name != ifname && view.bss[name].network == b.network)
			push(out, { ...view.bss[name], ifname: name });
	return out;
}

function learn(st, mac, band, now) {
	let k = st.known[mac] ??= { bands: {} };
	k.bands[band] = now;
	k.at = now;
}

function can(st, mac, band, now) {
	let t = st.known[mac]?.bands?.[band];
	return t != null && now - t < KNOWN_FOR;
}

function count(st, network, method, what) {
	let c = st.counts[network] ??= {};
	let m = c[method] ??= {};
	m[what] = (m[what] ?? 0) + 1;
}

function said(st, e) {
	push(st.events, e);
	while (length(st.events) > EVENTS_KEPT)
		shift(st.events);
}

// done ends an asking: counted, and kept as a record. One made on a
// client's probe alone, which may have been only a look around, is neither
// where the client then joined nowhere here.
function done(st, a, outcome, now) {
	delete st.tries[a.mac];
	if (a.quiet && outcome == 'left')
		return;
	count(st, a.network, a.method, 'asked');
	count(st, a.network, a.method, outcome);
	let e = {
		at: int(a.at + st.wall), mac: a.mac, network: a.network, method: a.method, from: a.from,
		asked: a.asked, outcome: outcome, took: int((now - a.at) * 10) / 10,
	};
	for (let k in ['to', 'answer', 'code', 'signal'])
		if (a[k] != null)
			e[k] = a[k];
	said(st, e);
	if (outcome == 'moved' && a.method != 'sticky')
		st.moved[a.mac] = { method: a.method, network: a.network, from: a.from, at: now };
	// Sent off a band where it was weak, it is not sent back up it.
	if (a.method == 'sticky' && outcome != 'recovered')
		st.down[a.mac] = now + REST;
	if (outcome == 'back')
		st.rest[a.mac] = now + REST;
}

// arrived notes a client where it now is. One that was moved up and is back
// where it came from soon after did not stay moved: said, and the client
// left alone for a while.
function arrived(st, view, mac, ifname, now) {
	let b = view.bss[ifname];
	st.on[mac] = { bss: ifname, network: b.network, band: b.band, since: now, asks: 0, last: 0, weak: 0 };
	learn(st, mac, b.band, now);
	let m = st.moved[mac];
	if (m && m.network == b.network && now - m.at < FELL_BACK && RANK[b.band] <= RANK[m.from]) {
		count(st, m.network, m.method, 'back');
		said(st, { at: int(now + st.wall), mac: mac, network: m.network, method: m.method, from: m.from, asked: 'none', outcome: 'back', took: int((now - m.at) * 10) / 10 });
		st.rest[mac] = now + REST;
		delete st.moved[mac];
	}
	return st.on[mac];
}

// judge says what came of an asking, or nothing while it is too soon to say.
function judge(st, view, a, now) {
	let o = st.on[a.mac], here = o ? view.bss?.[o.bss] : null;
	let same = o != null && o.bss == a.bss;
	let within = here != null && here.network == a.network;
	switch (a.method) {
	case 'suggest':
		if (within && !same && RANK[here.band] > RANK[a.from])
			return 'moved';
		if (same && a.answer == 'refused')
			return 'refused';
		if (now < a.until)
			return null;
		return same ? 'stayed' : 'left';
	case 'hold':
		if (within && RANK[here.band] > RANK[a.from])
			return 'moved';
		if (same)
			return 'joined';
		return now < a.until ? null : 'left';
	case 'sticky':
		// Put off, it is back where it was, elsewhere here, or gone.
		if (a.asked == 'kick') {
			if (same)
				return 'back';
			if (within)
				return 'moved';
			return now < a.until ? null : 'left';
		}
		if (within && !same)
			return 'moved';
		// Gone: a few seconds for it to show elsewhere here.
		if (!o)
			return now < a.gone + 5 ? null : 'left';
		return null;
	}
	return null;
}

// candidates is the transition request's list: each a neighbour report as
// hostapd has it, with how much it is preferred, the first the most.
function candidates(list) {
	return map(list, (nr, i) => nr + sprintf('0301%02x', 255 - i));
}

// hold keeps a client off 2.4 GHz for a moment: one known to use a higher
// band this AP has the network on, near enough for it, that is not on this
// AP now, and was not held a moment ago. The higher band must be there: a
// network hostapd has no neighbour report for is not up, as on a radio that
// listens for radar, and 2.4 GHz is then all the client has.
function hold(st, cfg, view, now, ifname, type, data, mac) {
	let b = view.bss[ifname], net = cfg.networks?.[b.network];
	if (!net?.hold || b.band != '2g' || st.on[mac] || st.tries[mac])
		return [];
	if (now < (st.rest[mac] ?? 0) || now < (st.down[mac] ?? 0) || now - (st.held[mac] ?? -HOLD_EVERY) < HOLD_EVERY)
		return [];
	let up = filter(siblings(view, ifname), s => RANK[s.band] > 0 && s.nr && can(st, mac, s.band, now) &&
		data.signal != null && data.signal >= NEAR['2g'][s.band]);
	if (!length(up))
		return [];
	st.held[mac] = now;
	st.tries[mac] = {
		method: 'hold', at: now, mac: mac, network: b.network, bss: ifname, from: '2g',
		to: map(up, s => s.band), asked: 'held', signal: data.signal, until: now + HOLD_WAIT, quiet: type == 'probe',
	};
	return [{ do: 'ban', bss: ifname, mac: mac, ms: HOLD_MS, quiet: type == 'probe' }];
}

// heard takes one of hostapd's notifications for a network that is steered.
function heard(st, cfg, view, now, ifname, type, data) {
	let b = view.bss?.[ifname], mac = lc(data?.address ?? '');
	if (!b || !cfg.networks?.[b.network] || !match(mac, /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/))
		return [];
	switch (type) {
	case 'sta-authorized': {
		arrived(st, view, mac, ifname, now);
		let a = st.tries[mac], r = a ? judge(st, view, a, now) : null;
		if (r)
			done(st, a, r, now);
		break;
	}
	case 'disassoc':
	case 'deauth': {
		if (st.on[mac]?.bss == ifname)
			delete st.on[mac];
		let a = st.tries[mac];
		if (a && a.bss == ifname && a.gone == null)
			a.gone = now;
		break;
	}
	case 'bss-transition-response': {
		let a = st.tries[mac];
		if (a && a.asked == 'btm' && a.bss == ifname && a.answer == null) {
			a.code = +(data['status-code'] ?? 0);
			a.answer = a.code == 0 ? 'accepted' : 'refused';
			let r = judge(st, view, a, now);
			if (r)
				done(st, a, r, now);
		}
		break;
	}
	case 'probe':
	case 'auth':
		return hold(st, cfg, view, now, ifname, type, data, mac);
	}
	return [];
}

// tick looks at who is where and how well each is heard, ends what is at
// its end, and asks what is now due. view is the AP's steered networks as
// they now are: bss, each one's network, band and own neighbour report, and
// those of the same SSID elsewhere; clients, each one's signal and whether
// it takes a transition request.
function tick(st, cfg, view, now, wall) {
	let acts = [];
	st.wall = wall - now;

	// Who is where. A client on two at once is moving: it is where it was
	// last heard to be let on.
	let places = {};
	for (let ifname in sort(keys(view.clients ?? {})))
		if (view.bss?.[ifname])
			for (let mac in keys(view.clients[ifname]))
				push(places[mac] ??= [], ifname);
	for (let mac in sort(keys(places)))
		if (!st.on[mac] || index(places[mac], st.on[mac].bss) < 0)
			arrived(st, view, mac, places[mac][0], now);
	for (let mac in keys(st.on))
		if (!places[mac]) {
			let a = st.tries[mac];
			if (a && a.bss == st.on[mac].bss && a.gone == null)
				a.gone = now;
			delete st.on[mac];
		}

	for (let mac in sort(keys(st.on))) {
		let o = st.on[mac], c = view.clients[o.bss][mac], net = cfg.networks?.[o.network];
		learn(st, mac, o.band, now);
		for (let band in c.bands ?? [])
			if (RANK[band] != null)
				learn(st, mac, band, now);
		if (c.btm != null)
			st.known[mac].btm = !!c.btm;
		o.weak = net?.sticky && c.signal != null && c.signal < net.weak ? o.weak + 1 : 0;
	}

	// What is at its end; and a weak client that was asked to go and stayed.
	for (let mac in sort(keys(st.tries))) {
		let a = st.tries[mac], r = judge(st, view, a, now);
		if (r) {
			done(st, a, r, now);
			continue;
		}
		if (a.method != 'sticky' || a.asked == 'kick' || now < a.kick_at)
			continue;
		let o = st.on[mac], c = o ? view.clients[o.bss]?.[mac] : null;
		if (!o || o.bss != a.bss || !c)
			continue;
		if (c.signal != null && c.signal >= (cfg.networks?.[a.network]?.weak ?? -75) + RECOVERED) {
			done(st, a, 'recovered', now);
			continue;
		}
		push(acts, { do: 'kick', bss: a.bss, mac: mac, ms: KICK_BAN_MS });
		a.asked = 'kick';
		a.until = now + BACK_WAIT;
		delete st.on[mac];
	}

	for (let mac in sort(keys(st.on))) {
		let o = st.on[mac], b = view.bss[o.bss], c = view.clients[o.bss][mac], net = cfg.networks?.[o.network];
		if (!net || st.tries[mac] || now < (st.rest[mac] ?? 0))
			continue;
		let sibs = siblings(view, o.bss);

		// Weak, and stays so: where the network has a better place for it,
		// a lower band here or a network of its name on another AP.
		if (net.sticky && o.weak >= WEAK_LOOKS && now - (st.asked[mac] ?? -STICKY_EVERY) >= STICKY_EVERY) {
			let down = filter(sibs, s => RANK[s.band] < RANK[o.band] && s.nr);
			let list = [...map(down, s => s.nr), ...(b.elsewhere ?? [])];
			if (length(list)) {
				st.asked[mac] = now;
				let a = st.tries[mac] = {
					method: 'sticky', at: now, mac: mac, network: o.network, bss: o.bss, from: o.band,
					asked: 'none', signal: c.signal, kick_at: now, until: now + KICK_AFTER + BACK_WAIT,
				};
				// One that takes no request cannot be told: it is put off.
				// One that does is told it is to go, with no hour named:
				// given one, hostapd puts the client off itself when it
				// comes, and the daemon then a second time.
				if (c.btm) {
					a.asked = 'btm';
					a.kick_at = now + KICK_AFTER;
					push(acts, { do: 'btm', bss: o.bss, mac: mac, neighbors: candidates(list), imminent: true, timer: 0, validity: 255, abridged: false });
				} else {
					a.asked = 'kick';
					a.until = now + BACK_WAIT;
					push(acts, { do: 'kick', bss: o.bss, mac: mac, ms: KICK_BAN_MS });
					delete st.on[mac];
				}
				continue;
			}
		}

		// Near, on a band below one it can use: asked up. Thrice at most
		// while it stays joined, and further apart each time.
		if (!net.suggest || !c.btm || c.signal == null || o.asks > length(SUGGEST_AGAIN) || now < (st.down[mac] ?? 0))
			continue;
		if (now - o.since < SUGGEST_AFTER || (o.asks > 0 && now - o.last < SUGGEST_AGAIN[o.asks - 1]))
			continue;
		// Near at this look and at the last: one reading is not a place.
		let near = (band) => c.signal >= NEAR[o.band][band] && o.heard != null && o.heard >= NEAR[o.band][band];
		let up = filter(sibs, s => RANK[s.band] > RANK[o.band] && s.nr && can(st, mac, s.band, now) && near(s.band));
		if (!length(up))
			continue;
		up = sort(up, (x, y) => RANK[y.band] - RANK[x.band]);
		o.asks++;
		o.last = now;
		st.tries[mac] = {
			method: 'suggest', at: now, mac: mac, network: o.network, bss: o.bss, from: o.band,
			to: map(up, s => s.band), asked: 'btm', signal: c.signal, until: now + ANSWER_WAIT,
		};
		push(acts, { do: 'btm', bss: o.bss, mac: mac, neighbors: candidates(map(up, s => s.nr)), imminent: false, timer: 0, validity: 100, abridged: true });
	}

	for (let mac, o in st.on)
		o.heard = view.clients[o.bss]?.[mac]?.signal;

	// What is remembered is let go when it no longer bears on anything.
	for (let mac, t in st.held)
		if (now - t >= HOLD_EVERY)
			delete st.held[mac];
	for (let mac, t in st.asked)
		if (now - t >= STICKY_EVERY)
			delete st.asked[mac];
	for (let mac, t in st.rest)
		if (now >= t)
			delete st.rest[mac];
	for (let mac, t in st.down)
		if (now >= t)
			delete st.down[mac];
	for (let mac, m in st.moved)
		if (now - m.at >= FELL_BACK)
			delete st.moved[mac];
	let known = keys(st.known);
	for (let mac in known)
		if (now - (st.known[mac].at ?? 0) >= KNOWN_FOR)
			delete st.known[mac];
	if (length(st.known) > KNOWN_MOST) {
		known = sort(keys(st.known), (x, y) => (st.known[x].at ?? 0) - (st.known[y].at ?? 0));
		for (let mac in slice(known, 0, length(known) - KNOWN_MOST))
			delete st.known[mac];
	}
	return acts;
}

// summary is what the agent reports: each steered network, what is on for
// it and on which bands, the count of what was asked and what came of it
// since the daemon started, and the last of the records.
function summary(st, cfg, view, wall, since) {
	let nets = [];
	for (let id in sort(keys(cfg.networks ?? {}))) {
		let n = cfg.networks[id], bands = {};
		for (let name, b in view.bss ?? {})
			if (b.network == id)
				bands[b.band] = true;
		let e = {
			network: id, suggest: !!n.suggest, hold: !!n.hold, sticky: !!n.sticky,
			bands: sort(keys(bands), (a, b) => RANK[a] - RANK[b]), counts: st.counts[id] ?? {},
		};
		if (n.sticky)
			e.weak = n.weak;
		push(nets, e);
	}
	return { at: wall, since: since, networks: nets, events: slice(st.events, -EVENTS_SAID), known: length(st.known) };
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export { RANK, HOLD_MS, KICK_BAN_MS, fresh, bands_of, btm_of, heard, tick, summary };
