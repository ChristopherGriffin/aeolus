// The Wi-Fi networks the APs here have of their own (0137): set on an AP
// itself and not by Aeolus, which leaves them as they are (0040). Shown
// under Aeolus's own on a folder's Networks tab, and an AP's, as each AP
// last reported them, a network a line whatever APs and radios it is on,
// and dull, as nothing here is Aeolus's to change. One with operator may
// have the APs take one out.

import { h, link } from '../dom.js';
import { post } from '../api.js';
import { bandName, security } from '../format.js';
import { flash } from '../refresh.js';

// asAeolus is a network's security as Aeolus names it, from its encryption
// as OpenWrt does; nothing for one Aeolus has no word for, as WPA alone.
export function asAeolus(enc) {
	return {
		'none': 'open', 'owe': 'owe', 'psk2': 'wpa2-psk', 'sae': 'wpa3-sae', 'sae-mixed': 'wpa2-wpa3',
		'wpa2': 'wpa2-enterprise', 'wpa3': 'wpa3-enterprise', 'wpa3-mixed': 'wpa2-wpa3-enterprise',
	}[(enc || 'none').split('+')[0]] || null;
}
const secured = (enc) => (asAeolus(enc) ? security(asAeolus(enc)) : enc);
const ORDER = ['2g', '5g', '6g'];

// gather is the APs' own networks by SSID: [{ssid, on: [{ap, entries}]}],
// entries being what an AP says of the network on each of its radios.
export function gather(reports) {
	const nets = new Map();
	for (const { ap, cfg } of reports) {
		for (const o of cfg?.condition?.state?.report?.others || []) {
			if (!nets.has(o.ssid)) nets.set(o.ssid, { ssid: o.ssid, on: new Map() });
			const on = nets.get(o.ssid).on;
			if (!on.has(ap.id)) on.set(ap.id, { ap, entries: [] });
			on.get(ap.id).entries.push(o);
		}
	}
	return [...nets.values()].map((n) => ({ ssid: n.ssid, on: [...n.on.values()] })).sort((a, b) => a.ssid.localeCompare(b.ssid));
}

const distinct = (list) => [...new Set(list)];
const named = (n) => (n.ssid ? `“${n.ssid}”` : 'the network with no name');
const listed = (names) => (names.length < 2 ? names.join('') : `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`);

// othersPanel is the panel, or nothing where no AP here has a network of
// its own. reports are the APs here with what each last said (configs).
export function othersPanel(page, reports) {
	const nets = gather(reports);
	if (!nets.length) return null;
	const canAct = page.role === 'operator' || page.role === 'admin';
	const box = h('div', { class: 'edit' });

	// Asked of each AP that has it, as an action of its own (0104): each
	// takes it out on its next poll.
	const remove = (n) => {
		const clients = n.on.reduce((sum, a) => sum + a.entries.reduce((s, e) => s + (e.clients || 0), 0), 0);
		const aps = n.on.map((a) => a.ap.name);
		box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
			h('div', null, h('strong', null, `Take ${named(n)} out of ${listed(aps)}?`)),
			h('div', { class: 'sub warn' }, `${clients === 0 ? 'No client is on it now.' : clients === 1 ? 'Its one client is disconnected, and cannot join it again.' : `Its ${clients} clients are disconnected, and cannot join it again.`} The other networks on the same radios drop for a few seconds while the Wi-Fi reloads.`),
			h('div', { class: 'sub' }, 'Aeolus has no copy of this network or its passphrase. Each AP keeps what it takes out, as it was, in /etc/aeolus/removed on the AP, to be put back by hand. It does it on its next poll, within about a minute.'),
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button danger', onclick: async (e) => {
					e.currentTarget.disabled = true;
					const failed = [];
					for (const a of n.on) {
						try {
							await post(`/v1/aps/${encodeURIComponent(a.ap.id)}/actions`, { kind: 'remove-network', ssid: n.ssid, sections: distinct(a.entries.map((x) => x.section)) });
						} catch (err) {
							failed.push(`${a.ap.name}: ${err.message}`);
						}
					}
					if (failed.length) {
						box.replaceChildren(h('div', { 'data-editing': true }, failed.map((f) => h('div', { class: 'error' }, f)),
							h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Close')));
						return;
					}
					box.replaceChildren();
					flash(`Asked of ${listed(aps)}: ${aps.length === 1 ? 'it takes' : 'each takes'} ${named(n)} out on its next poll.`);
				} }, 'Take it out'),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
		box.scrollIntoView({ block: 'nearest' });
	};

	const row = (n) => {
		const all = n.on.flatMap((a) => a.entries);
		const vlans = distinct(all.map((e) => e.vlan).filter((v) => v != null));
		const clients = all.reduce((s, e) => s + (e.clients || 0), 0);
		return h('tr', null,
			h('td', null, n.ssid || h('span', { class: 'sub' }, 'no name'),
				all.some((e) => e.hidden) && h('span', { class: 'chip idle', title: 'It does not say its name: a client must know it' }, 'hidden'),
				all.every((e) => e.off) && h('span', { class: 'chip idle', title: 'It is turned off on the AP, or its radio is' }, 'off')),
			h('td', null, distinct(all.map((e) => secured(e.security))).join(' / '),
				all.some((e) => e.ft) && h('span', { class: 'sub', title: 'It offers fast transition between APs (802.11r)' }, ' · 802.11r')),
			h('td', { title: vlans.length ? null : 'Its AP’s config does not say: as on the AP’s own LAN' }, vlans.length ? vlans.join(', ') : '—'),
			h('td', null, n.on.map((a) => h('div', null, link(`/aps/${encodeURIComponent(a.ap.id)}/networks`, a.ap.name),
				` · ${distinct(a.entries.map((e) => e.band)).sort((x, y) => ORDER.indexOf(x) - ORDER.indexOf(y)).map((b) => bandName(b) || 'a radio').join(', ')}`))),
			h('td', null, String(clients)),
			canAct && h('td', { class: 'rowacts' },
				h('button', { type: 'button', class: 'button small', 'aria-label': `Take ${named(n)} out`, title: 'Have each AP that has it take it out of its config', onclick: () => remove(n) }, 'Remove…')));
	};

	return h('section', { class: 'panel foreign' },
		h('h2', null, 'On the APs, not Aeolus’s', h('span', { class: 'note' }, 'networks set on an AP itself, as each AP last reported them: Aeolus leaves them as they are')),
		h('table', { class: 'list' },
			h('tr', null, ['Network', 'Security', 'VLAN', 'On', 'Clients'].map((c) => h('th', null, c)), canAct && h('th', null, h('span', { class: 'sr' }, 'Remove'))),
			nets.map(row)),
		box);
}
