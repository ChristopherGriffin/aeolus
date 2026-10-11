// An AP's radio neighbours (0073): the other Aeolus APs it hears in the air
// and exchanges hellos with over the wire, as each AP last reported, with
// how strongly each hears the other and the neighbour's channel; the
// channels as it rates them; and its moves. Radio resource management, and
// its policy for moves, are set here too, and power control (0077), with
// the radios it holds.

import { h, link } from '../dom.js';
import { bandName, origin, group, value, ago } from '../format.js';
import { schema } from '../api.js';
import { ask, confirm } from './confirm.js';
import { followButton } from './follow.js';
import { fieldsForm, changedValues } from './edit.js';
import { neighbourMap } from './map.js';

// A neighbour's state: its chip, and what it means.
const STATE = {
	up: ['ok', 'hellos go both ways'],
	'one-way': ['warn', 'its hellos come, but it gets none of this AP\'s'],
	heard: ['idle', 'heard in the air; no hellos yet'],
	down: ['bad', 'its hellos stopped'],
};

// The policy for moves (0073), power control's (0077), and what each means
// unset.
const POLICY = ['rrm.moves', 'rrm.window', 'rrm.margin'];
const APC = ['apc.neighbours', 'apc.target'];
const DEFAULT = { 'rrm.moves': true, 'rrm.window': '02:00-05:00', 'rrm.margin': 20, 'apc.neighbours': 3, 'apc.target': -70 };

// What turning each on or off does.
const TURNS = {
	'rrm.enabled': ['Neighbours',
		'Nothing restarts. Each AP\'s beacons say it is an Aeolus AP, it listens briefly on its other channels, one at a time, while its radio isn\'t busy, and it exchanges hellos with the APs it hears best. Unless moves are off, it moves a radio whose channel is automatic to its best channel, as the policy says, with an announcement clients can follow.',
		'Nothing restarts. Each AP takes the mark off its beacons, stops its hellos, and moves no radio.'],
	'apc.enabled': ['Power control',
		'Nothing restarts. A minute after it applies the config, each AP holds each radio\'s power, unless one is set for it, and steps it 3 dB at a time, at most every ten minutes: up while fewer of its neighbours on the band hear it than it looks for, or the weakest of them hears it below the target; down while all of them hear it 6 dB or more above. Clients don\'t notice a step.',
		'Nothing restarts. Each AP lets its radios go back to their configured power.'],
};

// neighboursSection draws the switch, the policy for moves, power control,
// the map, and the table. at is the node's page ({ node, nodeName, page,
// canEdit, parentName }); rows are its APs, each with its config and
// condition.
export function neighboursSection(ctx, at, rows) {
	return [switchRow(ctx, at), powerPanel(ctx, at, rows), neighbourMap(ctx, rows), table(ctx, rows)];
}

function switchRow(ctx, at) {
	const { node, nodeName, page, canEdit, parentName } = at;
	const path = 'rrm.enabled';
	const field = page.fields?.[path];
	const on = field?.value === true;
	const box = h('div', { class: 'edit' });
	const lockedAbove = field?.origin === 'locked' && field.from !== node;
	return h('section', { class: 'panel' },
		h('div', { class: 'row' },
			h('div', { class: 'label' }, 'Neighbours'),
			h('div', { class: 'value' }, field ? (on ? 'on' : 'off') : h('span', { class: 'sealed' }, 'off (not set)')),
			field && origin('locations', node, field, (id) => ctx.name('locations', id)),
			canEdit && !lockedAbove && h('span', { class: 'controls' },
				field?.origin === 'self' && followButton(ctx, 'locations', node, nodeName, parentName, [path], box),
				on && h('button', { type: 'button', class: 'button small', onclick: async () => policyEditor(ctx, await schema(), at, box, POLICY, 'Moves') }, 'Edit moves…'),
				h('button', { type: 'button', class: 'button small', onclick: () => turn(ctx, at, path, field, !on, box) },
					on ? 'Turn off…' : 'Turn on…'))),
		on && POLICY.map((p) => policyRow(ctx, at, p)),
		box);
}

