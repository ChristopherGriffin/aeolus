// AP templates (0085): a template's own page, where its settings are edited,
// and a Locations folder's Templates tab, where the folder picks the
// template each kind of AP below it takes, and makes templates of its own.
// A template downloads as a file, and a file imports as a template (0090):
// the repo's templates/ holds ready-made ones.

import { h, link } from '../dom.js';
import { get, schema } from '../api.js';
import { group, value, origin, templateSays } from '../format.js';
import { describe, fieldsForm, changedValues } from './edit.js';
import { ask, confirm, cancelButton } from './confirm.js';

// What a template may set (0085), as the editor offers it. Ports are named
// by each AP's own port names, so they are set as any folder's are.
const SECTIONS = [
	['2.4 GHz radio', (p) => p.startsWith('radio.2g.')],
	['5 GHz radio', (p) => p.startsWith('radio.5g.')],
	['6 GHz radio', (p) => p.startsWith('radio.6g.')],
	['System', (p) => p.startsWith('system.') && p !== 'system.agent' && !p.startsWith('system.management.')],
	['Radio resource management', (p) => p.startsWith('rrm.')],
	['Power control', (p) => p.startsWith('apc.')],
];
const MORE = 'Management';
const MANAGEMENT = (p) => p.startsWith('system.management.');

const slug = (name) => name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 64) || 'template';
// Boards are written as OpenWrt names them, with a comma inside
// (arista,c360), so a list of them is split at spaces and semicolons, and
// at a comma only where a space follows it.
const boardsOf = (text) => text.split(/[\s;]+|,\s+/).map((b) => b.trim().replace(/,$/, '')).filter(Boolean);

// A template file (0090): its kind and version, its name, the boards it is
// for, an optional line about it, and its settings by field path. Where it
// lives is picked when it is imported.
const FILE_KIND = 'aeolus_template';
const FILE_VERSION = 1;
const sealed = (v) => v && typeof v === 'object' && !Array.isArray(v) && v.sealed === true;

// templateFile is a template as a file: its settings but the sealed ones,
// which can't be read back (0027).
function templateFile(t) {
	const values = {};
	const left = [];
	for (const p of Object.keys(t.values).sort()) {
		if (sealed(t.values[p])) left.push(p);
		else values[p] = t.values[p];
	}
	const file = { [FILE_KIND]: FILE_VERSION, name: t.name, boards: t.boards, values };
	return { text: JSON.stringify(file, null, '\t') + '\n', left };
}

// download saves a template's file.
function download(t, note) {
	const { text, left } = templateFile(t);
	const a = h('a', { href: URL.createObjectURL(new Blob([text], { type: 'application/json' })), download: `${t.id}.json` });
	document.body.append(a);
	a.click();
	a.remove();
	note.textContent = left.length ? `Saved ${t.id}.json without ${left.join(', ')}: secrets can't be read back.` : '';
}

// readTemplateFile checks a template file's text and returns what it holds,
// or throws why not.
function readTemplateFile(text) {
	let f;
	try {
		f = JSON.parse(text);
	} catch {
		throw new Error('This is not a template file: it is not JSON.');
	}
	if (!f || f[FILE_KIND] !== FILE_VERSION) throw new Error(`This is not an Aeolus template file: it has no "${FILE_KIND}": ${FILE_VERSION}.`);
	if (typeof f.name !== 'string' || !f.name.trim()) throw new Error('The template file has no name.');
	if (!Array.isArray(f.boards) || !f.boards.length || f.boards.some((b) => typeof b !== 'string')) throw new Error('The template file names no boards.');
	if (!f.values || typeof f.values !== 'object' || Array.isArray(f.values)) throw new Error('The template file has no settings.');
	return { name: f.name.trim(), boards: f.boards, about: typeof f.about === 'string' ? f.about : '', values: f.values };
}

// freeID is an ID made from a name that no template in the library has.
function freeID(name, taken) {
	const base = slug(name);
	let id = base;
	for (let n = 2; taken.has(id); n++) id = `${base.slice(0, 60)}-${n}`;
	return id;
}

