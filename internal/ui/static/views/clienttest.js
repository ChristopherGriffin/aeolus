// The client test (0121), on the page of an AP with a scan radio: that
// radio joins one of the AP's own networks as a client would, on the AP
// chosen, and each step of its way online is timed. A form asks for one;
// each test asked is a line that opens to its steps, drawn as a client's
// own connection is (0118).

import { h } from '../dom.js';
import { get, post } from '../api.js';
import { ago, bandName } from '../format.js';
import { flash, hurry, redrawNow } from '../refresh.js';
import { journeyPanel } from './journey.js';

// The stages of a test, in order, as they are named.
const STAGES = [
	['find', 'Found'], ['auth', 'Authentication'], ['assoc', 'Association'], ['key', 'Keys'],
	['dhcp', 'DHCP'], ['gateway', 'Gateway'], ['dns', 'DNS'], ['speed', 'Speed'],
];
const NAME = Object.fromEntries(STAGES);
// How a network may be secured for the test to join it: with the network's
// own passphrase, or none.
const TESTABLE = new Set(['open', 'owe', 'wpa2-psk', 'wpa3-sae', 'wpa2-wpa3']);
const BANDS = [['', 'Any band'], ['2g', '2.4 GHz'], ['5g', '5 GHz'], ['6g', '6 GHz']];

const took = (ms) => (ms < 1000 ? `${ms} ms` : ms < 60000 ? `${(ms / 1000).toFixed(1)} s` : `${Math.round(ms / 60000)} min`);
const when = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });

// hasScanRadio says whether the AP said it has a radio another service
// owns (0081): the one a test borrows.
export function hasScanRadio(page) {
	return (page.facts?.radios || []).some((r) => r.reserved);
}

// chip says where a test stands: waiting for the AP, under way, passed and
// how fast, or failed and at which stage.
function chip(a) {
	const d = a.detail;
	if (a.state === 'pending') return h('span', { class: 'chip', title: 'It starts on the AP’s next poll, within about a minute' }, 'waiting for the AP');
	if (a.state === 'running') return h('span', { class: 'chip warn', title: 'It waits for the scan radio to be between its own jobs, tests for under a minute, then sees that the scan radio hears again' }, 'testing');
	if (a.state === 'expired') return h('span', { class: 'chip warn' }, 'expired: the AP did not finish it');
	if (d?.outcome === 'passed') return h('span', { class: 'chip ok' }, d.took_ms ? `Passed in ${took(d.took_ms)}` : 'Passed');
	return h('span', { class: 'chip bad' }, d?.stage ? `Failed at ${NAME[d.stage] || d.stage}` : 'Failed');
}

// rail is the stages in a row, each marked: done, failed, or not reached.
function rail(d) {
	const steps = d.steps || [];
	return h('div', { class: 'rail' }, STAGES.map(([k, name]) => {
		const mine = steps.filter((e) => e.stage === k);
		if (k === 'speed' && !d.speed && !mine.length) return null;
		const bad = mine.some((e) => e.ok === false) || (d.outcome === 'failed' && d.stage === k);
		const ok = !bad && mine.some((e) => e.ok === true);
		const [cls, says] = bad ? ['bad', 'failed'] : ok ? ['ok', 'done'] : ['idle', 'not reached'];
		return h('span', { class: `chip ${cls}`, title: `${name}: ${says}` }, name);
	}));
}

// rate is one way of the link: how fast, and how it is sent.
const rate = (r) => (r ? `${r.mbps} Mbit/s${r.how ? ` (${r.how.replace(/-/g, ' ')})` : ''}` : null);
// legacy says the link ran at rates older than 802.11n, which say nothing
// of how they are sent: the scan radio's own limit on a band where it does
// no better, and so the most a speed measured over it can show.
const legacy = (l) => [l?.tx, l?.rx].some((r) => r && !r.how);
const tenth = (v) => Math.round(v * 10) / 10;

