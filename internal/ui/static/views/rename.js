// Renaming a folder or an AP (0076). An AP's name is its hostname, so it must
// be one, and the AP's hostname changes with it; a folder's name is any 1 to
// 64 characters, and renaming it changes no AP's config. Previewed and
// logged as any change is.

import { h } from '../dom.js';
import { ask, confirm, cancelButton } from './confirm.js';

// HOSTNAME is what an AP's name must be, as its hostname.
const HOSTNAME = '[A-Za-z0-9]([A-Za-z0-9\\-]{0,61}[A-Za-z0-9])?';

// renameButton offers to rename node ({ id, name, kind }) in tree, its form
// opening in box.
export function renameButton(ctx, tree, node, box, cls = 'rename') {
	const ap = node.kind === 'ap';
	return h('button', {
		type: 'button', class: `button small ${cls}`,
		title: ap ? 'Rename this AP: its hostname changes with it' : 'Rename this folder',
		onclick: () => renameForm(ctx, tree, node, box),
	}, 'Rename…');
}

function renameForm(ctx, tree, node, box) {
	const ap = node.kind === 'ap';
	const input = h('input', ap
		? { type: 'text', value: node.name, maxlength: 63, pattern: HOSTNAME, spellcheck: 'false', 'aria-label': `New name for ${node.name}` }
		: { type: 'text', value: node.name, maxlength: 64, 'aria-label': `New name for ${node.name}` });
	const out = h('div');
	const msg = h('div', { class: 'error' });
	const review = async () => {
		msg.replaceChildren();
		const name = input.value.trim();
		if (!name || !input.checkValidity() || (!ap && [...name].length > 64)) {
			msg.replaceChildren(ap
				? 'An AP\'s name is its hostname: letters, digits and hyphens, at most 63, starting and ending with a letter or digit.'
				: 'A folder\'s name is 1 to 64 characters.');
			return;
		}
		if (name === node.name) {
			msg.replaceChildren('That is its name already.');
			return;
		}
		const op = { kind: 'rename', tree, node: node.id, name };
		const p = await ask(out, op);
		if (!p) return;
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, 'Rename: '), node.name, ' → ', name),
		], [
			h('div', { class: 'sub' }, 'The AP\'s hostname changes with it, as it applies the config. Nothing restarts.'),
		]);
	};
	input.addEventListener('keydown', (e) => { if (e.key === 'Enter') review(); });
	box.replaceChildren(h('div', { class: 'fieldform', 'data-editing': true },
		h('label', { class: 'field' }, h('span', { class: 'label' }, `New name for ${node.name}`), input),
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review'),
			cancelButton(box)),
		out));
	input.focus();
	input.select();
}
