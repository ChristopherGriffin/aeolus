// Networks heard (0106): every network the APs here hear but their radio
// neighbours, once each by BSSID, with the APs that hear it, the strongest
// first. A rogue broadcasts one of Aeolus's SSIDs from a BSSID no AP names
// as its own (0105): an evil twin, or a same-named AP Aeolus does not
// manage. Marked known, by rogues.known, it raises no alert. Only APs with
// Neighbours on listen.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { bandName, ago } from '../format.js';
import { ask, confirm } from './confirm.js';
import { mayEdit } from './clients.js';

// What each kind is, with its chip.
const KIND = {
	rogue: ['bad', 'rogue', 'broadcasts one of Aeolus\'s SSIDs from a BSSID no AP names as its own'],
	known: ['idle', 'known', 'marked known: it raises no alert'],
	aeolus: ['ok', 'Aeolus', 'one of the APs\' own, heard by an AP that is not its radio neighbour'],
	other: ['idle', 'other', 'a network of another name'],
};

// airspaceSection draws the networks heard below the node at ({ node,
// nodeName, page }).
export async function airspaceSection(ctx, at) {
	const res = await get(`/v1/airspace?under=${encodeURIComponent(at.node)}`);
	const out = h('div', { class: 'edit flush' });
	const nets = res.networks || [];
	const pressing = nets.filter((n) => n.kind !== 'other');
	const others = nets.filter((n) => n.kind === 'other');
	const c = res.counts || {};
	const head = ['Network', 'Band', 'Heard by', 'Last heard', ''].map((x) => h('th', null, x));
	return h('section', { class: 'panel' },
		h('h2', null, 'Networks heard', h('span', { class: 'note' }, 'as each AP last reported; only APs with Neighbours on listen')),
		nets.length > 0 && h('div', { class: 'sub' },
			['rogue', 'known', 'aeolus', 'other'].filter((k) => c[k]).map((k) => [h('span', { class: `chip ${KIND[k][0]}` }, `${c[k]} ${KIND[k][1]}`), ' '])),
		out,
		!nets.length
			? h('div', { class: 'sub' }, 'Nothing heard yet. Each AP with Neighbours on lists the networks it hears within a few minutes; turn it on under Radios › Neighbours.')
			: [
				pressing.length > 0 && h('table', { class: 'list' }, h('tr', null, head), pressing.map((n) => row(ctx, at, n, out))),
				others.length > 0 && h('details', null,
					h('summary', null, `${others.length} other network${others.length === 1 ? '' : 's'}`),
					h('table', { class: 'list' }, h('tr', null, head), others.map((n) => row(ctx, at, n, out)))),
			]);
}

function row(ctx, at, n, out) {
	const [chip, label, means] = KIND[n.kind] || ['idle', n.kind, ''];
	const top = n.heard_by.slice(0, 3);
	return h('tr', null,
		h('td', null, h('div', null, n.ssid), h('div', { class: 'sub mono' }, n.bssid)),
		h('td', null, `${bandName(n.band)} · ${n.channel}`),
		h('td', null, top.map((x) => h('div', null, link(`/aps/${encodeURIComponent(x.ap)}/interfaces/radios/airspace`, x.name), h('span', { class: 'sub mono' }, ` ${Math.round(x.signal)} dBm`))),
			n.heard_by.length > 3 && h('div', { class: 'sub' }, `and ${n.heard_by.length - 3} more`)),
		h('td', null, ago(n.heard_by[0].seen)),
		h('td', null, h('span', { class: `chip ${chip}`, title: means }, label),
			n.kind === 'aeolus' && n.ap && h('div', { class: 'sub' }, link(`/aps/${encodeURIComponent(n.ap)}`, ctx.name('locations', n.ap))),
			(n.kind === 'rogue' || n.kind === 'known') && knownButton(ctx, at, n, out)));
}

// knownButton marks a rogue known, or forgets a known one (0106): the BSSID
// joins or leaves rogues.known where that is set, else here, in one change
// previewed first. It re-versions no AP.
function knownButton(ctx, at, n, out) {
	const f = at.page.fields?.['rogues.known'];
	const node = f?.from ?? at.node;
	if (!mayEdit(ctx, 'locations', node)) return null;
	const now = (f?.value ?? []).map((b) => b.toLowerCase());
	const known = now.includes(n.bssid);
	// The button follows what the network is: Mark known for a rogue this
	// list leaves out, Forget for a known one it names. A rogue it names is a
	// rogue to an AP below whose own list leaves it out, and a known one it
	// leaves out is known by another list: changing this one helps neither.
	if ((n.kind === 'rogue') === known) return null;
	const where = ctx.name('locations', node);
	const there = ctx.trees.locations?.nodes.get(node)?.kind === 'ap' ? 'it' : 'the APs there';
	const go = async () => {
		const value = known ? now.filter((b) => b !== n.bssid) : [...now, n.bssid];
		const op = { kind: 'set', tree: 'locations', node, path: 'rogues.known', value };
		out.scrollIntoView({ block: 'nearest' });
		const p = await ask(out, op);
		if (!p) return;
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, `${known ? 'Forget' : 'Mark known'}: ${n.ssid} from ${n.bssid}`)),
			h('div', { class: 'sub' }, known
				? `Set on ${where}. Heard again by ${there}, it is a rogue, and raises an alert.`
				: `Set on ${where}. Heard by ${there}, it is known and raises no alert: a neighbour's AP of the same name, say. Mark known only a network you know is not an evil twin.`),
		], []);
	};
	return h('div', null, h('button', { type: 'button', class: 'button small', onclick: go }, known ? 'Forget…' : 'Mark known…'));
}