// policyRow shows one of the policy's fields, as set here or above, or its
// default.
function policyRow(ctx, at, path) {
	const { node, nodeName, page, canEdit, parentName } = at;
	const field = page.fields?.[path];
	const box = h('div', { class: 'edit' });
	return [h('div', { class: 'row' },
		h('div', { class: 'label' }, group(path).label),
		h('div', { class: 'value' }, field ? value(path, field.value) : h('span', { class: 'sealed' }, value(path, DEFAULT[path]), ' (not set)')),
		field && origin('locations', node, field, (id) => ctx.name('locations', id)),
		canEdit && field?.origin === 'self' && h('span', { class: 'controls' },
			followButton(ctx, 'locations', node, nodeName, parentName, [path], box))),
	box];
}

// policyEditor opens, in box, a policy's fields (paths) as the schema
// describes them (d), set on this node in one change.
function policyEditor(ctx, d, at, box, paths, title) {
	const { node, nodeName, page } = at;
	const { body, inputs, rows } = fieldsForm(d, [[null, paths]], page.fields || {}, node);
	const out = h('div', { class: 'edit flush' });
	const msg = h('div', { class: 'error' });
	const review = async () => {
		msg.replaceChildren();
		let values;
		try {
			values = changedValues(inputs, rows);
		} catch (e) {
			msg.replaceChildren(e.message);
			return;
		}
		const changed = Object.keys(values);
		if (!changed.length) {
			msg.replaceChildren('Nothing has changed.');
			return;
		}
		const op = changed.length === 1
			? { kind: 'set', tree: 'locations', node, path: changed[0], value: values[changed[0]] }
			: { kind: 'set', tree: 'locations', node, values };
		const p = await ask(out, op);
		if (!p) return;
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, `${title} on ${nodeName}`)),
			h('ul', { class: 'becomes' }, changed.map((path) => h('li', null, `${group(path).label}: `,
				page.fields?.[path] ? value(path, page.fields[path].value) : [value(path, DEFAULT[path]), ' (not set)'], ' → ', value(path, values[path])))),
		], [
			h('div', { class: 'sub' }, 'Nothing restarts: each AP takes it up once it applies the config.'),
		]);
	};
	box.replaceChildren(h('div', { class: 'fieldform', 'data-editing': true },
		body,
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review changes'),
			h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel')),
		out));
}

async function turn(ctx, at, path, field, on, box) {
	const { node, nodeName } = at;
	const [what, onText, offText] = TURNS[path];
	const op = { kind: 'set', tree: 'locations', node, path, value: on };
	const p = await ask(box, op);
	if (!p) return;
	confirm(ctx, box, op, p, [
		h('div', null, h('strong', null, `${what} on ${nodeName}: `), field?.value === true ? 'on' : 'off', ' → ', on ? 'on' : 'off'),
	], [
		h('div', { class: 'sub' }, on ? onText : offText),
	]);
}

// Why power control holds a radio where it does (0077), with its chip.
const POWER = {
	new: ['idle', 'just taken over'], looking: ['warn', 'looking for neighbours'], below: ['warn', 'below the target'],
	ceiling: ['warn', 'at its ceiling'], target: ['ok', 'at the target'], above: ['ok', 'above the target'],
	floor: ['ok', 'at its floor'], failed: ['bad', 'cannot set its power'],
};

