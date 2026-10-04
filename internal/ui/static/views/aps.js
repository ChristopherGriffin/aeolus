// Every AP at a glance (0042), and the devices that may be OpenWiFi APs not
// yet enrolled (0034, 0068).

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { ago } from '../format.js';
import { apStatus } from '../layout.js';

export async function apsPage(ctx) {
	const [{ aps }, found] = await Promise.all([get('/v1/aps'), get('/v1/detected').catch(() => null)]);
	const where = (a) => a.ancestry.slice(1, -1).map((id) => ctx.name('locations', id)).join(' › ') || ctx.org;
	const counts = {};
	const rows = aps.map((a) => {
		const st = apStatus(a);
		counts[st.label] = (counts[st.label] || 0) + 1;
		return h('tr', null,
			h('td', null, h('span', { class: 'chip ' + st.chip }, st.label)),
			h('td', null, link(`/aps/${encodeURIComponent(a.id)}`, a.name)),
			h('td', null, where(a)),
			h('td', { class: 'mono' }, a.seen?.running != null ? `${a.seen.running} / ${a.version}` : `— / ${a.version}`),
			h('td', null, ago(a.seen?.at)),
			h('td', { class: 'mono' }, a.seen?.source || '—'));
	});
	return {
		main: [
			h('div', { class: 'head' },
				h('div', null,
					h('h1', null, 'APs'),
					h('div', { class: 'sub' }, aps.length === 0 ? 'No APs yet.' : Object.entries(counts).map(([k, n]) => `${n} ${k.toLowerCase()}`).join(' · ')))),
			aps.length > 0 && h('section', { class: 'panel' },
				h('table', { class: 'list' },
					h('tr', null, ['State', 'AP', 'Where', 'Running / its version', 'Last seen', 'From'].map((c) => h('th', null, c))),
					rows)),
			detected(ctx, found),
		],
		refresh: 30,
	};
}

// detected lists the devices that may be unconfigured OpenWiFi APs: possible
// by their DHCP, which asks for options 138 and 224, and confirmed when one
// knocked on the manager's option 224 listener; then the knocks from
// addresses no device is known by.
function detected(ctx, found) {
	if (!found || (!found.detected.length && !found.knocks.length)) return null;
	const rows = found.detected.map((d) => h('tr', null,
		h('td', null, h('span', { class: 'chip ' + (d.status === 'confirmed' ? 'warn' : 'idle'), title: d.status === 'confirmed' ? 'it knocked on the option 224 listener' : 'its DHCP asks for options 138 and 224' }, d.status)),
		h('td', null, h('span', { class: 'mono' }, d.mac), h('div', { class: 'sub' }, [d.private ? 'private MAC' : d.maker, d.host].filter(Boolean).join(' · '))),
		h('td', null, h('span', { class: 'mono' }, d.address || '—'),
			h('div', { class: 'sub' }, d.seen_by === 'relay' ? `relayed from ${d.subnet}` : `a Wi-Fi client of ${ctx.name('locations', d.seen_by)}`)),
		h('td', null, [
			d.fingerprint && 'asks for 138 and 224',
			d.knocks > 0 && `knocked ${d.knocks === 1 ? 'once' : `${d.knocks} times`}${d.sni ? ` for ${d.sni}` : ''}`,
		].filter(Boolean).join('; ')),
		h('td', null, ago(d.last))));
	const knocks = found.knocks.map((k) => h('tr', null,
		h('td', null, h('span', { class: 'chip idle', title: 'no device is known by this address' }, 'knock')),
		h('td', null, '—'),
		h('td', { class: 'mono' }, k.source),
		h('td', null, `knocked ${k.count === 1 ? 'once' : `${k.count} times`}${k.sni ? ` for ${k.sni}` : ''}`, k.subject && h('div', { class: 'sub' }, k.subject)),
		h('td', null, ago(k.last))));
	return h('section', { class: 'panel' },
		h('h2', null, 'Detected, not enrolled', h('span', { class: 'note' }, 'devices that may be OpenWiFi APs, from relayed DHCP and the option 224 listener')),
		h('table', { class: 'list' },
			h('tr', null, ['Status', 'Device', 'Address', 'Evidence', 'Last seen'].map((c) => h('th', null, c))),
			rows, knocks));
}
