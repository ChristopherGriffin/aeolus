// The page is kept quiet (Griff, 2026-10-11): a heading is a name and a
// count, a line is a name, where and a state, and what a thing means is
// said on hover or once it is opened.
//
// The Assurance tab of a Locations folder (0123): the service tried from a
// client's side. The APs here that have a scan radio are asked for a client
// test (0121) on a schedule, each time another network, AP and band, and
// any AP for one over the wire (0128), where the folder says so; what
// each thing tested comes to is a line, the failing first, that opens to
// its latest test; and the latest tests follow, newest first. Before the
// tests, how the clients' own attempts to come online went (0127): what
// every AP records, a scan radio or none.

import { h, link } from '../dom.js';
import { get, post, schema } from '../api.js';
import { ago, bandName, value } from '../format.js';
import { flash, hurry, redrawNow } from '../refresh.js';
import { fieldPanels } from './fields.js';
import { only } from './sections.js';
import { systemEditor } from './system.js';
import { testChip, testLine, testOpened } from './clienttest.js';
import { typicalPath } from './way.js';

// What the tab sets: how often, which tests (0131), with speed or without,
// and the server a speed is measured against, which is the APs' own
// setting (0121). The setting before which tests, over the wire too, is
// set no more: it is shown with the others where a folder has it still,
// under Schedule, and is taken away there as any setting is (0046).
const SET = ['assurance.tests.every', 'assurance.tests.over', 'assurance.tests.speed', 'system.speed_server'];
const FIELDS = [...SET, 'assurance.tests.wire'];
const SECTIONS = [['Client tests on a schedule', SET]];
// The tests a round is asked for (0131), as the choice names them, and as
// a sentence does.
const OVER = [['both', 'Both'], ['air', 'Wireless'], ['wire', 'Wired']];
const OVERSAID = { air: ', through the air only', wire: ', over the wire only' };
// What was chosen last: kept here for as long as the page is open, as the
// tab is drawn again every few seconds, and in the browser for the next
// time, where it keeps anything.
const KEPT = 'aeolus.round.over';
let chosenOver = null;
function kept() {
	if (chosenOver) return chosenOver;
	try {
		const v = localStorage.getItem(KEPT);
		return OVER.some(([k]) => k === v) ? v : 'both';
	} catch { return 'both'; }
}
function keep(k) {
	chosenOver = k;
	try { localStorage.setItem(KEPT, k); } catch { /* storage blocked */ }
}

// asSet is the fields as the schedule's editor is shown them. Where which
// tests is not set and the setting before, over the wire too, is on, both
// is in force (0131): the editor is shown that, from where the setting
// before comes, so that choosing through the air there is a change.
function asSet(fields) {
	const before = fields['assurance.tests.wire'];
	if (fields['assurance.tests.over'] || before?.value !== true) return fields;
	// A lock on the setting before is not one on this: a lock is a field's.
	return { ...fields, 'assurance.tests.over': { ...before, value: 'both', origin: before.origin === 'locked' ? 'inherited' : before.origin } };
}

// The checks opened, by what they are of, so a redraw keeps them open.
const open = new Set();

// round is how long a tester takes to come back to the same thing.
function round(t) {
	const m = t.every * t.things;
	return m < 120 ? `${m} minutes` : m < 2880 ? `${Math.round(m / 60)} hours` : `${Math.round(m / 1440)} days`;
}

// tester says what an AP that tests is asked for (0131): through the air,
// with its scan radio; over the wire (0128), from its uplink; or both.
function tester(t) {
	const air = t.air && t.over !== 'wire';
	const how = air && t.wire ? 'has a scan radio, and tests over the wire too'
		: t.wire ? (t.air ? 'has a scan radio, and is asked for tests over the wire only' : 'tests over the wire, from its uplink')
			: air ? 'has a scan radio' : t.air ? 'has a scan radio, and is asked for tests over the wire, which its agent cannot make'
				: 'can test over the wire, from its uplink';
	// What it tests in a round asked by hand (0129) may be more than its
	// schedule's: over the wire, where its folders ask for none of that.
	const asked = t.round ? ` In a round asked by hand it tests ${many(t.round, 'thing')}.` : '';
	if (!t.things && !t.round) return `${t.name} ${how}, and offers no network a test can try.`;
	if (!t.every) return `${t.name} ${how}, and is not asked for tests on a schedule.${asked}`;
	if (!t.things) return `${t.name} ${how}, and has nothing its schedule can test.${asked}`;
	return `${t.name} ${how}. It is asked for one test ${value('assurance.tests.every', t.every)}${t.speed && air ? ', with speed through the air' : ''}: ${t.things} thing${t.things === 1 ? '' : 's'} in turn, so each about every ${round(t)}.`;
}

