// A Wi-Fi client's journey (0103): its sessions across the APs over the
// last day, where it roamed, and what looks wrong, from the APs' state
// reports. Opened from the Clients tab.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { bandName, ago } from '../format.js';

const when = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });

function span(s) {
	const min = Math.max(1, Math.round((new Date(s.to) - new Date(s.from)) / 60000));
	return min < 90 ? `${min} min` : `${Math.round(min / 6) / 10} h`;
}

// journeyPanel shows a client's journey in box, for the last hours.
export async function journeyPanel(box, mac, hours = 24) {
	box.replaceChildren(h('div', { class: 'sub', 'data-editing': true }, 'Reading its journey…'));
	let j;
	try {
		j = await get(`/v1/clients/${encodeURIComponent(mac)}?hours=${hours}`);
	} catch (e) {
		box.replaceChildren(h('div', { class: 'error' }, e.message));
		return;
	}
	const close = h('button', { type: 'button', class: 'button small', onclick: () => box.replaceChildren() }, 'Close');
	const longer = hours < 168 && h('button', { type: 'button', class: 'button small', onclick: () => journeyPanel(box, mac, 168) }, 'The last week');
	const sessions = [...j.sessions].reverse();
	box.replaceChildren(h('section', { class: 'panel journey', 'data-editing': true },
		h('h2', null, j.host || mac, h('span', { class: 'note mono' }, j.host ? mac : ''),
			h('span', { class: 'controls' }, longer, close)),
		!sessions.length
			? h('p', { class: 'sub' }, `No AP you can see reported it in the last ${hours === 168 ? 'week' : `${hours} hours`}.`)
			: h('p', { class: 'sub' }, `First seen ${ago(j.first)}, last ${ago(j.last)} · ${sessions.length} session${sessions.length === 1 ? '' : 's'} · ${j.roams} roam${j.roams === 1 ? '' : 's'}`),
		j.issues.length > 0 && h('ul', { class: 'issues' }, j.issues.map((i) => h('li', null,
			h('span', { class: `chip ${i.severity === 'warning' ? 'warn' : ''}` }, i.severity === 'warning' ? 'Look at' : 'Note'), ' ', i.message))),
		sessions.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['From', 'For', 'AP', 'Network', 'Band', 'Signal', 'Rate', 'Retries', 'Address'].map((t) => h('th', null, t))),
			sessions.map((s) => h('tr', null,
				h('td', null, when(s.from)),
				h('td', null, span(s)),
				h('td', null, link(`/locations/${encodeURIComponent(s.ap)}`, s.name)),
				h('td', null, s.ssid || s.network || '—'),
				h('td', null, bandName(s.band) || '—', s.gen && h('div', { class: 'sub' }, `802.11${s.gen}`)),
				h('td', null, s.signal_avg != null ? `${s.signal_avg} dBm` : '—', s.signal_min != null && s.signal_min !== s.signal_max && h('div', { class: 'sub' }, `${s.signal_min} to ${s.signal_max}`)),
				h('td', null, s.tx_rate ? `${Math.round(s.tx_rate)} Mbit/s` : '—'),
				h('td', null, s.retries != null ? `${Math.round(s.retries * 100)}%` : '—'),
				h('td', { class: 'mono' }, s.address || '—', s.dhcp && h('div', { class: 'sub' }, `DHCP ${s.dhcp}`)))))));
}
