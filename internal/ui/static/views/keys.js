// Per-user keys (0070): each its own passphrase on a shared WPA2-PSK network,
// and the VLAN its client lands in. They belong to the network as its
// Services folder offers it; an operator there (a leasing office, say) adds,
// rotates and revokes them, one at a time or many pasted at once. A key change
// reaches the APs in seconds and drops no one; passphrases are sealed, and
// never shown again once set, so new ones are shown once, to hand out.

import { h, link } from '../dom.js';
import { get, post } from '../api.js';
import { when } from '../format.js';

// keysSection is a Services folder's per-user keys: a panel for each WPA2-PSK
// network the folder offers.
export async function keysSection(ctx, folder, page) {
	const nets = Object.keys(page.fields || {})
		.map((p) => p.match(/^network\.([^.]+)\.security$/))
		.filter((m) => m && page.fields[m[0]].value === 'wpa2-psk')
		.map((m) => m[1]).sort();
	if (!nets.length) return null;
	return nets.map((id) => keysPanel(folder, id, page.fields[`network.${id}.ssid`]?.value || id, page.fields[`network.${id}.keys.vlans`]?.value || []));
}

// keysBlock is a network card's line for its keys, which opens to the list;
// they are managed on the network's Services folder.
export function keysBlock(ctx, n) {
	const box = h('div', null);
	const vlans = n.fields?.['keys.vlans']?.value || [];
	const count = h('span', { class: 'sub' }, '…');
	let open = false;
	const toggle = h('button', { type: 'button', class: 'button small' }, 'Keys');
	const load = async () => {
		const r = await fetchKeys(n.from, n.id);
		count.textContent = r ? `${r.keys.length} ${r.keys.length === 1 ? 'key' : 'keys'}${vlans.length ? ` · VLANs ${vlans.join(', ')}` : ''}` : '';
		if (open) box.replaceChildren(r ? list(n.from, n.id, r, vlans, load) : h('div', { class: 'sub' }, 'Cannot read the keys.'));
	};
	toggle.onclick = () => {
		open = !open;
		toggle.textContent = open ? 'Hide keys' : 'Keys';
		if (open) load(); else box.replaceChildren();
	};
	load();
	return h('div', { class: 'keys' },
		h('div', { class: 'row' }, h('div', { class: 'label' }, 'Per-user keys'),
			h('div', { class: 'value' }, count, ' ', toggle, ' ', link(`/services/${encodeURIComponent(n.from)}`, 'manage'))),
		box);
}

function fetchKeys(folder, network) {
	return get(`/v1/keys?folder=${encodeURIComponent(folder)}&network=${encodeURIComponent(network)}`).catch(() => null);
}

function keysPanel(folder, network, ssid, vlans) {
	const body = h('div', null, h('div', { class: 'sub' }, '…'));
	// What a paste of many keys made, and the lines it could not add: kept
	// here, outside the list, which is drawn again after each change.
	const bulk = { notice: h('div', null), draft: '' };
	const load = async () => {
		const r = await fetchKeys(folder, network);
		body.replaceChildren(r ? list(folder, network, r, vlans, load, bulk) : h('div', { class: 'sub' }, 'Cannot read the keys.'));
	};
	load();
	return h('section', { class: 'panel keys' },
		h('h2', null, `Per-user keys · ${ssid}`, h('span', { class: 'note' },
			vlans.length ? `a key may put its client in VLAN ${vlans.join(', ')}, or on the network's own segment` : 'every key puts its client on the network\'s own segment; VLANs for keys are set on the network')),
		body,
		bulk.notice);
}

// generate makes a passphrase that is easy to read out: four groups of four
// letters and digits, without ones that look alike.
function generate() {
	const alphabet = 'abcdefghjkmnpqrstuvwxyz23456789';
	const r = crypto.getRandomValues(new Uint8Array(16));
	return [...r].map((b, i) => (i && i % 4 === 0 ? '-' : '') + alphabet[b % alphabet.length]).join('');
}

function list(folder, network, r, vlans, reload, bulk) {
	const msg = h('div', { class: 'sub' });
	const change = (op) => post('/v1/changes', { op: { network, node: folder, ...op }, reason: '' });
	const run = async (op, done) => {
		msg.textContent = '';
		msg.className = 'sub';
		try {
			await change(op);
			done?.();
			reload();
		} catch (e) {
			msg.textContent = e.message;
			msg.className = 'sub warn';
		}
	};
	const rows = r.keys.map((k) => h('tr', null,
		h('td', null, k.name, h('div', { class: 'sub mono' }, k.id)),
		h('td', null, k.vlan ? `VLAN ${k.vlan}` : 'the network\'s own'),
		h('td', { class: 'mono' }, k.macs.length ? k.macs.join(', ') : 'any device'),
		h('td', null, k.expires ? h('span', { class: k.expired ? 'chip bad' : null }, k.expired ? 'expired' : when(k.expires)) : '—'),
		h('td', null, r.can_edit && [
			h('button', { type: 'button', class: 'button small', onclick: () => {
				const p = prompt(`A new passphrase for ${k.name}, 8 to 63 characters. Its devices must be given it.`, generate());
				if (p) run({ kind: 'set-key', key: k.id, value: { name: k.name, passphrase: p, vlan: k.vlan || undefined, macs: k.macs, expires: k.expires || undefined } });
			} }, 'Rotate'), ' ',
			h('button', { type: 'button', class: 'button small danger', onclick: () => {
				if (window.confirm(`Revoke ${k.name}? Its devices are refused from the next time they join.`)) run({ kind: 'remove-key', key: k.id });
			} }, 'Revoke'),
		])));
	return h('div', null,
		r.keys.length
			? h('table', { class: 'list' }, h('tr', null, ['Key', 'Lands on', 'Bound to', 'Expires', ''].map((c) => h('th', null, c))), rows)
			: h('div', { class: 'sub' }, 'No keys yet.'),
		r.can_edit && addForm(vlans, run),
		r.can_edit && bulk && bulkForm(vlans, change, reload, bulk),
		msg);
}