// testerRow is an AP that tests, as a line: its name, the tests it is asked
// for on its schedule, or can make when asked, and how often. The whole of
// it is said on hover.
function testerRow(t) {
	const scheduled = t.every > 0 && t.things > 0;
	const kinds = scheduled ? [t.air && t.over !== 'wire' && 'Wireless', t.wire && 'Wired'] : [t.air && 'Wireless', t.can_wire && 'Wired'];
	const when = scheduled ? `${value('assurance.tests.every', t.every)} · ${many(t.things, 'thing')}`
		: t.round ? `when asked · ${many(t.round, 'thing')}` : 'nothing to test';
	return h('tr', { title: tester(t) },
		h('td', null, t.name),
		h('td', null, kinds.filter(Boolean).map((k) => [h('span', { class: 'chip idle' }, k), ' '])),
		h('td', { class: 'sub' }, when));
}

const secs = (ms) => (ms < 1000 ? `${Math.round(ms)} ms` : `${(ms / 1000).toFixed(1)} s`);
const clock = (t) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

// span is the least, middle and most of a step's times over the tests that
// took it, as "0.5 to 0.7 s, mostly 0.6 s"; nothing with fewer than two.
function span(runs, key) {
	const v = runs.map((r) => r[key]).filter((x) => x != null && x > 0).sort((a, b) => a - b);
	if (v.length < 2) return null;
	const mid = v[Math.floor(v.length / 2)];
	return v[0] === v[v.length - 1] ? secs(mid) : `${secs(v[0])} to ${secs(v[v.length - 1])}, mostly ${secs(mid)}`;
}

// ran says one test of a thing: when, and how it went.
const ran = (r) => `${clock(r.at)}: ${r.passed ? `passed${r.joined_ms ? `, let on in ${secs(r.joined_ms)}` : ''}${r.dhcp_ms ? `, an address in ${secs(r.dhcp_ms)}` : ''}${r.down != null ? `, ${r.down} down / ${r.up ?? '?'} up Mbit/s` : ''}`
	: `failed at ${r.stage}`}`;

// over is how a thing did over the time shown: a mark for each test, the
// oldest first, green or red, which says when and how it went on hover; and
// how long its steps took across those that passed.
function over(c) {
	const runs = c.runs || [];
	if (runs.length < 2) return null;
	const good = runs.filter((r) => r.passed);
	const times = [['let on in', span(good, 'joined_ms')], ['an address in', span(good, 'dhcp_ms')], ['DNS in', span(good, 'dns_ms')]]
		.filter(([, v]) => v).map(([k, v]) => `${k} ${v}`);
	const speeds = good.filter((r) => r.down != null);
	if (speeds.length > 1) {
		const d = speeds.map((r) => r.down).sort((a, b) => a - b);
		times.push(`${d[0]} to ${d[d.length - 1]} Mbit/s down`);
	}
	return h('div', { class: 'runs' },
		h('span', { class: 'marks', role: 'img', 'aria-label': `${good.length} of ${runs.length} tests passed` },
			runs.map((r) => h('span', { class: `mark ${r.passed ? 'ok' : 'bad'}`, title: ran(r) }))),
		h('span', { class: 'sub' }, `${runs.length} tests since ${clock(runs[0].at)}${times.length ? ` · ${times.join(' · ')}` : ''}`));
}

// marks is how a thing did over the time shown, in its line: a mark a
// test, the latest twenty, the oldest first.
function marks(c) {
	const runs = (c.runs || []).slice(-20);
	if (!runs.length) return null;
	return h('span', { class: 'marks inline', role: 'img', 'aria-label': `${runs.filter((r) => r.passed).length} of its latest ${many(runs.length, 'test')} passed` },
		runs.map((r) => h('span', { class: `mark ${r.passed ? 'ok' : 'bad'}`, title: ran(r) })));
}

// The stages of coming online (0118), as a line says them.
const STAGE = { auth: 'authentication', assoc: 'association', signin: 'the sign-in', key: 'the keys', dhcp: 'DHCP', gateway: 'the gateway', dns: 'DNS', internet: 'the first connection' };
const many = (n, one, more) => `${n} ${n === 1 ? one : more || `${one}s`}`;

