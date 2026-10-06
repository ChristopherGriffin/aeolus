// When the page redraws itself. Pages that show live AP state redraw every
// 30 s; while a person is editing, they do not, so nothing they typed is
// lost; and just after a change, every page redraws every few seconds, so
// the tree's dots show the APs picking it up. Anything open for editing (a
// form, a preview) carries a data-editing attribute: while one is on the
// page, it is not redrawn; nor while someone is filling a field in.

let redraw = null;
let fastUntil = 0;

// setRedraw is called once by the app with its render function. It starts
// watching for fields people fill in, too.
export function setRedraw(fn) {
	redraw = fn;
	document.addEventListener('input', (e) => {
		if (e.target?.matches?.('input, textarea, select')) touched.add(e.target);
	}, true);
}

// redrawNow redraws the current page, keeping its scroll position.
export function redrawNow() {
	redraw?.(true);
}

let note = null;

// flash shows a message at the top of the page for a while, across redraws.
export function flash(text, seconds = 90) {
	note = { text, until: Date.now() + seconds * 1000 };
}

export function currentFlash() {
	return note && Date.now() < note.until ? note.text : null;
}

// The fields someone has typed in or picked from, since only a person's
// input fires an input event (setRedraw watches for them). A field counts
// while it still holds what they gave it: once cleared, as after a key is
// added, the page may redraw again.
const touched = new Set();

// FIELDS are the kinds of input a person types or picks in.
const FIELDS = 'textarea, select, input:not([type=button]):not([type=submit]):not([type=reset]):not([type=checkbox]):not([type=radio]):not([type=hidden])';

// typing says whether someone is in the middle of filling a field in: one
// has the focus, or holds what they typed or picked (Griff, 2026-10-06: a
// key half typed was lost to a redraw every few seconds).
function typing() {
	if (document.activeElement?.matches?.(FIELDS)) return true;
	for (const el of touched) {
		if (!el.isConnected) {
			touched.delete(el);
			continue;
		}
		if (el.tagName === 'SELECT') {
			const first = [...el.options].findIndex((o) => o.defaultSelected);
			if (el.selectedIndex !== Math.max(first, 0)) return true;
		} else if (el.value !== '' && el.value !== el.defaultValue) return true;
	}
	return false;
}

// isEditing says whether redrawing now would lose someone's work: a form or
// preview open for editing, or a field they are filling in.
export function isEditing() {
	return document.querySelector('[data-editing]') !== null || typing();
}

// hurry redraws often for a while, to watch a change land.
export function hurry(seconds) {
	fastUntil = Date.now() + seconds * 1000;
}

// interval is how long to wait before the next redraw, given the page's own:
// a few seconds just after a change, whatever the page.
export function interval(page) {
	if (Date.now() < fastUntil) return 5;
	return page || 0;
}
