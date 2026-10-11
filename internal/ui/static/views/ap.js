// One AP: whether it runs what it should, its settings in the same tabs as a
// folder's (0047), what it reported, and its history (0039, 0041).
//
// Its Overview is kept quiet (Griff, 2026-10-11): how it is at a glance, a
// tile each; what it can be asked to do; its usage. What it said of itself
// when it enrolled, its latest check and apply, and its history are closed
// until asked for. Its client tests have a tab of their own, as a folder's.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { bandName, when, ago, secondsAgo, uplinkJudgment, vlanUsers, switchPort, dhcpWarnings, templateSays } from '../format.js';
import { treeAside, crumbs, apStatus, fleetMap, tabBar, pick, keepPath, moved } from '../layout.js';
import { editing } from './fields.js';
import { followButton } from './follow.js';
import { systemSection } from './sections.js';
import { networksTab } from './networks.js';
import { interfacesTab } from './interfaces.js';
import { clientsTab } from './clients.js';
import { renameButton } from './rename.js';
import { moveButton } from './move.js';
import { actionsPanel } from './actions.js';
import { canWire, clientTestPanel, hasScanRadio } from './clienttest.js';
import { usagePanel } from './usage.js';
import { healthTab } from './health.js';
import { tile } from './overview.js';

const TABS = [['overview', 'Overview'], ['interfaces', 'Interfaces'], ['networks', 'Networks'], ['clients', 'Clients'], ['health', 'Health'], ['system', 'System']];
// An AP that can test a network from a client's side (0121, 0128) has its
// tests under Assurance, as a folder has (0123).
const ASSURANCE = ['assurance', 'Assurance'];

// What is opened on an AP's Overview, by AP and by what it is, so the
// page's own redraw every half minute does not close it: a folded panel,
// and a check or the state reports in its history. With reading, the page
// is not drawn again while it is open: a config being read keeps its place.
// One left open inside a panel that is then folded is out of sight, and
// holds nothing.
const opened = new Set();
const marks = new WeakMap();
function kept(key, d, reading) {
	const mark = () => {
		if (reading && d.open && !d.closest('details:not([open])')) d.setAttribute('data-editing', 'reading');
		else d.removeAttribute('data-editing');
	};
	// What is inside it is marked again as it opens and closes.
	const within = () => d.querySelectorAll('details').forEach((x) => marks.get(x)?.());
	marks.set(d, mark);
	d.open = opened.has(key);
	mark();
	within();
	d.addEventListener('toggle', () => { if (d.open) opened.add(key); else opened.delete(key); mark(); within(); });
	return d;
}

