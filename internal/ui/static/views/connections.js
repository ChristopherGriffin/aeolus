// A client's attempts to come online (0118): each one, passed or failed,
// opens to every step of it, as its AP recorded them: where it was heard
// asking, authentication, association, sign-in, its keys, DHCP's four
// messages, the gateway, its first DNS lookups and its first connection,
// each with its time.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { bandName, ago } from '../format.js';

// The stages of coming online, in order, as they are named.
const STAGES = [
	['auth', 'Authentication'], ['assoc', 'Association'], ['signin', 'Sign-in'], ['key', 'Keys'],
	['dhcp', 'DHCP'], ['gateway', 'Gateway'], ['dns', 'DNS'], ['internet', 'First connection'],
];
const NAME = { ...Object.fromEntries(STAGES), end: 'Left', note: 'hostapd says' };

const when = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
const took = (ms) => (ms < 1000 ? `${ms} ms` : ms < 60000 ? `${(ms / 1000).toFixed(1)} s` : `${Math.round(ms / 60000)} min`);

// outcomeChip says what an attempt came to: online, and how fast; failed,
// and at which stage; left; or connected, with nothing more seen of it.
// brief leaves out how fast and where, for a list of clients.
export function outcomeChip(c, brief) {
	const at = NAME[c.stage] || c.stage;
	if (c.outcome === 'online') return h('span', { class: 'chip ok', title: 'An address, and something beyond itself answered it' }, brief ? 'Online' : `Online in ${took(c.took_ms)}`);
	if (c.outcome === 'failed') return h('span', { class: 'chip bad' }, brief ? 'Failed' : `Failed at ${at}`);
	if (c.outcome === 'left') return h('span', { class: 'chip warn', title: 'It went of its own accord before it was seen online' }, brief ? 'Left' : `Left after ${at}`);
	return h('span', { class: 'chip idle', title: 'Let on to the network, with nothing more seen of it: a device that keeps its address and says little' }, 'Connected');
}

// rail is the stages in a row, each marked: done, failed, begun, or not
// seen. Sign-in is left out where the network has none.
function rail(c) {
	const ev = c.record?.events || [];
	return h('div', { class: 'rail' }, STAGES.map(([k, name]) => {
		const mine = ev.filter((e) => e.stage === k);
		const bad = mine.some((e) => e.ok === false) || (c.outcome === 'failed' && c.stage === k);
		if (k === 'signin' && !mine.length && !bad) return null;
		const ok = !bad && mine.some((e) => e.ok === true);
		const [cls, says] = bad ? ['bad', 'failed'] : ok ? ['ok', 'done'] : mine.length ? ['warn', 'begun, not finished'] : ['idle', 'not seen'];
		return h('span', { class: `chip ${cls}`, title: `${name}: ${says}` }, name);
	}));
}

// more is what else a step says: who answered, what DHCP gave, how long an
// answer took.
function more(e) {
	return [
		e.server && `from ${e.server}`,
		e.router && `gateway ${e.router}`,
		e.dns && `DNS ${e.dns.split(' ').join(', ')}`,
		e.holder && `at ${e.holder}`,
		e.ms != null && `in ${e.ms} ms`,
	].filter(Boolean).join(' · ');
}

// heard says where the client was heard asking for networks before it
// joined, the strongest first.
function heard(c) {
	const p = c.record?.probes;
	if (!p?.length) return 'Not recorded: band steering (usteer), which hears clients ask for networks, is not running on this AP.';
	return 'Heard asking for networks ' + p.map((x) => `${x.ap ? `by the AP at ${x.ap}` : `by this AP${x.band ? ` on ${bandName(x.band)}` : ''}`} at ${x.signal} dBm`).join('; ') + '.';
}

// steps is the attempt opened: its stages in a row, then each step with how
// long after the start it came.
function steps(c) {
	const r = c.record || {};
	const ev = r.events || [];
	return h('div', { class: 'attempt' },
		rail(c),
		c.reason && h('p', { class: c.outcome === 'failed' ? 'why bad' : 'why' }, c.outcome === 'failed' ? `Why: ${c.reason}.` : `${c.reason}.`),
		h('table', { class: 'list steps' },
			h('tr', null, ['After', 'Stage', 'What happened', ''].map((t) => h('th', null, t))),
			h('tr', null, h('td', { class: 'mono' }, 'before'), h('td', null, 'Probes'), h('td', { colspan: 2, class: 'sub' }, heard(c))),
			ev.map((e) => h('tr', { class: e.ok === false ? 'failed' : '' },
				h('td', { class: 'mono' }, `+${took(e.t)}`),
				h('td', null, NAME[e.stage] || e.stage),
				h('td', null, e.what, more(e) && h('div', { class: 'sub' }, more(e))),
				h('td', null, e.ok === true ? h('span', { class: 'chip ok' }, 'ok') : e.ok === false ? h('span', { class: 'chip bad' }, 'failed') : null))),
			r.more > 0 && h('tr', null, h('td', null), h('td', { colspan: 3, class: 'sub' }, `and ${r.more} more step${r.more === 1 ? '' : 's'}, not kept`))),
		h('p', { class: 'sub' }, [
			r.signal != null && `Heard at ${r.signal} dBm when it was let on`,
			r.address && `address ${r.address}`,
			r.host && `calls itself ${r.host}`,
			r.bss && `on ${r.bss}`,
		].filter(Boolean).join(' · ')));
}

// The attempts opened, by their IDs, so a panel drawn again keeps them open.
const opened = new Set();

// connectionsSection reads a client's attempts and returns what to show of
// them: each a line, latest first, that opens by its arrow.
export async function connectionsSection(mac) {
	let j;
	try {
		j = await get(`/v1/clients/${encodeURIComponent(mac)}/connections?limit=50`);
	} catch (e) {
		return [h('div', { class: 'error' }, e.message)];
	}
	const list = j.connections || [];
	const cl = j.client;
	const head = h('h3', { class: 'subhead' }, 'Connections',
		h('span', { class: 'note' }, cl
			? `${cl.attempts} attempt${cl.attempts === 1 ? '' : 's'} since ${ago(cl.first_seen)}, ${cl.failed} failed · each opens to its steps`
			: 'each opens to its steps'));
	if (!list.length)
		return [head, h('p', { class: 'sub' }, 'No attempt to come online is recorded for it. APs record them from agent v0.69.0 on; a client already on when recording began has none until it joins again.')];
	return [head, ...list.map((c) => {
		const d = h('details', { class: 'fold', open: opened.has(c.id) },
			h('summary', null,
				h('span', { class: 'when' }, when(c.started)),
				h('span', { class: 'who' }, c.ap_name || c.ap),
				h('span', { class: 'grow sub' }, [c.ssid || c.network, bandName(c.band)].filter(Boolean).join(' · '),
					c.outcome === 'failed' && c.reason ? ` · ${c.reason}` : ''),
				outcomeChip(c)),
			steps(c),
			h('p', { class: 'sub attemptlinks' }, link(`/aps/${encodeURIComponent(c.ap)}/clients`, `${c.ap_name || c.ap}'s clients`)));
		d.addEventListener('toggle', () => (d.open ? opened.add(c.id) : opened.delete(c.id)));
		return d;
	})];
}
