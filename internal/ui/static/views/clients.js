// The Clients tab (0066): every Wi-Fi client on an AP, or on the APs below a
// folder, as each last reported. Who it is, with its maker and a guess at
// what it is (0067); where and how it connects, the 802.11 features it
// supports, and how often frames to it are retried; its data; and how it
// does with DHCP (0065). One compact row a client; filters by network and AP,
// a search box, and sorting by any column. The filters stay as they are when
// the page refreshes.

import { h, link } from '../dom.js';
import { bandName, ago } from '../format.js';
import { configs } from './sections.js';

const DHCP = { ok: ['ok', 'DHCP'], static: ['idle', 'static'], none: ['bad', 'no address'], unknown: ['idle', 'not yet'] };

// What the person chose, kept across the page's refreshes.
const view = { network: '', ap: '', q: '', sort: 'connected', up: true };

// clientsTab draws the tab: of one AP (ap: {ap, cfg}), or of every AP below
// a folder's page.
export async function clientsTab(ctx, page, ap) {
	const rows = ap ? [ap] : await configs(page.hardware?.aps || []);
	const now = Date.now() / 1000;
	const all = rows.flatMap(({ ap: a, cfg }) => {
		const st = cfg?.condition?.state;
		const since = st ? Math.max(0, now - new Date(st.at).getTime() / 1000) : 0;
		// Connected for as long as it was at the report, and since.
		return (st?.report?.clients || []).map((c) => ({ ...c, ap: a, at: st.at, connected: c.connected + since }));
	});
	if (!all.length)
		return h('div', { class: 'banner info' }, rows.length
			? 'No Wi-Fi clients reported here. An AP whose agent is older than v0.33.0 doesn\'t report them.'
			: 'No APs here yet.');
	const box = h('div');
	const draw = () => box.replaceChildren(table(all, !ap, draw));
	draw();
	return h('section', { class: 'panel' },
		h('h2', null, 'Wi-Fi clients', h('span', { class: 'note' }, 'as each AP last reported; signal, rates and data refresh every few minutes')),
		controls(all, !ap, draw), box);
}

function controls(all, folder, draw) {
	const pick = (key, label, values) => {
		const sel = h('select', { onchange: () => { view[key] = sel.value; draw(); } },
			h('option', { value: '' }, label),
			values.map(([v, name]) => h('option', { value: v, selected: view[key] === v }, name)));
		return sel;
	};
	const ssids = [...new Set(all.map((c) => c.ssid).filter(Boolean))].sort();
	const aps = [...new Map(all.map((c) => [c.ap.id, c.ap.name])).entries()].sort((a, b) => a[1].localeCompare(b[1]));
	const q = h('input', { type: 'search', placeholder: 'Name, MAC or address', value: view.q, oninput: () => { view.q = q.value; draw(); } });
	return h('div', { class: 'filters' },
		pick('network', 'Every network', ssids.map((s) => [s, s])),
		folder && pick('ap', 'Every AP', aps),
		q,
		h('span', { class: 'note' }, `${all.length} client${all.length === 1 ? '' : 's'}`));
}

// The columns, each with how a row sorts by it.
const COLUMNS = [
	['client', 'Client', (c) => (c.host || c.mac).toLowerCase()],
	['ap', 'AP', (c) => c.ap.name.toLowerCase()],
	['ssid', 'Network', (c) => c.ssid || ''],
	['signal', 'Band', (c) => c.signal ?? -999],
	['rate', 'Rate', (c) => c.tx_rate ?? 0],
	['wifi', 'Wi-Fi', (c) => ['', 'n', 'ac', 'ax', 'be'].indexOf(c.gen || '') * 8 + (c.k ? 4 : 0) + (c.v ? 2 : 0) + (c.w ? 1 : 0)],
	['retries', 'Retries', (c) => retries(c).retried],
	['address', 'Address', (c) => (c.address || '').split('.').map((x) => x.padStart(3, '0')).join('.')],
	['connected', 'Connected', (c) => c.connected],
	['data', 'Data', (c) => c.tx_bytes + c.rx_bytes],
	['dhcp', 'DHCP', (c) => c.dhcp || ''],
];

function table(all, folder, draw) {
	const q = view.q.trim().toLowerCase();
	const shown = all.filter((c) => (!view.network || c.ssid === view.network) && (!view.ap || c.ap.id === view.ap) &&
		(!q || [c.host, c.mac, c.address, c.ssid, c.maker, c.kind, c.os].some((x) => (x || '').toLowerCase().includes(q))));
	const col = COLUMNS.find(([k]) => k === view.sort) || COLUMNS.find(([k]) => k === 'connected');
	shown.sort((a, b) => {
		const x = col[2](a), y = col[2](b);
		return (x < y ? -1 : x > y ? 1 : 0) * (view.up ? 1 : -1);
	});
	const head = COLUMNS.filter(([k]) => folder || k !== 'ap').map(([k, name]) => h('th', {
		class: 'sortable', onclick: () => { view.up = view.sort === k ? !view.up : true; view.sort = k; draw(); },
	}, name, view.sort === k ? (view.up ? ' ▴' : ' ▾') : ''));
	return h('table', { class: 'list clients' },
		h('tr', null, head),
		shown.map((c) => h('tr', null,
			h('td', null, c.host || h('span', { class: 'mono' }, c.mac), c.host && h('div', { class: 'sub mono' }, c.mac), who(c)),
			folder && h('td', null, link(`/aps/${encodeURIComponent(c.ap.id)}`, c.ap.name)),
			h('td', null, c.ssid || '—', c.vlan && h('div', { class: 'sub', title: 'a per-user key put it in this VLAN' }, `VLAN ${c.vlan}`)),
			h('td', null, bandName(c.band) || '—', c.signal != null && h('div', { class: 'sub' }, `${c.signal} dBm`)),
			h('td', null, rate(c)),
			h('td', null, features(c)),
			h('td', null, retryCell(c)),
			h('td', { class: 'mono' }, c.address || '—'),
			h('td', { title: `reported ${ago(c.at)}` }, duration(c.connected)),
			h('td', null, `↓ ${size(c.tx_bytes)}`, h('div', { class: 'sub' }, `↑ ${size(c.rx_bytes)}`)),
			h('td', null, c.dhcp ? h('span', { class: 'chip ' + DHCP[c.dhcp][0] }, DHCP[c.dhcp][1]) : '—'))),
		!shown.length && h('tr', null, h('td', { colspan: folder ? 11 : 10, class: 'sub' }, 'No client matches.')));
}