// found is what the test measured, a line each.
function found(d) {
	const row = (label, v) => v && h('div', { class: 'row' }, h('div', { class: 'label' }, label), h('div', { class: 'value' }, v));
	const l = d.link;
	const g = d.gateway;
	const n = d.dns;
	const s = d.speed;
	return [
		row('Joined', d.bssid && [h('span', { class: 'mono' }, d.bssid), ` · ${bandName(d.band)}${d.channel ? `, channel ${d.channel}` : ''}`,
			d.joined_ms != null && ` · let on after ${took(d.joined_ms)}`]),
		row('Link', l && [[l.signal != null && `${l.signal} dBm`, l.tx && `sends at ${rate(l.tx)}`, l.rx && `hears at ${rate(l.rx)}`].filter(Boolean).join(' · '),
			legacy(l) && h('div', { class: 'sub' }, 'A legacy rate: the test radio does no better on this band, so the speed below is its own limit, not the network’s.')]),
		row('Address', d.dhcp && [`${d.dhcp.address}/${d.dhcp.mask} in ${took(d.dhcp.ms)}`, d.dhcp.server && `from ${d.dhcp.server}`,
			d.dhcp.router && `gateway ${d.dhcp.router}`, d.dhcp.dns?.length > 0 && `DNS ${d.dhcp.dns.join(', ')}`].filter(Boolean).join(' · ')),
		row('Gateway', g && (g.got ? `${g.got} of ${g.sent} pings answered, in ${tenth(g.min)} to ${tenth(g.max)} ms, ${tenth(g.avg)} ms on average` : `none of ${g.sent} pings answered`)),
		row('DNS', n && (n.rcode == null ? `${n.server} did not answer for ${n.name}`
			: n.rcode === 0 && n.answers ? `${n.server} gave ${n.name} as ${n.address || `${n.answers} records`} in ${n.ms} ms` : `${n.server} had no address for ${n.name} (code ${n.rcode})`)),
		row('Speed', s && (s.error ? `not measured: ${s.error}` : `${s.down} Mbit/s down · ${s.up ?? '?'} Mbit/s up, to ${s.server.replace(/^[a-z]+:\/\//, '')}`)),
	];
}

// deaf says what the test did to the scan radio, where it left it hearing
// nothing, or not seen to hear: bound again and hearing, which is only
// worth a note, or still deaf, which someone has to see to.
function deaf(radio) {
	if (!radio?.deaf && !radio?.unseen) return null;
	const was = radio.unseen ? 'After the test airscan made no look to say the scan radio still hears.'
		: `After the test the scan radio heard nothing, where it heard ${radio.before} networks before.`;
	if (radio.again > 0)
		return h('p', { class: 'sub' }, `${was} Its driver was bound again, and it hears ${radio.again}.`);
	return h('div', { class: 'banner problems' }, `${was} ${radio.rebound ? 'Binding its driver again did not bring it back' : 'Its driver could not be bound again from here'}: until it is, on the AP, neither airscan nor another test can use it.`);
}

// opened is a test opened: its stages in a row, why it failed, what it
// measured, and each step with how long after its start it came.
function opened(a) {
	const d = a.detail;
	if (!d) return h('p', { class: 'sub' }, a.state === 'pending' || a.state === 'running'
		? 'Its result follows when the AP has done it and has seen its scan radio hear again: one to three minutes after its next poll.'
		: a.result || 'It left no result.');
	const steps = d.steps || [];
	const radio = d.scan_radio;
	// The same join, as the AP it joined recorded it (0118), opens below.
	const other = h('div', { class: 'testother' });
	return h('div', { class: 'attempt' },
		rail(d),
		d.reason && h('p', { class: 'why bad' }, `Why: ${d.reason}.`),
		deaf(radio),
		h('div', { class: 'testfound' }, found(d)),
		steps.length > 0 && h('table', { class: 'list steps' },
			h('tr', null, ['After', 'Stage', 'What happened', ''].map((t) => h('th', null, t))),
			steps.map((e) => h('tr', { class: e.ok === false ? 'failed' : '' },
				h('td', { class: 'mono' }, `+${took(e.t)}`),
				h('td', null, NAME[e.stage] || e.stage),
				h('td', null, e.what, e.ms != null && h('div', { class: 'sub' }, `in ${tenth(e.ms)} ms`)),
				h('td', null, e.ok === true ? h('span', { class: 'chip ok' }, 'ok') : e.ok === false ? h('span', { class: 'chip bad' }, 'failed') : null)))),
		h('p', { class: 'sub attemptlinks' }, [
			d.waited_ms > 1000 && `It waited ${took(d.waited_ms)} for the scan radio to be between its own jobs. `,
			radio?.was_deaf && 'Before the test the scan radio heard nothing, so its driver was bound again first. ',
			radio?.after_dfs && (radio.rebound ? 'The test had the scan radio on a DFS channel, after which it goes deaf, so its driver was bound again before airscan had it back. '
				: 'The test had the scan radio on a DFS channel, after which it goes deaf, and its driver could not be bound again from here. '),
			radio && !radio.deaf && !radio.unseen && `The scan radio heard ${radio.after} networks once airscan had it back (${radio.before} before). `,
			d.client && h('a', { href: '#', class: 'clientlink', title: 'The same join, as the AP it joined recorded it',
				onclick: (e) => { e.preventDefault(); journeyPanel(other, d.client); } }, `What the AP it joined saw of ${d.client}`),
		]),
		other);
}

// The tests opened, by their IDs, so a panel drawn again keeps them open.
const open = new Set();
// What the form last had chosen, so a redraw does not reset it.
const chosen = { network: '', on: '', band: '' };

