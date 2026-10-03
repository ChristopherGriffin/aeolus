// Inputs made from the schema's description of each field (0048), so a field
// added to the schema can be edited without new UI code. The server checks
// every value again before anything is recorded.

import { h } from '../dom.js';
import { bandName, security } from '../format.js';

// describe finds a field's description: network.lab.ssid is described as
// network.*.ssid.
export function describe(d, path) {
	if (d.fields[path]) return d.fields[path];
	const p = path.split('.');
	for (const [k, f] of Object.entries(d.fields)) {
		const q = k.split('.');
		if (q.length === p.length && q.every((s, i) => s === '*' || s === p[i])) return f;
	}
	return null;
}

// input makes an input for the field at path, described by f, showing
// current. changed() says whether the person changed it; read() returns the
// value to set, or throws with what is wrong. A secret is never shown: left
// empty, it stays as it is.
export function input(path, f, current) {
	const last = path.split('.').pop();
	current ??= f.default; // what an unset field means, where the schema says
	let el;
	let read;
	if (f.type === 'boolean') {
		el = h('input', { type: 'checkbox', checked: current === true });
		read = () => el.checked;
	} else if (f.enum) {
		el = h('select', null,
			current === undefined && h('option', { value: '' }, '—'),
			f.enum.map((v) => h('option', { value: String(v), selected: v === current }, last === 'security' ? security(v) : String(v))));
		read = () => (el.value === '' ? undefined : f.enum.find((v) => String(v) === el.value));
	} else if (f.type === 'array' && f.items?.enum) {
		const all = f.items.enum;
		const on = current ?? all; // a list nobody set means all of them
		const boxes = all.map((v) => h('input', { type: 'checkbox', value: v, checked: on.includes(v) }));
		el = h('span', { class: 'choices' }, all.map((v, i) => h('label', null, boxes[i], last === 'bands' ? bandName(v) : v)));
		read = () => {
			const list = all.filter((v, i) => boxes[i].checked);
			if (f.minItems && list.length < f.minItems) throw new Error(`choose at least ${f.minItems}`);
			return list;
		};
	} else if (f.type === 'integer') {
		el = h('input', { type: 'number', step: 1, min: f.minimum, max: f.maximum, value: current ?? '' });
		read = () => {
			if (el.value === '') return undefined;
			const n = Number(el.value);
			if (!Number.isInteger(n)) throw new Error('a whole number');
			return n;
		};
	} else if (f.writeOnly) {
		el = h('input', { type: 'password', autocomplete: 'new-password', maxlength: f.maxLength, placeholder: current !== undefined ? 'unchanged' : '' });
		read = () => (el.value === '' ? undefined : el.value);
	} else {
		el = h('input', { type: 'text', maxlength: f.maxLength, value: current ?? '' });
		// A field that is a word or a number, such as a channel: numbers as numbers.
		read = () => (el.value === '' ? undefined : f.type !== 'string' && /^\d+$/.test(el.value) ? Number(el.value) : el.value);
	}
	const initial = JSON.stringify(f.writeOnly ? undefined : read());
	return {
		el,
		read,
		changed: () => {
			try {
				return JSON.stringify(read()) !== initial;
			} catch {
				return true; // invalid now, so it was changed
			}
		},
	};
}
