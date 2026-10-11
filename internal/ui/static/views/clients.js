// The Clients tab (0066): every Wi-Fi client on an AP, or on the APs below a
// folder, as each last reported. Who it is, with its maker and a guess at
// what it is (0067); where and how it connects, the 802.11 features it
// supports, and how often frames to it are retried; its data; and how it
// does with DHCP (0065). One compact row a client; filters by network and AP,
// a search box, and sorting by any column. The filters stay as they are when
// the page refreshes.

import { h, link } from '../dom.js';
import { get, post } from '../api.js';
import { flash } from '../refresh.js';
import { bandName, ago, size } from '../format.js';
import { configs } from './sections.js';
import { ask, confirm } from './confirm.js';
import { journeyPanel } from './journey.js';
import { outcomeChip } from './connections.js';

const DHCP = { ok: ['ok', 'DHCP'], static: ['idle', 'static'], none: ['bad', 'no address'], unknown: ['idle', 'not yet'] };

// What the person chose, kept across the page's refreshes.
const view = { network: '', ap: '', q: '', sort: 'connected', up: true, which: 'now', find: '' };

// clientsTab draws the tab: of one AP (ap: {ap, cfg}), or of every AP below
// a folder's page. It shows the clients on now, or everyone ever seen here
// (0118); either way a client opens its journey and its connections.
export async function clientsTab(ctx, page, ap) {
	const rows = ap ? [ap] : await configs(page.hardware?.aps || []);
	if (!rows.length) return h('div', { class: 'banner info' }, 'No APs here yet.');
	const now = Date.now() / 1000;
	const all = rows.flatMap(({ ap: a, cfg }) => {
		const st = cfg?.condition?.state;
		const since = st ? Math.max(0, now - new Date(st.at).getTime() / 1000) : 0;
		// Connected for as long as it was at the report, and since.
		return (st?.report?.clients || []).map((c) => ({ ...c, ap: a, cfg, at: st.at, connected: c.connected + since }));
	});
	const under = ap ? ap.ap.id : page.node?.id;
	const box = h('div');
	// A block's preview, or a client's journey, opens here, above the
	// table (0100, 0103).
	const out = h('div', { class: 'edit' });
	const bar = h('div');
	const note = h('span', { class: 'note' });
	const which = h('span', { class: 'segmented', role: 'group', 'aria-label': 'Which clients' });
	const draw = () => {
		which.replaceChildren(...[['now', 'On now'], ['all', 'Everyone seen']].map(([k, name]) => h('button', {
			type: 'button', class: view.which === k ? 'on' : '', 'aria-pressed': view.which === k, onclick: () => { view.which = k; draw(); },
		}, name)));
		if (view.which === 'all') {
			note.textContent = 'every client an AP here has seen, the latest first; a client opens its journey and each time it came online';
			everyone(under, !ap, bar, box, out);
			return;
		}
		note.textContent = 'as each AP last reported; signal, rates and data refresh every few minutes; a client opens its journey';
		bar.replaceChildren(all.length ? controls(all, !ap, draw) : '');
		box.replaceChildren(all.length ? table(ctx, all, !ap, draw, out)
			: h('p', { class: 'sub pad' }, 'No Wi-Fi client is on here now. An AP whose agent is older than v0.33.0 doesn\'t report them.'));
	};
	draw();
	return h('section', { class: 'panel' },
		h('h2', null, h('span', { class: 'title' }, 'Wi-Fi clients', which), note),
		bar, out, box);
}

