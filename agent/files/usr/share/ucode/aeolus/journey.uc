// A Wi-Fi client coming online, step by step (0118): what hostapd's log
// says of it, what its first frames show, and the record of the attempt
// made from them. No I/O here: aeolus-journey listens and reads.
//
// An attempt runs from a client's authentication to its first traffic, or
// to where it stopped. Its stages, in order: auth and assoc (802.11),
// signin (802.1X, where the network has it), key (the handshake that makes
// its keys), dhcp, gateway (ARP), dns, and internet (a first connection
// answered).

'use strict';

const STAGES = ['auth', 'assoc', 'signin', 'key', 'dhcp', 'gateway', 'dns', 'internet'];
const EVENTS = 48;       // steps kept of one attempt
const LOOKUPS = 4;       // DNS lookups kept of one attempt
const CONNECTIONS = 2;   // first connections kept
const IDLE = 20;         // seconds without a step before a client not yet let on is given up
const LET_ON = 120;      // seconds at most to be let on
const WATCH = 30;        // seconds a client is watched after it is let on
const LINGER = 4;        // seconds more after it is online, for its first lookups and connection
const GAVE_UP_DHCP = 8;  // seconds without an answer to DHCP before a client's leaving is a failure
const GAVE_UP_GATEWAY = 4;   // and without one from its gateway

function get16(s, at) {
	return ord(s, at) << 8 | ord(s, at + 1);
}

function get32(s, at) {
	return get16(s, at) * 65536 + get16(s, at + 2);
}

function mac_text(s, at) {
	return join(':', map([0, 1, 2, 3, 4, 5], i => sprintf('%02x', ord(s, at + i))));
}

function ip4_text(s, at) {
	return join('.', map([0, 1, 2, 3], i => ord(s, at + i)));
}

// ip6_text is an IPv6 address's groups, written out in full.
function ip6_text(s, at) {
	return join(':', map([0, 2, 4, 6, 8, 10, 12, 14], i => sprintf('%x', get16(s, at + i))));
}

// text keeps what is printable of a string, at most n characters.
function text(s, n) {
	return substr(replace(s ?? '', /[^ -~]/g, '?'), 0, n);
}

// assemble makes a kernel packet filter from one written with named jumps.
// Each step is [code, where to if true, where to if false, k]; a jump is a
// label's name, or 0 for the next step. A string alone is a label.
function assemble(steps) {
	let at = {}, n = 0;
	for (let s in steps) {
		if (type(s) == 'string')
			at[s] = n;
		else
			n++;
	}
	let out = [], i = 0;
	for (let s in steps) {
		if (type(s) == 'string')
			continue;
		let to = l => type(l) == 'string' ? at[l] - i - 1 : 0;
		push(out, [s[0], to(s[1]), to(s[2]), s[3]]);
		i++;
	}
	return out;
}

// capture is the kernel filter that keeps, of what crosses a Wi-Fi
// interface, what tells how a client comes online: ARP from a client, and
// an ARP answer going out to one; DHCP both ways; and, wide, while a client
// is watched, DNS both ways (IPv4 and IPv6, over UDP) and the frames that
// open a TCP connection. Narrow, when no client is watched, it lets the AP's
// traffic be. (Not filter: a function of that name here would hide ucode's
// own, which record uses.)
function capture(wide) {
	return assemble([
		[0x28, 0, 0, 12],               // A = EtherType
		[0x15, 'arp', 0, 0x0806],
		[0x15, 'ip4', 0, 0x0800],
		[0x15, wide ? 'ip6' : 'drop', 'drop', 0x86dd],
		'arp',
		[0x20, 0, 0, 0xfffff004],       // A = the packet's direction
		[0x15, 0, 'keep', 4],           // coming in from a client: keep
		[0x28, 0, 0, 20],               // A = ARP's operation
		[0x15, 'keep', 'drop', 2],      // an answer going out: keep
		'ip4',
		[0x30, 0, 0, 23],               // A = the protocol
		[0x15, 'udp4', 0, 17],
		[0x15, wide ? 'tcp4' : 'drop', 'drop', 6],
		'udp4',
		[0x28, 0, 0, 20],               // A = flags and fragment offset
		[0x45, 'drop', 0, 0x1fff],      // a later fragment: drop
		[0xb1, 0, 0, 14],               // X = the IP header's length
		[0x48, 0, 0, 14],               // A = the source port
		[0x15, 'keep', 0, 67],
		[0x15, 'keep', 0, 68],
		[0x15, wide ? 'keep' : 0, 0, 53],
		[0x48, 0, 0, 16],               // A = the destination port
		[0x15, 'keep', 0, 67],
		[0x15, 'keep', 0, 68],
		[0x15, wide ? 'keep' : 'drop', 'drop', 53],
		'tcp4',
		[0x28, 0, 0, 20],
		[0x45, 'drop', 0, 0x1fff],
		[0xb1, 0, 0, 14],
		[0x50, 0, 0, 27],               // A = TCP's flags
		[0x45, 'keep', 'drop', 0x02],   // a SYN: keep
		'ip6',
		[0x30, 0, 0, 20],               // A = the next header
		[0x15, 'udp6', 0, 17],
		[0x15, 'tcp6', 'drop', 6],
		'udp6',
		[0x28, 0, 0, 54],               // A = the source port
		[0x15, 'keep', 0, 53],
		[0x28, 0, 0, 56],               // A = the destination port
		[0x15, 'keep', 'drop', 53],
		'tcp6',
		[0x30, 0, 0, 67],               // A = TCP's flags
		[0x45, 'keep', 'drop', 0x02],
		'keep',
		[0x06, 0, 0, 0x40000],
		'drop',
		[0x06, 0, 0, 0],
	]);
}

