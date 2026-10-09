// A Locations node's radios, one card per band (0047): its width, offered
// only within what every AP the node reaches can do (0044), and its channel,
// power and on/off, each with where it comes from, and beside each, its
// channel map (0075, 0087). A width some AP's channel
// cannot carry sets the channel to automatic in the same change, so each AP
// picks one that fits (0045). A value set here can follow the folder above
// again (0046). Whether 5 GHz avoids DFS channels is set on the channel map
// (0071, 0075).

import { h } from '../dom.js';
import { bandName, origin, ago, value } from '../format.js';
import { ask, confirm } from './confirm.js';
import { followButton } from './follow.js';

const BANDS = ['2g', '5g', '6g'];
const OTHER = [['channel', 'Channel'], ['power', 'Power'], ['enabled', 'Radio on']];

// radiosSection draws the cards. report is the AP's latest state report, for
// an AP's page; rows are the APs the node reaches, each with its config and
// condition, for what each runs where Aeolus doesn't set it. With mapFor,
// each band's card has its channel map beside it (0087).
export function radiosSection(ctx, node, nodeName, page, report, rows = [], mapFor = null) {
	const hw = page.hardware;
	if (!hw) return null;
	const isAP = page.node.kind === 'ap';
	const canEdit = page.role === 'operator' || page.role === 'admin';
	const parentName = page.node.parent ? ctx.name('locations', page.node.parent) : null;
	const reported = new Map((report?.report?.radios || []).map((r) => [r.band, r]));
	const offered = new Map(hw.bands.map((b) => [b.band, b]));
	const reach = hw.aps.length === 1 ? hw.aps[0].name : `${hw.aps.length} APs`;
	const at = { ctx, node, nodeName, page, isAP, canEdit, parentName, rows };
	return [
		h('div', { class: 'sub lead' }, isAP
			? (report ? `Reported ${ago(report.at)}.` : 'No report yet.')
			: hw.aps.length ? `Applies to ${reach} below, unless an AP sets its own.` : 'No APs here yet.'),
		hw.unknown.length > 0 && h('div', { class: 'banner info' },
			`${hw.unknown.map((a) => a.name).join(', ')} never said what its radios can do, so ${hw.unknown.length === 1 ? 'it is' : 'they are'} not counted.`),
		mapFor
			? BANDS.map((band) => h('div', { class: 'bandrow' },
				card(at, band, offered.get(band), reported.get(band)),
				offered.get(band) ? mapFor(band) : null))
			: h('div', { class: 'bands' }, BANDS.map((band) => card(at, band, offered.get(band), reported.get(band)))),
	];
}

function card(at, band, b, now) {
	const { ctx, node, page, isAP } = at;
	if (!b) {
		return h('section', { class: 'panel band none' },
			h('h2', null, bandName(band)),
			h('div', { class: 'empty' }, isAP ? 'This AP has no radio for this band.' : 'No AP here has a radio for this band.'));
	}
	const box = h('div', { class: 'edit' });
	const unset = isAP ? 'not set by Aeolus; the AP keeps its own' : 'not set; each AP keeps its own';
	return h('section', { class: 'panel band' },
		h('h2', null, bandName(band), h('span', { class: 'note' }, isAP
			? (now ? `now channel ${now.channel || '—'} · ${now.width ? now.width + ' MHz' : '—'} · ${now.clients} client${now.clients === 1 ? '' : 's'}` : 'no report yet')
			: `${b.aps} AP${b.aps === 1 ? '' : 's'} with this band`)),
		widthRow(at, b, now, box, unset),
		OTHER.map(([k, label]) => {
			const path = `radio.${band}.${k}`;
			const field = page.fields?.[path];
			const runs = !field && reported(at.rows, band, k);
			return [h('div', { class: 'row' },
				h('div', { class: 'label' }, label),
				h('div', { class: 'value' }, field ? value(path, field.value)
					: runs ? [runs, h('span', { class: 'sealed' }, isAP ? ' (its own)' : ' (each AP\'s own)')]
						: h('span', { class: 'sealed' }, unset)),
				field && origin('locations', node, field, (id) => ctx.name('locations', id)),
				isAP && field?.origin === 'self' && h('span', { class: 'chip warn' }, 'custom'),
				at.canEdit && field?.origin === 'self' && h('span', { class: 'controls' },
					followButton(ctx, 'locations', node, at.nodeName, at.parentName, [path], box))),
		];
		}),
		box);
}