export async function apPage(ctx, id, tab, sub, view) {
	const enc = encodeURIComponent(id);
	const [page, cfg, hist, fleet] = await Promise.all([
		get(`/v1/trees/locations/nodes/${enc}`),
		get(`/v1/aps/${enc}/config`),
		get(`/v1/aps/${enc}/history?limit=20`),
		get('/v1/aps'),
	]);
	const me = fleet.aps.find((a) => a.id === id);
	const st = me ? apStatus(me) : { chip: 'idle', label: 'Unknown', detail: '' };
	const cond = cfg.condition || {};
	const facts = page.facts || {};
	const edit = editing(ctx, 'locations', page);
	// Everything the AP sets for itself can go back to its folder in one
	// change (0046).
	const own = Object.keys(page.fields || {}).filter((p) => page.fields[p].origin === 'self').sort();
	const revertBox = h('div', { class: 'edit flush' });
	// Renamed, its hostname with it, or moved to another folder, by someone
	// who may change it (0076).
	const nameBox = h('div', { class: 'edit' });
	const base = `/aps/${enc}`;
	[tab, sub, view] = moved(base, tab, sub, view);
	const tests = hasScanRadio(page) || canWire(cfg);
	const tabs = tests ? [...TABS.slice(0, 4), ASSURANCE, ...TABS.slice(4)] : TABS;
	tab = pick(tabs, tab);
	const thisAP = { ap: { id, name: page.node.name }, cfg };
	// The template it follows is said with what it is; one it does not
	// follow whole has a banner of its own.
	const follows = cfg.template?.id && cfg.template.follows && !cfg.template.replaced?.length && !cfg.unassigned;
	const main = [
		crumbs(ctx, 'locations', page.ancestry),
		h('div', { class: 'head' },
			h('div', null,
				h('h1', null, page.node.name,
					edit && renameButton(ctx, 'locations', { id, name: page.node.name, kind: 'ap' }, nameBox, 'rename head'),
					edit && moveButton(ctx, 'locations', { id, name: page.node.name, kind: 'ap' }, nameBox, 'rename head')),
				h('div', { class: 'sub' }, ['AP', facts.model, cond.seen?.source].filter(Boolean).join(' · '),
					follows && [' · ', h('a', { href: `#/library/${encodeURIComponent(cfg.template.id)}`,
						title: cfg.template.at ? `Picked ${cfg.template.at === id ? 'on this AP' : `at ${ctx.name('locations', cfg.template.at)}`}` : null }, `template ${cfg.template.name}`)])),
			edit && own.length > 0 && followButton(ctx, 'locations', id, page.node.name, edit.parentName, own, revertBox,
				`Revert to ${edit.parentName} (${own.length} custom setting${own.length === 1 ? '' : 's'})`)),
		revertBox,
		nameBox,
		statusPanel(st, cfg, cond),
		trouble(cond),
		!follows && templateNote(ctx, id, cfg),
		cfg.check?.problems?.length > 0 && h('div', { class: 'banner problems' },
			h('strong', null, 'Its config breaks these rules, so it is not sent'),
			h('ul', null, cfg.check.problems.map((p) => h('li', null, p)))),
		tunnelTrouble(cfg, base),
		clockTrouble(cfg, base),
		tabBar(base, tabs, tab),
	];
	if (tab === 'overview') {
		main.push(glance(cond, base, page, cfg),
			await actionsPanel(id, page.node.name, !!edit),
			await usagePanel(id, false),
			about(id, cond, facts, cfg),
			history(id, hist));
	} else if (tab === 'assurance') {
		// An AP with a scan radio can test a network as a client would
		// (0121); any whose agent says so can over the wire (0128).
		main.push(await clientTestPanel(ctx, id, page, cfg, !!edit));
	} else if (tab === 'interfaces') main.push(await interfacesTab(ctx, base, id, page, sub, view, thisAP, edit));
	else if (tab === 'networks') main.push(await networksTab(ctx, id, page));
	else if (tab === 'clients') main.push(await clientsTab(ctx, page, thisAP));
	else if (tab === 'health') main.push(await healthTab(ctx, id));
	else main.push(await systemSection(ctx, id, page, edit));
	const keep = tab === 'overview' ? '' : keepPath(tab, sub, view);
	return { aside: treeAside(ctx, 'locations', id, fleetMap(fleet.aps), keep), main, refresh: 30 };
}