// joined is one network by its clients' own attempts to come online (0127):
// a line that opens to where the failed attempts stopped and to each AP's
// part.
function joined(n) {
	const key = `joins/${n.network || `ssid ${n.ssid}`}`;
	// Got on, of those that got on or did not, as the Overview counts them:
	// one that left of its own accord is neither, and is said apart.
	const tried = n.online + n.failed;
	const neither = n.clients - tried;
	const status = n.failing ? h('span', { class: 'chip bad', title: 'At an AP, in the last hour, three clients or more were let on and got no further, and they are half or more of those let on: an alert' }, 'Cannot get on')
		: n.failed ? h('span', { class: 'chip warn', title: `${many(n.failed, 'client')} did not get on` }, `${n.failed} did not`)
			: n.online ? h('span', { class: 'chip ok' }, 'All got on')
				: h('span', { class: 'chip idle' }, 'None seen through');
	const most = n.stops[0];
	const d = h('details', { class: 'fold', open: open.has(key) },
		h('summary', null,
			h('span', { class: 'who' }, n.ssid || n.network, !n.network && h('span', { class: 'note', title: 'A network of the AP’s own, not one of Aeolus’s' }, ' not Aeolus’s')),
			h('span', { class: 'grow sub', title: [`${many(n.clients, 'client')} tried`, `${n.online} got on`,
				neither > 0 && `${neither} left before either`,
				n.typical_ms > 0 && `typically online in ${secs(n.typical_ms)}`,
				most && `${n.failed ? 'most' : 'some'} stopped at ${STAGE[most.stage] || most.stage}`].filter(Boolean).join(' · ') },
			[tried > 0 && `${n.online} of ${tried} got on`,
				neither > 0 && `${neither} left`,
				n.failed > 0 && most ? `stopped at ${STAGE[most.stage] || most.stage}` : n.typical_ms > 0 && `typically ${secs(n.typical_ms)}`].filter(Boolean).join(' · ')),
			status),
		n.way?.some((p) => p.passed || p.stopped) && h('div', { class: 'attempt' }, typicalPath(n.way, `${n.way_of < n.attempts ? 'the latest ' : ''}${many(n.way_of, 'attempt')}`)),
		h('p', { class: 'sub' }, `${many(n.attempts, 'attempt')}, ${n.attempts_failed} failed. A client got on where one of its attempts came online or was let on; it did not where one failed and none got on. One that left of its own accord, as for another AP, is neither.`),
		n.stops.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['Failed attempts stopped at', 'Why, as the AP put it', 'Attempts', 'Clients', 'Latest'].map((t) => h('th', null, t))),
			n.stops.map((s) => h('tr', null,
				h('td', null, STAGE[s.stage] || s.stage), h('td', null, s.reason || '—'),
				h('td', null, String(s.attempts)), h('td', null, String(s.clients)), h('td', null, ago(s.last))))),
		h('table', { class: 'list' },
			h('tr', null, ['On', 'Clients tried', 'Got on', 'Did not', 'Attempts', 'Failed', ''].map((t) => h('th', null, t))),
			n.aps.map((a) => h('tr', null,
				h('td', null, link(`/aps/${encodeURIComponent(a.ap)}/clients`, a.name)),
				h('td', null, String(a.clients)), h('td', null, String(a.online)), h('td', null, String(a.failed)),
				h('td', null, String(a.attempts)), h('td', null, String(a.attempts_failed)),
				h('td', null, a.failing && h('span', { class: 'chip bad' }, 'Clients cannot get on'))))));
	d.addEventListener('toggle', () => (d.open ? open.add(key) : open.delete(key)));
	return d;
}

// fresh says whether a round was asked lately enough that it should still
// be moving: an AP polls once a minute, and takes a thing each time.
const fresh = (r) => Date.now() - new Date(r.at).getTime() < 20 * 60 * 1000;

// rounded says how a round of tests asked by hand is going (0129).
function rounded(r, here) {
	if (!r || !r.of) return null;
	const where = r.node === here ? '' : r.name ? ` at ${r.name}` : ' above here';
	// Who asked is said to one who may view where it was asked.
	const by = r.by ? ` by ${r.by}` : '';
	const left = r.of - r.done;
	const only = OVERSAID[r.over] || '';
	const asked = `Asked${where} ${ago(r.at)}${by}${only}`;
	if (left <= 0) return h('p', { class: 'sub roundline', title: asked }, h('span', { class: 'chip ok' }, 'Round done'), ` ${many(r.of, 'thing')} tested · asked ${ago(r.at)}`);
	// What is left a while after: an AP that is away, or behind on its
	// config, or a thing that cannot be asked for now.
	const stuck = !fresh(r) && r.asked === r.done;
	return h('p', { class: 'sub roundline', title: `${asked}. ` + (stuck ? 'What is left could not be asked for: an AP that is away or does not run its current config, or a thing that cannot be tested now. It is let go an hour after the round was asked.'
		: 'Each AP takes them one at a time, about one a minute; what is not asked for within the hour is let go.') },
	h('span', { class: `chip ${stuck ? 'warn' : 'idle'}` }, stuck ? 'Round stuck' : 'Round under way'),
	` ${r.done} of ${r.of} tested${stuck ? ' · the rest could not be asked for: an AP is away or behind on its config, or a thing cannot be tested now'
		: r.asked > r.done ? ' · one under way' : ''} · asked ${ago(r.at)}`);
}

