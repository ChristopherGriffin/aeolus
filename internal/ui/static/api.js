// Talking to the manager (0042): the same API every client uses, with the
// person's own token. The token is kept for this tab, or on this browser if
// the person asked.

const KEY = 'aeolus.token';

function stores() {
	const out = [];
	try { out.push(sessionStorage); } catch { /* storage blocked */ }
	try { out.push(localStorage); } catch { /* storage blocked */ }
	return out;
}

export function token() {
	for (const s of stores()) {
		const t = s.getItem(KEY);
		if (t) return t;
	}
	return null;
}

export function signIn(t, remember) {
	signOut();
	try {
		(remember ? localStorage : sessionStorage).setItem(KEY, t);
	} catch {
		throw new Error('This browser does not let the page keep the token. Allow site data for the manager and try again.');
	}
}

export function signOut() {
	for (const s of stores()) s.removeItem(KEY);
}

export class APIError extends Error {
	constructor(status, message) {
		super(message);
		this.status = status;
	}
}

// get reads one API path. A 401 means the token is gone or revoked: the
// person is sent back to sign in.
export async function get(path) {
	const t = token();
	if (!t) throw new APIError(401, 'not signed in');
	let res;
	try {
		res = await fetch(path, { headers: { Authorization: 'Bearer ' + t, Accept: 'application/json' } });
	} catch {
		throw new APIError(0, 'Cannot reach the manager.');
	}
	const body = await res.json().catch(() => ({}));
	if (res.status === 401) {
		signOut();
		throw new APIError(401, body.error || 'signed out');
	}
	if (!res.ok) throw new APIError(res.status, body.error || res.statusText);
	return body;
}

// health reads /healthz, which needs no token.
export async function health() {
	const res = await fetch('/healthz', { headers: { Accept: 'application/json' } });
	return res.json();
}

// post sends one write. A refused change comes back as an APIError carrying
// the manager's reason, which the page shows as it is.
export async function post(path, body) {
	const t = token();
	if (!t) throw new APIError(401, 'not signed in');
	let res;
	try {
		res = await fetch(path, {
			method: 'POST',
			headers: { Authorization: 'Bearer ' + t, Accept: 'application/json', 'Content-Type': 'application/json' },
			body: JSON.stringify(body),
		});
	} catch {
		throw new APIError(0, 'Cannot reach the manager.');
	}
	const out = await res.json().catch(() => ({}));
	if (res.status === 401) {
		signOut();
		throw new APIError(401, out.error || 'signed out');
	}
	if (!res.ok) throw new APIError(res.status, out.error || res.statusText);
	return out;
}

let described = null;

// schema reads which fields can be set and what each takes (0048), once a
// page load: it is the same for everyone and changes only with a release.
export function schema() {
	described ??= Promise.all([get('/v1/schema'), get('/v1/agent/versions').catch(() => null)]).then(([d, agents]) => {
		// The agent releases the manager keeps, for the Agent version setting
		// (0079): current, its own, and the ones before it.
		const f = d.fields?.['system.agent'];
		if (f && agents) f.enum = ['current', ...agents.versions.map((v) => v.version)];
		return d;
	}).catch((e) => {
		described = null;
		throw e;
	});
	return described;
}