// templatePage is one template: what it is for, where it is made and
// picked, its settings, and the APs that take it, each following it or with
// settings of their own.
export async function templatePage(ctx, id) {
	const [{ templates }, fleet] = await Promise.all([get('/v1/library'), get('/v1/aps')]);
	const t = templates.find((x) => x.id === id);
	if (!t) return { main: [h('div', { class: 'banner info' }, `No template ${id} that you can view.`)] };
	const names = new Map(fleet.aps.map((a) => [a.id, a]));
	const box = h('div', { class: 'edit flush' });
	const note = h('div', { class: 'sub' });
	const paths = Object.keys(t.values).sort();
	const fields = Object.fromEntries(paths.map((p) => [p, { value: t.values[p], from: t.at, origin: 'self' }]));
	const d = t.can_edit ? await schema() : null;
	const clear = (p) => h('button', { type: 'button', class: 'button small', onclick: async () => {
		const op = { kind: 'unset-template', template: t.id, path: p };
		const pv = await ask(box, op);
		if (pv) confirm(ctx, box, op, pv, [h('div', null, h('strong', null, `${t.name}: clear ${group(p).title} · ${group(p).label}`)),
			h('div', { class: 'sub' }, 'Its APs go back to what their folders set.')]);
	} }, 'Clear');
	return {
		main: [
			h('div', { class: 'crumbs' }, link('/library', 'Library'), ' › ', t.name),
			h('div', { class: 'head' },
				h('div', null,
					h('h1', null, t.name),
					h('div', { class: 'sub' }, 'AP template ', h('span', { class: 'mono' }, t.id), ' · for ', t.boards.join(', '),
						' · made at ', link(`/locations/${encodeURIComponent(t.at)}`, ctx.name('locations', t.at)),
						' · offered there and below')),
				h('div', { class: 'below', style: { display: 'flex', gap: '8px' } },
					t.can_edit && h('button', { type: 'button', class: 'button primary', onclick: () => templateEditor(ctx, d, t, fields, box) }, 'Edit settings'),
					t.can_edit && h('button', { type: 'button', class: 'button', onclick: () => aboutEditor(ctx, t, box) }, 'Name and boards'),
					h('button', { type: 'button', class: 'button', title: 'Save it as a file, to keep, share or import elsewhere', onclick: () => download(t, note) }, 'Download'),
					t.can_edit && h('button', { type: 'button', class: 'button', onclick: () => removeTemplate(ctx, t, box) }, 'Remove'))),
			note,
			box,
			h('section', { class: 'panel' },
				h('h2', null, 'Its settings', h('span', { class: 'note' }, 'counted as set where it is picked, ahead of that folder’s own')),
				paths.length === 0
					? h('div', { class: 'sub' }, 'None yet: its APs take what their folders set.')
					: paths.map((p) => h('div', { class: 'row' },
						h('div', { class: 'label' }, `${group(p).title} · ${group(p).label}`),
						h('div', { class: 'value' }, value(p, t.values[p])),
						t.can_edit && clear(p)))),
			h('section', { class: 'panel' },
				h('h2', null, 'APs that take it'),
				t.aps.length === 0
					? h('div', { class: 'sub' }, 'None: no folder above an AP of its boards picks it.')
					: h('table', { class: 'list' },
						h('tr', null, ['AP', 'Follows it?'].map((c) => h('th', null, c))),
						t.aps.map((a) => h('tr', null,
							h('td', null, link(`/aps/${encodeURIComponent(a.ap)}`, names.get(a.ap)?.name || a.ap)),
							h('td', null, a.follows ? 'Yes' : h('span', { class: 'chip warn' }, templateSays(names.get(a.ap)?.template)) ))))),
		],
	};
}

