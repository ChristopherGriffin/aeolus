// When the page redraws itself. Pages that show live AP state redraw every
// 30 s; while a person is editing, they do not, so nothing they typed is
// lost; and just after a change, every page redraws every few seconds, so
// the tree's dots show the APs picking it up. Anything open for editing (a form, a preview) carries a
// data-editing attribute: while one is on the page, it is not redrawn.

let redraw = null;
let fastUntil = 0;

// setRedraw is called once by the app with its render function.
export function setRedraw(fn) {
	redraw = fn;
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

export function isEditing() {
	return document.querySelector('[data-editing]') !== null;
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
