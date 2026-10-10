// The System tab's editor (0052): every system field the schema has, built
// from its description, set on this folder or AP in one change. SNMP's
// fields show once SNMP is turned on.

import { h } from '../dom.js';
import { group, value } from '../format.js';
import { describe, fieldsForm, changedValues } from './edit.js';
import { ask, confirm } from './confirm.js';

const MORE = 'Management and more';

// The order system fields are offered in. Any other the schema has, such as
// management addressing, which can cut an AP off if it is wrong, folds away
// under Management and more.
const SECTIONS = [
	['Place and time', ['system.country', 'system.tz', 'system.ntp']],
	['Logging', ['system.syslog']],
	['SNMP', ['system.snmp.enabled', 'system.snmp.community', 'system.snmp.v3.user', 'system.snmp.v3.auth',
		'system.snmp.v3.privacy', 'system.snmp.location', 'system.snmp.contact']],
	['Agent', ['system.poll', 'system.ssh_keys']],
	// Where the manager sends alerts for the APs here (0101).
	['Alerts', ['notify.ntfy', 'notify.webhook', 'notify.severity', 'notify.resolved']],
];

// systemEditor opens the editor in box, for the Locations node here, with
// the values in force there (fields, by path).
export function systemEditor(ctx, d, here, nodeName, fields, box) {
	const placed = new Set(SECTIONS.flatMap(([, paths]) => paths));
	const rest = Object.keys(d.fields).filter((p) => p.startsWith('system.') && !placed.has(p)).sort();
	const { body, inputs, rows } = fieldsForm(d, [...SECTIONS, [MORE, rest]], fields, here, MORE);
	const snmpOn = rows.get('system.snmp.enabled')?.it.el;
	const sync = () => {
		for (const [path, { row }] of rows)
			if (path.startsWith('system.snmp.') && path !== 'system.snmp.enabled') row.hidden = !snmpOn?.checked;
	};
	snmpOn?.addEventListener('change', sync);
	sync();
	// The time zones on offer are the country's, as it stands in the form
	// (0074).
	const country = rows.get('system.country')?.it.el;
	const narrow = () => rows.get('system.tz')?.it.narrow?.(country?.value.trim().toUpperCase());
	country?.addEventListener('input', narrow);
	country?.addEventListener('change', narrow);
	narrow();

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
		const op = paths.length === 1
			? { kind: 'set', tree: 'locations', node: here, path: paths[0], value: values[paths[0]] }
			: { kind: 'set', tree: 'locations', node: here, values };
		const p = await ask(out, op);
		if (!p) return;
		const secret = (path) => describe(d, path)?.writeOnly;
		let note = h('div', { class: 'sub' }, 'Applying does not interrupt Wi-Fi.');
		if (paths.includes('system.country'))
			note = h('div', { class: 'sub warn' }, 'Applying restarts the Wi-Fi on each AP listed, as the radios use the country; clients drop for a few seconds and reconnect.');
		if (paths.some((path) => path.startsWith('system.management.')))
			note = h('div', { class: 'sub warn' }, "Applying reloads each AP's network. An AP that can no longer reach Aeolus puts its old settings back within 90 seconds.");
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, `System settings on ${nodeName}`)),
			h('ul', { class: 'becomes' }, paths.map((path) => h('li', null,
				`${group(path).title} · ${group(path).label}: `,
				secret(path) ? 'a new one' : [fields[path] ? [value(path, fields[path].value), ' → '] : '', value(path, values[path])]))),
			h('div', { class: 'sub' }, 'APs below that set their own keep theirs.'),
		], [note]);
	};
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, 'Edit system settings'),
		h('div', { class: 'fieldform' },
			body,
			msg,
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review changes'),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel')),
			out)));
}