// templateEditor edits a template's settings, built from the schema like
// every other editor (0048), in one set-template.
function templateEditor(ctx, d, t, fields, box) {
	const all = Object.keys(d.fields).filter((p) => !p.includes('*'));
	const sections = SECTIONS.map(([title, keep]) => [title, all.filter(keep).sort()]);
	sections.push([MORE, all.filter(MANAGEMENT).sort()]);
	const { body, inputs, rows } = fieldsForm(d, sections, fields, t.at, MORE);
	const out = h('div', { class: 'edit flush' });
	const msg = h('div', { class: 'error' });
	const review = async () => {
		msg.replaceChildren();
		let values;
		try {
			values = changedValues(inputs, rows);
		} catch (e) {
			msg.replaceChildren(e.message);
			return;
		}
		const paths = Object.keys(values);
		if (!paths.length) {
			msg.replaceChildren('Nothing has changed.');
			return;
		}
		const op = { kind: 'set-template', template: t.id, values };
		const p = await ask(out, op);
		if (!p) return;
		const secret = (path) => describe(d, path)?.writeOnly;
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, `Template ${t.name}`)),
			h('ul', { class: 'becomes' }, paths.map((path) => h('li', null,
				`${group(path).title} · ${group(path).label}: `,
				secret(path) ? 'a new one' : [fields[path] ? [value(path, fields[path].value), ' → '] : '', value(path, values[path])]))),
			h('div', { class: 'sub' }, 'Its APs take these unless a folder below where it is picked, or the AP itself, sets its own.'),
		], [h('div', { class: 'sub warn' }, 'Applying a radio or country setting restarts the Wi-Fi on each AP listed; clients drop for a few seconds and reconnect.')]);
	};
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, `Edit ${t.name}`),
		h('div', { class: 'fieldform' },
			body,
			msg,
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review changes'),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel')),
			out)));
}

// aboutEditor renames a template and sets the boards it is for.
function aboutEditor(ctx, t, box) {
	const name = h('input', { type: 'text', value: t.name, maxlength: 64 });
	const boards = h('input', { type: 'text', class: 'mono', value: t.boards.join(', ') });
	const out = h('div', { class: 'edit flush' });
	const review = async () => {
		const op = { kind: 'edit-template', template: t.id, name: name.value.trim(), boards: boardsOf(boards.value) };
		const p = await ask(out, op);
		if (p) confirm(ctx, out, op, p, [h('div', null, h('strong', null, `${op.name}, for ${op.boards.join(', ')}`))]);
	};
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, 'Name and boards'),
		h('div', { class: 'fieldform' },
			h('div', { class: 'fields' },
				h('label', { class: 'field' }, h('span', { class: 'label' }, 'Name'), name),
				h('label', { class: 'field' }, h('span', { class: 'label' }, 'Boards'), boards,
					h('span', { class: 'sub' }, 'As OpenWrt names them, such as arista,c360; an AP shows its board under Enrollment.'))),
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review'),
				cancelButton(box)),
			out)));
}

async function removeTemplate(ctx, t, box) {
	const op = { kind: 'remove-template', template: t.id };
	const p = await ask(box, op);
	if (p) confirm(ctx, box, op, p, [h('div', null, h('strong', null, `Remove ${t.name}`)),
		h('div', { class: 'sub' }, 'Nothing picks it, so no AP takes it.')]);
}

