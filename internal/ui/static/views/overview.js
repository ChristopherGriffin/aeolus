// The Overview tab of a Locations folder or the Org (0102): the APs below it
// at a glance. How many are up, their clients by band, what needs attention,
// whether their configs are in force, whether their networks work when tried
// as a client (0123), how their clients roam (0136), their usage over the
// last day (0108),
// the busiest APs, the clients on each network, and the channels the APs
// share. It reads what the other tabs do,
// and sets nothing.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { apStatus } from '../layout.js';
import { bandName } from '../format.js';
import { configs } from './sections.js';
import { usagePanel } from './usage.js';
import { roamingPanel, showRoaming, took } from './roaming.js';

const BANDS = ['2g', '5g', '6g'];

export function tile(label, big, sub, cls, href) {
	const body = [h('div', { class: 'tlabel' }, label), h('div', { class: 'tbig' }, big), sub && h('div', { class: 'tsub' }, sub)];
	return href ? h('a', { class: `tile ${cls || ''}`, href }, body) : h('div', { class: `tile ${cls || ''}` }, body);
}

// assured is the service as the client tests find it (0123): of the things
// the APs here test, how many passed their last test, and how many are
// failing, which is their last two. No tile where no AP here can test.
function assured(a, base) {
	if (!a?.testers?.length) return null;
	const tried = a.checks.filter((c) => c.last?.detail);
	const passed = tried.filter((c) => c.last.detail.outcome === 'passed').length;
	const failing = a.checks.filter((c) => c.failing).length;
	// With no AP asked for tests any more, what the last ones found is not
	// how the service is now: only a thing still failing is said.
	const asked = a.testers.some((t) => t.every);
	if (!asked && !failing) return tile('Assurance', '—', 'no schedule set', '', `${base}/assurance`);
	const sub = failing ? `${failing} failing`
		: !tried.length ? 'no test made yet'
			: passed === tried.length ? 'tried as a client, all work' : `${tried.length - passed} failed its last test`;
	return tile('Assurance', tried.length ? `${passed} of ${tried.length}` : '—', sub, failing ? 'bad' : passed < tried.length ? 'warn' : tried.length ? 'ok' : '', `${base}/assurance`);
}

// gotOn is the service as the clients' own attempts find it (0127): of the
// clients that got on a network or did not, over the last day, how many
// did; bad where clients cannot get on one now. No tile where none tried.
function gotOn(a, base) {
	const joins = a?.joins || [];
	const online = joins.reduce((n, j) => n + j.online, 0), failed = joins.reduce((n, j) => n + j.failed, 0);
	if (!online && !failed) return null;
	const stuck = joins.filter((j) => j.failing);
	const sub = stuck.length ? `cannot get on ${stuck.map((j) => j.ssid || j.network).join(', ')}`
		: failed ? `${failed} did not, on ${joins.filter((j) => j.failed).map((j) => j.ssid || j.network).join(', ')}` : 'every client that tried, today';
	return tile('Clients getting on', `${online} of ${online + failed}`, sub, stuck.length ? 'bad' : failed ? 'warn' : 'ok', `${base}/assurance`);
}

// roamed is how the clients here roam (0136): how long a roam typically
// takes at the AP a client comes to, and how many there were over the last
// day; amber where some failed or were slow, red where many failed. It
// opens the panel below. No tile where no client roamed.
function roamed(r, panel) {
	if (!panel) return null;
	const sub = [`${r.roams} roam${r.roams === 1 ? '' : 's'} today`, r.failed > 0 && `${r.failed} failed`, r.slow > 0 && `${r.slow} slow`].filter(Boolean).join(' · ');
	const many = r.failed >= 3 && r.failed * 10 >= r.roams;
	const t = tile('Roaming', r.typical_ms != null ? took(r.typical_ms) : '—', sub, many ? 'bad' : r.failed || r.slow ? 'warn' : 'ok', '#');
	t.title = 'How long a roam typically took, at the AP the client came to, from its authentication there to its being let on. Opens the roams below';
	t.addEventListener('click', (e) => { e.preventDefault(); showRoaming(panel); });
	return t;
}

export async function overviewTab(ctx, id, page, fleet) {
	const below = page.hardware?.aps || [];
	if (!below.length) return [h('div', { class: 'banner info' }, 'No APs here yet.')];
	const ids = new Set(below.map((a) => a.id));
	const aps = (fleet?.aps || []).filter((a) => ids.has(a.id));
	const [rows, al, usage, assurance, roams] = await Promise.all([configs(below), get(`/v1/alerts?under=${encodeURIComponent(id)}`), usagePanel(id, true),
		get(`/v1/assurance?under=${encodeURIComponent(id)}`).catch(() => null),
		// An older manager has none.
		get(`/v1/roaming?under=${encodeURIComponent(id)}`).catch(() => null)]);
	const roaming = roamingPanel(roams);
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
		tile('Configs in force', `${inSync} of ${st.length}`, held ? `${held} held` : 'none held', held ? 'warn' : inSync === st.length ? 'ok' : 'warn', `${base}/aps`),
		assured(assurance, base), gotOn(assurance, base), roamed(roams, roaming));

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
		roaming,
		usage,
		h('div', { class: 'grid2' }, h('div', { class: 'col' }, busy), h('div', { class: 'col' }, netList, chans)),
	].filter(Boolean);
}
