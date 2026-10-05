// Making a folder inside another (0076): by its name, in either tree. Its ID,
// which goes into addresses, is made from the name. A new folder changes no
// AP's config until something is set on it or moved into it.

import { h } from '../dom.js';
import { ask, confirm, cancelButton } from './confirm.js';

// newFolderButton offers to make a folder inside parent ({ id, name }) in
// tree, its form opening in box.
export function newFolderButton(ctx, tree, parent, box, cls = 'rename head') {
	return h('button', {
		type: 'button', class: `button small ${cls}`, title: `Make a folder inside ${parent.name}`,
		onclick: () => newFolderForm(ctx, tree, parent, box),
	}, 'New folder…');
}

// idFor makes a folder's ID from its name: lowercase letters, digits and
// hyphens, at most 32, and not one the tree has already.
function idFor(ctx, tree, name) {
	const base = name.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 28).replace(/-+$/, '') || 'folder';
	const taken = ctx.trees[tree].nodes;
	let id = base;
	for (let n = 2; taken.has(id); n++) id = `${base}-${n}`;
	return id;
}

function newFolderForm(ctx, tree, parent, box) {
	const input = h('input', { type: 'text', maxlength: 64, placeholder: 'Name', 'aria-label': `Name of the new folder in ${parent.name}` });
	const out = h('div');
	const msg = h('div', { class: 'error' });
	const review = async () => {
		msg.replaceChildren();
		const name = input.value.trim();
		if (!name || [...name].length > 64) {
			msg.replaceChildren('A folder\'s name is 1 to 64 characters.');
			return;
		}
		const op = { kind: 'add-folder', tree, node: idFor(ctx, tree, name), name, parent: parent.id };
		const p = await ask(out, op);
		if (!p) return;
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, 'New folder: '), name, ` in ${parent.name}`),
			h('div', { class: 'sub' }, 'It inherits everything from where it is. Set things on it, or move APs and folders into it, from its page.'),
		], []);
	};
	input.addEventListener('keydown', (e) => { if (e.key === 'Enter') review(); });
	box.replaceChildren(h('div', { class: 'fieldform', 'data-editing': true },
		h('label', { class: 'field' }, h('span', { class: 'label' }, `New folder in ${parent.name}`), input),
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review'),
			cancelButton(box)),
		out));
	input.focus();
}