// What hostapd's events mean for a client coming online: the stage, what
// to say, and whether it went well (true), badly (false) or neither.
const EVENT = {
	'AP-STA-CONNECTED': ['key', 'let on to the network', true],
	'AP-STA-POSSIBLE-PSK-MISMATCH': ['key', 'wrong passphrase', false],
	'EAPOL-4WAY-HS-COMPLETED': ['key', 'key handshake completed', true],
	'CTRL-EVENT-EAP-STARTED': ['signin', 'sign-in started', null],
	'CTRL-EVENT-EAP-SUCCESS2': ['signin', 'the server accepted the sign-in', true],
	'CTRL-EVENT-EAP-FAILURE2': ['signin', 'the server refused the sign-in', false],
	'CTRL-EVENT-EAP-TIMEOUT-FAILURE2': ['signin', 'the sign-in timed out', false],
	'CTRL-EVENT-EAP-RETRANSMIT2': ['signin', 'a sign-in message was sent again', null],
	'CTRL-EVENT-SAE-UNKNOWN-PASSWORD-IDENTIFIER': ['key', 'unknown password identifier', false],
};

// log_event reads one line of the system log for what hostapd says of a
// client: { bss, mac, stage, what, ok } and, where it is the client being
// let on, on; where it is the client leaving, end. A line that names a
// client and is not known is kept as it reads, with no stage: it may be the
// one that explains a failure. Null for any other line.
function log_event(line) {
	let m = match(line ?? '', /hostapd: ([A-Za-z0-9_.-]+): (.*)$/);
	if (!m)
		return null;
	let bss = m[1], rest = m[2];
	let s = match(rest, /^STA ([0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}) ([A-Za-z0-9. ]+): (.*)$/);
	if (s) {
		let e = { bss: bss, mac: s[1], stage: null, what: text(s[3], 120), ok: null }, sub = s[2], t = s[3];
		if (sub == 'IEEE 802.11') {
			if (t == 'authenticated')
				return { ...e, stage: 'auth', ok: true };
			if (match(t, /^associated/))
				return { ...e, stage: 'assoc', what: 'associated', ok: true };
			if (match(t, /^(disassociated|deauthenticated)/))
				return { ...e, stage: 'end', end: true };
			if (match(t, /did not acknowledge authentication response/))
				return { ...e, stage: 'auth', what: 'did not acknowledge the AP\'s answer', ok: false };
			if (match(t, /did not acknowledge association response/))
				return { ...e, stage: 'assoc', what: 'did not acknowledge the AP\'s answer', ok: false };
			return e;
		}
		if (sub == 'WPA') {
			if (match(t, /^pairwise key handshake completed/))
				return { ...e, stage: 'key', what: 'key handshake completed', ok: true };
			if (match(t, /^group key handshake completed/))
				return null;
			return { ...e, stage: 'key' };
		}
		if (sub == 'IEEE 802.1X') {
			let a = match(t, /^authenticated - EAP type: [0-9]+ \(([A-Za-z0-9_-]+)\)/);
			if (a)
				return { ...e, stage: 'signin', what: 'signed in with ' + a[1], ok: true };
			if (match(t, /^authentication (failed|timeout)/))
				return { ...e, stage: 'signin', ok: false };
			return { ...e, stage: 'signin' };
		}
		if (sub == 'RADIUS')
			return match(t, /accounting session/) ? null : { ...e, stage: 'signin' };
		return sub == 'MLME' ? null : e;
	}
	let c = match(rest, /^([A-Z][A-Z0-9-]+) ([0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2})( .*)?$/);
	if (!c)
		return null;
	if (c[1] == 'AP-STA-DISCONNECTED')
		return { bss: bss, mac: c[2], stage: 'end', what: 'disconnected', ok: null, end: true };
	let k = EVENT[c[1]];
	if (!k)
		return null;
	let e = { bss: bss, mac: c[2], stage: k[0], what: k[1], ok: k[2] };
	if (c[1] == 'AP-STA-CONNECTED')
		e.on = true;
	return e;
}