// powerPanel is power control (0077): its switch and policy, which need
// neighbours, and the radios each AP holds, as each last reported.
function powerPanel(ctx, at, rows) {
	const { node, nodeName, page, canEdit, parentName } = at;
	const path = 'apc.enabled';
	const field = page.fields?.[path];
	const on = field?.value === true;
	const rrm = page.fields?.['rrm.enabled']?.value === true;
	const box = h('div', { class: 'edit' });
	const lockedAbove = field?.origin === 'locked' && field.from !== node;
	const lines = rows.flatMap(({ ap, cfg }) => (cfg?.condition?.state?.report?.rrm?.apc || []).map((x, i) => {
		const [chip, says] = POWER[x.why] || ['idle', x.why];
		return h('tr', null,
			h('td', null, i === 0 && link(`/aps/${encodeURIComponent(ap.id)}/interfaces/radios/neighbours`, ap.name)),
			h('td', null, bandName(x.band)),
			h('td', { class: 'mono' }, x.power != null ? `${x.power} dBm` : '—', x.ceiling != null && h('span', { class: 'sub' }, ` of ${x.ceiling}`)),
			h('td', { class: 'mono' }, x.count != null ? `${x.count} of ${x.wanted}` : '—', x.weakest != null && h('span', { class: 'sub' }, `, weakest ${x.weakest} dBm`)),
			h('td', null, h('span', { class: `chip ${chip}` }, says),
				x.step ? h('span', { class: 'sub' }, ` ${x.step > 0 ? '+' : ''}${x.step} dB${x.ago != null ? `, ${x.ago < 120 ? `${x.ago} s` : `${Math.round(x.ago / 60)} min`} ago` : ''}`) : null));
	}));
	return h('section', { class: 'panel' },
		h('div', { class: 'row' },
			h('div', { class: 'label' }, 'Power control'),
			h('div', { class: 'value' }, field ? (on ? 'on' : 'off') : h('span', { class: 'sealed' }, 'off (not set)'),
				!rrm && on && h('span', { class: 'sub' }, ' (needs neighbours on)')),
			field && origin('locations', node, field, (id) => ctx.name('locations', id)),
			canEdit && !lockedAbove && h('span', { class: 'controls' },
				field?.origin === 'self' && followButton(ctx, 'locations', node, nodeName, parentName, [path], box),
				on && h('button', { type: 'button', class: 'button small', onclick: async () => policyEditor(ctx, await schema(), at, box, APC, 'Power control') }, 'Edit…'),
				(on || rrm) && h('button', { type: 'button', class: 'button small', onclick: () => turn(ctx, at, path, field, !on, box) },
					on ? 'Turn off…' : 'Turn on…'))),
		on && APC.map((p) => policyRow(ctx, at, p)),
		box,
		on && (lines.length
			? h('table', { class: 'list' },
				h('tr', null, ['AP', 'Band', 'Power', 'Heard by', 'Now'].map((c) => h('th', null, c))),
				lines)
			: h('div', { class: 'sub' }, 'No AP holds a radio yet: each takes its radios over a minute after it applies the config.')));
}

// deaf lists an AP's radios that can't scan (2026-10-06), as rows across
// span columns: on a DFS channel, Linux keeps an AP there listening for
// radar and refuses every scan, so what the radio rates and hears on that
// band ages until it leaves; a radio may refuse every scan otherwise too.
function deaf(r, span, first) {
	const dur = (s) => (s < 120 ? `${s} s` : s < 7200 ? `${Math.round(s / 60)} min` : `${Math.round(s / 3600)} h`);
	return (r?.cannot_scan || []).map((d, i) => h('tr', null,
		h('td', null, i === 0 && first),
		h('td', { colspan: span, class: 'sub' }, h('span', { class: 'chip warn' }, `${bandName(d.band)} can't scan`), ' ',
			d.why === 'dfs'
				? `For ${dur(d.ago)}: on DFS channel ${d.channel} the radio must keep listening for radar, so it can't visit other channels or hear its neighbours there. Its ${bandName(d.band)} ratings age until it leaves the channel, and lapse after 30 minutes.`
				: `For ${dur(d.ago)}: its radio refuses every scan on channel ${d.channel}, so its ${bandName(d.band)} ratings age.`)));
}

