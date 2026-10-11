// The Assurance tab of a Locations folder (0123): the service tried from a
// client's side. The APs here that have a scan radio are asked for a client
// test (0121) on a schedule, each time another network, AP and band; what
// each thing tested comes to is a line, the failing first, that opens to
// its latest test; and the latest tests follow, newest first.

import { h } from '../dom.js';
import { get, schema } from '../api.js';
import { ago, bandName, value } from '../format.js';
import { fieldPanels } from './fields.js';
import { only } from './sections.js';
import { systemEditor } from './system.js';
import { testChip, testLine, testOpened } from './clienttest.js';

// What the tab sets: how often, with speed or without, and the server a
// speed is measured against, which is the APs' own setting (0121).
const FIELDS = ['assurance.tests.every', 'assurance.tests.speed', 'system.speed_server'];
const SECTIONS = [['Client tests on a schedule', FIELDS]];

// The checks opened, by what they are of, so a redraw keeps them open.
const open = new Set();

// round is how long a tester takes to come back to the same thing.
function round(t) {
	const m = t.every * t.things;
	return m < 120 ? `${m} minutes` : m < 2880 ? `${Math.round(m / 60)} hours` : `${Math.round(m / 1440)} days`;
}

function tester(t) {
	if (!t.things) return `${t.name} has a scan radio, and offers no network a test can join.`;
	if (!t.every) return `${t.name} has a scan radio, and is not asked for tests on a schedule. It could test ${t.things} thing${t.things === 1 ? '' : 's'} in turn.`;
	return `${t.name} is asked for one ${value('assurance.tests.every', t.every)}${t.speed ? ', with speed' : ''}: ${t.things} thing${t.things === 1 ? '' : 's'} in turn, so each about every ${round(t)}.`;
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

// over is how a thing did over the time shown: a mark for each test, the
// oldest first, green or red, which says when and how it went on hover; and
// how long its steps took across those that passed.
function over(c) {
	const runs = c.runs || [];
	if (runs.length < 2) return null;
	const good = runs.filter((r) => r.passed);
	const said = (r) => `${clock(r.at)}: ${r.passed ? `passed${r.joined_ms ? `, let on in ${secs(r.joined_ms)}` : ''}${r.dhcp_ms ? `, an address in ${secs(r.dhcp_ms)}` : ''}${r.down != null ? `, ${r.down} down / ${r.up ?? '?'} up Mbit/s` : ''}`
		: `failed at ${r.stage}`}`;
	const times = [['let on in', span(good, 'joined_ms')], ['an address in', span(good, 'dhcp_ms')], ['DNS in', span(good, 'dns_ms')]]
		.filter(([, v]) => v).map(([k, v]) => `${k} ${v}`);
	const speeds = good.filter((r) => r.down != null);
	if (speeds.length > 1) {
		const d = speeds.map((r) => r.down).sort((a, b) => a - b);
		times.push(`${d[0]} to ${d[d.length - 1]} Mbit/s down`);
	}
	return h('div', { class: 'runs' },
		h('span', { class: 'marks', role: 'img', 'aria-label': `${good.length} of ${runs.length} tests passed` },
			runs.map((r) => h('span', { class: `mark ${r.passed ? 'ok' : 'bad'}`, title: said(r) }))),
		h('span', { class: 'sub' }, `${runs.length} tests since ${clock(runs[0].at)}${times.length ? ` · ${times.join(' · ')}` : ''}`));
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
			h('span', { class: 'grow sub' }, [`on ${where}`, c.on !== c.tester && `tried from ${c.tester_name}`,
				tried > 0 && `${c.passed} of ${tried} passed`, c.last && `last ${ago(c.last.at)}`,
				!c.scheduled && tried > 0 && 'no longer on the schedule'].filter(Boolean).join(' · ')),
			status),
		over(c),
		c.last ? testOpened(c.last) : h('p', { class: 'sub' }, 'No test of it has been made yet.'));
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
	const fields = only(page.fields, (p) => FIELDS.includes(p));
	const box = h('div', { class: 'edit flush' });
	const failing = checks.filter((c) => c.failing).length;
	const name = (ap) => ctx.name('locations', ap);
	const out = [];

	out.push(h('section', { class: 'panel' },
		h('h2', null, 'Client tests on a schedule', h('span', { class: 'note' }, 'an AP with a scan radio joins its networks as a client would, in turn, and says how each step went')),
		testers.length
			? h('ul', { class: 'plain' }, testers.map((t) => h('li', null, tester(t))))
			: h('div', { class: 'banner info' }, 'No AP here has a scan radio, so none can test from a client’s side. A schedule set here waits for one.'),
		edit && h('div', { class: 'below' },
			h('button', { type: 'button', class: 'button', onclick: async () => systemEditor(ctx, await schema(), here, page.node.name, page.fields, box,
				{ sections: SECTIONS, heading: 'Edit the schedule', what: 'Client tests' }) }, 'Edit the schedule')),
		box));
	if (Object.keys(fields).length) out.push(fieldPanels(ctx, 'locations', here, fields, edit));

	out.push(h('section', { class: 'panel' },
		h('h2', null, 'What is tested', h('span', { class: 'note' },
			checks.length ? `${failing ? `${failing} failing · ` : ''}over the last ${view.hours} hours; a line opens to its latest test` : '')),
		checks.length ? checks.map((c) => check(ctx, c))
			: h('p', { class: 'sub' }, testers.length ? 'Nothing yet: no schedule is set, and no test was asked by hand.' : 'Nothing.')));

	out.push(h('section', { class: 'panel' },
		h('h2', null, 'Tests', h('span', { class: 'note' }, 'the latest, newest first; one asked by hand is among them')),
		tests.length ? tests.map((a) => testLine(a, name, true)) : h('p', { class: 'sub' }, 'None yet.')));
	return out;
}