// tunnelTrouble warns of a tunnel the AP's prober finds down, and of a port
// its loop guard took off its tunnels (0059); of a network carried by its
// fallback, or that cannot switch to it (0061); and of a VLAN the AP carries
// that doesn't reach it from the switch, with what needs it and the switch
// port to look at (0064); and of a network whose DHCP isn't answering, has
// more than one server, or leaves a client with no address (0065). The
// Tunnels, Networks and Ethernet views say more.
function tunnelTrouble(cfg, base) {
	const st = cfg.condition?.state;
	const r = st?.report;
	const x = r?.vxlan;
	const down = (x?.tunnels || []).filter((t) => t.probe?.verdict === 'down');
	const loops = x?.loops || [];
	const nets = Object.entries(r?.transports || {}).sort().filter(([, t]) => t.active === 'fallback' || t.cannot_switch);
	const ssid = (id) => cfg.document?.network?.[id]?.ssid || id;
	const port = switchPort(r?.uplink_neighbor);
	const vlans = (r?.uplink_vlans || []).map((v) => [v, uplinkJudgment(v, r.uplink_neighbor)[2]]).filter(([, says]) => says);
	const dhcp = Object.entries(r?.dhcp || {}).sort().flatMap(([id, d]) => dhcpWarnings(ssid(id), d));
	// A RADIUS server that does not answer this AP (0114).
	const radius = (r?.radius || []).filter((x) => x.verdict === 'silent');
	if (!down.length && !loops.length && !nets.length && !vlans.length && !dhcp.length && !radius.length) return null;
	return h('div', { class: 'banner problems' },
		h('strong', null, radius.length ? 'Its uplink, transports, sign-in or DHCP need a look' : 'Its uplink, transports or DHCP need a look'),
		h('ul', null,
			down.map((t) => h('li', null, `The tunnel to ${t.peer}, VNI ${t.vni}, is down: ${t.probe.underlay === false ? `${t.peer} cannot be reached` : `nothing on VNI ${t.vni} answers`}.`)),
			loops.map((l) => h('li', null, `${l.port} is off its tunnels: VNI ${l.vni ?? '?'} loops.`)),
			nets.map(([id, t]) => h('li', null, t.active === 'fallback'
				? `${ssid(id)} is on its fallback${t.last_switch?.to === 'fallback' ? `, switched ${secondsAgo(t.last_switch.ago, st.at)}: ${t.last_switch.why}` : ''}.`
				: `${ssid(id)} cannot switch: ${t.cannot_switch}.`)),
			vlans.map(([v, says]) => {
				const users = vlanUsers(cfg.document, v.vlan);
				return h('li', null, `${says}.${users.length ? ` ${users.join(', ')} ${users.length === 1 ? 'needs' : 'need'} it.` : ''}${port ? ` The AP is on ${port}.` : ''}`);
			}),
			radius.map((x) => h('li', null, `The RADIUS server ${x.server} does not answer this AP, so no one can sign in to ${ssid(x.network)} here: it is down, out of reach, or does not know this AP or its secret.`)),
			dhcp.map((line) => h('li', null, line + '.'))),
		h('div', null, link(`${base}/interfaces/tunnels`, 'Interfaces › Tunnels'), ' · ', link(`${base}/interfaces/ethernet`, 'Interfaces › Ethernet'), ' · ', link(`${base}/networks`, 'Networks')));
}

// clockTrouble warns of an AP whose clock isn't synchronized, or that has no
// time server to ask (0069): its TLS to the manager, and every time it
// reports, depend on the clock. The site's own time servers are set under
// System, or given by its DHCP.
function clockTrouble(cfg, base) {
	const t = cfg.condition?.state?.report?.time;
	if (!t) return null;
	const says = t.synced === false
		? `Its clock isn't synchronized: ntpd ${t.servers?.length ? `asks ${t.servers.join(', ')}` : 'has no time server'}.`
		: t.synced == null && !t.servers?.length ? 'It has no time server, so its clock is never set.' : null;
	if (!says) return null;
	return h('div', { class: 'banner problems' },
		h('strong', null, 'Its clock needs a look'),
		h('div', null, says, ' Set its time servers under ', link(`${base}/system`, 'System'), ", or have the site's DHCP give one."));
}

// keysLine says whether the AP has the per-user keys it should (0070), from
// the version it last reported.
function keysLine(cfg) {
	const want = cfg.keys;
	const have = cfg.condition?.state?.report?.keys;
	if (!want || (!want.count && !have)) return null;
	const current = have?.version === want.version;
	return [' · ', h('span', { class: current ? null : 'warn', title: current ? 'its keys are the latest' : 'its key agent has not yet reported the latest keys' },
		`${want.count} per-user ${want.count === 1 ? 'key' : 'keys'}${current ? '' : ', not yet on the AP'}`)];
}

