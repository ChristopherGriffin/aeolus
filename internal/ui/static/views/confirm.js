// A change on its way to the log (0042): previewed first, showing what it
// does, the APs that get a new config and any rule it would break; then a
// reason and Apply. Nothing is recorded until Apply, and the change is
// logged under the person's name.

import { h } from '../dom.js';
import { post } from '../api.js';
import { startEditing, stopEditing, hurry, flash, redrawNow } from '../refresh.js';

// ask previews op, showing progress and any error in box. It returns the
// preview, or null if there is none.
export async function ask(box, op) {
	startEditing();
	box.replaceChildren(h('div', { class: 'sub' }, 'Checking…'));
	try {
		return await post('/v1/preview', { op });
	} catch (e) {
		box.replaceChildren(h('div', { class: 'error' }, e.message), cancelButton(box));
		return null;
	}
}

// confirm fills box with the preview p of op: lines say what it does, and
// notes, shown when APs would apply it, say what applying means for them.
export function confirm(ctx, box, op, p, lines, notes) {
	const name = (id) => ctx.name('locations', id);
	const problems = Object.entries(p.checks || {});
	const affected = p.reversioned || [];
	const reason = h('input', { type: 'text', class: 'reason', placeholder: 'Why? (logged with your name)', maxlength: 500 });
	const apply = h('button', { type: 'button', class: 'button primary', disabled: true }, 'Apply');
	reason.addEventListener('input', () => { apply.disabled = problems.length > 0 || !reason.value.trim(); });
	apply.addEventListener('click', async () => {
		apply.disabled = true;
		try {
			const res = await post('/v1/changes', { op, reason: reason.value.trim() });
			stopEditing();
			hurry(150);
			const who = affected.length === 1 ? name(affected[0]) : `The ${affected.length} APs`;
			flash(`Saved as change #${res.change.seq}. ${affected.length ? `${who} pick${affected.length === 1 ? 's' : ''} it up on the next poll, within about a minute.` : 'No AP\'s config changed.'}`);
			redrawNow();
		} catch (e) {
			apply.disabled = false;
			box.querySelector('.error')?.remove();
			box.append(h('div', { class: 'error' }, e.message));
		}
	});
	box.replaceChildren(h('div', { class: 'preview' },
		lines,
		h('div', { class: 'sub' }, affected.length
			? `New config version for ${affected.length <= 5 ? affected.map(name).join(', ') : `${affected.length} APs`}.`
			: 'No AP\'s config changes: the APs below set their own, or there are none.'),
		problems.length > 0 && h('div', { class: 'banner problems' },
			h('strong', null, 'Aeolus would hold these configs, so the APs would not apply them:'),
			h('ul', null, problems.flatMap(([id, list]) => list.map((x) => h('li', null, `${name(id)}: ${x}`))))),
		problems.length === 0 && affected.length > 0 && notes,
		h('div', { class: 'actions' }, reason, apply, cancelButton(box))));
	reason.focus();
}

export function cancelButton(box) {
	return h('button', { type: 'button', class: 'button', onclick: () => { box.replaceChildren(); stopEditing(); } }, 'Cancel');
}
