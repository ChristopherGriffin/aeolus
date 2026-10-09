// The channel map (0075, 0087): for each band, every real channel, shaded in
// the blocks the band's width makes, where the channels an automatic channel
// may be are picked a block at a time, as sets the APs can jump to. The APs
// on each channel now are marked under it. On 6 GHz, the preferred scanning
// channels are marked, and the set can be kept to them, and to one channel
// a block.

import { h } from '../dom.js';
import { bandName, origin, value } from '../format.js';
import { ask, confirm } from './confirm.js';
import { followButton } from './follow.js';

// The 5 GHz blocks at each width, by their lowest and highest 20 MHz
// channel, as internal/radio has them.
const BLOCKS5 = {
	40: [[36, 40], [44, 48], [52, 56], [60, 64], [100, 104], [108, 112], [116, 120], [124, 128], [132, 136], [140, 144], [149, 153], [157, 161]],
	80: [[36, 48], [52, 64], [100, 112], [116, 128], [132, 144], [149, 161]],
	160: [[36, 64], [100, 128]],
};

const step = (lo, hi, by) => Array.from({ length: (hi - lo) / by + 1 }, (_, i) => lo + i * by);

// The 6 GHz blocks at each width, 1–233, as internal/radio has them (0087).
// 320 MHz blocks come in two families that overlap; the map draws the first
// (1–61, 65–125, 129–189), so 193–233 are in no block it draws at 320.
const BLOCKS6 = {
	40: step(1, 225, 8).map((lo) => [lo, lo + 4]),
	80: step(1, 209, 16).map((lo) => [lo, lo + 12]),
	160: step(1, 193, 32).map((lo) => [lo, lo + 28]),
	320: [[1, 61], [65, 125], [129, 189]],
};

// psc says whether a 6 GHz channel is a preferred scanning channel, one
// clients look for 6 GHz networks on by themselves: 5, 21, 37 … 229.
export const psc = (c) => c >= 5 && c <= 229 && (c - 5) % 16 === 0;

// ranges are a band's 20 MHz channels, in the stretches of spectrum they
// fall in: 2.4 GHz's 1–11, or 1–13 outside North America; 5 GHz's three;
// 6 GHz's 1–233 in four.
function ranges(band, country) {
	if (band === '2g') return [step(1, country === 'US' || country === 'CA' ? 11 : 13, 1)];
	// 6 GHz in 320 MHz stretches, which wrap as 5 GHz's ranges do: each 40,
	// 80 and 160 MHz block falls wholly in one (Griff, 2026-10-09).
	if (band === '6g') return [step(1, 61, 4), step(65, 125, 4), step(129, 189, 4), step(193, 233, 4)];
	return [step(36, 64, 4), step(100, 144, 4), step(149, 165, 4)];
}

// DEFAULT is what an unset set means: 1, 6 and 11 on 2.4 GHz, every
// channel on 5 and 6 GHz.
const DEFAULT = { '2g': () => [1, 6, 11], '5g': () => ranges('5g').flat(), '6g': () => ranges('6g').flat() };

const radar = (band, c) => band === '5g' && c >= 52 && c <= 144;

// block is the channels a radio at width takes up with ch among them, or
// null where no block at that width holds it.
function block(band, ch, width) {
	if (band === '2g' || width <= 20) return [ch];
	const b = ((band === '6g' ? BLOCKS6 : BLOCKS5)[width] || []).find(([lo, hi]) => ch >= lo && ch <= hi);
	return b ? step(b[0], b[1], 4) : null;
}

// usable is what the APs may go to from a set, as internal/radio's Usable
// has it: the set's whole blocks; on 6 GHz, with onlyPSC, preferred
// scanning channels alone, and with spread, one channel to a block.
function usable(band, chosen, width, avoid, onlyPSC, spread) {
	const out = [];
	const seen = new Set();
	for (const c of [...chosen].sort((a, b) => a - b)) {
		const b = block(band, c, width);
		if (!b || !b.every((x) => chosen.has(x)) || (avoid && radar(band, c))) continue;
		if (spread && width > 20) {
			if (seen.has(b[0])) continue;
			const first = b.find((x) => !onlyPSC || psc(x));
			if (first === undefined) continue;
			seen.add(b[0]);
			out.push(first);
		} else if (!onlyPSC || psc(c)) out.push(c);
	}
	return out;
}