// clientTestPanel is the panel: the form, for someone who may ask, and the
// tests asked of the AP, latest first. cfg is the AP's config with its
// condition; names gives an AP's name by its ID.
export async function clientTestPanel(ctx, id, page, cfg, canAct) {
	const enc = encodeURIComponent(id);
	let list = [];
	try {
		list = ((await get(`/v1/aps/${enc}/actions`)).actions || []).filter((a) => a.kind === 'client-test');
	} catch { /* an older manager has none */ }
	const name = (ap) => (ap === id ? `${page.node.name} itself` : ctx.name('locations', ap));
	const nets = Object.entries(cfg.document?.network || {}).sort(([a], [b]) => a.localeCompare(b));
	const can = (n) => (n.radius?.passphrases === true ? 'each device has its own passphrase'
		: TESTABLE.has(n.security) ? '' : 'signs clients in by 802.1X');
	const near = (cfg.condition?.state?.report?.rrm?.neighbours || []).filter((n) => n.state !== 'down');
	const speed = page.fields?.['system.speed_server']?.value;
	const busy = list.some((a) => a.state === 'pending' || a.state === 'running');
	if (busy) hurry(20);

	const box = h('div', { class: 'edit' });
	const net = h('select', { 'aria-label': 'Network to join' },
		nets.map(([k, n]) => h('option', { value: k, disabled: !!can(n) }, `${n.ssid}${can(n) ? ` (${can(n)}: not testable yet)` : ''}`)));
	if (nets.some(([k, n]) => k === chosen.network && !can(n))) net.value = chosen.network;
	else net.value = nets.find(([, n]) => !can(n))?.[0] ?? '';
	const on = h('select', { 'aria-label': 'AP to join it on' },
		h('option', { value: '' }, 'the strongest other AP it hears'),
		h('option', { value: id }, 'this AP itself'),
		near.map((n) => h('option', { value: n.ap }, `${ctx.name('locations', n.ap)}${n.bands?.some((b) => b.signal != null)
			? ` (${n.bands.filter((b) => b.signal != null).map((b) => `${bandName(b.band)} ${b.signal} dBm`).join(', ')})` : ''}`)));
	on.value = [...on.options].some((o) => o.value === chosen.on) ? chosen.on : '';
	const bandSeg = h('span', { class: 'segmented', role: 'group', 'aria-label': 'Band' });
	const paint = () => bandSeg.replaceChildren(...BANDS.map(([v, label]) => h('button', {
		type: 'button', class: chosen.band === v ? 'on' : '', 'aria-pressed': chosen.band === v, onclick: () => { chosen.band = v; paint(); },
	}, label)));
	paint();
	net.addEventListener('change', () => { chosen.network = net.value; });
	on.addEventListener('change', () => { chosen.on = on.value; });
	const run = h('button', { type: 'button', class: 'button primary', disabled: busy || !net.value }, busy ? 'A test is under way' : 'Run the test');
	run.addEventListener('click', async () => {
		run.disabled = true;
		box.replaceChildren();
		try {
			const body = { kind: 'client-test', network: net.value };
			if (on.value) body.on = on.value;
			if (chosen.band) body.band = chosen.band;
			await post(`/v1/aps/${enc}/actions`, body);
			flash(`Client test asked of ${page.node.name}: it starts on its next poll, within about a minute, and takes one to three.`, 60);
			hurry(240);
			redrawNow();
		} catch (err) {
			run.disabled = false;
			box.replaceChildren(h('div', { class: 'error' }, err.message));
		}
	});
	const form = canAct && (nets.length
		? h('div', { class: 'testform', 'data-editing': 'false' },
			h('div', { class: 'row' }, h('div', { class: 'label' }, 'Join'), h('div', { class: 'value' }, net, ' on ', on, ' ', bandSeg)),
			h('div', { class: 'sub' }, speed
				? `Its speed is measured against ${String(speed).replace(/^[a-z]+:\/\//, '')}, the speed server set for this AP.`
				: 'No speed server is set here (System › Client tests), so it measures no speed: it joins, takes an address, and tries its gateway and DNS.'),
			h('div', { class: 'actions' }, run))
		: h('p', { class: 'sub' }, 'This AP offers no network to test.'));

	return h('section', { class: 'panel' },
		h('h2', null, 'Client tests', h('span', { class: 'note' }, 'its scan radio joins a network as a client would; the radio is borrowed from airscan for under a minute')),
		form,
		box,
		list.length
			? list.map((a) => {
				const p = a.params || {};
				const d = h('details', { class: 'fold', open: open.has(a.id) },
					h('summary', null,
						h('span', { class: 'when', title: when(a.at) }, ago(a.at)),
						h('span', { class: 'who' }, p.ssid || p.network || 'a network'),
						h('span', { class: 'grow sub' }, [`on ${p.on ? name(p.on) : 'the strongest other AP'}`, p.band && bandName(p.band), `asked by ${a.actor}`,
							a.detail?.outcome === 'failed' && a.detail.reason].filter(Boolean).join(' · ')),
						chip(a)),
					opened(a));
				d.addEventListener('toggle', () => (d.open ? open.add(a.id) : open.delete(a.id)));
				return d;
			})
			: h('p', { class: 'sub' }, canAct ? 'None asked yet.' : 'None asked yet. Asking needs operator on this AP.'));
}
