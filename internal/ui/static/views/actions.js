// An AP's actions (0104): Locate, Restart Wi-Fi and Reboot, asked of it
// once each, which it takes up on its next poll, within about a minute; and
// what came of the latest ones.

import { h } from '../dom.js';
import { get, post } from '../api.js';
import { ago } from '../format.js';
import { flash } from '../refresh.js';

const KINDS = [
	['locate', 'Locate', 'Blink every LED on it for a minute, to find it.', null],
	['restart-wifi', 'Restart Wi-Fi', 'Restart its Wi-Fi: every client on it drops, and joins again in a few seconds.', 'Its clients drop for a few seconds.'],
	['reboot', 'Reboot', 'Reboot it: it is off the air, and its wired clients cut off, for about two minutes.', 'It is off the air for about two minutes.'],
];
const STATE = { pending: ['', 'waiting for the AP'], done: ['ok', 'done'], failed: ['bad', 'failed'], expired: ['warn', 'expired: the AP did not take it up'] };

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
	return h('section', { class: 'panel' },
		h('h2', null, 'Actions'),
		canAct && h('div', { class: 'actions' }, KINDS.map(([kind, label, title, warn]) =>
			h('button', { type: 'button', class: 'button small', title, onclick: () => (warn ? ask(kind, label, warn) : post(`/v1/aps/${encodeURIComponent(id)}/actions`, { kind })
				.then(() => flash(`${label}: asked of ${name}; it acts on its next poll.`)).catch((err) => box.replaceChildren(h('div', { class: 'error' }, err.message)))) }, label))),
		box,
		list.length ? h('table', { class: 'list' },
			h('tr', null, ['Action', 'Asked', 'By', 'What came of it'].map((t) => h('th', null, t))),
			list.slice(0, 8).map((a) => h('tr', null,
				h('td', null, KINDS.find(([k]) => k === a.kind)?.[1] ?? a.kind),
				h('td', null, ago(a.at)),
				h('td', null, a.actor),
				h('td', null, h('span', { class: `chip ${STATE[a.state]?.[0] ?? ''}` }, STATE[a.state]?.[1] ?? a.state), a.result && h('span', { class: 'sub' }, ` ${a.result}`)))))
			: h('p', { class: 'sub' }, canAct ? 'None asked yet.' : 'None asked yet. Asking needs operator on this AP.'));
}
