// The Wi-Fi networks the APs here have of their own (0137): set on an AP
// itself and not by Aeolus, which leaves them as they are (0040). Shown
// under Aeolus's own on a folder's Networks tab, and an AP's, as each AP
// last reported them, a network a line whatever APs and radios it is on,
// and dull, as nothing here is Aeolus's to change. One with operator may
// have the APs take one out, or make one Aeolus's own: import, which asks
// an AP for the network as it has it, its passphrase with it, and makes
// the network of that in a Services folder.

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

// idFor is an ID for a network of this SSID that none here has: lower
// case, letters, digits and hyphens, a letter first, and not one of the
// names Aeolus keeps for its own sections on an AP (the schema's networkId).
export function idFor(ssid, taken) {
	let id = String(ssid || '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 24);
	if (!/^[a-z]/.test(id) || /^(vlan[0-9]+($|-)|port-|n[0-9a-f]{8}($|-))/.test(id)) id = `net-${id}`.replace(/-+$/, '');
	let out = id;
	for (let i = 2; taken.has(out); i++) out = `${id}-${i}`;
	return out;
}
const ID = /^[a-z][a-z0-9-]{0,31}$/;

// secOf is the one security of a network's entries as Aeolus names it, or
// null where they have more than one. WPA3 alone on 6 GHz beside WPA2/WPA3
// on the other bands is WPA2/WPA3: it is WPA3 alone there by rule (0086).
function secOf(entries) {
	const off6 = distinct(entries.filter((e) => e.band !== '6g').map((e) => asAeolus(e.security)));
	const on6 = distinct(entries.filter((e) => e.band === '6g').map((e) => asAeolus(e.security)));
	if (off6.length > 1 || on6.length > 1) return null;
	if (!off6.length) return on6[0] ?? null;
	if (!on6.length || on6[0] === off6[0] || (off6[0] === 'wpa2-wpa3' && on6[0] === 'wpa3-sae')) return off6[0];
	return null;
}
const SIX = new Set(['owe', 'wpa3-sae', 'wpa2-wpa3', 'wpa3-enterprise', 'wpa2-wpa3-enterprise']);
const named = (n) => (n.ssid ? `“${n.ssid}”` : 'the network with no name');
const listed = (names) => (names.length < 2 ? names.join('') : `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`);

// othersPanel is the panel, or nothing where no AP here has a network of
// its own. reports are the APs here with what each last said (configs).
// into is where one may be imported: folders, the Services folders that
// apply here and the person may change, [{id, name}]; taken, the IDs
// networks here have; bands, those some AP here has a radio for.
export function othersPanel(page, reports, into) {
	const nets = gather(reports);
	if (!nets.length) return null;
	const canAct = page.role === 'operator' || page.role === 'admin';
	const box = h('div', { class: 'edit' });

	// Import: where the network goes and as what is asked here; the rest,
	// its passphrase above all, only the AP has, and is read from it when
	// the change is made. It is asked of the AP that has the most clients
	// on the network.
	const bring = (n) => {
		const all = n.on.flatMap((a) => a.entries);
		const sec = secOf(all);
		// Asked of an AP that has it as that security: one that has it on
		// 6 GHz alone has it as WPA3 there, whatever the others have.
		const has = (a) => a.entries.some((e) => asAeolus(e.security) === sec);
		const from = [...n.on].filter(has).sort((a, b) => b.entries.reduce((s, e) => s + (e.clients || 0), 0) - a.entries.reduce((s, e) => s + (e.clients || 0), 0))[0];
		const offered = ORDER.filter((b) => into.bands.has(b) && (b !== '6g' || SIX.has(sec)));
		const now = new Set(all.map((e) => e.band));
		const folder = h('select', null, into.folders.map((f) => h('option', { value: f.id }, `Services › ${f.name}`)));
		const id = h('input', { type: 'text', value: idFor(n.ssid, into.taken), maxlength: 32, spellcheck: false, 'aria-label': 'Its ID in Aeolus' });
		const vlans = distinct(all.map((e) => e.vlan).filter((v) => v != null));
		const vlan = h('input', { type: 'number', min: 1, max: 4094, value: vlans.length === 1 ? vlans[0] : '', 'aria-label': 'Its VLAN' });
		const ticks = offered.map((b) => [b, h('input', { type: 'checkbox', checked: now.has(b) })]);
		const msg = h('div', { class: 'error' });
		// An AP that has it on fewer bands than it will be on as Aeolus's.
		const wider = () => {
			const want = ticks.filter(([, t]) => t.checked).map(([b]) => b);
			return n.on.map((a) => {
				const has = new Set(a.entries.map((e) => e.band));
				const more = want.filter((b) => !has.has(b));
				return more.length ? `${a.ap.name} will have it on ${more.map((b) => bandName(b)).join(' and ')} too` : null;
			}).filter(Boolean);
		};
		const widerNote = h('div', { class: 'sub' });
		const sync = () => widerNote.replaceChildren(wider().length ? `${listed(wider())}, where it is not now.` : '');
		for (const [, t] of ticks) t.addEventListener('change', sync);
		sync();
		const go = async (e) => {
			msg.replaceChildren();
			const want = ticks.filter(([, t]) => t.checked).map(([b]) => b);
			const v = Number(vlan.value);
			if (!ID.test(id.value)) return msg.replaceChildren('Its ID is lower-case letters, digits and hyphens, a letter first, 32 at the most.');
			if (into.taken.has(id.value)) return msg.replaceChildren(`There is a network ${id.value} here already: give this one another ID.`);
			if (!Number.isInteger(v) || v < 1 || v > 4094) return msg.replaceChildren('Give the VLAN its clients are on, from 1 to 4094.');
			if (!want.length) return msg.replaceChildren('Tick a band at least.');
			e.currentTarget.disabled = true;
			try {
				const res = await post(`/v1/aps/${encodeURIComponent(from.ap.id)}/actions`, {
					kind: 'import-network', ssid: n.ssid, sections: distinct(from.entries.map((x) => x.section)),
					folder: folder.value, network: id.value, bands: want, vlan: v, security: sec });
				const got = res.action?.params || {};
				if (res.action?.kind !== 'import-network' || got.network !== id.value || got.folder !== folder.value || got.vlan !== v || (got.bands || []).join() !== want.join())
					throw new Error('another asking of this AP is waiting: try again in a minute');
				box.replaceChildren();
				flash(`Asked of ${from.ap.name}: it says the network on its next poll, and Aeolus makes ${named(n)} its own then, within about a minute. What came of it is under Actions on ${from.ap.name}’s page.`);
			} catch (err) {
				e.target.disabled = false;
				msg.replaceChildren(err.message);
			}
		};
		const row = (label, v) => h('div', { class: 'row' }, h('div', { class: 'label' }, label), h('div', { class: 'value' }, v));
		box.replaceChildren(h('div', { class: 'preview testform', 'data-editing': true },
			h('div', null, h('strong', null, `Make ${named(n)} Aeolus’s own`)),
			row('Into', folder),
			row('ID', [id, h('span', { class: 'sub' }, ' its name in Aeolus, which clients never see')]),
			row('SSID', n.ssid || h('span', { class: 'sub' }, 'no name')),
			row('Security', [security(sec), all.some((x) => x.ft) && h('span', { class: 'sub' }, ' · 802.11r')]),
			row('Bands', ticks.map(([b, t]) => h('label', { class: 'tick' }, t, ` ${bandName(b)}`))),
			row('VLAN', [vlan, vlans.length !== 1 && h('span', { class: 'sub' }, vlans.length ? ` its APs have it on ${vlans.join(' and ')}` : ' its AP’s config does not say: the VLAN its clients are to be on')]),
			widerNote,
			h('div', { class: 'sub' }, `Its passphrase and its other settings are read from ${from.ap.name} when it next polls, within about a minute, and go into the network as they are: none is shown here, and the passphrase is sealed as any is. What has no place in Aeolus is said in what came of it; a network that has what Aeolus cannot keep, and its clients may depend on, is not imported, and that says why.`),
			h('div', { class: 'sub warn' }, `Each AP that has ${named(n)} of its own, with the same passphrase, takes its own out as Aeolus’s goes in, and keeps it in /etc/aeolus/removed. Clients drop for a few seconds and join again, by the same name and passphrase. An AP whose own has another passphrase keeps it, beside Aeolus’s.`),
			msg,
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button primary', onclick: go }, 'Import'),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
		box.scrollIntoView({ block: 'nearest' });
	};
	// Whether one can be imported, and why not where it cannot.
	const cannot = (n) => {
		const all = n.on.flatMap((a) => a.entries);
		if (all.some((e) => asAeolus(e.security) === null)) return 'Aeolus has no name for its security, so it cannot be made Aeolus’s own';
		if (secOf(all) === null) return 'Its APs have it with more than one kind of security: import it from one AP’s own Networks tab';
		if (all.some((e) => e.wds)) return 'Another AP may reach the network through it (WDS), which Aeolus cannot keep';
		if (!into?.folders?.length) return 'No Services folder that applies here is yours to change';
		return null;
	};

	// Asked of each AP that has it, as an action of its own (0104): each
	// takes it out on its next poll.
	const remove = (n) => {
		const clients = n.on.reduce((sum, a) => sum + a.entries.reduce((s, e) => s + (e.clients || 0), 0), 0);
		const aps = n.on.map((a) => a.ap.name);
		const wds = n.on.some((a) => a.entries.some((e) => e.wds));
		box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
			h('div', null, h('strong', null, `Take ${named(n)} out of ${listed(aps)}?`)),
			wds && h('div', { class: 'sub warn' }, 'Another AP may reach the network through it (WDS). Taking it out would cut that AP off.'),
			h('div', { class: 'sub warn' }, `${clients === 0 ? 'No client is on it now.' : clients === 1 ? 'Its one client is disconnected, and cannot join it again.' : `Its ${clients} clients are disconnected, and cannot join it again.`} The other networks on the same radios drop for a few seconds while the Wi-Fi reloads.`),
			h('div', { class: 'sub' }, 'Aeolus has no copy of this network or its passphrase. Each AP keeps what it takes out, as it was, in /etc/aeolus/removed on the AP, to be put back by hand. It does it on its next poll, within about a minute.'),
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button danger', onclick: async (e) => {
					e.currentTarget.disabled = true;
					const failed = [], asked = [];
					for (const a of n.on) {
						try {
							const res = await post(`/v1/aps/${encodeURIComponent(a.ap.id)}/actions`, { kind: 'remove-network', ssid: n.ssid, sections: distinct(a.entries.map((x) => x.section)) });
							// What was asked is what the manager has: not another asking that stood in for it.
							if (res.action?.params?.ssid !== n.ssid) throw new Error('another asking of this AP is waiting: try again in a minute');
							asked.push(a.ap.name);
						} catch (err) {
							failed.push(`${a.ap.name}: ${err.message}`);
						}
					}
					if (failed.length) {
						box.replaceChildren(h('div', { 'data-editing': true }, failed.map((f) => h('div', { class: 'error' }, f)),
							asked.length > 0 && h('div', { class: 'sub' }, `Asked of ${listed(asked)} all the same: ${asked.length === 1 ? 'it takes' : 'each takes'} it out on its next poll.`),
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
				all.every((e) => e.off) && h('span', { class: 'chip idle', title: 'It is turned off on the AP, or its radio is' }, 'off'),
				all.some((e) => e.wds) && h('span', { class: 'chip idle', title: 'Another AP may reach the network through it (WDS): taking it out would cut that AP off' }, 'WDS')),
			h('td', null, distinct(all.map((e) => secured(e.security))).join(' / '),
				all.some((e) => e.ft) && h('span', { class: 'sub', title: 'It offers fast transition between APs (802.11r)' }, ' · 802.11r')),
			h('td', { title: vlans.length ? null : 'Its AP’s config does not say: as on the AP’s own LAN' }, vlans.length ? vlans.join(', ') : '—'),
			h('td', null, n.on.map((a) => h('div', null, link(`/aps/${encodeURIComponent(a.ap.id)}/networks`, a.ap.name),
				` · ${distinct(a.entries.map((e) => e.band)).sort((x, y) => ORDER.indexOf(x) - ORDER.indexOf(y)).map((b) => bandName(b) || 'a radio').join(', ')}`))),
			h('td', null, String(clients)),
			canAct && h('td', { class: 'rowacts' },
				h('button', { type: 'button', class: 'button small', 'aria-label': `Make ${named(n)} Aeolus’s own`, disabled: !!cannot(n),
					title: cannot(n) || 'Make it a network of Aeolus’s, with its settings and its passphrase, read from the AP', onclick: () => bring(n) }, 'Import…'),
				h('button', { type: 'button', class: 'button small', 'aria-label': `Take ${named(n)} out`, title: 'Have each AP that has it take it out of its config', onclick: () => remove(n) }, 'Remove…')));
	};

	return h('section', { class: 'panel foreign' },
		h('h2', null, 'On the APs, not Aeolus’s', h('span', { class: 'note' }, 'networks set on an AP itself, as each AP last reported them: Aeolus leaves them as they are')),
		h('table', { class: 'list' },
			h('tr', null, ['Network', 'Security', 'VLAN', 'On', 'Clients'].map((c) => h('th', null, c)), canAct && h('th', null, h('span', { class: 'sr' }, 'Import or remove'))),
			nets.map(row)),
		box);
}