// who says what a client is (0067): its maker by OUI, or that its MAC is
// private; and the manager's guess at its kind and system, with what the
// guess went on.
function who(c) {
	const maker = c.private ? 'private MAC' : (c.maker || '').replace(/,? (inc|ltd|llc|co|corp|corporation|gmbh|limited)\.?$/i, '');
	const kind = [c.kind, c.os].filter(Boolean).join(', ');
	const line = [maker, kind].filter(Boolean).join(' · ');
	return line && h('div', { class: 'sub', title: c.basis ? `guessed from its ${c.basis}` : '' }, line);
}

// features shows the 802.11 generation and the features k, v and w that a
// client's connection uses, as small chips (0067). The AP's radio and the
// SSID cap them, so a client may be able to do more. 802.11r isn't known:
// this AP's hostapd doesn't say which key management a client chose.
function features(c) {
	if (c.k == null) return '—';   // hostapd didn't say
	const gen = { n: 'Wi-Fi 4', ac: 'Wi-Fi 5', ax: 'Wi-Fi 6', be: 'Wi-Fi 7' }[c.gen] || 'a/b/g';
	const chip = (on, name, what) => h('span', { class: 'chip ' + (on ? 'ok' : 'idle'), title: `${what}: ${on ? 'in use' : 'not in use'} on this connection` }, name);
	return [
		h('div', { title: "what this connection uses; the AP's radio may cap it" }, gen),
		h('div', { class: 'chips' },
			chip(c.k, 'k', '802.11k, neighbour reports'), ' ', chip(c.v, 'v', '802.11v, BSS transition'), ' ',
			chip(c.w, 'w', '802.11w, protected management frames'), ' ',
			h('span', { class: 'chip idle', title: "802.11r: not known; this AP's hostapd doesn't say" }, 'r?')),
	];
}

// retries is the share, in percent, of frames to a client that were
// retried, and that failed, since it joined (0067).
function retries(c) {
	const sent = (c.tx_packets || 0) + (c.tx_failed || 0);
	return { retried: sent ? 100 * (c.tx_retries || 0) / sent : 0, failed: sent ? 100 * (c.tx_failed || 0) / sent : 0, sent };
}

function retryCell(c) {
	const r = retries(c);
	if (!r.sent) return '—';
	const worst = Math.max(r.retried, r.failed);
	const pct = (x) => x < 1 && x > 0 ? '<1 %' : `${Math.round(x)} %`;
	return [
		h('span', { class: 'chip ' + (worst > 25 ? 'bad' : worst > 10 ? 'warn' : 'ok'), title: `${c.tx_retries} retried and ${c.tx_failed} failed of ${r.sent} frames sent` }, `${pct(r.retried)} retried`),
		r.failed >= 1 && h('div', { class: 'sub' }, `${pct(r.failed)} failed`),
	];
}

// rate writes a client's rates, down (the AP sending to it) and up, in
// Mbit/s, with the MCS of each, and the spatial streams where the AP says.
function rate(c) {
	const one = (r, mcs, nss) => r == null ? '—' : `${Math.round(r)}${mcs != null ? ` MCS ${mcs}${nss ? `×${nss}` : ''}` : ''}`;
	return [`↓ ${one(c.tx_rate, c.tx_mcs, c.tx_nss)}`, h('div', { class: 'sub' }, `↑ ${one(c.rx_rate, c.rx_mcs, c.rx_nss)} Mbit/s`)];
}

// duration writes seconds the short way: 47 s, 12 min, 3 h 5 min, 2 d 4 h.
function duration(s) {
	s = Math.floor(s);
	if (s < 60) return `${s} s`;
	if (s < 3600) return `${Math.floor(s / 60)} min`;
	if (s < 86400) return `${Math.floor(s / 3600)} h ${Math.floor((s % 3600) / 60)} min`;
	return `${Math.floor(s / 86400)} d ${Math.floor((s % 86400) / 3600)} h`;
}

// size writes bytes the short way: 812 B, 1.9 MB, 83.7 GB.
function size(n) {
	for (const [d, unit] of [[1e12, 'TB'], [1e9, 'GB'], [1e6, 'MB'], [1e3, 'kB']])
		if (n >= d) return `${(n / d).toFixed(1)} ${unit}`;
	return `${n} B`;
}