// spans writes a set of channels as runs, such as 36–48, 149–161, each run
// channels by apart.
function spans(list, by) {
	const runs = [];
	for (const c of [...list].sort((a, b) => a - b)) {
		const last = runs[runs.length - 1];
		if (last && c === last[1] + by) last[1] = c;
		else runs.push([c, c]);
	}
	return runs.map(([a, b]) => (a === b ? String(a) : `${a}–${b}`)).join(', ');
}

// bandMap is one band's channel map, for the node at.
export function bandMap(ctx, at, band, rows) {
	const { node, nodeName, page, canEdit, parentName } = at;
	const path = `radio.${band}.channels`;
	const field = page.fields?.[path];
	const lockedAbove = field?.origin === 'locked' && field.from !== node;
	const editable = canEdit && !lockedAbove;
	const country = page.fields?.['system.country']?.value;
	const avoid = band === '5g' && page.fields?.['radio.5g.dfs']?.value === 'avoid';
	// On 6 GHz: preferred scanning channels only, and one channel a block.
	const onlyPSC = band === '6g' && page.fields?.['radio.6g.psc']?.value === true;
	const spread = band === '6g' && page.fields?.['radio.6g.non_overlapping']?.value === true;
	const auto = page.fields?.[`radio.${band}.channel`]?.value === 'auto';

	// The width the blocks are drawn at: the band's in force here, or what
	// most of the APs below report.
	const reports = rows.flatMap(({ ap, cfg }) => (cfg?.condition?.state?.report?.radios || [])
		.filter((r) => r.band === band).map((r) => ({ ap, r })));
	let width = page.fields?.[`radio.${band}.width`]?.value;
	const fromAPs = width == null;
	if (fromAPs) {
		const count = {};
		for (const { r } of reports) if (r.width) count[r.width] = (count[r.width] || 0) + 1;
		width = Number(Object.entries(count).sort((a, b) => b[1] - a[1])[0]?.[0] || 20);
	}

	// The APs on each channel now.
	const on = new Map();
	for (const { ap, r } of reports) {
		for (const c of (r.channel && block(band, r.channel, r.width || 20)) || []) {
			if (!on.has(c)) on.set(c, []);
			on.get(c).push(ap.name);
		}
	}

	const saved = new Set(field ? field.value : DEFAULT[band]());
	let chosen = new Set(saved);
	const box = h('div', { class: 'edit' });
	const map = h('div', { class: 'chmap' });
	// How many channels the set leaves the APs, beside the band's name.
	const count = h('span', { class: 'chip' });
	const save = h('button', { type: 'button', class: 'button small primary', disabled: true }, 'Save…');
	const reset = h('button', { type: 'button', class: 'button small', disabled: true }, 'Undo');
	const panel = h('section', { class: 'panel' });

	const same = (a, b) => a.size === b.size && [...a].every((c) => b.has(c));
	const draw = () => {
		const dirty = !same(chosen, saved);
		if (dirty) panel.setAttribute('data-editing', 'true');
		else panel.removeAttribute('data-editing');
		save.disabled = !dirty;
		reset.disabled = !dirty;
		// Each range's channels, grouped by block, so each block reads as one.
		const all = ranges(band, country).map((list) => {
			const groups = [];
			for (const c of list) {
				const b = block(band, c, width);
				const key = b ? b[0] : `x${c}`;
				const last = groups[groups.length - 1];
				if (last && last.key === key) last.chans.push(c);
				else groups.push({ key, chans: [c], whole: b });
			}
			return groups;
		});
		// The DFS bracket, over 52–144: a piece above each block, joined to
		// the next across the gap, with a tick and the label where it starts
		// and a tick where it ends.
		const isDFS = (g) => g && g.chans.some((c) => radar(band, c));
		const flat = all.flat();
		const bracket = (g) => {
			if (band !== '5g') return null;
			if (!isDFS(g)) return h('div', { class: 'chbr' });
			const i = flat.indexOf(g);
			const start = !isDFS(flat[i - 1]);
			const end = !isDFS(flat[i + 1]);
			const r = all.findIndex((groups) => groups.includes(g));
			const join = !end && all[r][all[r].length - 1] === g; // across the gap between ranges
			const cont = !start && all[r][0] === g; // the bracket's continuation from the range before
			return h('div', { class: `chbr dfs${start ? ' start' : ''}${end ? ' end' : ''}${join ? ' join' : ''}${cont ? ' cont' : ''}` },
				h('span', { class: 'lbl' }, 'DFS'));
		};
		// The channels the APs go to from the set, marked where that is not
		// every channel picked: with preferred scanning channels only, or one
		// channel a block (0087).
		const goes = new Set(usable(band, chosen, width, avoid, onlyPSC, spread));
		const marked = onlyPSC || spread;
		map.replaceChildren(...all.map((groups) => {
			return h('div', { class: 'chrange' }, groups.map((g, i) => {
				const all = g.whole && g.whole.every((c) => chosen.has(c));
				const some = g.whole && !all && g.whole.some((c) => chosen.has(c));
				const off = !g.whole || (avoid && g.chans.some((c) => radar(band, c))) || (onlyPSC && !g.whole.some(psc));
				// At 20 MHz each channel is its own block: one shade for all.
				return h('div', { class: 'chstack' }, bracket(g), h('div', { class: `chblock ${width > 20 && band !== '2g' && i % 2 ? 'b' : 'a'}` }, g.chans.map((c) => {
					const aps = on.get(c) || [];
					const why = !g.whole ? `No ${width} MHz block includes ${c}`
						: avoid && radar(band, c) ? 'DFS is avoided here'
							: onlyPSC && !g.whole.some(psc) ? 'No preferred scanning channel in this block'
								: radar(band, c) ? 'Shared with radar (DFS)' : '';
					return h('button', {
						type: 'button',
						class: `ch${all ? ' on' : ''}${some ? ' part' : ''}${off ? ' off' : ''}${radar(band, c) ? ' dfs' : ''}${band === '6g' && psc(c) ? ' psc' : ''}${marked && goes.has(c) ? ' goes' : ''}`,
						disabled: !editable || off,
						title: [why, band === '6g' && psc(c) && 'Preferred scanning channel', marked && goes.has(c) && 'The APs go to this channel',
							some && 'Part of this block was picked at another width', aps.length && `Now: ${aps.join(', ')}`].filter(Boolean).join(' · '),
						onclick: () => {
							const next = new Set(chosen);
							for (const x of g.whole) (all ? next.delete(x) : next.add(x));
							chosen = next;
							draw();
						},
					}, h('span', null, String(c)), h('span', { class: aps.length ? 'ap' : 'ap none' }));
				})));
			}));
		}));
		requestAnimationFrame(joinBracket);
		count.textContent = `${goes.size} Channel${goes.size === 1 ? '' : 's'} Available`;
		count.className = goes.size ? 'chip ok' : 'chip bad';
		count.title = goes.size ? '' : `No whole ${width} MHz block${onlyPSC ? ' with a preferred scanning channel' : ''} is picked: the APs would have nowhere to go.`;
		save.disabled = save.disabled || !goes.size;
	};
	// joinBracket draws the DFS bracket as one where 52–64 and 100–144 sit on
	// one line, and as two, each with its ticks and label, where they wrap.
	const joinBracket = () => {
		const ranges = [...map.querySelectorAll('.chrange')];
		for (const [i, r] of ranges.entries()) {
			const first = r.querySelector('.chbr.cont');
			if (!first) continue;
			const prev = ranges[i - 1]?.querySelector('.chbr.join');
			const wrapped = !prev || r.offsetTop !== ranges[i - 1].offsetTop;
			first.classList.toggle('start', wrapped);
			prev?.classList.toggle('end', wrapped);
		}
	};
	new ResizeObserver(() => joinBracket()).observe(map);
	reset.addEventListener('click', () => { chosen = new Set(saved); draw(); });
	save.addEventListener('click', async () => {
		const value = [...chosen].sort((a, b) => a - b);
		const op = { kind: 'set', tree: 'locations', node, path, value };
		const p = await ask(box, op);
		if (!p) return;
		const list = (s) => [...s].sort((a, b) => a - b).join(', ');
		confirm(ctx, box, op, p, [
			h('div', null, h('strong', null, `${bandName(band)} channels on ${nodeName}: `),
				field ? list(saved) : `${list(saved)} (not set)`, ' → ', list(chosen)),
		], [
			h('div', { class: 'sub warn' }, `Applying restarts the ${bandName(band)} radio on each AP listed whose channel is automatic: its clients drop for a few seconds and reconnect, and it picks a channel from the set.${band === '6g' ? '' : ' RRM then moves it only within the set.'}`),
		]);
	});
	draw();

	// append, unlike h, would write a false out as text.
	panel.append(...[
		h('h2', null, h('span', { class: 'title' }, bandName(band), count), h('span', { class: 'note' },
			`${width} MHz blocks${fromAPs ? ', the width most APs here report' : ''}${avoid ? ' · DFS avoided' : ''}${onlyPSC ? ' · preferred scanning channels only' : ''}${spread ? ' · one channel a block' : ''}`)),
		!auto && h('div', { class: 'sub' }, `The set is for an automatic channel. Here the ${bandName(band)} channel is ${page.fields?.[`radio.${band}.channel`] ? `set to ${page.fields[`radio.${band}.channel`].value}` : 'not set, so each AP keeps its own'}: make it automatic on the band's card for the set to take effect.`),
		h('div', { class: 'row' },
			h('div', { class: 'label' }, 'Channels'),
			h('div', { class: 'value' }, field ? spans(field.value, band === '2g' ? 1 : 4) : h('span', { class: 'sealed' }, band === '2g' ? '1, 6 and 11 (not set)' : 'any (not set)')),
			field && origin('locations', node, field, (id) => ctx.name('locations', id)),
			h('span', { class: 'controls' },
				editable && field?.origin === 'self' && followButton(ctx, 'locations', node, nodeName, parentName, [path], box),
				editable && reset, editable && save)),
		band === '5g' && dfsRow({ ctx, node, nodeName, page, isAP: page.node.kind === 'ap', canEdit, parentName },
			reports.find((x) => x.r.band === '5g')?.r, box),
		band === '6g' && switchRow({ ctx, node, nodeName, page, isAP: page.node.kind === 'ap', canEdit, parentName }, 'radio.6g.psc',
			'Preferred scanning channels only', 'Clients look for 6 GHz networks on 5, 21, 37 and every 16th to 229 by themselves; elsewhere only where another band’s beacons send them.', box),
		band === '6g' && switchRow({ ctx, node, nodeName, page, isAP: page.node.kind === 'ap', canEdit, parentName }, 'radio.6g.non_overlapping',
			'One channel a block', 'Each block of the width offers one channel, so radios that pick different channels never share a block: at 160 MHz with preferred scanning channels only, 5, 37, 69 …, not also 21, 53 ….', box),
		map,
		box,
	].filter(Boolean));
	return panel;
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

// switchRow is one of 6 GHz's on/off settings for an automatic channel
// (0087): preferred scanning channels only, or one channel a block. Off
// unless set.
function switchRow(at, path, label, about, box) {
	const { ctx, node, nodeName, page, isAP, canEdit, parentName } = at;
	const field = page.fields?.[path];
	const lockedAbove = field?.origin === 'locked' && field.from !== node;
	const on = field?.value === true;
	const preview = async () => {
		const op = { kind: 'set', tree: 'locations', node, path, value: !on };
		const p = await ask(box, op);
		if (!p) return;
		confirm(ctx, box, op, p, [
			h('div', null, h('strong', null, `${label} on ${nodeName}: `), on ? 'on' : 'off', ' → ', on ? 'off' : 'on'),
			h('div', { class: 'sub' }, about),
		], [
			h('div', { class: 'sub warn' }, 'Applying restarts the 6 GHz radio on each AP listed whose channel is automatic: its clients drop for a few seconds and reconnect, and it picks a channel again.'),
		]);
	};
	return h('div', { class: 'row' },
		h('div', { class: 'label', title: about }, label),
		h('div', { class: 'value' }, field ? value(path, field.value) : h('span', { class: 'sealed' }, 'off (not set)')),
		field && origin('locations', node, field, (id) => ctx.name('locations', id)),
		isAP && field?.origin === 'self' && h('span', { class: 'chip warn' }, 'custom'),
		canEdit && !lockedAbove && h('span', { class: 'controls' },
			field?.origin === 'self' && followButton(ctx, 'locations', node, nodeName, parentName, [path], box),
			h('button', { type: 'button', class: 'button small', onclick: preview }, on ? 'Turn off…' : 'Turn on…')));
}