// statusPanel is how the AP stands, in a line: its state, why, and when it
// was last seen, where why does not say. Which version it runs, and from
// where it called, are under About; its per-user keys are said only where
// it has not the latest.
function statusPanel(st, cfg, cond) {
	const keys = cfg.keys && cfg.condition?.state?.report?.keys?.version !== cfg.keys.version ? keysLine(cfg) : null;
	const said = typeof st.detail === 'string' && /seen/i.test(st.detail);
	return h('section', { class: 'panel' },
		h('div', { class: 'status' },
			h('span', { class: 'chip ' + st.chip + ' big' }, st.label),
			h('div', { class: 'grow' }, st.detail),
			(!said || keys) && h('div', { class: 'detail' }, !said && (cond.seen ? `seen ${ago(cond.seen.at)}` : 'never seen'), keys)));
}

// trouble says what must not be folded away: an apply that failed, or
// that no check covered, and a render check the AP refused. Each is a
// critical alert (0041), and an AP's page has no Alerts tab.
function trouble(cond) {
	const c = cond.check, a = cond.apply;
	const lines = [
		a && !a.ok && `Its last apply failed: version ${a.version}, ${ago(a.at)}${a.error ? `. ${a.error}` : ''}`,
		a && a.ok && !a.checked && `Its last apply, of version ${a.version}, ${ago(a.at)}, was not covered by an ok check`,
		c && c.result !== 'ok' && c.result !== 'stale' && `Its last render check came to ${c.result}: version ${c.version}, ${ago(c.at)}`,
	].filter(Boolean);
	return lines.length ? h('div', { class: 'banner problems' }, lines.map((l) => h('div', null, l))) : null;
}

// glance is the AP at a glance, a tile each, from its latest report: its
// clients, its radios and their channels, its uplink, how long it has been
// up, and how hard it works. Nothing where it has not reported.
function glance(cond, base, page, cfg) {
	const r = cond.state?.report;
	if (!r) return null;
	// Its radios that serve: not one kept for scanning (0081), which the
	// AP reports as a radio that is not up. One its config turns off is
	// off, and no fault.
	const scans = new Set((page.facts?.radios || []).filter((x) => x.reserved).map((x) => x.radio));
	const radios = (r.radios || []).filter((x) => !scans.has(x.radio));
	const off = (x) => cfg.document?.radio?.[x.band]?.enabled === false;
	const on = radios.filter((x) => x.up !== false);
	const down = radios.filter((x) => x.up === false && !off(x));
	const band = (x) => bandName(x.band).replace(' GHz', '');
	const clients = r.clients ? r.clients.length : radios.reduce((n, x) => n + (x.clients || 0), 0);
	const byBand = ['2g', '5g', '6g'].map((b) => [b, (r.clients || []).filter((c) => c.band === b).length]).filter(([, n]) => n);
	const up = (r.ports || []).find((p) => p.path) || (r.ports || []).find((p) => p.uplink);
	const m = /^(\d+)([FH])$/.exec(up?.speed || '');
	const speed = m ? (Number(m[1]) >= 1000 ? `${Number(m[1]) / 1000}G` : `${m[1]}M`) : null;
	const sw = r.uplink_neighbor;
	const x = r.health || {};
	// Memory in use, counted as Health counts what is free.
	const used = x.mem_total ? 100 - Math.floor(100 * (x.mem_available || 0) / x.mem_total) : null;
	const s = r.uptime;
	const upFor = s == null ? null : s >= 86400 ? `${Math.floor(s / 86400)} d ${Math.floor((s % 86400) / 3600)} h`
		: s >= 3600 ? `${Math.floor(s / 3600)} h ${Math.floor((s % 3600) / 60)} m` : `${Math.max(1, Math.round(s / 60))} min`;
	const hard = x.cpu >= 90 || used >= 90;
	const out = [
		tile('Clients', String(clients), clients === 0 ? 'none now' : byBand.map(([b, n]) => `${bandName(b)} ${n}`).join(' · '), '', `#${base}/clients`),
		radios.length > 0 && tile('Radios', `${on.length} of ${radios.length}`,
			radios.map((x) => `${band(x)} GHz ${off(x) ? 'off' : x.up === false ? 'down' : `ch ${x.channel || '—'}`}`).join(' · '), down.length ? 'warn' : '', `#${base}/interfaces/radios`),
		up && tile('Uplink', up.carrier ? speed || 'up' : 'no link', [up.name, sw && [sw.system, sw.port].filter(Boolean).join(' ')].filter(Boolean).join(' · '),
			up.carrier ? '' : 'bad', `#${base}/interfaces/ethernet`),
		upFor && tile('Up for', upFor, r.openwrt || '', ''),
		x.cpu != null ? tile('Processor', `${x.cpu}%`, used != null ? `memory ${used}%` : '', hard ? 'warn' : '', `#${base}/health`)
			: used != null && tile('Memory', `${used}%`, 'in use', hard ? 'warn' : '', `#${base}/health`),
	].filter(Boolean);
	if (!out.length) return null;
	// A report from a while ago is said to be: an AP not heard from since
	// is not as its tiles say now.
	const old = cond.state?.at && Date.now() - new Date(cond.state.at).getTime() > 10 * 60 * 1000;
	return [old && h('p', { class: 'sub tilesnote' }, `As it last reported, ${ago(cond.state.at)}.`), h('div', { class: `tiles${old ? ' stale' : ''}` }, out)];
}

