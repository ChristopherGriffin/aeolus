// The Alerts tab of a Locations folder (0099): what needs attention on the
// APs below it, most urgent first, each a link to its AP. The manager works
// them out on each look from what it has, so one goes when its cause does.
// Below them, the history (0109): each alert of the last day that lasted
// two minutes or more, when it began and how long it lasted.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { ago, when } from '../format.js';

const SEVERITY = { critical: ['bad', 'Critical'], warning: ['warn', 'Warning'], info: ['', 'Info'] };

const chip = (s) => h('span', { class: `chip ${SEVERITY[s]?.[0] ?? ''}` }, SEVERITY[s]?.[1] ?? s);

// lasted writes how long an alert lasted: 4 min, 2 h 10 min, 3 d.
function lasted(ms) {
	const m = Math.round(ms / 60000);
	if (m < 60) return `${Math.max(1, m)} min`;
	if (m < 2880) return `${Math.floor(m / 60)} h${m % 60 ? ` ${m % 60} min` : ''}`;
	return `${Math.round(m / 1440)} d`;
}

export async function alertsTab(ctx, id) {
	const [res, past] = await Promise.all([
		get(`/v1/alerts?under=${encodeURIComponent(id)}`),
		get(`/v1/alerts/history?under=${encodeURIComponent(id)}&hours=24`).catch(() => null), // an older manager has none
	]);
	const list = res.alerts || [];
	const c = res.counts || {};
	const summary = h('div', { class: 'alertsum' },
		['critical', 'warning', 'info'].map((s) => h('span', { class: `chip ${SEVERITY[s][0]}` }, `${c[s] || 0} ${SEVERITY[s][1].toLowerCase()}`)));
	const now = list.length
		? h('section', { class: 'panel' }, summary,
			h('table', { class: 'alerts' },
				h('thead', null, h('tr', null, ['', 'AP', 'What', 'Since'].map((t) => h('th', null, t)))),
				h('tbody', null, list.map((a) => h('tr', { class: a.severity },
					h('td', null, chip(a.severity)),
					h('td', null, link(`/locations/${encodeURIComponent(a.ap)}`, a.name)),
					h('td', null, a.message),
					h('td', { class: 'sub' }, a.since ? ago(a.since) : ''))))))
		: h('section', { class: 'panel' }, summary, h('p', { class: 'sub' }, 'Nothing needs attention: every AP here is calling in, runs its config, and reports nothing wrong.'));
	return [now, past && historyPanel(past.alerts || [])];
}

function historyPanel(list) {
	return h('section', { class: 'panel' },
		h('h2', null, 'History', h('span', { class: 'note' }, 'the last 24 hours: each alert that lasted two minutes or more, newest first')),
		list.length
			? h('table', { class: 'alerts' },
				h('thead', null, h('tr', null, ['', 'AP', 'What', 'Began', 'Lasted'].map((t) => h('th', null, t)))),
				h('tbody', null, list.map((a) => h('tr', { class: a.ended ? null : a.severity },
					h('td', null, chip(a.severity)),
					h('td', null, link(`/locations/${encodeURIComponent(a.ap)}`, a.name)),
					h('td', null, a.message),
					h('td', { class: 'sub', title: ago(a.began) }, when(a.began)),
					h('td', { class: 'sub' }, a.ended ? lasted(new Date(a.ended) - new Date(a.began)) : `still, ${lasted(Date.now() - new Date(a.began))}`)))))
			: h('p', { class: 'sub' }, 'No alert lasted in the last 24 hours.'));
}