// templatesTab is a Locations folder's Templates tab: each kind of AP below
// it, by board, the template it takes from here, picked here or above, and
// whether its APs follow it; the templates offered here; and, for someone
// who may change the folder, picking and making templates.
export async function templatesTab(ctx, here, page, edit) {
	const [{ templates }, fleet] = await Promise.all([get(`/v1/library?at=${encodeURIComponent(here)}`), get('/v1/aps')]);
	const below = fleet.aps.filter((a) => a.ancestry.includes(here) && a.template);
	const boards = new Set();
	for (const a of below) if (a.template.board) boards.add(a.template.board);
	for (const p of Object.keys(page.fields || {})) if (p.startsWith('templates.')) boards.add(p.slice('templates.'.length));
	const box = h('div', { class: 'edit flush' });
	const nodeName = (id) => ctx.name('locations', id);
	const rows = [...boards].sort().map((b) => {
		const r = page.fields?.[`templates.${b}`];
		const aps = below.filter((a) => a.template.board === b);
		const own = aps.filter((a) => a.template.id && !a.template.follows);
		const offered = templates.filter((t) => t.boards.includes(b));
		const takes = r && templates.find((t) => t.id === r.value);
		return h('tr', null,
			h('td', { class: 'mono' }, b),
			h('td', null, r
				? [takes ? link(`/library/${encodeURIComponent(r.value)}`, takes.name) : r.value, ' ', origin('locations', here, r, nodeName)]
				: h('span', { class: 'sub' }, 'none picked')),
			h('td', null, `${aps.length} AP${aps.length === 1 ? '' : 's'}`, own.length > 0 && h('div', { class: 'sub' }, `${own.length} with settings of their own`)),
			h('td', null, edit && pickControl(ctx, here, page.node.name, b, r, offered, box)));
	});
	return [
		box,
		h('section', { class: 'panel' },
			h('h2', null, 'Kinds of AP here', h('span', { class: 'note' }, 'each takes the template picked nearest above it')),
			rows.length === 0
				? h('div', { class: 'sub' }, 'No adopted AP below here has reported its board yet.')
				: h('table', { class: 'list' },
					h('tr', null, ['Board', 'Template', 'APs below', ''].map((c) => h('th', null, c))),
					rows)),
		h('section', { class: 'panel' },
			h('h2', null, 'Templates offered here', h('span', { class: 'note' }, 'made here or above, nearest first')),
			templates.length === 0
				? h('div', { class: 'sub' }, 'None yet.')
				: h('table', { class: 'list' },
					h('tr', null, ['Template', 'Boards', 'Made at', 'Settings'].map((c) => h('th', null, c))),
					templates.map((t) => h('tr', null,
						h('td', null, link(`/library/${encodeURIComponent(t.id)}`, t.name)),
						h('td', { class: 'mono' }, t.boards.join(', ')),
						h('td', null, t.at === here ? 'here' : nodeName(t.at)),
						h('td', null, String(Object.keys(t.values).length))))),
			edit && h('div', { class: 'below', style: { display: 'flex', gap: '8px' } },
				h('button', { type: 'button', class: 'button', onclick: () => newTemplate(ctx, here, page.node.name, [...boards].sort(), box) }, `New template at ${page.node.name}`),
				importButton(ctx, here, page.node.name, box))),
	];
}

// importButton imports a template file at this folder (0090): it is read
// here, shown with its settings, and made, settings and all, as one change,
// picked here for its boards unless that is unticked.
function importButton(ctx, here, nodeName, box) {
	const input = h('input', { type: 'file', accept: '.json,application/json', hidden: true });
	input.addEventListener('change', async () => {
		const file = input.files?.[0];
		input.value = '';
		if (!file) return;
		let f;
		try {
			f = readTemplateFile(await file.text());
		} catch (e) {
			box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
				h('h2', null, `Import ${file.name}`), h('div', { class: 'error' }, e.message), h('div', { class: 'actions' }, cancelButton(box))));
			return;
		}
		const { templates } = await get('/v1/library');
		importForm(ctx, here, nodeName, f, new Set(templates.map((t) => t.id)), box);
	});
	return [input, h('button', { type: 'button', class: 'button', title: 'From a file, such as one of the repo\'s templates/', onclick: () => input.click() }, 'Import template…')];
}