// check is one thing tested by one AP: a line that opens to its latest test.
function check(ctx, c) {
	const key = `${c.tester}/${c.network}/${c.on}/${c.band}`;
	const where = [c.on === c.tester ? `${c.on_name} itself` : c.on_name || 'the strongest AP heard', c.band && bandName(c.band)].filter(Boolean).join(' · ');
	const tried = c.passed + c.failed;
	const status = c.failing ? h('span', { class: 'chip bad', title: 'Its last two tests failed' }, `Failing${c.since ? ` for ${ago(c.since).replace(/ ago$/, '')}` : ''}`)
		: c.last ? testChip(c.last)
			: h('span', { class: 'chip idle' }, c.scheduled ? 'not tried yet' : 'not on the schedule');
	const d = h('details', { class: 'fold', open: open.has(key) },
		h('summary', null,
			h('span', { class: 'who' }, c.ssid || c.network),
			h('span', { class: 'grow sub', title: tried > 0 ? `${c.passed} of ${many(tried, 'test')} passed in the time shown` : null },
				[where, c.on !== c.tester && `from ${c.tester_name}`, c.failed > 0 && `${c.passed} of ${tried} passed`,
					c.last && ago(c.last.at), !c.scheduled && tried > 0 && 'not on the schedule'].filter(Boolean).join(' · ')),
			marks(c),
			status),
		over(c),
		c.way?.some((p) => p.passed || p.stopped) && tried > 1 && h('div', { class: 'attempt' }, typicalPath(c.way, many(tried, 'test'))),
		c.last && tried > 1 && h('p', { class: 'sub attemptlinks' }, 'Its latest test:'),
		c.last ? testOpened(c.last, (ap) => ctx.name('locations', ap)) : h('p', { class: 'sub' }, 'No test of it has been made yet.'));
	d.addEventListener('toggle', () => (d.open ? open.add(key) : open.delete(key)));
	return d;
}

