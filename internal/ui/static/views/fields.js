// A node's values, grouped as in the mockup, each with where it comes from.

import { h } from '../dom.js';
import { group, value, origin } from '../format.js';
import { followButton } from './follow.js';

// The order fields appear in within a panel; others follow by path.
const RANK = [
	'ssid', 'security', 'passphrase', 'bands', 'enabled', 'hidden', 'isolation',
	'roaming.ft', 'roaming.rrm', 'roaming.btm',
	'transport.primary.type', 'transport.primary.vlan', 'transport.primary.concentrator', 'transport.primary.vni',
	'transport.fallback.type', 'transport.fallback.vlan', 'transport.fallback.concentrator', 'transport.fallback.vni',
	'transport.ha', 'transport.failback', 'transport.holddown', 'rate_limit.down_kbps', 'rate_limit.up_kbps',
	'channel', 'width', 'power', 'country', 'tz', 'ntp', 'poll', 'syslog', 'ssh_keys',
	'vlan', 'addressing', 'address', 'gateway', 'dns', 'uplink', 'mode', 'untagged', 'tagged', 'bond',
];

// rank places a field by what follows its panel's own part of the path:
// "ssid" in network.sweet.ssid, "width" in radio.5g.width.
function rank(path) {
	const p = path.split('.');
	const own = p[0] === 'system' ? (p[1] === 'management' ? 2 : 1) : 2;
	const i = RANK.indexOf(p.slice(own).join('.'));
	return i < 0 ? RANK.length : i;
}

// fieldPanels shows fields ({path: {value, from, origin}}) as panels in two
// columns. With edit ({nodeName, parentName}), for someone who may change
// the node, each value set here can follow the folder above again (0046).
export function fieldPanels(ctx, tree, here, fields, edit) {
	const groups = new Map();
	for (const [path, r] of Object.entries(fields || {})) {
		const g = group(path);
		if (!groups.has(g.key)) groups.set(g.key, { ...g, rows: [] });
		groups.get(g.key).rows.push({ path, r, label: g.label });
	}
	const nodeName = (id) => ctx.name(tree, id);
	const serviceName = (id) => ctx.name('services', id);
	const panels = [...groups.values()]
		.sort((a, b) => a.order - b.order || a.key.localeCompare(b.key))
		.map((g) => {
			const ssid = g.network ? fields[`network.${g.network}.ssid`]?.value : null;
			return h('section', { class: 'panel' },
				h('h2', null, ssid || g.title, g.network && h('span', { class: 'note' }, 'network ' + g.network)),
				g.rows
					.sort((a, b) => rank(a.path) - rank(b.path) || a.path.localeCompare(b.path))
					.map(({ path, r, label }) => {
						const box = h('div', { class: 'edit' });
						return [h('div', { class: 'row' },
							h('div', { class: 'label' }, label),
							h('div', { class: 'value' }, value(path, r.value, serviceName)),
							origin(tree, here, r, nodeName),
							edit && r.origin === 'self' && followButton(ctx, tree, here, edit.nodeName, edit.parentName, [path], box)),
						box];
					}));
		});
	if (!panels.length) return h('div', { class: 'banner info' }, 'Nothing is set here or in the folders above.');
	const cols = [[], []];
	panels.forEach((p, i) => cols[i % 2].push(p));
	return h('div', { class: 'grid2' }, cols.map((c) => h('div', { class: 'col' }, c)));
}

// editing is what field panels need to let someone who may change a node
// have it follow the folder above again (0046), or null for a viewer.
export function editing(ctx, tree, page) {
	if (page.role !== 'operator' && page.role !== 'admin') return null;
	const n = page.node;
	return { nodeName: n.name, parentName: n.parent ? ctx.name(tree, n.parent) : null };
}
