// The Alerts tab of a Locations folder (0099): what needs attention on the
// APs below it, most urgent first, each a link to its AP. The manager works
// them out on each look from what it has, so one goes when its cause does.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { ago } from '../format.js';

const SEVERITY = { critical: ['bad', 'Critical'], warning: ['warn', 'Warning'], info: ['', 'Info'] };

export async function alertsTab(ctx, id) {
	const res = await get(`/v1/alerts?under=${encodeURIComponent(id)}`);
	const list = res.alerts || [];
	const c = res.counts || {};
	const summary = h('div', { class: 'alertsum' },
		['critical', 'warning', 'info'].map((s) => h('span', { class: `chip ${SEVERITY[s][0]}` }, `${c[s] || 0} ${SEVERITY[s][1].toLowerCase()}`)));
	if (!list.length)
		return h('section', { class: 'panel' }, summary, h('p', { class: 'sub' }, 'Nothing needs attention: every AP here is calling in, runs its config, and reports nothing wrong.'));
	return h('section', { class: 'panel' }, summary,
		h('table', { class: 'alerts' },
			h('thead', null, h('tr', null, ['', 'AP', 'What', 'Since'].map((t) => h('th', null, t)))),
			h('tbody', null, list.map((a) => h('tr', { class: a.severity },
				h('td', null, h('span', { class: `chip ${SEVERITY[a.severity]?.[0] ?? ''}` }, SEVERITY[a.severity]?.[1] ?? a.severity)),
				h('td', null, link(`/locations/${encodeURIComponent(a.ap)}`, a.name)),
				h('td', null, a.message),
				h('td', { class: 'sub' }, a.since ? ago(a.since) : ''))))));
}