// about is what is seldom asked of an AP, closed until it is: its latest
// check and apply, and what it said of itself when it enrolled.
function about(id, cond, facts, cfg) {
	return kept(`${id}/about`, h('details', { class: 'panel logpanel' },
		h('summary', null, h('h2', null, 'About this AP', h('span', { class: 'note' }, [facts.board, facts.mac].filter(Boolean).join(' · ')))),
		h('div', { class: 'grid2 inpanel' }, latest(cond, cfg), enrollment(facts))));
}

function latest(cond, cfg) {
	const c = cond.check;
	const a = cond.apply;
	return h('section', null,
		h('h3', { class: 'subhead' }, 'Latest check and apply'),
		h('dl', { class: 'kv' },
			h('dt', null, 'Config'),
			h('dd', null, `its version is ${cfg.version}${cond.seen?.running != null ? ` · running ${cond.seen.running}` : ''}`),
			h('dt', null, 'Last seen'),
			h('dd', null, cond.seen ? `${ago(cond.seen.at)} from ${cond.seen.source}` : 'never'),
			h('dt', null, 'Render check'),
			h('dd', null, c ? `${c.result} · version ${c.version} · ${ago(c.at)}` : 'none yet'),
			h('dt', null, 'Apply'),
			h('dd', null, a ? `${a.ok ? 'worked' : 'failed'} · version ${a.version} · ${ago(a.at)}${a.checked ? '' : ' · NOT covered by an ok check'}` : 'none yet'),
			a?.error && [h('dt', null, 'Error'), h('dd', null, a.error)],
			h('dt', null, 'Uptime'),
			h('dd', null, cond.state?.report?.uptime != null ? uptime(cond.state.report.uptime) : '—'),
			h('dt', null, 'OpenWrt'),
			h('dd', null, cond.state?.report?.openwrt || '—')));
}

function uptime(s) {
	const d = Math.floor(s / 86400), hr = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
	return [d && d + ' d', hr && hr + ' h', m + ' min'].filter(Boolean).join(' ');
}

// enrollment is what the AP said about itself when it enrolled (0033).
function enrollment(facts) {
	if (!facts.mac) return null;
	return h('section', null,
		h('h3', { class: 'subhead' }, 'Enrollment'),
		h('dl', { class: 'kv' },
			h('dt', null, 'MAC'), h('dd', null, facts.mac),
			h('dt', null, 'All MACs'), h('dd', null, (facts.macs || []).join(', ')),
			h('dt', null, 'Hostname'), h('dd', null, facts.hostname || '—'),
			h('dt', null, 'Board'), h('dd', null, facts.board || '—'),
			h('dt', null, 'Radios'), h('dd', null, (facts.radios || []).map((r) => `${r.radio} ${bandName(r.band)}${r.reserved ? ', a scan radio Aeolus leaves to its own service' : ''}: ${(r.htmodes || []).join(' ')}`).map((x) => h('div', null, x))),
			h('dt', null, 'Enrolled from'), h('dd', null, facts.source || '—')));
}

