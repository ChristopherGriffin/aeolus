// Radio hardware settings on a Locations folder or AP (0044). A folder offers
// only what every AP it reaches can do, so the whole folder runs the same; an
// AP offers what its own radios can do, as its own custom setting. The
// manager works out what is offered (the page's "hardware"); this draws it.
// A width some AP's channel cannot carry sets the channel to automatic in
// the same change, so each AP picks one that fits (0045). A band set here
// can follow the folder above again (0046).

import { h } from '../dom.js';
import { bandName, origin, ago } from '../format.js';
import { ask, confirm } from './confirm.js';
import { followButton } from './follow.js';

// hardwarePanel draws the panel. report is the AP's latest state report, for
// an AP's page.
export function hardwarePanel(ctx, node, nodeName, page, report) {
	const hw = page.hardware;
	if (!hw) return null;
	const isAP = page.node.kind === 'ap';
	const canEdit = page.role === 'operator' || page.role === 'admin';
	const reported = new Map((report?.report?.radios || []).map((r) => [r.band, r]));
	const reach = hw.aps.length === 1 ? hw.aps[0].name : `${hw.aps.length} APs`;
	return h('section', { class: 'panel' },
		h('h2', null, isAP ? 'Radios' : 'Hardware',
			h('span', { class: 'note' }, isAP
				? (report ? 'reported ' + ago(report.at) : 'no report yet')
				: hw.aps.length ? `applies to ${reach} below, unless an AP sets its own` : 'no APs here yet')),
		hw.unknown.length > 0 && h('div', { class: 'empty' },
			`${hw.unknown.map((a) => a.name).join(', ')} never said what its radios can do, so ${hw.unknown.length === 1 ? 'it is' : 'they are'} not counted.`),
		hw.bands.length === 0
			? h('div', { class: 'empty' }, isAP ? 'This AP has not said what radios it has.' : 'No AP here has said what radios it has.')
			: hw.bands.map((b) => band(ctx, node, nodeName, page, b, reported.get(b.band), isAP, canEdit)));
}

function band(ctx, node, nodeName, page, b, now, isAP, canEdit) {
	const path = `radio.${b.band}.width`;
	const field = page.fields?.[path];
	const lockedAbove = field?.origin === 'locked' && field.from !== node;
	const box = h('div', { class: 'edit' });

	const select = h('select', { 'aria-label': `${bandName(b.band)} width` },
		h('option', { value: '' }, field ? `Keep ${field.value} MHz` : 'Choose a width'),
		b.widths.map((w) => h('option', { value: String(w.width), disabled: !w.ok }, `${w.width} MHz`,
			!w.ok ? ` (${w.why})` : w.auto ? ' (channel becomes automatic)' : '')));
	const change = h('button', { type: 'button', class: 'button', disabled: true }, 'Change…');
	select.addEventListener('change', () => { change.disabled = select.value === ''; });
	change.addEventListener('click', () => preview(ctx, node, nodeName, isAP, b, now, path, field, select.value, box));

	// Following the folder again takes the band's channel too, if it is set
	// here: a width set with an automatic channel goes back as one.
	const channel = `radio.${b.band}.channel`;
	const follow = field?.origin === 'self' && followButton(ctx, 'locations', node, nodeName,
		page.node.parent ? ctx.name('locations', page.node.parent) : null,
		page.fields?.[channel]?.origin === 'self' ? [path, channel] : [path], box);

	let current;
	if (field) current = `${field.value} MHz`;
	else if (isAP) current = h('span', { class: 'sealed' }, `not set by Aeolus; the AP keeps its own${now?.width ? ` (${now.width} MHz now)` : ''}`);
	else current = h('span', { class: 'sealed' }, 'not set; each AP keeps its own');

	return h('div', { class: 'radio' },
		h('div', { class: 'radio-head' },
			h('strong', null, bandName(b.band)),
			!isAP && h('span', { class: 'sub' }, `${b.aps} AP${b.aps === 1 ? '' : 's'} with this band`),
			isAP && now && h('span', { class: 'sub' }, `channel ${now.channel || '—'} · ${now.width ? now.width + ' MHz' : '—'} · ${now.clients} client${now.clients === 1 ? '' : 's'}`)),
		h('div', { class: 'row' },
			h('div', { class: 'label' }, 'Width'),
			h('div', { class: 'value' }, current),
			field && origin('locations', node, field, (id) => ctx.name('locations', id)),
			isAP && field?.origin === 'self' && h('span', { class: 'chip warn' }, 'custom'),
			canEdit && !lockedAbove && h('span', { class: 'controls' }, follow, select, change)),
		box);
}

// preview shows what a width change would do, before anything is recorded.
async function preview(ctx, node, nodeName, isAP, b, now, path, field, choice, box) {
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
