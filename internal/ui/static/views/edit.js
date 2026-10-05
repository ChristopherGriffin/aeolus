// Inputs made from the schema's description of each field (0048), so a field
// added to the schema can be edited without new UI code. The server checks
// every value again before anything is recorded.

import { h } from '../dom.js';
import { bandName, security, group, value } from '../format.js';

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
	} else if (f['x-aeolus-enum'] === 'zones' && f.enum) {
		el = h('select');
		const zones = zoneSelect(el, f, current);
		read = () => (el.value === '' || el.value === ALL ? undefined : el.value);
		const it = { el, read, narrow: zones.narrow };
		const initial = JSON.stringify(read());
		it.changed = () => JSON.stringify(read()) !== initial;
		return it;
	} else if (f.enum) {
		el = h('select', null,
			current === undefined && h('option', { value: '' }, '—'),
			f.enum.map((v) => h('option', { value: String(v), selected: v === current }, last === 'security' ? security(v) : last === 'mode' ? value(path, v) : String(v))));
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
	} else if (f.type === 'array' && f.items?.type === 'integer') {
		// A list of numbers, such as tagged VLANs: "10, 20".
		const { minimum: lo, maximum: hi } = f.items;
		el = h('input', { type: 'text', value: (current ?? []).join(', '), placeholder: '10, 20' });
		read = () => el.value.split(/[\s,]+/).filter(Boolean).map((x) => {
			const n = Number(x);
			if (!Number.isInteger(n) || (lo != null && n < lo) || (hi != null && n > hi)) throw new Error(`${x} is not a whole number from ${lo} to ${hi}`);
			return n;
		});
	} else if (f.type === 'array') {
		// A list of words, such as NTP servers or SSH keys: one a line.
		el = h('textarea', { rows: Math.max(2, (current ?? []).length + 1) }, (current ?? []).join('\n'));
		read = () => {
			const list = el.value.split('\n').map((x) => x.trim()).filter(Boolean);
			return list.length ? list : undefined;
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

// ALL is the zone dropdown's last choice when it offers one country's
// zones: it offers them all instead.
const ALL = '*';

// zoneSelect fills select with time zones (0074), showing current. All of
// them, by region as Linux lists them, each by its city; or, narrowed to a
// country (narrow('US')), UTC and that country's, in tzselect's order and
// with its notes ("Central (most areas) · Chicago"), and a last choice that
// shows them all. A zone set outside the list, or the country, shows as it
// is, so nothing changes unless the person changes it.
function zoneSelect(select, f, current) {
	const places = f['x-aeolus-zones'] || [];
	const city = (z) => z.slice(z.indexOf('/') + 1).replaceAll('_', ' ');
	const option = (z, label) => h('option', { value: z }, label);
	let country = null;
	const fill = () => {
		const keep = select.value && select.value !== ALL ? select.value : current;
		const mine = country ? places.filter((p) => p.country === country) : [];
		let body;
		if (mine.length) {
			body = [option('UTC', 'UTC'), mine.map((p) => option(p.zone, p.note ? `${p.note} · ${city(p.zone)}` : city(p.zone))),
				option(ALL, 'Show all zones…')];
		} else {
			const regions = new Map();
			for (const z of f.enum) {
				const i = z.indexOf('/');
				const region = i < 0 ? '' : z.slice(0, i);
				if (!regions.has(region)) regions.set(region, []);
				regions.get(region).push(z);
			}
			body = [...regions].map(([region, list]) => (region
				? h('optgroup', { label: region }, list.map((z) => option(z, city(z))))
				: list.map((z) => option(z, z))));
		}
		const shown = new Set([...(mine.length ? ['UTC', ...mine.map((p) => p.zone)] : f.enum)]);
		select.replaceChildren(...[
			keep === undefined && option('', '—'),
			typeof keep === 'string' && !shown.has(keep) &&
				option(keep, f.enum.includes(keep) ? `${keep} (outside ${country})` : `${keep} (not in the list)`),
			body,
		].flat(Infinity).filter(Boolean));
		select.value = keep ?? '';
	};
	let last = current;
	select.addEventListener('change', () => {
		if (select.value !== ALL) {
			last = select.value || undefined;
			return;
		}
		country = null;
		select.value = '';
		current = last;
		fill();
	});
	fill();
	return {
		narrow(c) {
			const next = /^[A-Z]{2}$/.test(c || '') ? c : null;
			if (next === country) return;
			country = next;
			fill();
		},
	};
}

// fieldsForm lays out an input for each field in sections ([[title,
// [path]]]), with the values in force (fields, by path: {value, from,
// origin}). A field locked above the node at cannot be changed there. The
// section titled more folds away. It returns the body, the inputs that can
// be changed, and every field's input and row, by path.
export function fieldsForm(d, sections, fields, at, more) {
	const inputs = new Map();
	const rows = new Map();
	const rowFor = (path) => {
		const f = describe(d, path);
		if (!f) return null;
		const cur = fields[path];
		const it = input(path, f, cur?.value);
		const locked = cur?.origin === 'locked' && cur.from !== at;
		if (locked) it.el.disabled = true;
		else inputs.set(path, it);
		const row = h('label', { class: 'field' },
			h('span', { class: 'label' }, group(path).label),
			it.el,
			locked && h('span', { class: 'sub' }, 'locked above'));
		rows.set(path, { it, row });
		return row;
	};
	const body = sections.map(([title, paths]) => {
		const list = paths.map(rowFor).filter(Boolean);
		if (!list.length) return null;
		if (title === more) return h('details', null, h('summary', null, title), h('div', { class: 'fields' }, list));
		return [title && h('h3', null, title), h('div', { class: 'fields' }, list)];
	});
	return { body, inputs, rows };
}

// changedValues reads what the person changed into {path: value}, leaving
// out fields that are not on show, or throws with the first that is wrong.
export function changedValues(inputs, rows) {
	const out = {};
	for (const [path, it] of inputs) {
		if (rows.get(path)?.row.hidden || !it.changed()) continue;
		let v;
		try {
			v = it.read();
		} catch (e) {
			throw new Error(`${group(path).label}: ${e.message}`);
		}
		if (v !== undefined) out[path] = v;
	}
	return out;
}
