// Signing in with an API token (0042), until 0024's password and one-time
// code arrive.

import { h } from '../dom.js';
import { signIn, signOut, get } from '../api.js';

export function signinPage(done) {
	const input = h('input', { type: 'password', autocomplete: 'off', spellcheck: 'false', placeholder: 'aeolus1.…', 'aria-label': 'API token' });
	const remember = h('input', { type: 'checkbox' });
	const error = h('div', { class: 'error', hidden: true });
	const submit = async (e) => {
		e.preventDefault();
		error.hidden = true;
		const t = input.value.trim();
		if (!t.startsWith('aeolus1.')) {
			error.textContent = 'That is not an Aeolus account token. It starts with aeolus1.';
			error.hidden = false;
			return;
		}
		try {
			signIn(t, remember.checked);
			await get('/v1/whoami');
			done();
		} catch (err) {
			signOut();
			error.textContent = err.status === 401 ? 'The manager does not know that token, or it was revoked.' : err.message;
			error.hidden = false;
		}
	};
	return h('form', { class: 'signin', onsubmit: submit },
		h('h1', null, 'Sign in to Aeolus'),
		h('p', null, 'Paste your own API token. Every change you make is logged under your name.'),
		input,
		h('label', null, remember, 'Keep me signed in on this browser'),
		error,
		h('button', { type: 'submit' }, 'Sign in'));
}