// importForm shows what a template file holds, its name and boards to
// change if need be, and makes it.
function importForm(ctx, here, nodeName, f, taken, box) {
	const name = h('input', { type: 'text', maxlength: 64, value: f.name });
	const boards = h('input', { type: 'text', class: 'mono', value: f.boards.join(', ') });
	const pick = h('input', { type: 'checkbox', checked: true });
	const out = h('div', { class: 'edit flush' });
	const paths = Object.keys(f.values).sort();
	const review = async () => {
		const op = { kind: 'add-template', template: freeID(name.value, taken), name: name.value.trim(), parent: here,
			boards: boardsOf(boards.value), default: pick.checked, values: f.values };
		const p = await ask(out, op);
		if (p) confirm(ctx, out, op, p, [h('div', null, h('strong', null, `Import ${op.name} at ${nodeName}`)),
			h('div', { class: 'sub' }, `For ${op.boards.join(', ')}, offered at ${nodeName} and below${op.default ? ', and picked here for those boards nothing is picked for here yet' : ''}, with ${paths.length} setting${paths.length === 1 ? '' : 's'}.`)],
		[h('div', { class: 'sub warn' }, 'Where it is picked, its radio and country settings restart the Wi-Fi on the APs it reaches; clients drop for a few seconds and reconnect.')]);
	};
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, `Import ${f.name}`),
		f.about && h('div', { class: 'sub' }, f.about),
		h('div', { class: 'fieldform' },
			h('div', { class: 'fields' },
				h('label', { class: 'field' }, h('span', { class: 'label' }, 'Name'), name),
				h('label', { class: 'field' }, h('span', { class: 'label' }, 'Boards'), boards),
				h('label', { class: 'field' }, h('span', { class: 'label' }, 'Pick it here'), pick)),
			h('div', { class: 'sub' }, `Its settings (${paths.length}):`),
			h('ul', { class: 'becomes' }, paths.map((p) => h('li', null, `${group(p).title} · ${group(p).label}: `, value(p, f.values[p])))),
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review'),
				cancelButton(box)),
			out)));
	name.focus();
}

// pickControl picks a board's template at this folder, from those offered
// here, or has the folder follow the one picked above.
function pickControl(ctx, here, nodeName, board, r, offered, box) {
	const path = `templates.${board}`;
	const select = h('select', null,
		h('option', { value: '' }, 'Pick here…'),
		offered.map((t) => h('option', { value: t.id, selected: r?.origin === 'self' && r.value === t.id }, t.name)));
	select.addEventListener('change', async () => {
		if (!select.value) return;
		const t = offered.find((x) => x.id === select.value);
		const op = { kind: 'set', tree: 'locations', node: here, path, value: select.value };
		const p = await ask(box, op);
		if (p) confirm(ctx, box, op, p, [h('div', null, h('strong', null, `${board} APs below ${nodeName} take ${t.name}`)),
			h('div', { class: 'sub' }, 'Its settings count as set here, ahead of this folder’s own; folders below and the APs themselves keep what they set.')]);
	});
	const follow = r?.origin === 'self' && h('button', { type: 'button', class: 'button small', onclick: async () => {
		const op = { kind: 'unset', tree: 'locations', node: here, path };
		const p = await ask(box, op);
		if (p) confirm(ctx, box, op, p, [h('div', null, h('strong', null, `${board} APs below ${nodeName} take the template picked above`))]);
	} }, 'Follow above');
	return [select, ' ', follow];
}

// newTemplate makes a template at this folder, for boards seen below it,
// and picks it here for those it is for, where nothing is picked here yet.
function newTemplate(ctx, here, nodeName, seen, box) {
	const name = h('input', { type: 'text', maxlength: 64, placeholder: 'C-360, no 6 GHz' });
	const boards = h('input', { type: 'text', class: 'mono', value: seen.join(', ') });
	const pick = h('input', { type: 'checkbox', checked: true });
	const out = h('div', { class: 'edit flush' });
	const review = async () => {
		const op = { kind: 'add-template', template: slug(name.value), name: name.value.trim(), parent: here, boards: boardsOf(boards.value), default: pick.checked };
		const p = await ask(out, op);
		if (p) confirm(ctx, out, op, p, [h('div', null, h('strong', null, `New template ${op.name} at ${nodeName}`)),
			h('div', { class: 'sub' }, `For ${op.boards.join(', ')}, offered at ${nodeName} and below${op.default ? ', and picked here for those boards nothing is picked for here yet' : ''}. It has no settings until you give it some.`)]);
	};
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, `New template at ${nodeName}`),
		h('div', { class: 'fieldform' },
			h('div', { class: 'fields' },
				h('label', { class: 'field' }, h('span', { class: 'label' }, 'Name'), name),
				h('label', { class: 'field' }, h('span', { class: 'label' }, 'Boards'), boards,
					h('span', { class: 'sub' }, 'As OpenWrt names them, such as arista,c360.')),
				h('label', { class: 'field' }, h('span', { class: 'label' }, 'Pick it here'), pick)),
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review'),
				cancelButton(box)),
			out)));
	name.focus();
}