// reported says what the APs run for power or whether the radio is on (k),
// on a band, as they last reported it: one value where all agree, else each
// AP's. Null where none reported it. A radio's power is what power control
// holds it at (0077), or else at most what it reports: a radio may not
// send all it says.
function reported(rows, band, k) {
	if (k !== 'power' && k !== 'enabled') return null;
	const seen = rows.map(({ ap, cfg }) => {
		const rep = cfg?.condition?.state?.report;
		const r = (rep?.radios || []).find((x) => x.band === band);
		const held = (rep?.rrm?.apc || []).find((x) => x.band === band && x.power != null);
		if (k === 'power' && held) return { name: ap.name, text: `${held.power} dBm (power control)` };
		const v = k === 'power' ? r?.txpower : r?.up;
		return v == null ? null : { name: ap.name, text: k === 'power' ? `up to ${v} dBm` : v ? 'on' : 'off' };
	}).filter(Boolean);
	if (!seen.length) return null;
	const texts = [...new Set(seen.map((s) => s.text))];
	return texts.length === 1 ? texts[0] : seen.map((s) => `${s.name} ${s.text}`).join(' · ');
}

function widthRow(at, b, now, box, unset) {
	const { ctx, node, nodeName, page, isAP, canEdit, parentName } = at;
	const path = `radio.${b.band}.width`;
	const field = page.fields?.[path];
	const lockedAbove = field?.origin === 'locked' && field.from !== node;

	const select = h('select', { 'aria-label': `${bandName(b.band)} width` },
		h('option', { value: '' }, field ? `Keep ${field.value} MHz` : 'Choose a width'),
		b.widths.map((w) => h('option', { value: String(w.width), disabled: !w.ok }, `${w.width} MHz`,
			!w.ok ? ` (${w.why})` : w.auto ? ' (channel becomes automatic)' : '')));
	const change = h('button', { type: 'button', class: 'button', disabled: true }, 'Change…');
	select.addEventListener('change', () => { change.disabled = select.value === ''; });
	change.addEventListener('click', () => preview(at, b, now, path, field, select.value, box));

	// Following the folder again takes the band's channel too, if it is set
	// here: a width set with an automatic channel goes back as one.
	const channel = `radio.${b.band}.channel`;
	const follow = field?.origin === 'self' && followButton(ctx, 'locations', node, nodeName, parentName,
		page.fields?.[channel]?.origin === 'self' ? [path, channel] : [path], box);

	return h('div', { class: 'row' },
		h('div', { class: 'label' }, 'Width'),
		h('div', { class: 'value' }, field ? `${field.value} MHz` : h('span', { class: 'sealed' }, unset)),
		field && origin('locations', node, field, (id) => ctx.name('locations', id)),
		isAP && field?.origin === 'self' && h('span', { class: 'chip warn' }, 'custom'),
		canEdit && !lockedAbove && h('span', { class: 'controls' }, follow, select, change));
}

// preview shows what a width change would do, before anything is recorded.
async function preview(at, b, now, path, field, choice, box) {
	const { ctx, node, nodeName, isAP } = at;
	const w = b.widths.find((x) => String(x.width) === choice);
	const op = w.auto
		? { kind: 'set', tree: 'locations', node, values: { [path]: w.width, [`radio.${b.band}.channel`]: 'auto' } }
		: { kind: 'set', tree: 'locations', node, path, value: w.width };
	const p = await ask(box, op);
	if (!p) return;
	const name = (id) => ctx.name('locations', id);
	const was = field ? `${field.value} MHz${field.from !== node ? ` (from ${name(field.from)})` : ''}` : 'not set by Aeolus';
	confirm(ctx, box, op, p, [
		h('div', null, h('strong', null, `${bandName(b.band)} width on ${nodeName}: `), was, ' → ', `${w.width} MHz`),
		w.auto && h('div', null, h('strong', null, 'Channel: '),
			`set to automatic on ${nodeName}, so each AP picks a free channel that fits the width`,
			w.moves?.length ? `. ${w.moves.map((m) => `${m.name} leaves channel ${m.from}`).join(', ')}.` : '.'),
		isAP && field && field.from !== node && h('div', { class: 'sub' },
			`This becomes a custom setting on ${nodeName}, instead of following ${name(field.from)}.`),
	], [
		w.radar && h('div', { class: 'sub warn' },
			`${w.width} MHz here uses radar (DFS) channels: after the change the radio listens for radar for about a minute before it transmits, and moves to another channel by itself if it hears any.`),
		h('div', { class: 'sub warn' },
			`Applying restarts the ${bandName(b.band)} radio on ${p.reversioned.length === 1 ? 'that AP' : 'each of them'}`,
			isAP && now?.clients === 0
				? '; no clients are on it right now.'
				: `; ${isAP && now ? `its ${now.clients} client${now.clients === 1 ? '' : 's'}` : 'clients on it'} drop for ${w.radar ? 'about a minute' : 'a few seconds'} and reconnect.`),
	]);
}
