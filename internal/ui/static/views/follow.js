// Following the folder again (0046): values a node sets for itself are unset
// in one change, so it inherits from above once more. The preview says what
// each field becomes and where that comes from.

import { h } from '../dom.js';
import { group, value, probeOnly, PROBE_ONLY } from '../format.js';
import { ask, confirm } from './confirm.js';

// followButton offers to unset paths at node, previewing in box. parentName
// is the folder above, or null at the root, where nothing is above. heading,
// when given, names the change in the preview, such as a delete.
export function followButton(ctx, tree, node, nodeName, parentName, paths, box, label, heading) {
	label ??= parentName ? `Follow ${parentName}` : 'Stop setting it here';
	return h('button', { type: 'button', class: 'button small', onclick: () => follow(ctx, tree, node, nodeName, parentName, paths, box, heading) }, label);
}

async function follow(ctx, tree, node, nodeName, parentName, paths, box, heading) {
	const op = paths.length === 1
		? { kind: 'unset', tree, node, path: paths[0] }
		: { kind: 'unset', tree, node, paths };
	const p = await ask(box, op);
	if (!p) return;
	const names = (id) => ctx.name('services', id);
	const before = paths.length === 1 ? { [paths[0]]: p.effect?.before } : (p.effect?.before || {});
	const radios = paths.some((x) => x.startsWith('radio.'));
	// What the prober asks, and how often, reloads nothing (0059).
	const probes = probeOnly(paths);
	const vnis = !probes && paths.some((x) => /^ports\.[^.]+\.vxlan\./.test(x));
	const ports = !probes && !vnis && paths.some((x) => x.startsWith('ports.'));
	const tunnels = !probes && paths.some((x) => x.startsWith('concentrators.'));
	confirm(ctx, box, op, p, [
		h('div', null, h('strong', null, heading ?? (parentName ? `${nodeName} follows ${parentName} again` : `${nodeName} stops setting ${paths.length === 1 ? 'this' : 'these'}`))),
		h('ul', { class: 'becomes' }, paths.map((path) => {
			const g = group(path);
			const after = p.resolved?.[path];
			return h('li', null,
				h('span', null, `${g.title} · ${g.label}: `),
				value(path, before[path], names), ' → ',
				after ? [value(path, after.value, names), ` (from ${ctx.name(tree, after.from)})`] : h('span', { class: 'sealed' }, 'not set by Aeolus any more'));
		})),
	], [
		radios && h('div', { class: 'sub warn' }, 'Applying restarts each radio whose settings change; its clients drop briefly and reconnect.'),
		probes && h('div', { class: 'sub warn' }, PROBE_ONLY),
		ports && h('div', { class: 'sub warn' }, "Applying reloads each AP's network; wired clients on the port drop briefly. Each AP leaves a port Aeolus no longer sets as it is."),
		// A tunnel port carries only the VNIs in force, so one removed comes off (0058).
		vnis && h('div', { class: 'sub warn' }, "Applying reloads each AP's network; wired clients on the port drop briefly. Each AP takes the port off a VNI it no longer carries; the port stays a tunnel port."),
		tunnels && h('div', { class: 'sub warn' }, "A network whose transport names a tunnel not set at an AP leaves that transport out there; where it was the network's only one, the preview shows the config Aeolus would hold."),
	]);
}
