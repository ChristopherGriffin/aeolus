// Interfaces › Radios › Bands and Channels (0047, 0075, 0087): one panel a
// band. The radio's on/off and its width head it; the band's tick boxes
// (DFS channels on 5 GHz; preferred scanning channels and one channel a
// block on 6 GHz) sit under that; then the channel map; then the channel,
// automatic or manual, the power, the 802.11 generations clients may use
// and the guard intervals (0089). Everything is a draft until Save, which
// previews it as one change (Griff, 2026-10-09).
//
// Automatic, the map picks the channels an automatic channel may be, a block
// at a time. Manual, it picks the one channel, its block shown at the width.
// A width some AP's channel can't carry makes the channel automatic in the
// same change (0045). Where a value comes from is said beside it: from a
// folder above, or on an AP's page its own where nothing sets it. Set here,
// or unset on a folder, it says nothing (Griff, 2026-10-09).

import { h } from '../dom.js';
import { bandName, ago } from '../format.js';
import { ask, confirm } from './confirm.js';
import { followButton } from './follow.js';

const BANDS = ['2g', '5g', '6g'];

// The 802.11 generations of each band, oldest first; the htmode family
// each takes; and the widest that family goes, as internal/radio has them
// (0089).
const GENERATIONS = { '2g': ['b', 'g', 'n', 'ax', 'be'], '5g': ['a', 'n', 'ac', 'ax', 'be'], '6g': ['ax', 'be'] };
const FAMILY_WIDTH = { b: 20, g: 20, a: 20, n: 40, ac: 160, ax: 160, be: 320 };
const HE_GI = [800, 1600, 3200];

// run is a band's generations from the oldest to the newest of a list.
function run(band, list) {
	const order = GENERATIONS[band];
	const at = list.map((m) => order.indexOf(m)).filter((i) => i >= 0);
	return at.length ? order.slice(Math.min(...at), Math.max(...at) + 1) : [];
}

const step = (lo, hi, by) => Array.from({ length: (hi - lo) / by + 1 }, (_, i) => lo + i * by);

// The blocks at each width, by their lowest and highest 20 MHz channel, as
// internal/radio has them. 6 GHz's 320 MHz blocks come in two families
// that overlap; the map draws the first.
const BLOCKS = {
	'5g': {
		40: [[36, 40], [44, 48], [52, 56], [60, 64], [100, 104], [108, 112], [116, 120], [124, 128], [132, 136], [140, 144], [149, 153], [157, 161]],
		80: [[36, 48], [52, 64], [100, 112], [116, 128], [132, 144], [149, 161]],
		160: [[36, 64], [100, 128]],
	},
	'6g': {
		40: step(1, 225, 8).map((lo) => [lo, lo + 4]),
		80: step(1, 209, 16).map((lo) => [lo, lo + 12]),
		160: step(1, 193, 32).map((lo) => [lo, lo + 28]),
		320: [[1, 61], [65, 125], [129, 189]],
	},
};

// psc says whether a 6 GHz channel is a preferred scanning channel, one
// clients look for 6 GHz networks on by themselves: 5, 21, 37 … 229.
const psc = (c) => c >= 5 && c <= 229 && (c - 5) % 16 === 0;
const radar = (band, c) => band === '5g' && c >= 52 && c <= 144;

// ranges are a band's 20 MHz channels, in rows that wrap: 2.4 GHz's 1–11,
// or 1–13 outside North America; 5 GHz's three stretches; 6 GHz's 1–233 in
// four of 320 MHz, each holding its 40, 80 and 160 MHz blocks whole.
function ranges(band, country) {
	if (band === '2g') return [step(1, country === 'US' || country === 'CA' ? 11 : 13, 1)];
	if (band === '6g') return [step(1, 61, 4), step(65, 125, 4), step(129, 189, 4), step(193, 233, 4)];
	return [step(36, 64, 4), step(100, 144, 4), step(149, 165, 4)];
}

