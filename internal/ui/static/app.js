// The Aeolus UI (0042): a hash-routed page over the API. Part 1 reads only.

import { h } from './dom.js';
import { get, token, signOut, APIError } from './api.js';
import { signinPage } from './views/signin.js';
import { treePage } from './views/tree.js';
import { apPage } from './views/ap.js';
import { apsPage } from './views/aps.js';
import { libraryPage } from './views/library.js';
import { changesPage } from './views/changes.js';

const routes = [
	[/^\/(locations|services)(?:\/([^/]+))?$/, (ctx, m) => treePage(ctx, m[1], m[2] && decodeURIComponent(m[2]))],
	[/^\/aps$/, (ctx) => apsPage(ctx)],
	[/^\/aps\/([^/]+)$/, (ctx, m) => apPage(ctx, decodeURIComponent(m[1]))],
	[/^\/library$/, (ctx) => libraryPage(ctx)],
	[/^\/changes$/, (ctx) => changesPage(ctx)],
];

const TABS = [['/locations', 'Locations'], ['/services', 'Services'], ['/aps', 'APs'], ['/library', 'Library'], ['/changes', 'Changes']];

let timer = null;
let rendering = 0;

function route() {
	return decodeURI(location.hash.replace(/^#/, '')) || '/locations';
}

// context is what every page needs: who is signed in, and both trees.
async function context() {
	const [who, loc, svc] = await Promise.all([get('/v1/whoami'), get('/v1/trees/locations'), get('/v1/trees/services')]);
	const trees = { locations: index(loc.nodes), services: index(svc.nodes) };
	return {
		who,
		trees,
		org: trees.locations.list[0]?.name || trees.services.list[0]?.name || '',
		name: (tree, id) => trees[tree].nodes.get(id)?.name || id,
	};
}

function index(list) {
	const nodes = new Map(list.map((n) => [n.id, n]));
	return { list, nodes, root: list[0]?.id };
}

async function render(quiet) {
	clearTimeout(timer);
	const app = document.getElementById('app');
	const r = route();
	if (!token() || r === '/signin') {
		app.replaceChildren(signinPage(() => { location.hash = '#/locations'; render(); }));
		return;
	}
	const mine = ++rendering;
	const old = app.querySelector('main');
	const scroll = quiet && old ? old.scrollTop : 0;
	let page;
	let ctx;
	try {
		ctx = await context();
		const match = routes.map(([re, fn]) => [r.match(re), fn]).find(([m]) => m);
		page = match ? await match[1](ctx, match[0]) : { main: [h('div', { class: 'error' }, 'There is no page at ', r)] };
	} catch (e) {
		if (e instanceof APIError && e.status === 401) {
			signOut();
			location.hash = '#/signin';
			return;
		}
		page = { main: [h('div', { class: 'error' }, e.message || String(e))] };
	}
	if (mine !== rendering || !page) return; // a newer render took over
	const main = h('main', null, page.main);
	app.replaceChildren(
		header(ctx, r),
		h('div', { class: 'body' }, page.aside || null, main),
	);
	main.scrollTop = scroll;
	if (page.refresh) timer = setTimeout(() => render(true), page.refresh * 1000);
}

function header(ctx, r) {
	return h('header', { class: 'top' },
		h('div', { class: 'brand' }, h('b', null, 'AEOLUS'), h('span', null, ctx?.org || '')),
		h('nav', { class: 'tabs', 'aria-label': 'Sections' },
			TABS.map(([path, label]) => h('a', { href: '#' + path, class: r.startsWith(path) ? 'on' : null }, label))),
		h('div', { class: 'spacer' }),
		ctx && h('div', { class: 'who' },
			h('span', null, ctx.who.name || ctx.who.account),
			h('button', { type: 'button', onclick: () => { signOut(); location.hash = '#/signin'; render(); } }, 'Sign out')),
	);
}

window.addEventListener('hashchange', () => render());
render();