function table(ctx, rows) {
	const name = (id) => ctx.name('locations', id);
	const lines = rows.flatMap(({ ap, cfg }) => {
		const rep = cfg?.condition?.state;
		const r = rep?.report?.rrm;
		const apLink = link(`/aps/${encodeURIComponent(ap.id)}/interfaces/radios/neighbours`, ap.name);
		const none = (why) => [h('tr', null, h('td', null, apLink), h('td', { colspan: 7, class: 'sub' }, why))];
		if (!cfg) return none('You cannot see this AP.');
		if (rep?.report?.wireless_missing) return none(`Can't see its own radios: netifd lost its network.wireless object. Restarting the network on the AP brings it back; its Wi-Fi drops for about 30 seconds.`);
		if (!r) return none(rep ? 'Off, or not reported yet.' : 'No report yet.');
		if (!r.neighbours.length) return [...none(`Hears no other Aeolus AP yet. Its address for hellos is ${r.address || 'unknown'}.`), ...deaf(r, 7, null)];
		return [...r.neighbours.flatMap((n, i) => (n.bands.length ? n.bands : [{ band: null }]).map((b, j) => {
			const [chip, means] = STATE[n.state] || ['idle', ''];
			return h('tr', null,
				h('td', null, i === 0 && j === 0 && apLink),
				h('td', null, j === 0 && [link(`/aps/${encodeURIComponent(n.ap)}/interfaces/radios/neighbours`, name(n.ap)),
					!n.chosen && h('div', { class: 'sub' }, 'not among the three it hears best')]),
				h('td', null, b.band ? bandName(b.band) : '—'),
				h('td', { class: 'mono' }, b.signal != null ? `${b.signal} dBm` : '—'),
				h('td', { class: 'mono' }, b.their_signal != null ? `${b.their_signal} dBm` : '—'),
				h('td', { class: 'mono' }, b.channel ? `${b.channel}${b.width ? ` · ${b.width} MHz` : ''}` : '—'),
				h('td', null, j === 0 && h('span', { class: `chip ${chip}`, title: means }, n.state)),
				h('td', null, j === 0 && (n.hello_ago != null ? `${n.hello_ago} s ago` : '—')));
		})), ...deaf(r, 7, null)];
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'Radio neighbours', h('span', { class: 'note' }, rows.length === 1 ? 'as it last reported' : 'as each AP last reported')),
		rows.length
			? h('table', { class: 'list' },
				h('tr', null, ['AP', 'Neighbour', 'Band', 'Hears it', 'It hears this AP', 'Its channel', 'State', 'Last hello'].map((c) => h('th', null, c))),
				lines)
			: h('div', { class: 'sub' }, 'No APs here yet.'));
}

// ratingsSection lists each AP's channel ratings (0073), lower being better:
// the lasting one, earned over many visits, and the one now; what went into
// the last visit's; the channel its radio is on, the neighbours that blot
// channels out, and the AP's best on each band, where its radio would move:
// the best rated no neighbour uses, or where they use them all, the one
// whose nearest user is furthest away. Then each AP's moves.
//
// Each band has a panel of its own, and in it each AP's channels open by
// the arrow beside its name (Griff, 2026-10-10): one long table of every
// AP's every channel was hard to read.
export function ratingsSection(ctx, rows, own) {
	return [...ratingsPanels(ctx, rows, own), movesPanel(ctx, rows)];
}

// The AP lists opened on the Ratings page, as '<band> <ap>', so a page
// drawn again keeps them open.
const opened = new Set();

// own says the page is an AP's own, where its lists start open; a folder
// with one AP in it is still a folder.
function ratingsPanels(ctx, rows, own) {
	const note = 'lower is better; the rating is earned over many visits, now is the last few';
	const apLink = (ap) => link(`/aps/${encodeURIComponent(ap.id)}/interfaces/radios/ratings`, ap.name);
	// What each AP has to show, or why it has nothing.
	const of = rows.map(({ ap, cfg }) => {
		const report = cfg?.condition?.state?.report, r = report?.rrm;
		const why = !cfg ? 'You cannot see this AP.' : !r ? 'Off, or not reported yet.'
			: !r.ratings?.length && !r.cannot_scan?.length ? 'No channel rated yet.' : null;
		return { ap, report, r, why };
	});
	const panels = ['2g', '5g', '6g'].map((band) => {
		const folds = of.filter((x) => !x.why).map((x) => ratingsFold(ctx, band, x, !!own)).filter(Boolean);
		return folds.length > 0 && h('section', { class: 'panel' },
			h('h2', null, `${bandName(band)} channel ratings`, h('span', { class: 'note' }, note)),
			folds);
	}).filter(Boolean);
	const quiet = of.filter((x) => x.why);
	if (!quiet.length && panels.length) return panels;
	return [...panels, h('section', { class: 'panel' },
		h('h2', null, 'Channel ratings', !panels.length && h('span', { class: 'note' }, note)),
		rows.length
			? h('table', { class: 'list' }, quiet.map((x) => h('tr', null, h('td', null, apLink(x.ap)), h('td', { class: 'sub' }, x.why))))
			: h('div', { class: 'sub' }, 'No APs here yet.'))];
}