const DHCP_KINDS = [null, 'discover', 'offer', 'request', 'decline', 'ack', 'nak', 'release', 'inform'];
const RCODES = ['NOERROR', 'FORMERR', 'SERVFAIL', 'NXDOMAIN', 'NOTIMP', 'REFUSED'];
const QTYPES = { '1': 'A', '5': 'CNAME', '12': 'PTR', '28': 'AAAA', '33': 'SRV', '64': 'SVCB', '65': 'HTTPS' };

// dhcp reads a DHCP message at b in a frame: its kind, the client's MAC,
// the address given, the server's ID, and of an offer or an ack the router
// and the first DNS servers; of a client's, the host name it gives.
function dhcp(f, b) {
	if (length(f) < b + 240 || get32(f, b + 236) != 0x63825363 || ord(f, b + 2) != 6)
		return null;
	let opts = {};
	for (let at = b + 240; at + 1 < length(f) && ord(f, at) != 255; ) {
		if (ord(f, at) == 0) {
			at++;
			continue;
		}
		opts[ord(f, at)] = substr(f, at + 2, ord(f, at + 1));
		at += 2 + ord(f, at + 1);
	}
	let type = opts[53] != null ? DHCP_KINDS[ord(opts[53], 0)] : null;
	if (!type)
		return null;
	let ip = k => length(opts[k] ?? '') >= 4 ? ip4_text(opts[k], 0) : null;
	let dns = [];
	for (let i = 0; i + 4 <= length(opts[6] ?? '') && i < 12; i += 4)
		push(dns, ip4_text(opts[6], i));
	let given = ip4_text(f, b + 16);
	return {
		kind: 'dhcp', mac: mac_text(f, b + 28), type: type,
		address: given != '0.0.0.0' ? given : null, server: ip(54), router: ip(3), dns: dns,
		host: opts[12] != null ? text(opts[12], 64) || null : null,
	};
}

// dns reads a DNS message at b: whether it asks or answers, its ID, the
// name and type asked, and of an answer how it went and how many records
// came. The name is kept to 64 characters.
function dns(f, b) {
	if (length(f) < b + 17)
		return null;
	let flags = get16(f, b + 2);
	if (get16(f, b + 4) < 1)
		return null;
	let labels = [], at = b + 12;
	while (at < length(f) && ord(f, at) > 0 && ord(f, at) < 64 && length(labels) < 32) {
		push(labels, substr(f, at + 1, ord(f, at)));
		at += 1 + ord(f, at);
	}
	if (at + 2 >= length(f) || ord(f, at) != 0)
		return null;
	let qtype = get16(f, at + 1);
	return {
		kind: 'dns', id: get16(f, b), answer: (flags & 0x8000) != 0,
		name: lc(text(join('.', labels), 64)) || '.', type: QTYPES['' + qtype] ?? 'type ' + qtype,
		rcode: RCODES[flags & 15] ?? 'error ' + (flags & 15), records: get16(f, b + 6),
	};
}

