// A Locations node's radios, one card per band (0047): its width, offered
// only within what every AP the node reaches can do (0044), and its channel,
// power and on/off, each with where it comes from. A width some AP's channel
// cannot carry sets the channel to automatic in the same change, so each AP
// picks one that fits (0045). A value set here can follow the folder above
// again (0046). On 5 GHz, DFS channels can be avoided (0071).

import { h } from '../dom.js';
import { bandName, origin, ago, value } from '../format.js';
import { ask, confirm } from './confirm.js';
import { followButton } from './follow.js';

const BANDS = ['2g', '5g', '6g'];
const OTHER = [['channel', 'Channel'], ['power', 'Power'], ['enabled', 'Radio on']];

// radiosSection draws the cards. report is the AP's latest state report, for
// an AP's page.
export function radiosSection(ctx, node, nodeName, page, report) {
	const hw = page.hardware;
	if (!hw) return null;
	const isAP = page.node.kind === 'ap';
	const canEdit = page.role === 'operator' || page.role === 'admin';
	const parentName = page.node.parent ? ctx.name('locations', page.node.parent) : null;
	const reported = new Map((report?.report?.radios || []).map((r) => [r.band, r]));
	const offered = new Map(hw.bands.map((b) => [b.band, b]));
	const reach = hw.aps.length === 1 ? hw.aps[0].name : `${hw.aps.length} APs`;
	const at = { ctx, node, nodeName, page, isAP, canEdit, parentName };
	return [
		h('div', { class: 'sub lead' }, isAP
			? (report ? `Reported ${ago(report.at)}.` : 'No report yet.')
			: hw.aps.length ? `Applies to ${reach} below, unless an AP sets its own.` : 'No APs here yet.'),
		hw.unknown.length > 0 && h('div', { class: 'banner info' },
			`${hw.unknown.map((a) => a.name).join(', ')} never said what its radios can do, so ${hw.unknown.length === 1 ? 'it is' : 'they are'} not counted.`),
		h('div', { class: 'bands' }, BANDS.map((band) => card(at, band, offered.get(band), reported.get(band)))),
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
			return [h('div', { class: 'row' },
				h('div', { class: 'label' }, label),
				h('div', { class: 'value' }, field ? value(path, field.value) : h('span', { class: 'sealed' }, unset)),
				field && origin('locations', node, field, (id) => ctx.name('locations', id)),
				isAP && field?.origin === 'self' && h('span', { class: 'chip warn' }, 'custom'),
				at.canEdit && field?.origin === 'self' && h('span', { class: 'controls' },
					followButton(ctx, 'locations', node, at.nodeName, at.parentName, [path], box))),
			k === 'channel' && band === '5g' && dfsRow(at, now, box)];
		}),
		box);
}

// dfsRow is whether the 5 GHz radio may use the DFS channels, 52–144, which
// it shares with radar (0071). Avoided, an automatic channel is picked
// outside them; allowed, the default, it may be any.
function dfsRow(at, now, box) {
	const { ctx, node, nodeName, page, isAP, canEdit, parentName } = at;
	const path = 'radio.5g.dfs';
	const field = page.fields?.[path];
	const lockedAbove = field?.origin === 'locked' && field.from !== node;
	const next = field?.value === 'avoid' ? 'allow' : 'avoid';
	return h('div', { class: 'row' },
		h('div', { class: 'label' }, 'DFS channels'),
		h('div', { class: 'value' }, field ? value(path, field.value) : h('span', { class: 'sealed' }, 'allowed (not set)')),
		field && origin('locations', node, field, (id) => ctx.name('locations', id)),
		isAP && field?.origin === 'self' && h('span', { class: 'chip warn' }, 'custom'),
		canEdit && !lockedAbove && h('span', { class: 'controls' },
			field?.origin === 'self' && followButton(ctx, 'locations', node, nodeName, parentName, [path], box),
			h('button', { type: 'button', class: 'button small', onclick: () => dfsPreview(at, now, path, field, next, box) },
				next === 'avoid' ? 'Avoid…' : 'Allow…')));
}

async function dfsPreview(at, now, path, field, choice, box) {
	const { ctx, node, nodeName, isAP } = at;
	const op = { kind: 'set', tree: 'locations', node, path, value: choice };
	const p = await ask(box, op);
	if (!p) return;
	const was = field ? `${value(path, field.value)}${field.from !== node ? ` (from ${ctx.name('locations', field.from)})` : ''}` : 'allowed (not set)';
	const clients = isAP && now ? `its ${now.clients} client${now.clients === 1 ? '' : 's'}` : 'clients on it';
	confirm(ctx, box, op, p, [
		h('div', null, h('strong', null, `DFS channels on ${nodeName}: `), was, ' → ', value(path, choice)),
	], [
		h('div', { class: 'sub warn' }, choice === 'avoid'
			? `Applying restarts the 5 GHz radio where its channel is automatic, and it picks one outside 52–144; ${clients} drop for a few seconds and reconnect. A set channel stays as it is.`
			: `Applying restarts the 5 GHz radio where its channel is automatic, and it may pick a DFS channel: it then listens for radar for about a minute before it transmits, and ${clients} wait that long to reconnect.`),
	]);
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