function addForm(vlans, run) {
	const name = h('input', { type: 'text', placeholder: 'Unit 101', maxlength: 64 });
	const pass = h('input', { type: 'text', placeholder: 'passphrase, 8 to 63 characters', maxlength: 63, autocomplete: 'off' });
	const vlan = h('select', null, h('option', { value: '' }, 'the network\'s own segment'), vlans.map((v) => h('option', { value: v }, `VLAN ${v}`)));
	const expires = h('input', { type: 'date' });
	return h('form', { class: 'inline', onsubmit: (e) => {
		e.preventDefault();
		const value = { name: name.value.trim(), passphrase: pass.value };
		if (vlan.value) value.vlan = Number(vlan.value);
		if (expires.value) value.expires = new Date(expires.value + 'T23:59:59').toISOString();
		run({ kind: 'add-key', value }, () => { name.value = ''; pass.value = ''; expires.value = ''; });
	} }, name, ' ', pass, ' ',
	h('button', { type: 'button', class: 'button small', onclick: () => { pass.value = generate(); } }, 'Generate'), ' ',
	vlan, ' ', h('span', { class: 'sub' }, 'expires'), ' ', expires, ' ',
	h('button', { type: 'submit', class: 'button' }, 'Add key'));
}

// bulkForm adds many keys pasted at once, one a line: a name, a passphrase
// (blank or * to make one), and a VLAN, by commas or tabs. The keys it made
// are shown once with their passphrases, to hand out, and can be saved as a
// CSV file.
function bulkForm(vlans, change, reload, bulk) {
	const text = h('textarea', { rows: 5, placeholder: `Unit 101, , ${vlans[0] ?? ''}\nUnit 102, their-own-passphrase\nUnit 103\t*\t${vlans[1] ?? vlans[0] ?? ''}`, spellcheck: 'false' });
	const out = bulk.notice;
	text.value = bulk.draft;
	const button = h('button', { type: 'submit', class: 'button' }, 'Add these keys');
	const form = h('form', { class: 'bulk', onsubmit: async (e) => {
		e.preventDefault();
		const lines = text.value.split('\n').map((l) => l.trim()).filter(Boolean);
		if (!lines.length) return;
		button.disabled = true;
		const made = [], failed = [], left = [];
		for (const [i, line] of lines.entries()) {
			const [name = '', given = '', v = ''] = line.split(/\t|,/).map((x) => x.trim());
			const passphrase = !given || given === '*' ? generate() : given;
			const value = { name, passphrase };
			if (v) value.vlan = Number(v);
			out.replaceChildren(h('div', { class: 'sub' }, `Adding ${i + 1} of ${lines.length}…`));
			try {
				await change({ kind: 'add-key', value });
				made.push({ name, passphrase, vlan: v });
			} catch (err) {
				failed.push(`${name || `line ${i + 1}`}: ${err.message}`);
				left.push(line);
			}
		}
		button.disabled = false;
		bulk.draft = text.value = left.join('\n'); // what was not added stays, to fix and try again
		out.replaceChildren(...[
			made.length > 0 && handout(made),
			failed.length > 0 && h('div', { class: 'sub warn' }, h('div', null, `${failed.length} not added:`), failed.map((f) => h('div', null, f))),
		].filter(Boolean));
		reload();
	} }, h('div', { class: 'sub' }, 'Add many: one key a line, its name, passphrase (blank or * makes one) and VLAN, by commas or tabs.'), text, ' ', button, out);
	return form;
}

// handout shows the keys just made, with their passphrases, this once, and
// offers them as a CSV file.
function handout(made) {
	const csv = ['name,passphrase,vlan', ...made.map((k) => [k.name, k.passphrase, k.vlan].map((x) => `"${String(x ?? '').replace(/"/g, '""')}"`).join(','))].join('\n');
	const save = h('button', { type: 'button', class: 'button small', onclick: () => {
		const a = document.createElement('a');
		a.href = URL.createObjectURL(new Blob([csv + '\n'], { type: 'text/csv' }));
		a.download = 'keys.csv';
		a.click();
		URL.revokeObjectURL(a.href);
	} }, 'Save as CSV');
	return h('div', { class: 'handout' },
		h('div', null, h('strong', null, `${made.length} ${made.length === 1 ? 'key' : 'keys'} added.`), ' Their passphrases are shown this once; save them now. ', save),
		h('table', { class: 'list' }, h('tr', null, ['Key', 'Passphrase', 'VLAN'].map((c) => h('th', null, c))),
			made.map((k) => h('tr', null, h('td', null, k.name), h('td', { class: 'mono' }, k.passphrase), h('td', null, k.vlan || '—')))));
}