// frame reads a frame from a Wi-Fi interface for what it tells of a client
// coming online; out says it was going out to the client. It names the
// client by its MAC. Null for a frame that tells nothing.
function frame(f, out) {
	if (length(f ?? '') < 42)
		return null;
	let client = mac_text(f, out ? 0 : 6);
	let type = get16(f, 12);
	if (type == 0x0806) {
		if (get16(f, 16) != 0x0800 || ord(f, 18) != 6 || ord(f, 19) != 4)
			return null;
		let op = get16(f, 20), from = ip4_text(f, 28), about = ip4_text(f, 38);
		if (out)
			return op == 2 ? { kind: 'arp', mac: mac_text(f, 32), what: 'answer', address: from, holder: mac_text(f, 22) } : null;
		if (from == '0.0.0.0')
			return null;   // looking whether an address is free: it has none yet
		return { kind: 'arp', mac: mac_text(f, 22), what: op == 1 && about != from ? 'ask' : 'tell', address: from, about: about };
	}
	let l4, proto, from, to;
	if (type == 0x0800) {
		if (get16(f, 20) & 0x1fff)
			return null;
		l4 = 14 + (ord(f, 14) & 15) * 4;
		proto = ord(f, 23);
		from = ip4_text(f, 26);
		to = ip4_text(f, 30);
	} else if (type == 0x86dd && length(f) >= 62) {
		l4 = 54;
		proto = ord(f, 20);
		from = ip6_text(f, 22);
		to = ip6_text(f, 38);
	} else
		return null;
	if (length(f) < l4 + 8)
		return null;
	let sport = get16(f, l4), dport = get16(f, l4 + 2);
	if (proto == 17) {
		if (type == 0x0800 && (sport in [67, 68] || dport in [67, 68]))
			return dhcp(f, l4 + 8);
		if (sport != 53 && dport != 53)
			return null;
		let d = dns(f, l4 + 8);
		// A lookup is the client's to a server; an answer, a server's to it.
		if (!d || d.answer != !!out || (out ? sport : dport) != 53)
			return null;
		d.mac = client;
		d.server = out ? from : to;
		if (!out)
			d.own = from;
		return d;
	}
	if (proto != 6 || length(f) < l4 + 14)
		return null;
	let flags = ord(f, l4 + 13);
	if (!(flags & 2))
		return null;
	if (!out && !(flags & 16))
		return { kind: 'tcp', mac: client, what: 'open', address: to, port: dport, own: from };
	if (out && (flags & 16))
		return { kind: 'tcp', mac: client, what: 'answer', address: from, port: sport };
	return null;
}

// begin starts the record of an attempt: who, on which network, and when,
// at by the AP's steady clock in seconds and wall by the calendar's in ms.
function begin(who, at, wall) {
	return {
		bss: who.bss, mac: who.mac, network: who.network ?? '', ssid: who.ssid ?? '', band: who.band ?? '',
		started: at, wall: wall, last: at, events: [], more: 0,
		done: {}, failed: null, on: null, ended: null, left: null, online: null,
		address: null, host: null, router: null, signal: null, probes: null,
		dhcp: { asked: 0, first: null, offer: false, ack: false, nak: false },
		arp: { asked: null, at: null, answered: false }, lookups: {}, dns: { asked: 0, answered: 0, kept: 0 },
		tcp: { opened: {}, kept: 0, answered: false },
	};
}

// step adds one step to an attempt: its stage, what happened, whether it
// went well, and any more to say of it. One like the last, within a second
// of it, is not said twice: hostapd says some things twice over. Past
// EVENTS, steps are only counted.
function step(a, at, stage, what, ok, more) {
	a.last = at;
	let t = max(0, int((at - a.started) * 1000 + 0.5));
	let last = a.events[length(a.events) - 1];
	if (last && last.stage == stage && last.what == what && t - last.t < 1000)
		return;
	if (length(a.events) >= EVENTS) {
		a.more++;
		return;
	}
	let e = { t: t, stage: stage, what: what };
	if (ok != null)
		e.ok = ok;
	for (let k, v in more ?? {})
		if (v != null)
			e[k] = v;
	push(a.events, e);
}

// reached notes what makes a client online: an address, and something
// beyond itself that answered it.
function reached(a, at) {
	if (a.online == null && a.on != null && a.address && (a.arp.answered || a.dns.answered > 0 || a.tcp.answered))
		a.online = at;
}

