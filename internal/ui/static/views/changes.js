// The change log, newest first (0009): who changed what, when and why.

import { h } from '../dom.js';
import { get, health } from '../api.js';
import { when } from '../format.js';

const PAGE = 50;

export async function changesPage(ctx) {
	const { seq } = await health();
	const list = h('div');
	const more = h('div', { class: 'more' });
	let next = seq; // the newest change not yet shown

	const load = async () => {
		const after = Math.max(0, next - PAGE);
		const { changes } = await get(`/v1/changes?after=${after}&limit=${PAGE}`);
		for (const c of changes.filter((x) => x.seq <= next).reverse()) list.append(entry(ctx, c));
		next = after;
		more.replaceChildren(next > 0 ? h('button', { type: 'button', onclick: load }, 'Older changes') : 'That is the first change.');
	};
	await load();
	return {
		main: [
			h('div', { class: 'head' },
				h('div', null,
					h('h1', null, 'Changes'),
					h('div', { class: 'sub' }, `${seq} changes. Each one is logged with who made it and when, and none can be edited or removed.`))),
			h('section', { class: 'panel' }, list, more),
		],
	};
}

function entry(ctx, c) {
	return h('div', { class: 'change' },
		h('span', { class: 'seq' }, '#' + c.seq),
		h('span', null, when(c.at)),
		h('strong', null, c.actor),
		h('span', { class: 'what' }, summary(ctx, c.op)),
		c.reason && h('span', { class: 'why' }, c.reason));
}

// summary says what a change did, in a line.
function summary(ctx, op) {
	const tree = op.tree || 'locations';
	const node = (id, t = tree) => ctx.name(t, id);
	const val = (v) => (v && typeof v === 'object' && v.sealed ? '(sealed)' : JSON.stringify(v));
	switch (op.kind) {
	case 'set': return `${op.tree} › ${node(op.node)}: ` + (op.values
		? Object.keys(op.values).sort().map((p) => `${p} = ${val(op.values[p])}`).join(', ')
		: `${op.path} = ${val(op.value)}`);
	case 'unset': return `${op.tree} › ${node(op.node)}: unset ${op.paths ? op.paths.join(', ') : op.path}`;
	case 'lock': return `${op.tree} › ${node(op.node)}: lock ${op.path}`;
	case 'unlock': return `${op.tree} › ${node(op.node)}: unlock ${op.path}`;
	case 'add-folder': return `${op.tree}: add folder ${op.name} under ${node(op.parent)}`;
	case 'add-ap': return `add AP ${op.name} under ${node(op.parent)}`;
	case 'move': return `${op.tree}: move ${node(op.node)} to ${node(op.parent)}`;
	case 'break-hierarchy': return `${op.tree} › ${node(op.node)}: break hierarchy`;
	case 'assign-services': return `${node(op.node, 'locations')} offers ${(op.services || []).map((s) => node(s, 'services')).join(' + ') || 'nothing'}`;
	case 'enroll': return `AP ${op.name} enrolled (${op.value?.mac || op.node})`;
	case 'remove-ap': return `remove AP ${op.node}`;
	case 'set-concentrator': return `library: set concentrator ${op.concentrator}`;
	case 'remove-concentrator': return `library: remove concentrator ${op.concentrator}`;
	case 'set-vni': return `library: VNI ${op.vni} on ${op.concentrator} = ${op.name}`;
	case 'remove-vni': return `library: remove VNI ${op.vni} from ${op.concentrator}`;
	case 'grant': return `grant ${op.account} ${op.role} on ${op.tree} › ${node(op.node)}`;
	case 'revoke': return `revoke ${op.account} ${op.role} on ${op.tree} › ${node(op.node)}`;
	case 'add-account': return `add account ${op.account}`;
	case 'issue-token': return `issue a token for ${op.account}`;
	case 'revoke-token': return `revoke token ${op.token_id}`;
	case 'create-org': return `create the Org ${op.name}, admin ${op.account}`;
	case 'add-builtins': return 'add the built-in folders';
	}
	return op.kind;
}