// What an unset set means: 1, 6 and 11 on 2.4 GHz; every channel elsewhere.
const allOf = (band) => (band === '2g' ? [1, 6, 11] : ranges(band).flat());

// block is the channels a radio at width takes up with ch among them, or
// null where no block at that width holds it.
function block(band, ch, width) {
	if (band === '2g' || width <= 20) return [ch];
	const b = (BLOCKS[band]?.[width] || []).find(([lo, hi]) => ch >= lo && ch <= hi);
	return b ? step(b[0], b[1], 4) : null;
}

// usable is what the APs may go to from a set, as internal/radio's Usable
// has it: the set's whole blocks, outside DFS where it is not allowed; on
// 6 GHz, with onlyPSC, preferred scanning channels alone, and with spread,
// one channel a block.
function usable(band, chosen, width, noDFS, onlyPSC, spread) {
	const out = [];
	const seen = new Set();
	for (const c of [...chosen].sort((a, b) => a - b)) {
		const b = block(band, c, width);
		if (!b || !b.every((x) => chosen.has(x)) || (noDFS && b.some((x) => radar(band, x)))) continue;
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

// reported says what the APs run, as they last reported it, for power
// (what power control holds a radio at, 0077, else at most what it sends)
// or whether the radio is on: one value where all agree, else each AP's.
function reported(rows, band, k) {
	const seen = rows.map(({ ap, cfg }) => {
		const rep = cfg?.condition?.state?.report;
		const r = (rep?.radios || []).find((x) => x.band === band);
		const held = (rep?.rrm?.apc || []).find((x) => x.band === band && x.power != null);
		if (k === 'power' && held) return { name: ap.name, text: `${held.power} dBm, power control` };
		const v = k === 'power' ? r?.txpower : r?.up;
		return v == null ? null : { name: ap.name, text: k === 'power' ? `${v} dBm` : v ? 'on' : 'off' };
	}).filter(Boolean);
	if (!seen.length) return null;
	const texts = [...new Set(seen.map((s) => s.text))];
	return texts.length === 1 ? texts[0] : seen.map((s) => `${s.name} ${s.text}`).join(' · ');
}

// bandsSection is the view: a line on what it applies to, then a panel a
// band. report is the AP's latest state report, on an AP's page; rows are
// the APs the node reaches, each with its config and condition.
export function bandsSection(ctx, node, page, report, rows = []) {
	const hw = page.hardware;
	if (!hw) return null;
	const isAP = page.node.kind === 'ap';
	const reach = hw.aps.length === 1 ? hw.aps[0].name : `${hw.aps.length} APs`;
	const at = {
		ctx, node, page, isAP, rows,
		nodeName: page.node.name,
		canEdit: page.role === 'operator' || page.role === 'admin',
		parentName: page.node.parent ? ctx.name('locations', page.node.parent) : null,
	};
	const offered = new Map(hw.bands.map((b) => [b.band, b]));
	return [
		h('div', { class: 'sub lead' }, isAP
			? (report ? `Reported ${ago(report.at)}.` : 'No report yet.')
			: hw.aps.length ? `Applies to ${reach} below.` : 'No APs here yet.'),
		hw.unknown.length > 0 && h('div', { class: 'banner info' },
			`${hw.unknown.map((a) => a.name).join(', ')} never said what its radios can do, so ${hw.unknown.length === 1 ? 'it is' : 'they are'} not counted.`),
		BANDS.map((band) => (offered.get(band)
			? bandPanel(at, band, offered.get(band))
			: h('section', { class: 'panel band none' }, h('h2', null, bandName(band)),
				h('div', { class: 'empty' }, isAP ? 'This AP has no radio for this band.' : 'No AP here has a radio for this band.')))),
	];
}

// bandPanel is one band: its draft, drawn, and saved as one change.
function bandPanel(at, band, b) {
	const { ctx, node, page, isAP, rows, nodeName, canEdit, parentName } = at;
	// What a pulldown shows where nothing sets the value: the AP's own, on
	// its page; on a folder, a dash.
	const unset = isAP ? 'Its own' : '—';
	const f = (k) => page.fields?.[`radio.${band}.${k}`];
	const editable = (k) => canEdit && !(f(k)?.origin === 'locked' && f(k).from !== node);
	// whence says where a value comes from: nothing where it is set here,
	// or on a folder where nothing sets it.
	const whence = (k) => {
		const r = f(k);
		if (!r) return isAP ? 'its own' : '';
		if (r.from === node) return r.origin === 'locked' ? 'locked here' : '';
		return `${r.origin === 'locked' ? 'locked by' : 'from'} ${ctx.name('locations', r.from)}`;
	};
	const reports = rows.flatMap(({ ap, cfg }) => (cfg?.condition?.state?.report?.radios || [])
		.filter((r) => r.band === band).map((r) => ({ ap, r })));
	const country = page.fields?.['system.country']?.value;

	// The width the map is drawn at, where none is set: what most APs report.
	let apsWidth = 20;
	{
		const count = {};
		for (const { r } of reports) if (r.width) count[r.width] = (count[r.width] || 0) + 1;
		apsWidth = Number(Object.entries(count).sort((x, y) => y[1] - x[1])[0]?.[0] || 20);
	}
	const chan = f('channel')?.value;
	const saved = {
		enabled: f('enabled')?.value,
		width: f('width')?.value,
		mode: chan === undefined ? 'unset' : chan === 'auto' ? 'auto' : 'manual',
		channel: typeof chan === 'number' ? chan : null,
		channels: new Set(f('channels')?.value ?? allOf(band)),
		dfs: f('dfs')?.value,
		psc: f('psc')?.value,
		non_overlapping: f('non_overlapping')?.value,
		power: f('power')?.value,
		modes: f('modes')?.value,
		short_gi: f('short_gi')?.value,
		he_gi: f('he_gi')?.value,
	};
	let d = { ...saved, channels: new Set(saved.channels) };
	const width = () => d.width ?? apsWidth;
	const noDFS = () => band === '5g' && d.dfs === 'avoid';
	const onlyPSC = () => band === '6g' && d.psc === true;
	const spread = () => band === '6g' && d.non_overlapping === true;

	// The APs on each channel now.
	const on = new Map();
	for (const { ap, r } of reports)
		for (const c of (r.channel && block(band, r.channel, r.width || 20)) || []) {
			if (!on.has(c)) on.set(c, []);
			on.get(c).push(ap.name);
		}

	const box = h('div', { class: 'edit' });
	const panel = h('section', { class: 'panel bandpanel' });
	const count = h('span', { class: 'chip' });
	const map = h('div', { class: 'chmap' });

	// The radio's on/off, top left.
	const radioOn = h('input', { type: 'checkbox', class: 'switch', disabled: !editable('enabled'),
		title: `Radio on${f('enabled') ? ` · ${whence('enabled') || 'set here'}` : ''}` });
	radioOn.checked = d.enabled ?? (reported(rows, band, 'enabled') !== 'off');
	radioOn.addEventListener('change', () => { d.enabled = radioOn.checked; draw(); });

	// The width, top right.
	const widthSel = h('select', { 'aria-label': `${bandName(band)} width`, disabled: !editable('width'),
		title: `Width${f('width') ? ` · ${whence('width') || 'set here'}` : ''}` },
		saved.width === undefined && h('option', { value: '' }, isAP ? `${apsWidth} MHz, its own` : '—'),
		b.widths.map((w) => h('option', { value: String(w.width), disabled: !w.ok, selected: w.width === saved.width },
			`${w.width} MHz`, !w.ok ? ` (${w.why})` : '')));
	const widthOpts = [...widthSel.options].filter((o) => o.value !== '');
	widthSel.addEventListener('change', () => {
		d.width = widthSel.value === '' ? undefined : Number(widthSel.value);
		// A width some AP's channel can't carry makes it automatic (0045).
		const w = b.widths.find((x) => x.width === d.width);
		if (w?.auto && d.mode !== 'auto') d.mode = 'auto';
		if (d.mode === 'manual' && d.channel != null && !block(band, d.channel, width())) d.channel = null;
		draw();
	});

	// The band's tick boxes: each with how it reads the draft, to draw it
	// again on Undo.
	const tick = (k, label, about, isOn, set) => {
		const el = h('input', { type: 'checkbox', disabled: !editable(k) });
		el.checked = isOn();
		el.addEventListener('change', () => { set(el.checked); draw(); });
		const row = h('label', { class: 'tick', title: about }, el, ' ', label, whence(k) && h('span', { class: 'whence' }, whence(k)));
		return { row, reset: () => { el.checked = isOn(); } };
	};
	const ticks = [
		band === '5g' && tick('dfs', 'DFS channels', 'Channels 52–144, shared with radar: a radio there listens for radar for about a minute before it transmits, and moves off if it hears any (0071).',
			() => d.dfs !== 'avoid', (v) => { d.dfs = v ? 'allow' : 'avoid'; }),
		band === '6g' && tick('psc', 'Preferred scanning channels only', 'Clients look for 6 GHz networks on 5, 21, 37 and every 16th to 229 by themselves; elsewhere only where another band’s beacons send them (0087).',
			() => d.psc === true, (v) => { d.psc = v; }),
		band === '6g' && tick('non_overlapping', 'One channel a block', 'Each block of the width offers one channel, so radios that pick different channels never share a block (0087).',
			() => d.non_overlapping === true, (v) => { d.non_overlapping = v; }),
	].filter(Boolean);

	// The channel: automatic or manual, below the map.
	const modeSel = h('select', { 'aria-label': `${bandName(band)} channel`, disabled: !editable('channel') },
		saved.mode === 'unset' && h('option', { value: 'unset' }, unset),
		h('option', { value: 'auto' }, 'Automatic'),
		h('option', { value: 'manual' }, 'Manual'));
	modeSel.value = d.mode;
	modeSel.addEventListener('change', () => { d.mode = modeSel.value; draw(); });
	const modeNote = h('span', { class: 'sub' });

	// The power: automatic or a figure, below the map too.
	const powerSel = h('select', { 'aria-label': `${bandName(band)} power`, disabled: !editable('power') },
		saved.power === undefined && h('option', { value: '' }, unset),
		h('option', { value: 'auto' }, 'Automatic'),
		h('option', { value: 'manual' }, 'Manual'));
	const dbm = h('select', { 'aria-label': `${bandName(band)} power in dBm`, disabled: !editable('power') },
		step(1, 30, 1).map((x) => h('option', { value: String(x) }, `${x} dBm`)));
	powerSel.value = saved.power === undefined ? '' : saved.power === 'auto' ? 'auto' : 'manual';
	dbm.value = String(typeof saved.power === 'number' ? saved.power : 20);
	const setPower = () => {
		d.power = powerSel.value === '' ? undefined : powerSel.value === 'auto' ? 'auto' : Number(dbm.value);
		draw();
	};
	powerSel.addEventListener('change', setPower);
	dbm.addEventListener('change', setPower);
	const powerNow = reported(rows, band, 'power');

	// The 802.11 generations clients may use (0089): tick boxes, one
	// unbroken run, oldest to newest. Unset, they show what the APs serve,
	// any of them, but 802.11b, which OpenWrt leaves off. 802.11be shows
	// only where an AP serves it, or it is set.
	const gens = new Map((b.modes || []).map((m) => [m.mode, m]));
	const served = GENERATIONS[band].filter((m) => gens.get(m)?.any);
	const shownModes = () => d.modes ?? served.filter((m) => m !== 'b');
	const offered = GENERATIONS[band].filter((m) => m !== 'be' || gens.get('be')?.any || (saved.modes || []).includes('be'));
	const modeBoxes = h('span', { class: 'ticks inline' });
	const modesNote = h('div', { class: 'sub warn' });
	const drawModes = () => {
		const now = shownModes();
		modeBoxes.replaceChildren(...offered.map((m) => {
			const el = h('input', { type: 'checkbox', disabled: !editable('modes') });
			el.checked = now.includes(m);
			el.addEventListener('change', () => {
				const order = GENERATIONS[band];
				let next;
				if (el.checked) next = run(band, [...now, m]);
				else if (m === now[now.length - 1]) next = now.slice(0, -1);
				else next = now.filter((x) => order.indexOf(x) > order.indexOf(m));  // raises the oldest
				if (!next.length) { el.checked = true; return; }
				d.modes = next;
				// A width the newest can't carry narrows to its widest.
				const cap = FAMILY_WIDTH[next[next.length - 1]];
				if (d.width !== undefined && d.width > cap) d.width = cap;
				if (d.mode === 'manual' && d.channel != null && !block(band, d.channel, width())) d.channel = null;
				draw();
			});
			const g = gens.get(m);
			return h('label', { class: 'tick', title: g && !g.ok ? g.why : '' }, el, ' ', `802.11${m}`);
		}));
	};
	// The guard intervals, one for each kind (0089): 802.11n and 802.11ac
	// short or long; 802.11ax automatic or held to one.
	const htKind = band === '2g' ? '802.11n' : '802.11n/ac';
	const sgiSel = band !== '6g' && h('select', { 'aria-label': `${bandName(band)} ${htKind} guard interval`, disabled: !editable('short_gi') },
		saved.short_gi === undefined && h('option', { value: '' }, unset),
		h('option', { value: 'short' }, 'Short, 400 ns (default)'),
		h('option', { value: 'long' }, 'Long, 800 ns'));
	const setSGI = () => { sgiSel.value = d.short_gi === undefined ? '' : d.short_gi ? 'short' : 'long'; };
	if (sgiSel) {
		setSGI();
		sgiSel.addEventListener('change', () => { d.short_gi = sgiSel.value === '' ? undefined : sgiSel.value === 'short'; draw(); });
	}
	const heSel = h('select', { 'aria-label': `${bandName(band)} 802.11ax guard interval`, disabled: !editable('he_gi') },
		saved.he_gi === undefined && h('option', { value: '' }, unset),
		h('option', { value: 'auto' }, 'Automatic'),
		HE_GI.map((ns) => h('option', { value: String(ns) }, `${ns / 1000} µs${ns === 800 ? ' (default)' : ''}`)));
	const setHE = () => { heSel.value = d.he_gi === undefined ? '' : String(d.he_gi); };
	setHE();
	heSel.addEventListener('change', () => { d.he_gi = heSel.value === '' ? undefined : heSel.value === 'auto' ? 'auto' : Number(heSel.value); draw(); });
	const sgiPart = sgiSel && h('span', { class: 'gi' }, h('span', { class: 'sub' }, `${htKind} `), sgiSel);
	const hePart = h('span', { class: 'gi' }, h('span', { class: 'sub' }, '802.11ax '), heSel);

	const save = h('button', { type: 'button', class: 'button small primary' }, 'Save…');
	const undo = h('button', { type: 'button', class: 'button small' }, 'Undo');

	// changes are the fields the draft changes, as one set's values.
	const changes = () => {
		const out = {};
		const p = (k) => `radio.${band}.${k}`;
		if (d.enabled !== saved.enabled && d.enabled !== undefined) out[p('enabled')] = d.enabled;
		if (d.width !== saved.width && d.width !== undefined) out[p('width')] = d.width;
		if (d.mode === 'auto' && saved.mode !== 'auto') out[p('channel')] = 'auto';
		if (d.mode === 'manual' && d.channel != null && d.channel !== saved.channel) out[p('channel')] = d.channel;
		const same = (x, y) => x.size === y.size && [...x].every((c) => y.has(c));
		if (d.mode === 'auto' && !same(d.channels, saved.channels)) out[p('channels')] = [...d.channels].sort((x, y) => x - y);
		if (d.dfs !== saved.dfs && d.dfs !== undefined) out[p('dfs')] = d.dfs;
		if (d.psc !== saved.psc && d.psc !== undefined) out[p('psc')] = d.psc;
		if (d.non_overlapping !== saved.non_overlapping && d.non_overlapping !== undefined) out[p('non_overlapping')] = d.non_overlapping;
		if (d.power !== saved.power && d.power !== undefined) out[p('power')] = d.power;
		if (d.modes !== undefined && (d.modes || []).join() !== (saved.modes || []).join()) out[p('modes')] = d.modes;
		if (d.short_gi !== saved.short_gi && d.short_gi !== undefined) out[p('short_gi')] = d.short_gi;
		if (d.he_gi !== saved.he_gi && d.he_gi !== undefined) out[p('he_gi')] = d.he_gi;
		return out;
	};

	// allowed says whether a channel may be the one, manual: in a block of
	// the width, outside DFS where it is not allowed, and a preferred
	// scanning channel where only those are.
	const allowed = (c) => {
		const blk = block(band, c, width());
		return Boolean(blk) && !(noDFS() && blk.some((x) => radar(band, x))) && !(onlyPSC() && !psc(c));
	};

	const draw = () => {
		const manual = d.mode === 'manual';
		const goes = new Set(manual ? [] : usable(band, d.channels, width(), noDFS(), onlyPSC(), spread()));
		const marked = !manual && (onlyPSC() || spread());
		const picked = manual && d.channel != null ? new Set(block(band, d.channel, width()) || []) : new Set();
		map.replaceChildren(...ranges(band, country).map((list) => {
			const groups = [];
			for (const c of list) {
				const blk = block(band, c, width());
				const key = blk ? blk[0] : `x${c}`;
				const last = groups[groups.length - 1];
				if (last && last.key === key) last.chans.push(c);
				else groups.push({ key, chans: [c], whole: blk });
			}
			return h('div', { class: 'chrange' }, groups.map((g, i) => {
				const all = !manual && g.whole && g.whole.every((c) => d.channels.has(c));
				const some = !manual && g.whole && !all && g.whole.some((c) => d.channels.has(c));
				const off = !g.whole || (noDFS() && g.whole.some((c) => radar(band, c))) || (onlyPSC() && !g.whole.some(psc));
				return h('div', { class: `chblock ${width() > 20 && band !== '2g' && i % 2 ? 'b' : 'a'}` }, g.chans.map((c) => {
					const aps = on.get(c) || [];
					const can = manual ? allowed(c) : !off;
					const why = !g.whole ? `No ${width()} MHz block includes ${c}`
						: noDFS() && radar(band, c) ? 'DFS channels are not allowed here'
							: onlyPSC() && !psc(c) && manual ? 'Not a preferred scanning channel'
								: onlyPSC() && !g.whole.some(psc) ? 'No preferred scanning channel in this block'
									: radar(band, c) ? 'Shared with radar (DFS)' : '';
					const isOn = manual ? picked.has(c) : all;
					// A dot marks the AP's own channel on its page; a folder's map
					// has none (Griff, 2026-10-09).
					return h('button', {
						type: 'button',
						class: `ch${isOn ? ' on' : ''}${some ? ' part' : ''}${can ? '' : ' off'}${radar(band, c) ? ' dfs' : ''}${band === '6g' && psc(c) ? ' psc' : ''}${(marked && goes.has(c)) || (manual && d.channel === c) ? ' goes' : ''}`,
						disabled: d.mode === 'unset' || !editable(manual ? 'channel' : 'channels') || !can,
						title: [why, band === '6g' && psc(c) && 'Preferred scanning channel', manual && d.channel === c && 'The channel',
							marked && goes.has(c) && 'The APs go to this channel', aps.length && `Now: ${aps.join(', ')}`].filter(Boolean).join(' · '),
						onclick: () => {
							if (manual) d.channel = c;
							else {
								const next = new Set(d.channels);
								for (const x of g.whole) (all ? next.delete(x) : next.add(x));
								d.channels = next;
							}
							draw();
						},
					}, h('span', null, String(c)), isAP && h('span', { class: aps.length ? 'ap' : 'ap none' }));
				}));
			}));
		}));
		count.textContent = manual
			? (d.channel != null ? `Channel ${d.channel}` : 'Pick a channel')
			: d.mode === 'unset' ? 'Not set' : `${goes.size} Channel${goes.size === 1 ? '' : 's'} Available`;
		const bad = (manual && d.channel == null) || (d.mode === 'auto' && goes.size === 0);
		count.className = bad ? 'chip bad' : d.mode === 'unset' ? 'chip' : 'chip ok';
		count.title = d.mode === 'auto' && goes.size === 0
			? `No whole ${width()} MHz block${onlyPSC() ? ' with a preferred scanning channel' : ''} is picked: the APs would have nowhere to go.` : '';
		modeSel.value = d.mode;
		modeNote.textContent = manual ? 'click the one channel on the map'
			: d.mode === 'auto' ? 'click blocks on the map to pick the channels it may be'
				: 'the map is for an automatic or manual channel set here';
		dbm.hidden = powerSel.value !== 'manual';
		drawModes();
		const now = shownModes();
		const cap = FAMILY_WIDTH[now[now.length - 1]] ?? 320;
		for (const o of widthOpts) {
			const w = b.widths.find((x) => x.width === Number(o.value));
			o.disabled = !w?.ok || Number(o.value) > cap;
			o.textContent = `${o.value} MHz${!w?.ok ? ` (${w?.why})` : Number(o.value) > cap ? ` (needs newer than 802.11${now[now.length - 1]})` : ''}`;
		}
		widthSel.value = d.width === undefined ? '' : String(d.width);
		// The oldest is the least a client must support: every AP must serve it.
		const oldest = gens.get(now[0]);
		const cannot = d.modes !== undefined && oldest && !oldest.ok;
		modesNote.textContent = cannot ? `${oldest.why.replace(/it$/, `802.11${now[0]}`)}: as the oldest allowed, no client could join.` : '';
		modesNote.hidden = !cannot;
		if (sgiSel) sgiPart.hidden = !now.some((m) => m === 'n' || m === 'ac');
		hePart.hidden = !now.includes('ax') && !now.includes('be');
		setHE();
		if (sgiSel) setSGI();
		const dirty = Object.keys(changes()).length > 0;
		save.disabled = !dirty || bad || cannot;
		undo.disabled = !dirty;
		if (dirty) panel.setAttribute('data-editing', 'true');
		else panel.removeAttribute('data-editing');
	};

	undo.addEventListener('click', () => {
		d = { ...saved, channels: new Set(saved.channels) };
		radioOn.checked = d.enabled ?? (reported(rows, band, 'enabled') !== 'off');
		widthSel.value = saved.width === undefined ? '' : String(saved.width);
		powerSel.value = saved.power === undefined ? '' : saved.power === 'auto' ? 'auto' : 'manual';
		for (const t of ticks) t.reset();
		draw();
	});
	save.addEventListener('click', async () => {
		const values = changes();
		const paths = Object.keys(values);
		const op = paths.length === 1
			? { kind: 'set', tree: 'locations', node, path: paths[0], value: values[paths[0]] }
			: { kind: 'set', tree: 'locations', node, values };
		const p = await ask(box, op);
		if (!p) return;
			const show = (k, v) => (k === 'channels' ? v.join(', ') : k === 'width' ? `${v} MHz` : k === 'power' ? (v === 'auto' ? 'automatic' : `${v} dBm`)
			: k === 'channel' ? (v === 'auto' ? 'automatic' : String(v)) : k === 'modes' ? v.map((m) => `802.11${m}`).join(', ')
				: k === 'short_gi' ? (v ? 'short, 400 ns' : 'long, 800 ns') : k === 'he_gi' ? (v === 'auto' ? 'automatic' : `${v / 1000} µs`)
					: typeof v === 'boolean' ? (v ? 'on' : 'off') : String(v));
		const names = { enabled: 'Radio on', width: 'Width', channel: 'Channel', channels: 'Channels it may be', dfs: 'DFS channels', psc: 'Preferred scanning channels only', non_overlapping: 'One channel a block', power: 'Power', modes: 'Protocols', short_gi: `Guard interval, ${htKind}`, he_gi: 'Guard interval, 802.11ax' };
		const radarNow = paths.some((x) => x.endsWith('.dfs')) && values[`radio.${band}.dfs`] === 'allow';
		confirm(ctx, box, op, p, [
			h('div', null, h('strong', null, `${bandName(band)} on ${nodeName}`)),
			h('ul', { class: 'becomes' }, paths.map((path) => {
				const k = path.split('.').pop();
				return h('li', null, `${names[k] || k}: `, show(k, values[path]));
			})),
			b.widths.find((w) => w.width === values[`radio.${band}.width`])?.auto && values[`radio.${band}.channel`] === 'auto'
				&& h('div', { class: 'sub' }, 'The channel becomes automatic: some AP’s channel cannot carry the new width.'),
		], [
			radarNow && h('div', { class: 'sub warn' }, 'With DFS channels allowed, a radio may pick one: it then listens for radar for about a minute before it transmits.'),
				paths.some((x) => !x.endsWith('.he_gi'))
				? h('div', { class: 'sub warn' }, `Applying restarts the ${bandName(band)} radio on each AP listed: its clients drop for a few seconds and reconnect.`)
				: h('div', { class: 'sub' }, 'The 802.11ax guard interval changes at once, without restarting the radio.'),
		]);
	});

	const own = ['enabled', 'width', 'channel', 'channels', 'dfs', 'psc', 'non_overlapping', 'power', 'modes', 'short_gi', 'he_gi']
		.filter((k) => f(k)?.from === node && f(k)?.origin === 'self').map((k) => `radio.${band}.${k}`);
	const label = (text, k) => h('div', { class: 'label' }, text, whence(k) && h('span', { class: 'whence' }, whence(k)));

	draw();
	panel.append(...[
		h('h2', { class: 'bandhead' },
			h('label', { class: 'radio-on', title: 'Radio on' }, radioOn, h('span', { class: 'title' }, bandName(band))),
			count,
			h('span', { class: 'width' }, widthSel)),
		ticks.length > 0 && h('div', { class: 'ticks' }, ticks.map((t) => t.row)),
		map,
		h('div', { class: 'row' }, label('Channel', 'channel'), h('div', { class: 'value' }, modeSel, ' ', modeNote)),
		h('div', { class: 'row' }, label('Power', 'power'),
			h('div', { class: 'value' }, powerSel, ' ', dbm, powerNow && h('span', { class: 'sub' }, ` now ${powerNow}`))),
		h('div', { class: 'row' }, label('Protocols', 'modes'), h('div', { class: 'value' }, modeBoxes, modesNote)),
		h('div', { class: 'row' }, label('Guard interval', band === '6g' ? 'he_gi' : 'short_gi'),
			h('div', { class: 'value gis' }, sgiPart, hePart)),
		canEdit && h('div', { class: 'row actions' },
			own.length > 0 && followButton(ctx, 'locations', node, nodeName, parentName, own, box,
				`Follow ${parentName ?? 'above'} (${own.length} set here)`),
			undo, save),
		box,
	].filter(Boolean));
	return panel;
}