// history lists recent checks (with the UCI the AP sent, secrets blanked:
// 0041), applies and state reports, newest first.
function history(id, hist) {
	const checks = hist.checks || [];
	const applies = hist.applies || [];
	const states = hist.states || [];
	const many = (n, one, more) => `${n} ${n === 1 ? one : more}`;
	const said = [checks.length && many(checks.length, 'check', 'checks'), applies.length && many(applies.length, 'apply', 'applies'),
		states.length && many(states.length, 'state report', 'state reports')].filter(Boolean).join(' · ');
	return kept(`${id}/history`, h('details', { class: 'panel logpanel' },
		h('summary', null, h('h2', { title: 'Its latest checks, with the config it sent; its applies; and its state reports, newest first' },
			'History', h('span', { class: 'note' }, said ? `latest ${said}` : 'nothing yet'))),
		checks.map((c) => kept(`${id}/check/${c.at}/${c.hash}`, h('details', null,
			h('summary', null,
				h('span', { class: 'when' }, when(c.at)),
				h('span', { class: 'chip ' + (c.result === 'ok' ? 'ok' : c.result === 'stale' ? 'idle' : 'bad') }, 'check ' + c.result),
				h('span', { class: 'grow' }, `version ${c.version}`, c.problems?.length ? ` · ${c.problems.length} problem${c.problems.length === 1 ? '' : 's'}` : ''),
				h('span', { class: 'mono' }, c.hash.slice(0, 12))),
			c.problems?.length > 0 && h('ul', { class: 'problems' }, c.problems.map((p) => h('li', null, p))),
			c.uci ? h('pre', { class: 'uci' }, c.uci) : h('div', { class: 'empty' }, 'No UCI kept for this check.')), true)),
		applies.map((a) => h('details', null,
			h('summary', null,
				h('span', { class: 'when' }, when(a.at)),
				h('span', { class: 'chip ' + (a.ok ? 'ok' : 'bad') }, a.ok ? 'applied' : 'apply failed'),
				h('span', { class: 'grow' }, `version ${a.version}`, a.checked ? '' : ' · not covered by an ok check', a.error ? ' · ' + a.error : ''),
				h('span', { class: 'mono' }, a.hash.slice(0, 12))))),
		states.length > 0 && kept(`${id}/states`, h('details', null,
			h('summary', null, h('span', { class: 'when' }, when(states[0].at)), h('span', { class: 'chip idle' }, 'state'),
				h('span', { class: 'grow' }, `${states.length} recent state reports`)),
			h('pre', { class: 'uci' }, states.map((s) => `${when(s.at)}  ${JSON.stringify(s.report)}`).join('\n'))), true),
		!checks.length && !applies.length && !states.length && h('div', { class: 'empty' }, 'Nothing yet.')));
}

// templateNote says which AP template the AP takes (0085), and whether it
// follows it: each of the template's settings that something set closer to
// the AP replaces, and where.
function templateNote(ctx, id, cfg) {
	const t = cfg.template;
	if (!t || cfg.unassigned) return null;
	if (!t.id) return h('div', { class: 'banner info' }, templateSays(t));
	const at = (node) => (node === id ? 'on this AP' : ['at ', link(`/locations/${encodeURIComponent(node)}`, ctx.name('locations', node))]);
	return h('div', { class: 'banner ' + (t.follows ? 'info' : 'branch'), style: { 'flex-direction': 'column', gap: '6px' } },
		h('div', null, h('strong', null, templateSays(t)), ' · picked ', at(t.at), ' · ', link(`/library/${encodeURIComponent(t.id)}`, 'the template')),
		t.replaced.length > 0 && h('ul', { style: { margin: '0', 'padding-left': '18px' } }, t.replaced.map((r) => h('li', null,
			h('span', { class: 'mono' }, r.path), ' is ', h('span', { class: 'mono' }, JSON.stringify(r.value)), ', set ', at(r.node)))));
}
