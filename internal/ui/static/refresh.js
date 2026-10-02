// When the page redraws itself. Pages that show live AP state redraw every
// 30 s; while a person is editing, they do not, so nothing they typed is
// lost; and just after a change, they redraw every few seconds to show the
// AP picking it up.

let redraw = null;
let editing = 0;
let fastUntil = 0;

// setRedraw is called once by the app with its render function.
export function setRedraw(fn) {
	redraw = fn;
}

// redrawNow redraws the current page, keeping its scroll position.
export function redrawNow() {
	redraw?.(true);
}

// stopAllEditing forgets any editing, when the person leaves the page.
export function stopAllEditing() {
	editing = 0;
}

let note = null;

// flash shows a message at the top of the page for a while, across redraws.
export function flash(text, seconds = 90) {
	note = { text, until: Date.now() + seconds * 1000 };
}

export function currentFlash() {
	return note && Date.now() < note.until ? note.text : null;
}

export function startEditing() {
	editing++;
}

export function stopEditing() {
	editing = Math.max(0, editing - 1);
}

export function isEditing() {
	return editing > 0;
}

// hurry redraws often for a while, to watch a change land.
export function hurry(seconds) {
	fastUntil = Date.now() + seconds * 1000;
}

// interval is how long to wait before the next redraw, given the page's own.
export function interval(page) {
	if (!page) return 0;
	if (Date.now() < fastUntil) return 5;
	return page;
}