// ratingsFold is one AP's channels on one band, shut until its arrow is
// pressed, or open where the page is the AP's own. Shut, it says the
// channel the radio is on, the best there, and how many are rated. Null
// where the AP has nothing on the band.
function ratingsFold(ctx, band, { ap, report, r }, alone) {
	const name = (id) => ctx.name('locations', id);
	const list = (r.ratings || []).filter((x) => x.band === band);
	const deafHere = { cannot_scan: (r.cannot_scan || []).filter((d) => d.band === band) };
	if (!list.length && !deafHere.cannot_scan.length) return null;
	const radio = (report?.radios || []).find((x) => x.band === band && x.up !== false && x.channel);
	const best = list.find((x) => x.best);
	const says = [
		radio && `on channel ${radio.channel}${radio.width ? `, ${radio.width} MHz` : ''}`,
		best && (best.own ? 'its channel rates best' : `channel ${best.channel} rates best`),
		`${list.length} channel${list.length === 1 ? '' : 's'} rated`,
	].filter(Boolean).join(' · ');
	const key = `${band} ${ap.id}`;
	const d = h('details', { class: 'fold', open: alone || opened.has(key) },
		h('summary', null,
			h('span', { class: 'who' }, ap.name),
			h('span', { class: 'grow sub' }, says),
			deafHere.cannot_scan.length > 0 && h('span', { class: 'chip warn' }, "can't scan"),
			!alone && link(`/aps/${encodeURIComponent(ap.id)}/interfaces/radios/ratings`, 'Its page')),
		h('table', { class: 'list' },
			list.length > 0 && h('tr', null, ['Channel', 'Rating', 'Now', 'Busy', 'Noise', 'Networks', 'Last visit', ''].map((c) => h('th', null, c))),
			list.map((x) => h('tr', null,
				h('td', { class: 'mono' }, String(x.channel)),
				h('td', { class: 'mono' }, String(x.cost)),
				h('td', { class: 'mono' }, String(x.now)),
				h('td', { class: 'mono' }, `${x.busy}%`),
				h('td', { class: 'mono' }, x.noise != null ? `${x.noise} dBm` : '—'),
				h('td', null, String(x.networks)),
				h('td', null, `${x.ago} s ago`),
				h('td', null,
					x.own && h('span', { class: 'chip here' }, 'in use'), ' ',
					x.best && h('span', { class: 'chip ok' }, x.blotted_by.length ? 'best (all used)' : 'best'),
					x.blotted_by.length > 0 && h('div', { class: 'sub' }, `used by ${x.blotted_by.map(name).join(', ')}`)))),
			deaf(deafHere, 7, null)));
	d.addEventListener('toggle', () => (d.open ? opened.add(key) : opened.delete(key)));
	return d;
}

// Why a move was made, and what came of it, with its chip.
const WHY = {
	start: 'the radio had just started', shared: 'a neighbour uses its channel',
	better: 'another rates better', interference: 'interference on its channel',
};
const CAME = { announced: 'warn', moved: 'ok', yielded: 'idle', withdrawn: 'idle', failed: 'bad' };

// movesPanel lists each AP's last moves, newest first (0073).
function movesPanel(ctx, rows) {
	const name = (id) => ctx.name('locations', id);
	const lines = rows.flatMap(({ ap, cfg }) => {
		const moves = [...(cfg?.condition?.state?.report?.rrm?.moves || [])].reverse();
		const apLink = link(`/aps/${encodeURIComponent(ap.id)}/interfaces/radios/ratings`, ap.name);
		return moves.map((m, i) => h('tr', null,
			h('td', null, i === 0 && apLink),
			h('td', null, bandName(m.band)),
			h('td', { class: 'mono' }, `${m.from} → ${m.to}`),
			h('td', null, WHY[m.why] || m.why),
			h('td', null, h('span', { class: `chip ${CAME[m.state] || 'idle'}` }, m.state === 'yielded' && m.ap ? `yielded to ${name(m.ap)}` : m.state)),
			h('td', null, ago(m.at * 1000))));
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'Moves', h('span', { class: 'note' }, 'newest first; a move is announced to neighbours before it is made')),
		lines.length
			? h('table', { class: 'list' },
				h('tr', null, ['AP', 'Band', 'Channel', 'Why', 'What came of it', 'When'].map((c) => h('th', null, c))),
				lines)
			: h('div', { class: 'sub' }, 'No moves yet.'));
}
