// The channel map (0075): for each band, every real channel, shaded in the
// blocks the band's width makes, where the channels an automatic channel may
// be are picked a block at a time, as sets the APs can jump to. The APs on
// each channel now are marked under it.

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

// ranges are a band's 20 MHz channels, in the stretches of spectrum they
// fall in: 2.4 GHz's 1–11, or 1–13 outside North America; 5 GHz's three.
function ranges(band, country) {
	if (band === '2g') return [step(1, country === 'US' || country === 'CA' ? 11 : 13, 1)];
	return [step(36, 64, 4), step(100, 144, 4), step(149, 165, 4)];
}

// DEFAULT is what an unset set means: 1, 6 and 11 on 2.4 GHz, every
// channel on 5 GHz.
const DEFAULT = { '2g': () => [1, 6, 11], '5g': () => ranges('5g').flat() };

const radar = (band, c) => band === '5g' && c >= 52 && c <= 144;

// block is the channels a radio at width takes up with ch among them, or
// null where no block at that width holds it.
function block(band, ch, width) {
	if (band !== '5g' || width <= 20) return [ch];
	const b = (BLOCKS5[width] || []).find(([lo, hi]) => ch >= lo && ch <= hi);
	return b ? step(b[0], b[1], 4) : null;
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

// channelMap draws a map for each band the APs here have. at is the node's
// page ({ node, nodeName, page, canEdit, parentName }); rows are its APs,
// each with its config and condition.
export function channelMap(ctx, at, rows) {
	const bands = (at.page.hardware?.bands || []).map((b) => b.band).filter((b) => b === '2g' || b === '5g');
	if (!bands.length) return h('div', { class: 'banner info' }, 'No AP here has a 2.4 or 5 GHz radio.');
	return [
		h('div', { class: 'sub lead' }, 'The channels an automatic channel may be, for the APs here: picked a block at a time, at the band\'s width. RRM moves radios only within them.'),
		bands.map((band) => bandMap(ctx, at, band, rows)),
	];
}

function bandMap(ctx, at, band, rows) {
	const { node, nodeName, page, canEdit, parentName } = at;
	const path = `radio.${band}.channels`;
	const field = page.fields?.[path];
	const lockedAbove = field?.origin === 'locked' && field.from !== node;
	const editable = canEdit && !lockedAbove;
	const country = page.fields?.['system.country']?.value;
	const avoid = band === '5g' && page.fields?.['radio.5g.dfs']?.value === 'avoid';

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
		map.replaceChildren(...all.map((groups) => {
			return h('div', { class: 'chrange' }, groups.map((g, i) => {
				const all = g.whole && g.whole.every((c) => chosen.has(c));
				const some = g.whole && !all && g.whole.some((c) => chosen.has(c));
				const off = !g.whole || (avoid && g.chans.some((c) => radar(band, c)));
				// At 20 MHz each channel is its own block: one shade for all.
				return h('div', { class: 'chstack' }, bracket(g), h('div', { class: `chblock ${width > 20 && band === '5g' && i % 2 ? 'b' : 'a'}` }, g.chans.map((c) => {
					const aps = on.get(c) || [];
					const why = !g.whole ? `No ${width} MHz block includes ${c}`
						: avoid && radar(band, c) ? 'DFS is avoided here'
							: radar(band, c) ? 'Shared with radar (DFS)' : '';
					return h('button', {
						type: 'button',
						class: `ch${all ? ' on' : ''}${some ? ' part' : ''}${off ? ' off' : ''}${radar(band, c) ? ' dfs' : ''}`,
						disabled: !editable || off,
						title: [why, some && 'Part of this block was picked at another width', aps.length && `Now: ${aps.join(', ')}`].filter(Boolean).join(' · '),
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
		const usable = [...chosen].filter((c) => block(band, c, width)?.every((x) => chosen.has(x)) && !(avoid && radar(band, c)));
		count.textContent = `${usable.length} Channel${usable.length === 1 ? '' : 's'} Available`;
		count.className = usable.length ? 'chip ok' : 'chip bad';
		count.title = usable.length ? '' : `No whole ${width} MHz block is picked: the APs would have nowhere to go.`;
		save.disabled = save.disabled || !usable.length;
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
			h('div', { class: 'sub warn' }, `Applying restarts the ${bandName(band)} radio on each AP listed whose channel is automatic: its clients drop for a few seconds and reconnect, and it picks a channel from the set. RRM then moves it only within the set.`),
		]);
	});
	draw();

	panel.append(
		h('h2', null, h('span', { class: 'title' }, bandName(band), count), h('span', { class: 'note' },
			`${width} MHz blocks${fromAPs ? ', the width most APs here report' : ''}${avoid ? ' · DFS avoided' : ''}`)),
		h('div', { class: 'row' },
			h('div', { class: 'label' }, 'Channels'),
			h('div', { class: 'value' }, field ? spans(field.value, band === '2g' ? 1 : 4) : h('span', { class: 'sealed' }, band === '2g' ? '1, 6 and 11 (not set)' : 'any (not set)')),
			field && origin('locations', node, field, (id) => ctx.name('locations', id)),
			h('span', { class: 'controls' },
				editable && field?.origin === 'self' && followButton(ctx, 'locations', node, nodeName, parentName, [path], box),
				editable && reset, editable && save)),
		band === '5g' && dfsRow({ ctx, node, nodeName, page, isAP: page.node.kind === 'ap', canEdit, parentName },
			reports.find((x) => x.r.band === '5g')?.r, box),
		map,
		box);
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