// everyone draws every client ever seen below a node (0118), the latest
// first, a hundred at a time: when and where each was last seen, what it
// says of itself, and how its attempts to come online went.
async function everyone(under, folder, bar, box, out) {
	let rows = [], total = 0;
	const count = h('span', { class: 'note' });
	const q = h('input', { type: 'search', placeholder: 'Name, user, MAC or address', value: view.find, 'data-kept': true });
	bar.replaceChildren(h('div', { class: 'filters' }, q, count));
	const paint = (err) => {
		count.textContent = err || `${total} client${total === 1 ? '' : 's'}`;
		box.replaceChildren(h('table', { class: 'list clients' },
			h('tr', null, ['Client', 'Last seen', folder && 'Last on', 'Network', 'Address', 'Came online', 'Last time'].filter(Boolean).map((t) => h('th', null, t))),
			rows.map((c) => h('tr', null,
				h('td', null, h('a', { href: '#', class: 'clientlink', title: 'Its journey, and each time it came online',
					onclick: (e) => { e.preventDefault(); out.scrollIntoView({ block: 'nearest' }); journeyPanel(out, c.mac); } },
				c.host || h('span', { class: 'mono' }, c.mac)), c.host && h('div', { class: 'sub mono' }, c.mac),
				c.user && h('div', { class: 'sub' }, 'signed in as ', h('strong', null, c.user)),
				(c.maker || c.private) && h('div', { class: 'sub' }, c.private ? 'private address' : c.maker)),
				h('td', null, ago(c.last_seen), h('div', { class: 'sub' }, `first ${ago(c.first_seen)}`)),
				folder && h('td', null, c.ap_name ? link(`/aps/${encodeURIComponent(c.ap)}`, c.ap_name) : c.ap),
				h('td', null, c.ssid || c.network || '—'),
				h('td', { class: 'mono' }, c.address || '—'),
				h('td', null, c.attempts ? `${c.attempts} time${c.attempts === 1 ? '' : 's'}` : '—',
					c.failed > 0 && h('div', { class: 'sub' }, h('span', { class: 'chip bad' }, `${c.failed} failed`))),
				h('td', null, c.last_outcome ? outcomeChip({ outcome: c.last_outcome, stage: '', took_ms: 0 }, true) : '—'))),
			!rows.length && h('tr', null, h('td', { colspan: folder ? 7 : 6, class: 'sub' }, err ? '' : view.find ? 'No client matches.' : 'No client has been seen here yet.'))),
		rows.length < total && h('div', { class: 'below' }, h('button', { type: 'button', class: 'button', onclick: () => load(true) }, `More (${total - rows.length} left)`)));
	};
	const load = async (more) => {
		try {
			const r = await get(`/v1/clients?under=${encodeURIComponent(under || '')}&q=${encodeURIComponent(view.find)}&limit=100&offset=${more ? rows.length : 0}`);
			rows = more ? rows.concat(r.clients) : r.clients;
			total = r.total;
			paint();
		} catch (e) {
			paint(e.message);
		}
	};
	let timer;
	q.addEventListener('input', () => {
		view.find = q.value.trim();
		clearTimeout(timer);
		timer = setTimeout(() => load(false), 250);
	});
	await load(false);
}

