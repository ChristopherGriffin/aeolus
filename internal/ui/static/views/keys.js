// Per-user keys (0070): each its own passphrase on a shared WPA2-PSK network,
// and the VLAN its client lands in. They belong to the network as its
// Services folder offers it; an operator there (a leasing office, say) adds,
// rotates and revokes them. A key change reaches the APs in seconds and drops
// no one; passphrases are sealed, and never shown again once set.

import { h } from '../dom.js';
import { get, post } from '../api.js';
import { when } from '../format.js';

// keysBlock is a network card's line for its keys, which opens to the list.
export function keysBlock(ctx, n) {
	const box = h('div', null);
	const vlans = n.fields?.['keys.vlans']?.value || [];
	const count = h('span', { class: 'sub' }, '…');
	let open = false;
	const toggle = h('button', { type: 'button', class: 'button small' }, 'Keys');
	const load = async () => {
		const r = await get(`/v1/keys?folder=${encodeURIComponent(n.from)}&network=${encodeURIComponent(n.id)}`).catch(() => null);
		count.textContent = r ? `${r.keys.length} ${r.keys.length === 1 ? 'key' : 'keys'}${vlans.length ? ` · VLANs ${vlans.join(', ')}` : ''}` : '';
		if (open) box.replaceChildren(r ? list(n, r, vlans, load) : h('div', { class: 'sub' }, 'Cannot read the keys.'));
	};
	toggle.onclick = () => {
		open = !open;
		toggle.textContent = open ? 'Hide keys' : 'Keys';
		if (open) load(); else box.replaceChildren();
	};
	load();
	return h('div', { class: 'keys' },
		h('div', { class: 'row' }, h('div', { class: 'label' }, 'Per-user keys'), h('div', { class: 'value' }, count, ' ', toggle)),
		box);
}

function list(n, r, vlans, reload) {
	const msg = h('div', { class: 'sub' });
	const run = async (op, done) => {
		msg.textContent = '';
		msg.className = 'sub';
		try {
			await post('/v1/changes', { op: { network: n.id, node: n.from, ...op }, reason: '' });
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
				const p = prompt(`A new passphrase for ${k.name} (8 to 63 characters). Its devices must be given it.`);
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
		msg);
}

function addForm(vlans, run) {
	const name = h('input', { type: 'text', placeholder: 'Unit 101', maxlength: 64 });
	const pass = h('input', { type: 'text', placeholder: 'passphrase, 8 to 63 characters', maxlength: 63, autocomplete: 'off' });
	const vlan = h('select', null, h('option', { value: '' }, 'the network\'s own segment'), vlans.map((v) => h('option', { value: v }, `VLAN ${v}`)));
	const expires = h('input', { type: 'date' });
	const submit = h('button', { type: 'submit', class: 'button' }, 'Add key');
	return h('form', { class: 'inline', onsubmit: (e) => {
		e.preventDefault();
		const value = { name: name.value.trim(), passphrase: pass.value };
		if (vlan.value) value.vlan = Number(vlan.value);
		if (expires.value) value.expires = new Date(expires.value + 'T23:59:59').toISOString();
		run({ kind: 'add-key', value }, () => { name.value = ''; pass.value = ''; expires.value = ''; });
	} }, name, ' ', pass, ' ', vlan, ' ', h('span', { class: 'sub' }, 'expires'), ' ', expires, ' ', submit);
}