// on_log takes what hostapd said of the client into its attempt.
function on_log(a, e, at) {
	if (e.end) {
		step(a, at, 'end', e.what, null);
		a.ended = at;
		a.left = e.what;
		return;
	}
	step(a, at, e.stage ?? 'note', e.what, e.ok);
	if (e.stage && e.ok === true)
		a.done[e.stage] = true;
	if (e.stage && e.ok === false && !a.failed)
		a.failed = { stage: e.stage, why: e.what };
	if (e.on && a.on == null)
		a.on = at;
	reached(a, at);
}

// on_frame takes one of the client's first frames into its attempt; a
// frame that told nothing, null, changes nothing.
function on_frame(a, p, at) {
	if (!p)
		return;
	if (p.kind == 'dhcp') {
		if (p.type in ['discover', 'request']) {
			a.dhcp.asked++;
			a.dhcp.first ??= at;
			if (p.host)
				a.host = p.host;
			step(a, at, 'dhcp', p.type == 'discover' ? 'asks for an address (discover)' : 'asks to use the address (request)', null);
		} else if (p.type == 'offer') {
			a.dhcp.offer = true;
			step(a, at, 'dhcp', 'offered ' + p.address, null, { server: p.server });
		} else if (p.type == 'ack') {
			a.dhcp.ack = true;
			a.done.dhcp = true;
			if (p.address)
				a.address = p.address;
			a.router = p.router ?? a.router;
			step(a, at, 'dhcp', 'given ' + (p.address ?? 'its address'), true,
				{ server: p.server, router: p.router, dns: length(p.dns) ? join(' ', p.dns) : null });
		} else if (p.type == 'nak') {
			a.dhcp.nak = true;
			step(a, at, 'dhcp', 'the server refused the request (nak)', false, { server: p.server });
		}
	} else if (p.kind == 'arp') {
		if (p.what == 'answer') {
			if (!a.arp.answered || p.address == a.router) {
				a.done.gateway = true;
				step(a, at, 'gateway', p.address + (p.address == a.router ? ', its gateway,' : '') + ' answers', true, { holder: p.holder });
			}
			a.arp.answered = true;
		} else {
			// The address the client speaks from is the one it uses, given
			// by DHCP or kept from before.
			if (!a.address && p.address) {
				a.address = p.address;
				if (!a.dhcp.ack)
					step(a, at, 'dhcp', 'uses ' + p.address + ', which it had already', null);
			}
			if (p.what == 'ask' && (a.arp.asked == null || p.about == a.router)) {
				a.arp.asked = p.about;
				a.arp.at ??= at;
				step(a, at, 'gateway', 'asks who has ' + p.about + (p.about == a.router ? ', its gateway' : ''), null);
			}
		}
	} else if (p.kind == 'dns') {
		if (!a.address && p.own)
			a.address = p.own;
		let k = p.id + ' ' + p.name + ' ' + p.type;
		if (!p.answer) {
			a.dns.asked++;
			if (a.lookups[k] == null && a.dns.kept < LOOKUPS) {
				a.lookups[k] = at;
				a.dns.kept++;
				step(a, at, 'dns', 'asks ' + p.server + ' for ' + p.name + ' (' + p.type + ')', null);
			}
		} else {
			a.dns.answered++;
			a.done.dns = true;
			if (a.lookups[k] != null) {
				let ms = int((at - a.lookups[k]) * 1000 + 0.5);
				a.lookups[k] = null;
				step(a, at, 'dns', p.server + ' answers ' + p.name + ': ' +
					(p.rcode == 'NOERROR' ? p.records + (p.records == 1 ? ' record' : ' records') : p.rcode), p.rcode in ['NOERROR', 'NXDOMAIN'], { ms: ms });
			}
		}
	} else if (p.kind == 'tcp') {
		if (!a.address && p.own)
			a.address = p.own;
		let k = p.address + ' ' + p.port;
		if (p.what == 'open') {
			if (a.tcp.opened[k] == null && a.tcp.kept < CONNECTIONS) {
				a.tcp.opened[k] = at;
				a.tcp.kept++;
				step(a, at, 'internet', 'opens a connection to ' + p.address + ' port ' + p.port, null);
			}
		} else if (a.tcp.opened[k] != null) {
			let ms = int((at - a.tcp.opened[k]) * 1000 + 0.5);
			a.tcp.opened[k] = null;
			a.tcp.answered = true;
			a.done.internet = true;
			step(a, at, 'internet', p.address + ' answers', true, { ms: ms });
		}
	}
	reached(a, at);
}

