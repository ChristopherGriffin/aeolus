// An AP's radio neighbours (0073): the other Aeolus APs it hears in the air
// and exchanges hellos with over the wire, as each AP last reported, with
// how strongly each hears the other and the neighbour's channel; and the
// channels as it rates them. Radio resource management is turned on and
// off here too.

import { h, link } from '../dom.js';
import { bandName, origin } from '../format.js';
import { ask, confirm } from './confirm.js';
import { followButton } from './follow.js';

// A neighbour's state: its chip, and what it means.
const STATE = {
	up: ['ok', 'hellos go both ways'],
	'one-way': ['warn', 'its hellos come, but it gets none of this AP\'s'],
	heard: ['idle', 'heard in the air; no hellos yet'],
	down: ['bad', 'its hellos stopped'],
};

// neighboursSection draws the switch and the table. at is the node's page
// ({ node, nodeName, page, canEdit, parentName }); rows are its APs, each
// with its config and condition.
export function neighboursSection(ctx, at, rows) {
	return [switchRow(ctx, at), table(ctx, rows)];
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
				h('button', { type: 'button', class: 'button small', onclick: () => turn(ctx, at, path, field, !on, box) },
					on ? 'Turn off…' : 'Turn on…'))),
		box);
}

async function turn(ctx, at, path, field, on, box) {
	const { node, nodeName } = at;
	const op = { kind: 'set', tree: 'locations', node, path, value: on };
	const p = await ask(box, op);
	if (!p) return;
	confirm(ctx, box, op, p, [
		h('div', null, h('strong', null, `Neighbours on ${nodeName}: `), field?.value === true ? 'on' : 'off', ' → ', on ? 'on' : 'off'),
	], [
		h('div', { class: 'sub' }, on
			? 'Nothing restarts. Each AP\'s beacons say it is an Aeolus AP, it listens briefly on its other channels, one at a time, while its radio isn\'t busy, and it exchanges hellos with the APs it hears best.'
			: 'Nothing restarts. Each AP takes the mark off its beacons and stops its hellos.'),
	]);
}

function table(ctx, rows) {
	const name = (id) => ctx.name('locations', id);
	const lines = rows.flatMap(({ ap, cfg }) => {
		const rep = cfg?.condition?.state;
		const r = rep?.report?.rrm;
		const apLink = link(`/aps/${encodeURIComponent(ap.id)}/interfaces/radios/neighbours`, ap.name);
		const none = (why) => [h('tr', null, h('td', null, apLink), h('td', { colspan: 7, class: 'sub' }, why))];
		if (!cfg) return none('You cannot see this AP.');
		if (!r) return none(rep ? 'Off, or not reported yet.' : 'No report yet.');
		if (!r.neighbours.length) return none(`Hears no other Aeolus AP yet. Its address for hellos is ${r.address || 'unknown'}.`);
		return r.neighbours.flatMap((n, i) => (n.bands.length ? n.bands : [{ band: null }]).map((b, j) => {
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
		}));
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
// the last visit's; the channel its radio is on, the best one no neighbour
// uses, and the neighbours that blot the others out.
export function ratingsSection(ctx, rows) {
	const name = (id) => ctx.name('locations', id);
	const lines = rows.flatMap(({ ap, cfg }) => {
		const r = cfg?.condition?.state?.report?.rrm;
		const apLink = link(`/aps/${encodeURIComponent(ap.id)}/interfaces/radios/ratings`, ap.name);
		const none = (why) => [h('tr', null, h('td', null, apLink), h('td', { colspan: 9, class: 'sub' }, why))];
		if (!cfg) return none('You cannot see this AP.');
		if (!r) return none('Off, or not reported yet.');
		if (!r.ratings?.length) return none('No channel rated yet.');
		const best = {};
		for (const x of r.ratings) {
			if (!x.blotted_by.length && (!best[x.band] || x.cost < best[x.band].cost)) best[x.band] = x;
		}
		return r.ratings.map((x, i) => h('tr', null,
			h('td', null, i === 0 && apLink),
			h('td', null, bandName(x.band)),
			h('td', { class: 'mono' }, String(x.channel)),
			h('td', { class: 'mono' }, String(x.cost)),
			h('td', { class: 'mono' }, String(x.now)),
			h('td', { class: 'mono' }, `${x.busy}%`),
			h('td', { class: 'mono' }, x.noise != null ? `${x.noise} dBm` : '—'),
			h('td', null, String(x.networks)),
			h('td', null, `${x.ago} s ago`),
			h('td', null,
				x.own && h('span', { class: 'chip here' }, 'in use'), ' ',
				best[x.band] === x && h('span', { class: 'chip ok' }, 'best'),
				x.blotted_by.length > 0 && h('div', { class: 'sub' }, `used by ${x.blotted_by.map(name).join(', ')}`))));
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'Channel ratings', h('span', { class: 'note' }, 'lower is better; the rating is earned over many visits, now is the last few')),
		rows.length
			? h('table', { class: 'list' },
				h('tr', null, ['AP', 'Band', 'Channel', 'Rating', 'Now', 'Busy', 'Noise', 'Networks', 'Last visit', ''].map((c) => h('th', null, c))),
				lines)
			: h('div', { class: 'sub' }, 'No APs here yet.'));
}