function controls(all, folder, draw) {
	const pick = (key, label, values) => {
		// data-kept: the page keeps a filter across redraws (refresh.js).
		const sel = h('select', { 'data-kept': true, onchange: () => { view[key] = sel.value; draw(); } },
			h('option', { value: '' }, label),
			values.map(([v, name]) => h('option', { value: v, selected: view[key] === v }, name)));
		return sel;
	};
	const ssids = [...new Set(all.map((c) => c.ssid).filter(Boolean))].sort();
	const aps = [...new Map(all.map((c) => [c.ap.id, c.ap.name])).entries()].sort((a, b) => a[1].localeCompare(b[1]));
	const q = h('input', { type: 'search', placeholder: 'Name, user, MAC or address', value: view.q, 'data-kept': true, oninput: () => { view.q = q.value; draw(); } });
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

function table(ctx, all, folder, draw, out) {
	const q = view.q.trim().toLowerCase();
	const shown = all.filter((c) => (!view.network || c.ssid === view.network) && (!view.ap || c.ap.id === view.ap) &&
		(!q || [c.host, c.mac, c.address, c.ssid, c.maker, c.kind, c.os, c.user].some((x) => (x || '').toLowerCase().includes(q))));
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
			h('td', null, h('a', { href: '#', class: 'clientlink', title: 'Its journey: sessions, roams and issues over the last day', onclick: (e) => { e.preventDefault(); out.scrollIntoView({ block: 'nearest' }); journeyPanel(out, c.mac); } },
				c.host || h('span', { class: 'mono' }, c.mac)), c.host && h('div', { class: 'sub mono' }, c.mac),
					c.user && h('div', { class: 'sub', title: 'who it signed in as, by 802.1X' }, 'signed in as ', h('strong', null, c.user)), who(c),
					h('div', { class: 'rowbuttons' }, reconnectButton(ctx, c, out), blockButton(ctx, c, out))),
			folder && h('td', null, link(`/aps/${encodeURIComponent(c.ap.id)}`, c.ap.name)),
			h('td', null, c.ssid || '—', c.vlan && h('div', { class: 'sub', title: vlanBy(c) }, `VLAN ${c.vlan}`)),
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

// vlanBy says what put a client in its VLAN: on WPA Enterprise, the RADIUS
// server (0111); otherwise its per-user key (0070).
function vlanBy(c) {
	const security = c.network && c.cfg?.networks?.[c.network]?.fields?.security?.value;
	return String(security || '').endsWith('-enterprise') ? 'the RADIUS server put it in this VLAN' : 'a per-user key put it in this VLAN';
}

// blockButton blocks a client from its network, by MAC (0100), where Aeolus
// gives the network: it joins the network's blocked list where that is set,
// else where the network is, in one change previewed first.
function blockButton(ctx, c, out) {
	const net = c.network && c.cfg?.networks?.[c.network];
	if (!net || !c.mac) return null;
	const f = net.fields?.blocked;
	const node = f?.from ?? net.from;
	// Only for someone who may change the network there (0030).
	if (!mayEdit(ctx, 'services', node)) return null;
	const now = f?.value ?? [];
	if (now.map((m) => m.toLowerCase()).includes(c.mac.toLowerCase())) return h('span', { class: 'chip bad' }, 'blocked');
	const block = async () => {
		const op = { kind: 'set', tree: 'services', node, path: `network.${c.network}.blocked`, value: [...now, c.mac.toLowerCase()] };
		out.scrollIntoView({ block: 'nearest' });
		const p = await ask(out, op);
		if (!p) return;
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, `Block ${c.host || c.mac} from ${c.ssid || c.network}`)),
			h('div', { class: 'sub' }, `Every AP that offers ${c.ssid || c.network} refuses ${c.mac}, on every band. It is set on ${ctx.name('services', node)}, and takes it off the air within about a minute.`),
			c.private && h('div', { class: 'sub warn' }, 'Its MAC is private: the device may pick a new one, and join again.'),
		], [h('div', { class: 'sub warn' }, 'Applying reloads the Wi-Fi of each AP listed: its clients drop for a moment.')]);
	};
	return h('button', { type: 'button', class: 'button small', title: `Refuse ${c.mac} on ${c.ssid || c.network}`, onclick: block }, 'Block');
}

// reconnectButton asks the client's AP to disconnect it, once (0107): it
// drops off every network it is on there, and may join again at once,
// usually within seconds. For someone with operator on the AP (0104).
function reconnectButton(ctx, c, out) {
	if (!c.mac || !mayEdit(ctx, 'locations', c.ap.id)) return null;
	const who = c.host || c.mac;
	const go = async (e) => {
		e.currentTarget.disabled = true;
		try {
			await post(`/v1/aps/${encodeURIComponent(c.ap.id)}/actions`, { kind: 'disconnect', target: c.mac });
			flash(`Reconnect: asked of ${c.ap.name}. ${who} drops on its next poll, within about a minute, and joins again.`);
		} catch (err) {
			out.replaceChildren(h('div', { class: 'error' }, err.message));
		}
	};
	return h('button', { type: 'button', class: 'button small', title: `Have ${c.ap.name} disconnect ${who}, so it joins again`, onclick: go }, 'Reconnect');
}

// mayEdit says whether the person signed in may change a node: an operator
// or admin grant on it or above it (0030). The manager decides; this only
// keeps a button that would be refused off the page.
export function mayEdit(ctx, tree, node) {
	const up = new Set();
	for (let n = ctx.trees[tree]?.nodes.get(node); n; n = ctx.trees[tree].nodes.get(n.parent)) up.add(n.id);
	return (ctx.who?.grants || []).some((g) => g.tree === tree && up.has(g.node) && (g.role === 'operator' || g.role === 'admin'));
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

