// The way online, drawn (0130): the points a client passes on its way
// online, in order, as a tunnel's path is drawn (path.js), each with how
// long it took. A client test's way (0121, 0128) and a real client's own
// attempt (0118) are drawn the same, from what the manager made of their
// steps; and so is what many of either come to, with the middle of their
// times at each point and how many stopped there.
//
// The keys are drawn with the four messages of their handshake under them
// (0133), where the AP said them: which was sent or came, in what time, and
// the one it stopped at. A wrong passphrase stops at the second, the
// client's answer.

import { h } from '../dom.js';
import { pathOf } from './path.js';

// The points, as they are named on the path: short, as a path of nine
// has little room, with the whole of it said on hover.
const POINT = {
	find: 'Found', auth: 'Auth', assoc: 'Assoc', signin: 'Sign-in', key: 'Keys',
	offer: 'Offer', dhcp: 'Address', gateway: 'Gateway', dns: 'DNS', internet: 'Internet', speed: 'Speed', release: 'Given back',
};
const WHOLE = { ...POINT, auth: 'Authentication', assoc: 'Association', dhcp: 'An address', internet: 'Its first connection', release: 'The address given back' };
// And as a sentence says them.
const SAID = {
	find: 'finding the network', auth: 'authentication', assoc: 'association', signin: 'the sign-in', key: 'the keys',
	offer: 'the offer of an address', dhcp: 'the address', gateway: 'the gateway', dns: 'DNS', internet: 'the first connection',
	speed: 'the speed', release: 'giving the address back',
};

// took says a time: under ten ms to a tenth, as an ARP answer is; then
// whole ms; then seconds.
export function took(ms) {
	if (ms < 10) return `${Math.round(ms * 10) / 10} ms`;
	if (ms < 1000) return `${Math.round(ms)} ms`;
	return ms < 60000 ? `${(ms / 1000).toFixed(1)} s` : `${Math.round(ms / 60000)} min`;
}

const STATE = { ok: 'ok', bad: 'bad', begun: 'warn' };
const WENT = { ok: 'done', bad: 'failed', begun: 'begun, and not seen through', none: 'not seen' };

// A point's parts (0133): the four messages of the key handshake, as they
// are named under the keys, and in full.
const PART = { m1: 'M1', m2: 'M2', m3: 'M3', m4: 'M4' };
const PARTWHOLE = {
	m1: 'Message 1 of the key handshake, the AP’s first',
	m2: 'Message 2, the client’s answer, which shows whether it has the passphrase',
	m3: 'Message 3, the AP’s, sent once the answer checked out',
	m4: 'Message 4, the client’s last',
};
const PARTWENT = { ok: 'done', bad: 'failed', begun: 'waited for', none: 'not come to' };
// What is said of a message the handshake stopped at, in full.
const PARTSAID = { 'wrong passphrase': 'it came, and did not check out: a wrong passphrase', 'no answer': 'it did not come' };

// parts draws a point's parts, to go under it: each by its name, with what
// its own words give it. says gives a part's words: [what is shown, its
// state, what it says in full].
function parts(list, says) {
	return h('span', { class: 'parts' }, list.map((m) => {
		const [main, state, full] = says(m);
		return h('span', { class: `part ${state || ''}`, title: full }, h('b', null, PART[m.k] || m.k), ` ${main}`);
	}));
}

// wayPath draws one way, a test's or an attempt's. opts.lead are hops put
// before it, as where a test over the wire was made from; opts.main and
// opts.sub give a point's own words by its key, in place of its time and
// under it; opts.what says whose way it is, for a screen reader.
export function wayPath(way, opts = {}) {
	if (!way?.length) return null;
	const hops = [...(opts.lead || [])];
	const told = hops.map((p) => p.title).filter(Boolean);
	for (const p of way) {
		const name = POINT[p.k] || p.k;
		const time = p.ms != null ? took(p.ms) : null;
		const bad = p.state === 'bad';
		const main = opts.main?.[p.k] || (bad ? 'failed' : time || (p.state === 'ok' ? 'done' : p.state === 'begun' ? 'begun' : '—'));
		const says = `${WHOLE[p.k] || name}: ${WENT[p.state] || p.state}${time ? (p.state === 'bad' ? `, after ${time}` : `, in ${time}`) : ''}`;
		// Its parts, each with its time, or what is said of the one it
		// stopped at.
		const each = [];
		const under = p.parts?.length ? parts(p.parts, (m) => {
			const t = m.ms != null ? took(m.ms) : null;
			const full = `${PARTWHOLE[m.k] || m.k}: ${PARTSAID[m.said] || PARTWENT[m.state] || m.state}${t ? (m.state === 'ok' ? `, in ${t}` : `, after ${t}`) : ''}`;
			each.push(full);
			return [m.said || t || (m.state === 'ok' ? 'done' : '—'), STATE[m.state], full];
		}) : null;
		// Under a point that failed, how long it was at it.
		hops.push({ top: name, main, sub: under || (bad && time ? `after ${time}` : opts.sub?.[p.k]) || null, state: STATE[p.state] || null, title: says });
		told.push(each.length ? `${says} (${each.join('; ')})` : says);
	}
	return h('div', { class: 'way' }, pathOf(hops, `${opts.what || 'Its way online'}. ${told.join('; ')}.`));
}

// typicalPath draws what many ways come to: at each point the middle of
// the times of those that passed it, with the least and the most under it,
// and how many stopped there. of says how many ways, and of what, as "14
// tests".
export function typicalPath(way, of) {
	if (!way?.length) return null;
	const told = [];
	const hops = way.map((p) => {
		const name = POINT[p.k] || p.k;
		const range = p.ms != null && p.low !== p.high ? `${took(p.low)} to ${took(p.high)}` : null;
		const stopped = p.stopped > 0 ? `${p.stopped} stopped` : null;
		const says = `${WHOLE[p.k] || name}: ${p.passed} passed it${p.ms != null ? `, typically in ${took(p.ms)}${range ? `, from ${range}` : ''}` : ''}${p.stopped ? `; ${p.stopped} stopped at ${SAID[p.k] || p.k}` : ''}`;
		told.push(says);
		// Its parts over the ways that said them: at each how long it
		// typically took, or how many stopped there.
		const under = p.parts?.some((m) => m.passed || m.stopped) ? parts(p.parts, (m) => {
			const full = `${PARTWHOLE[m.k] || m.k}: ${m.passed} passed it${m.ms != null ? `, typically in ${took(m.ms)}${m.low !== m.high ? `, from ${took(m.low)} to ${took(m.high)}` : ''}` : ''}${m.stopped ? `; ${m.stopped} stopped there` : ''}`;
			told.push(full);
			return [m.stopped ? `${m.stopped} stopped` : m.ms != null ? took(m.ms) : m.passed ? 'done' : '—',
				m.stopped ? (m.passed ? 'warn' : 'bad') : m.passed ? 'ok' : null, full];
		}) : null;
		return {
			top: name,
			main: p.ms != null ? took(p.ms) : p.passed ? 'done' : p.stopped ? 'failed' : '—',
			// Under it, its parts; or who stopped there, which matters more
			// than the spread of the times; that is on hover.
			sub: under || stopped || range,
			state: p.stopped ? (p.passed ? 'warn' : 'bad') : p.passed ? 'ok' : null,
			title: says,
		};
	});
	return h('div', { class: 'way' },
		pathOf(hops, `The way online over ${of}. ${told.join('; ')}.`),
		h('div', { class: 'sub' }, `Over ${of}: at each point, how long it typically took those that passed it, the middle of their times.`));
}
