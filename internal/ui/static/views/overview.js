// The Overview tab of a Locations folder or the Org (0102): the APs below it
// at a glance. How many are up, their clients by band, what needs attention,
// whether their configs are in force, the busiest APs, the clients on each
// network, and the channels the APs share. It reads what the other tabs do,
// and sets nothing.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { apStatus } from '../layout.js';
import { bandName } from '../format.js';
import { configs } from './sections.js';

const BANDS = ['2g', '5g', '6g'];

function tile(label, big, sub, cls, href) {
	const body = [h('div', { class: 'tlabel' }, label), h('div', { class: 'tbig' }, big), sub && h('div', { class: 'tsub' }, sub)];
	return href ? h('a', { class: `tile ${cls || ''}`, href }, body) : h('div', { class: `tile ${cls || ''}` }, body);
}

export async function overviewTab(ctx, id, page, fleet) {
	const below = page.hardware?.aps || [];
	if (!below.length) return [h('div', { class: 'banner info' }, 'No APs here yet.')];
	const ids = new Set(below.map((a) => a.id));
	const aps = (fleet?.aps || []).filter((a) => ids.has(a.id));
	const [rows, al] = await Promise.all([configs(below), get(`/v1/alerts?under=${encodeURIComponent(id)}`)]);
	const base = `#/locations/${encodeURIComponent(id)}`;

	// APs: up is heard from lately and not idle in Landing Zone.
	const st = aps.map((a) => ({ a, s: apStatus(a) }));
	const down = st.filter((x) => x.s.label === 'Not heard from' || x.s.label === 'Never seen');
	const up = st.length - down.length;
	const inSync = st.filter((x) => x.a.in_sync === true).length;
	const held = st.filter((x) => x.a.config === 'held').length;

	// Clients, from each AP's last report.
	const reports = rows.map(({ ap, cfg }) => ({ ap, r: cfg?.condition?.state?.report }));
	const clients = reports.flatMap(({ ap, r }) => (r?.clients || []).map((c) => ({ ...c, ap })));
	const byBand = Object.fromEntries(BANDS.map((b) => [b, clients.filter((c) => c.band === b).length]));
	const c = al.counts || {};

	const tiles = h('div', { class: 'tiles' },
		tile('APs up', `${up} of ${st.length}`, down.length ? `${down.length} not heard from` : 'all calling in', down.length ? 'bad' : 'ok', `${base}/aps`),
		tile('Wi-Fi clients', String(clients.length), BANDS.filter((b) => byBand[b]).map((b) => `${bandName(b)} ${byBand[b]}`).join(' · ') || 'none now', '', `${base}/clients`),
		tile('Alerts', String((c.critical || 0) + (c.warning || 0)), `${c.critical || 0} critical · ${c.warning || 0} warning`,
			c.critical ? 'bad' : c.warning ? 'warn' : 'ok', `${base}/alerts`),
		tile('Configs in force', `${inSync} of ${st.length}`, held ? `${held} held` : 'none held', held ? 'warn' : inSync === st.length ? 'ok' : 'warn', `${base}/aps`));

	// The busiest APs.
	const busiest = reports.map(({ ap, r }) => ({ ap, n: (r?.clients || []).length, radios: r?.radios || [] }))
		.sort((x, y) => y.n - x.n).slice(0, 6);
	const busy = h('section', { class: 'panel' }, h('h2', null, 'Busiest APs'),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'Clients', ...BANDS.map(bandName)].map((t) => h('th', null, t))),
			busiest.map(({ ap, n, radios }) => h('tr', null,
				h('td', null, link(`/locations/${encodeURIComponent(ap.id)}`, ap.name)),
				h('td', null, String(n)),
				BANDS.map((b) => {
					const rs = radios.filter((x) => x.band === b);
					return h('td', { class: 'sub' }, rs.length ? rs.map((x) => `ch ${x.channel}${x.width ? `/${x.width}` : ''} · ${x.clients ?? 0}`).join(', ') : '—');
				})))));

	// Clients on each network.
	const nets = new Map();
	for (const cl of clients) {
		const k = cl.ssid || '(the AP\'s own)';
		if (!nets.has(k)) nets.set(k, { n: 0, bands: {} });
		const e = nets.get(k);
		e.n++;
		e.bands[cl.band] = (e.bands[cl.band] || 0) + 1;
	}
	const netList = h('section', { class: 'panel' }, h('h2', null, 'Clients by network'),
		nets.size ? h('table', { class: 'list' },
			h('tr', null, ['Network', 'Clients', ...BANDS.map(bandName)].map((t) => h('th', null, t))),
			[...nets.entries()].sort((x, y) => y[1].n - x[1].n).map(([ssid, e]) => h('tr', null,
				h('td', null, ssid), h('td', null, String(e.n)), BANDS.map((b) => h('td', { class: 'sub' }, String(e.bands[b] || 0))))))
			: h('p', { class: 'sub' }, 'No Wi-Fi clients reported here now.'));

	// The channels the APs use, by band: a channel more than one AP shares is
	// marked, as their clients share its airtime.
	const chans = h('section', { class: 'panel' }, h('h2', null, 'Channels in use'),
		BANDS.map((b) => {
			// Each AP once a channel, though it may have two radios on it.
			const on = new Map();
			for (const { ap, r } of reports)
				for (const x of (r?.radios || []).filter((x) => x.band === b && x.channel)) {
					if (!on.has(x.channel)) on.set(x.channel, []);
					if (!on.get(x.channel).includes(ap.name)) on.get(x.channel).push(ap.name);
				}
			if (!on.size) return null;
			return h('div', { class: 'row' }, h('div', { class: 'label' }, bandName(b)),
				h('div', { class: 'value chanlist' }, [...on.entries()].sort((x, y) => x[0] - y[0]).map(([ch, names]) =>
					h('span', { class: `chip ${names.length > 1 ? 'warn' : ''}`, title: names.join(', ') + (names.length > 1 ? ': sharing it' : '') }, `${ch}${names.length > 1 ? ` ×${names.length}` : ''}`))));
		}));

	const trouble = (al.alerts || []).filter((a) => a.severity === 'critical').slice(0, 5);
	return [
		tiles,
		trouble.length > 0 && h('div', { class: 'banner problems' },
			h('strong', null, 'Critical'),
			h('ul', null, trouble.map((a) => h('li', null, link(`/locations/${encodeURIComponent(a.ap)}`, a.name), `: ${a.message}`)))),
		h('div', { class: 'grid2' }, h('div', { class: 'col' }, busy), h('div', { class: 'col' }, netList, chans)),
	].filter(Boolean);
}