export async function assuranceTab(ctx, here, page, edit) {
	let view;
	try {
		view = await get(`/v1/assurance?under=${encodeURIComponent(here)}`);
	} catch {
		return [h('div', { class: 'banner info' }, 'This manager has no assurance yet.')];
	}
	const testers = view.testers || [];
	const checks = view.checks || [];
	const tests = view.tests || [];
	const box = h('div', { class: 'edit flush' });
	const failing = checks.filter((c) => c.failing).length;
	const name = (ap) => ctx.name('locations', ap);
	const out = [];
	const joins = view.joins || [];
	const stuck = joins.filter((n) => n.failing).length;
	// A round asked by hand (0129): every AP here tests each thing it can,
	// once, now. While one is being worked at, the page looks again soon.
	// The page looks again soon only while a round should still be moving:
	// one left unfinished, as by an AP that is away, is not watched all hour.
	const going = view.round && view.round.of > 0 && view.round.done < view.round.of;
	if (going && (fresh(view.round) || view.round.asked > view.round.done)) hurry(60);
	// Another round may be asked at any time, and takes the place of one
	// under way: what was fixed can be tried again at once.
	const said = h('div');
	// Which tests (0131): both, or one kind alone, of those an AP here can
	// make. The choice is kept for the next time.
	const can = { both: testers.some((t) => t.round), air: testers.some((t) => t.air), wire: testers.some((t) => t.can_wire) };
	let over = can[kept()] ? kept() : 'both';
	const round = h('button', { type: 'button', class: 'button small' }, going ? 'Test everything again' : 'Test everything now');
	const seg = h('span', { class: 'segmented', role: 'group', 'aria-label': 'Which tests' });
	const TITLE = {
		both: 'Every AP here tests each thing it can once, now, whatever its schedule: through the air where it has a scan radio, and over the wire',
		air: 'Every AP here with a scan radio joins each of its networks as a client would, once, now, whatever its schedule',
		wire: 'Every AP here asks each of its networks for an address, its gateway and a name, over the wire, once, now, whatever its schedule',
	};
	const pick = () => {
		seg.replaceChildren(...OVER.map(([k, name]) => h('button', { type: 'button', class: k === over ? 'on' : '', 'aria-pressed': String(k === over), disabled: !can[k],
			title: can[k] ? TITLE[k] : k === 'air' ? 'No AP here has a scan radio' : 'No AP here can test that way',
			onclick: () => { over = k; keep(k); pick(); } }, name)));
		round.disabled = !can[over];
		round.title = TITLE[over];
	};
	pick();
	round.addEventListener('click', async () => {
		round.disabled = true;
		said.replaceChildren();
		try {
			const r = await post('/v1/assurance/round', { under: here, over });
			flash(`A round asked of ${page.node.name}${OVERSAID[over] || ''}: ${many(r.things, 'thing')} to test, each AP one at a time from its next poll.`, 60);
			hurry(600);
			redrawNow();
		} catch (err) {
			round.disabled = false;
			said.replaceChildren(h('div', { class: 'error' }, err.message));
		}
	});

	const lately = `last ${view.hours} h`;
	out.push(h('section', { class: 'panel' },
		h('h2', { title: 'Every client’s own attempts to come online, as the APs recorded them. A line opens to where they stopped; each client’s attempts, step by step, are under Clients, Everyone seen.' },
			'Clients coming online', h('span', { class: 'note' }, stuck ? `${many(stuck, 'network')} failing · ${lately}` : lately)),
		joins.length ? joins.map(joined) : h('p', { class: 'sub' }, 'None tried.')));

	out.push(h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols', title: 'An AP with a scan radio joins its networks as a client would, in turn, and says how each step went. Over the wire, any AP asks each of its networks for an address, its gateway and a name.' },
			'Client tests',
			edit && h('span', { class: 'controls' }, seg, round,
				h('button', { type: 'button', class: 'button small', onclick: async () => systemEditor(ctx, await schema(), here, page.node.name, asSet(page.fields), box,
					{ sections: SECTIONS, heading: 'Edit the schedule', what: 'Client tests' }) }, 'Schedule…'))),
		// What a button here says comes right below it.
		said,
		box,
		testers.length
			? h('table', { class: 'list testers' }, testers.map(testerRow))
			: h('p', { class: 'sub pad' }, 'No AP here can test yet: one needs a scan radio, or an agent that tests over the wire (v0.79.0 on).'),
		rounded(view.round, here)));

	out.push(h('section', { class: 'panel' },
		h('h2', { title: 'Each thing an AP tests, the failing first, with a mark for each of its latest tests, twenty at the most. A line opens to what its tests come to and to its latest.' },
			'What is tested', h('span', { class: 'note' }, checks.length ? (failing ? `${failing} failing · ${lately}` : lately) : '')),
		checks.length ? checks.map((c) => check(ctx, c)) : h('p', { class: 'sub' }, 'Nothing yet.')));

	// The tests themselves, newest first, are a log: closed until asked for.
	const fold = (key, title, heading, note, body) => {
		const d = h('details', { class: 'panel logpanel', open: open.has(key) },
			h('summary', null, h('h2', { title }, heading, h('span', { class: 'note' }, note))), body);
		d.addEventListener('toggle', () => (d.open ? open.add(key) : open.delete(key)));
		return d;
	};
	out.push(fold(`tests/${here}`, 'The latest tests, newest first, on the schedule or asked by hand', 'Tests', tests.length ? `latest ${tests.length}` : 'none yet',
		tests.length ? tests.map((a) => testLine(a, name, true)) : h('p', { class: 'sub pad' }, 'No test has been made here yet.')));

	// The schedule as it is set: where each setting comes from, and for
	// one set here the way back to the folder above (0046). Folded too.
	const set = asSet(page.fields || {});
	const every = set['assurance.tests.every']?.value;
	const which = OVER.find(([k]) => k === (set['assurance.tests.over']?.value || 'air'))[1];
	out.push(fold(`schedule/${here}`, 'How often the APs here are asked for a test, and which tests; where each setting comes from; and, for one set here, the way back to the folder above',
		'Schedule', every ? `${value('assurance.tests.every', every)} · ${which.toLowerCase()}` : 'none',
		fieldPanels(ctx, 'locations', here, only(page.fields, (p) => FIELDS.includes(p)), edit)));
	return out;
}