// due says whether an attempt is over: the client left; it was not let on,
// and nothing more came for IDLE seconds, or LET_ON passed; or it was let
// on and has been watched long enough.
function due(a, at) {
	if (a.ended != null)
		return true;
	if (a.on == null)
		return at - a.last > IDLE || at - a.started > LET_ON;
	if (a.online != null && at - a.online >= LINGER)
		return true;
	return at - a.on > WATCH;
}

// record is the attempt as it is reported. Its outcome: online, an address
// and an answer from beyond itself; connected, let on, with nothing more
// seen of it, as a device that keeps its address and says little; failed,
// with the stage it stopped at and why; left, gone of its own accord before
// anything failed, wherever it had got to.
function record(a, at) {
	let outcome, stage = null, reason = '';
	let furthest = null;
	for (let s in STAGES)
		if (a.done[s])
			furthest = s;
	// Where a client not let on stopped: the stage after the last it got
	// through. A network that signs clients in has that between association
	// and the keys.
	let signin = length(filter(a.events, e => e.stage == 'signin')) > 0;
	let stopped = a.done.assoc ? (signin && !a.done.signin ? 'signin' : 'key') : a.done.auth ? 'assoc' : 'auth';
	let no_dhcp = a.dhcp.nak ? 'the DHCP server refused its request'
		: a.dhcp.offer ? 'an address was offered, and its request for it was not answered'
		: 'no answer to its ' + a.dhcp.asked + (a.dhcp.asked == 1 ? ' DHCP request' : ' DHCP requests');
	let no_gateway = a.arp.asked + (a.arp.asked == a.router ? ', its gateway,' : '') + ' did not answer';
	if (a.failed) {
		outcome = 'failed';
		stage = a.failed.stage;
		reason = a.failed.why;
	} else if (a.online != null) {
		outcome = 'online';
	} else if (a.ended != null) {
		// It left before it was seen online. That is a failure only where it
		// had waited for an answer that never came; a client that goes of
		// its own accord, as one that picks another AP, has failed nothing.
		if (a.on != null && a.dhcp.asked > 0 && !a.dhcp.ack && a.ended - a.dhcp.first >= GAVE_UP_DHCP) {
			outcome = 'failed';
			stage = 'dhcp';
			reason = no_dhcp + ', and it left';
		} else if (a.on != null && a.address && a.arp.asked != null && !a.arp.answered && a.ended - a.arp.at >= GAVE_UP_GATEWAY) {
			outcome = 'failed';
			stage = 'gateway';
			reason = no_gateway + ', and it left';
		} else {
			outcome = 'left';
			if (a.on == null)
				stage = stopped;
			reason = a.on == null ? 'before it was let on: ' + (a.left ?? '') : a.left ?? '';
		}
	} else if (a.on == null) {
		outcome = 'failed';
		stage = stopped;
		reason = 'it was never let on';
	} else if (a.dhcp.asked > 0 && !a.dhcp.ack) {
		outcome = 'failed';
		stage = 'dhcp';
		reason = no_dhcp;
	} else if (a.address && a.arp.asked != null && !a.arp.answered) {
		outcome = 'failed';
		stage = 'gateway';
		reason = no_gateway;
	} else if (a.address && a.dns.asked > 0 && a.dns.answered == 0) {
		outcome = 'failed';
		stage = 'dns';
		reason = 'no answer to its ' + a.dns.asked + (a.dns.asked == 1 ? ' DNS lookup' : ' DNS lookups');
	} else {
		outcome = 'connected';
	}
	let end = a.online ?? a.ended ?? at;
	let r = {
		mac: a.mac, bss: a.bss, network: a.network, ssid: a.ssid, band: a.band,
		started: a.wall, took_ms: max(0, int((end - a.started) * 1000 + 0.5)),
		outcome: outcome, stage: stage ?? furthest ?? 'auth', reason: text(reason, 160),
		events: a.events,
	};
	if (a.more)
		r.more = a.more;
	for (let k in ['address', 'host', 'signal', 'probes'])
		if (a[k] != null)
			r[k] = a[k];
	return r;
}

export { STAGES, WATCH, assemble, capture, log_event, frame, begin, on_log, on_frame, due, record };
