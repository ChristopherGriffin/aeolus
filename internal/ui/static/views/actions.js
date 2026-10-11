// An AP's actions (0104): Locate, Restart Wi-Fi and Reboot, asked of it
// once each, which it takes up on its next poll, within about a minute; and
// what came of the latest ones, a client's Reconnect among them (0107).

import { h } from '../dom.js';
import { get, post } from '../api.js';
import { ago } from '../format.js';
import { flash } from '../refresh.js';

const KINDS = [
	['locate', 'Locate', 'Blink every LED on it for a minute, to find it.', null],
	['restart-wifi', 'Restart Wi-Fi', 'Restart its Wi-Fi: every client on it drops, and joins again in a few seconds.', 'Its clients drop for a few seconds.'],
	['reboot', 'Reboot', 'Reboot it: it is off the air, and its wired clients cut off, for about two minutes.', 'It is off the air for about two minutes.'],
];
// What else an AP may be asked, from elsewhere on its page: named here for
// the list of what was asked.
const ALSO = { 'client-test': 'Client test' };
const STATE = { pending: ['', 'waiting for the AP'], running: ['', 'taken up by the AP'], done: ['ok', 'done'], failed: ['bad', 'failed'], expired: ['warn', 'expired: the AP did not take it up'] };

export async function actionsPanel(id, name, canAct) {
	let list = [];
	try {
		list = (await get(`/v1/aps/${encodeURIComponent(id)}/actions`)).actions || [];
	} catch { /* an older manager has none */ }
	const box = h('div', { class: 'edit' });
	const ask = (kind, label, warn) => {
		box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
			h('div', null, h('strong', null, `${label} ${name}?`)),
			warn && h('div', { class: 'sub warn' }, warn),
			h('div', { class: 'sub' }, 'It does it on its next poll, within about a minute.'),
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: `button ${kind === 'reboot' ? 'danger' : 'primary'}`, onclick: async (e) => {
					e.currentTarget.disabled = true;
					try {
						await post(`/v1/aps/${encodeURIComponent(id)}/actions`, { kind });
						box.replaceChildren();
						flash(`${label}: asked of ${name}; it acts on its next poll.`);
					} catch (err) {
						box.replaceChildren(h('div', { class: 'error' }, err.message));
					}
				} }, label),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
	};
	// Its client tests are under Assurance, each with what it found: here,
	// the others, the latest few, a line each.
	const others = list.filter((a) => a.kind !== 'client-test').slice(0, 4);
	return h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols', title: 'What it can be asked to do. It does it on its next poll, within about a minute.' }, 'Actions',
			canAct && h('span', { class: 'controls' }, KINDS.map(([kind, label, title, warn]) =>
				h('button', { type: 'button', class: 'button small', title, onclick: () => (warn ? ask(kind, label, warn) : post(`/v1/aps/${encodeURIComponent(id)}/actions`, { kind })
					.then(() => flash(`${label}: asked of ${name}; it acts on its next poll.`)).catch((err) => box.replaceChildren(h('div', { class: 'error' }, err.message)))) }, label)))),
		box,
		// What came of one is said where it did not go well; of one that did, on hover.
		others.length > 0 && h('table', { class: 'list' },
			others.map((a) => h('tr', { title: a.result || null },
				h('td', null, a.kind === 'disconnect' ? ['Reconnect ', h('span', { class: 'mono' }, a.target)] : KINDS.find(([k]) => k === a.kind)?.[1] ?? ALSO[a.kind] ?? a.kind),
				h('td', { class: 'sub' }, `${ago(a.at)} · ${a.actor}`),
				h('td', null, h('span', { class: `chip ${STATE[a.state]?.[0] ?? ''}` }, STATE[a.state]?.[1] ?? a.state),
					a.result && a.state !== 'done' && h('span', { class: 'sub' }, ` ${a.result}`))))),
		// One who may not ask is told so, where there is nothing else to show.
		!canAct && !others.length && h('p', { class: 'sub pad' }, 'Asking needs operator on this AP.'));
}
