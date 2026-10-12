// What Aeolus's steering asks of clients and what comes of it (0138):
// TestAgentSteer runs this with the real ucode and compares what it prints
// with steer.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/steer.uc

'use strict';

import * as steer from 'aeolus.steer';

// An AP with the network test on all three bands and lab on two, as the
// daemon sees them: each BSS's network, band and own neighbour report, and
// the networks of its name on other APs.
const NR = { g2: '36862d0317d0ef4900005106070603000600', g5: '36862d0317c0ef590000742c090603012e00', g6: '30862d0317e0ef40000086c5040603030000', far: '2005b6018be9ef19000074950906030199' + '00' };
function ap() {
	return {
		bss: {
			'phy1-ap2': { network: 'test', band: '2g', up: true, nr: NR.g2, elsewhere: [] },
			'phy0-ap2': { network: 'test', band: '5g', up: true, nr: NR.g5, elsewhere: [] },
			'phy3-ap0': { network: 'test', band: '6g', up: true, nr: NR.g6, elsewhere: [] },
			'phy1-ap1': { network: 'lab', band: '2g', up: true, nr: NR.g2, elsewhere: [NR.far] },
			'phy0-ap1': { network: 'lab', band: '5g', up: true, nr: NR.g5, elsewhere: [NR.far] },
		},
		clients: { 'phy1-ap2': {}, 'phy0-ap2': {}, 'phy3-ap0': {}, 'phy1-ap1': {}, 'phy0-ap1': {} },
	};
}

const ALL = { networks: { test: { suggest: true, hold: true, sticky: true, weak: -75 }, lab: { suggest: false, hold: false, sticky: true, weak: -75 } } };
const WALL = 1791770000;

let st, view, cfg, t;

function begin(what, c) {
	printf('\n== %s\n', what);
	st = steer.fresh();
	view = ap();
	cfg = c ?? ALL;
	t = 0;
}

// show says what the daemon is to ask, and takes it as asked.
function show(acts) {
	for (let a in acts) {
		steer.sent(st, a);
		let more = '';
		if (a.do == 'btm')
			more = sprintf(' %s%s to %s', a.imminent ? 'it is to go, ' : '', a.abridged ? 'these only' : 'these or others',
				join(' ', map(a.neighbors, n => substr(n, 0, 12) + '..' + substr(n, -6))));
		else
			more = sprintf(' for %d ms', a.ms);
		printf('%4d  %s %s on %s%s\n', t, a.do, a.mac, a.bss, more);
	}
}

// put has a client on a BSS, as hostapd lists it; off takes it away.
function put(mac, ifname, signal, c) {
	for (let name in keys(view.clients))
		delete view.clients[name][mac];
	view.clients[ifname][mac] = { signal: signal, btm: true, ...(c ?? {}) };
}

function off(mac) {
	for (let name in keys(view.clients))
		delete view.clients[name][mac];
}

// look is the daemon's look at its clients, every five seconds; pass lets
// that many seconds go by, looking as it goes.
function look() {
	show(steer.tick(st, cfg, view, t, WALL + t));
}

function pass(seconds) {
	for (let end = t + seconds; t < end;) {
		t += 5;
		look();
	}
}

function hear(ifname, type, data) {
	show(steer.heard(st, cfg, view, t, ifname, type, data));
}

function records() {
	for (let e in st.events)
		printf('      %s\n', sprintf('%J', e));
	st.events = [];
}

const A = '02:00:00:00:00:0a', B = '02:00:00:00:00:0b', C = '02:00:00:00:00:0c', D = '02:00:00:00:00:0d';

begin('Suggest: a client near enough, that says it can use 5 GHz, is asked up once it has been on ten seconds, and moves');
put(A, 'phy1-ap2', -50, { bands: ['2g', '5g'] });
look();
pass(10);
hear('phy1-ap2', 'bss-transition-response', { address: A, 'status-code': 0 });
t += 2;
put(A, 'phy0-ap2', -58);
hear('phy0-ap2', 'sta-authorized', { address: A });
records();
print('  On 5 GHz it is not asked on to 6 GHz, which it never said it can use:\n');
pass(60);
records();

begin('Suggest: one that says no is asked twice more, further apart, and then left alone');
put(B, 'phy1-ap2', -48, { bands: ['2g', '5g'] });
look();
for (let i = 0; i < 4; i++) {
	let before = length(st.events);
	for (let n = 0; n < 400 && !st.tries[B]; n++)
		pass(5);
	if (!st.tries[B]) {
		print('  and no more, in the half hour after\n');
		break;
	}
	hear('phy1-ap2', 'bss-transition-response', { address: B, 'status-code': 6 });
}
records();

begin('Suggest: one that says nothing and stays is counted as having stayed; one that is far, one that takes no request, and one never seen higher are not asked');
put(A, 'phy1-ap2', -52, { bands: ['2g', '5g'] });
put(B, 'phy1-ap2', -72, { bands: ['2g', '5g'] });
view.clients['phy1-ap2'][C] = { signal: -45, btm: false, bands: ['2g', '5g'] };
view.clients['phy1-ap2'][D] = { signal: -45, btm: true };
look();
pass(40);
records();

begin('Suggest: on 5 GHz, near, and able to use 6 GHz, a client is asked to 6 GHz; on 2.4 GHz and very near, to 6 GHz before 5');
put(A, 'phy0-ap2', -55, { bands: ['2g', '5g', '6g'] });
view.clients['phy1-ap2'][B] = { signal: -40, btm: true, bands: ['2g', '5g', '6g'] };
look();
pass(10);
off(A);
t += 1;
put(A, 'phy3-ap0', -62);
hear('phy3-ap0', 'sta-authorized', { address: A });
pass(20);
records();

begin('Hold: a client that has been on 5 GHz here is kept off 2.4 GHz for a moment when it comes to join, and joins 5 GHz');
put(A, 'phy0-ap2', -60);
look();
off(A);
pass(60);
hear('phy1-ap2', 'auth', { address: A, signal: -52 });
t += 1;
put(A, 'phy0-ap2', -60);
hear('phy0-ap2', 'sta-authorized', { address: A });
records();
print('  It leaves; later it joins 2.4 GHz all the same, once the moment is over, and its next try is not held:\n');
off(A);
pass(400);
hear('phy1-ap2', 'auth', { address: A, signal: -52 });
t += 4;
hear('phy1-ap2', 'auth', { address: A, signal: -52 });
put(A, 'phy1-ap2', -52);
hear('phy1-ap2', 'sta-authorized', { address: A });
records();

begin('Hold: not one that is far, one never seen higher, or one that is on this AP already and comes down; not while the higher band is not up; and a probe that is only a look around leaves no record');
put(A, 'phy0-ap2', -60);
put(B, 'phy0-ap2', -60);
view.clients['phy0-ap2'][B] = { signal: -60, btm: true };
look();
off(A);
pass(60);
hear('phy1-ap2', 'auth', { address: A, signal: -80 });
// 5 GHz is not up, as while its radio listens for radar.
view.bss['phy0-ap2'].up = false;
hear('phy1-ap2', 'auth', { address: A, signal: -40 });
view.bss['phy0-ap2'].up = true;
hear('phy1-ap2', 'auth', { address: C, signal: -40 });
hear('phy1-ap2', 'auth', { address: B, signal: -40 });
hear('phy1-ap2', 'probe', { address: A, signal: -50 });
pass(30);
printf('  records: %d\n', length(st.events));
print('  A probe, and then it joins 5 GHz: that is a record:\n');
pass(30);
off(A);
look();
hear('phy1-ap2', 'probe', { address: A, signal: -50 });
t += 2;
put(A, 'phy0-ap2', -60);
hear('phy0-ap2', 'sta-authorized', { address: A });
records();

begin('Hold: one the daemon got to late, the client let on before hostapd was asked to keep it off, is no hold and no record; a network with only Hold back on needs no neighbour report of the higher band');
cfg = { networks: { test: { suggest: false, hold: true, sticky: false, weak: -75 } } };
for (let name in keys(view.bss))
	view.bss[name].nr = null;
put(A, 'phy0-ap2', -60);
look();
off(A);
pass(60);
// What hostapd said of its authentication and of its being let on are
// both taken before anything is asked: the daemon then asks nothing.
let late = steer.heard(st, cfg, view, t, 'phy1-ap2', 'auth', { address: A, signal: -50 });
put(A, 'phy1-ap2', -50);
steer.heard(st, cfg, view, t + 0.3, 'phy1-ap2', 'sta-authorized', { address: A });
printf('  decided: %s; then let on before it was asked: records %d, counted %J\n', join(', ', map(late, a => a.do)), length(st.events), st.counts);
off(A);
pass(60);
hear('phy1-ap2', 'auth', { address: A, signal: -50 });
t += 1;
put(A, 'phy0-ap2', -60);
hear('phy0-ap2', 'sta-authorized', { address: A });
records();

begin('A client moved up that is back down within five minutes did not stay moved: said, and it is left alone for half an hour');
put(A, 'phy1-ap2', -50, { bands: ['2g', '5g'] });
look();
pass(10);
t += 2;
put(A, 'phy0-ap2', -70);
hear('phy0-ap2', 'sta-authorized', { address: A });
pass(30);
put(A, 'phy1-ap2', -50, { bands: ['2g', '5g'] });
hear('phy1-ap2', 'sta-authorized', { address: A });
pass(600);
records();

begin('Sticky: a client weak on 5 GHz for three looks is told to go, to 2.4 GHz here first, and goes; there, though near enough, it is not asked back up');
put(A, 'phy0-ap2', -80, { bands: ['2g', '5g'] });
look();
pass(15);
t += 3;
put(A, 'phy1-ap2', -60, { bands: ['2g', '5g'] });
hear('phy1-ap2', 'sta-authorized', { address: A });
pass(900);
records();

begin('Sticky: one that takes no request is put off at once where another AP has the network; back within the minute, it is left alone, and twice as long each time it comes back again');
view.clients['phy1-ap1'][B] = { signal: -82, btm: false };
look();
// A day and a half: put off at the start, after half an hour, an hour
// after that, then two, four, eight and sixteen hours.
for (let end = 36 * 3600; t < end;) {
	let was = length(st.events);
	pass(5);
	if (st.tries[B]?.asked == 'kick' && !st.on[B] && view.clients['phy1-ap1'][B]) {
		hear('phy1-ap1', 'deauth', { address: B });
		off(B);
		pass(5);
		view.clients['phy1-ap1'][B] = { signal: -82, btm: false };
		hear('phy1-ap1', 'sta-authorized', { address: B });
	}
}
printf('  put off %d times in a day and a half, at: %s\n', length(st.events), join(' ', map(st.events, e => sprintf('%dh%02d', (e.at - WALL) / 3600, ((e.at - WALL) % 3600) / 60))));
st.events = slice(st.events, 0, 1);
records();

begin('Sticky: one told to go that stays is put off after eight seconds, and is gone; one whose signal comes back is left; one with nowhere better to go is never asked');
put(A, 'phy0-ap1', -80);
view.clients['phy0-ap1'][C] = { signal: -80, btm: true };
view.clients['phy1-ap2'][D] = { signal: -85, btm: true };
look();
pass(15);
hear('phy0-ap1', 'bss-transition-response', { address: A, 'status-code': 1 });
view.clients['phy0-ap1'][C].signal = -60;
pass(5);
hear('phy0-ap1', 'deauth', { address: A });
off(A);
pass(70);
records();

begin('Sticky: one whose signal is not known when its time is up is not put off; one that has gone to another of the AP\'s networks has left this one');
put(A, 'phy0-ap1', -80);
view.clients['phy0-ap1'][B] = { signal: -80, btm: true };
look();
pass(10);
view.clients['phy0-ap1'][A].signal = null;
delete view.clients['phy0-ap1'][B];
view.clients['phy0-ap2'][B] = { signal: -50, btm: true };
hear('phy0-ap2', 'sta-authorized', { address: B });
pass(15);
records();

begin('Suggest and Sticky together: near enough on 2.4 GHz to be asked up, and weak on 5 GHz once there, a client is not sent up and down every half hour');
put(A, 'phy1-ap2', -64, { bands: ['2g', '5g'] });
let asked = [];
for (let end = 48 * 3600; t < end;) {
	t += 5;
	for (let a in steer.tick(st, cfg, view, t, WALL + t)) {
		if (a.do != 'btm' || a.mac != A)
			continue;
		// It does as it is asked, two seconds on.
		let up = a.bss == 'phy1-ap2';
		push(asked, sprintf('%s %dh%02d', up ? 'up' : 'down', t / 3600, (t % 3600) / 60));
		t += 2;
		put(A, up ? 'phy0-ap2' : 'phy1-ap2', up ? -78 : -64, { bands: ['2g', '5g'] });
		steer.heard(st, cfg, view, t, up ? 'phy0-ap2' : 'phy1-ap2', 'sta-authorized', { address: A });
	}
}
printf('  in two days it is asked: %s\n', join(', ', asked));
st.events = [];

begin('A network with nothing on is not looked at; one with only Sticky on is not asked up');
put(A, 'phy1-ap2', -50, { bands: ['2g', '5g'] });
put(B, 'phy1-ap1', -50, { bands: ['2g', '5g'] });
cfg = { networks: { lab: { suggest: false, hold: false, sticky: true, weak: -75 } } };
look();
pass(60);
hear('phy1-ap2', 'auth', { address: C, signal: -40 });
printf('  records: %d, on: %J\n', length(st.events), sort(keys(st.on)));

begin('What the agent reports');
put(A, 'phy1-ap2', -50, { bands: ['2g', '5g'] });
look();
pass(10);
t += 2;
put(A, 'phy0-ap2', -58);
hear('phy0-ap2', 'sta-authorized', { address: A });
printf('%.J\n', steer.summary(st, cfg, view, WALL + t, WALL));

print('\n== What a client says of itself\n');
// Element 59, Supported Operating Classes, among others: the current class
// and those it can use.
function ies(...els) {
	let s = '';
	for (let e in els)
		s += chr(e[0], length(e[1]), ...e[1]);
	return s;
}
printf('2.4 and 5 GHz: %J\n', steer.bands_of(ies([0, [116, 101]], [59, [81, 81, 83, 84, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130]], [127, [0, 0, 8]])));
printf('all three: %J\n', steer.bands_of(ies([59, [131, 81, 115, 131, 132, 133, 134]])));
printf('says nothing: %J\n', steer.bands_of(ies([0, [116]], [127, [0, 0, 8]])));
printf('cut short: %J\n', steer.bands_of(substr(ies([59, [81, 115, 116]]), 0, 4)));
printf('takes a transition request: %J %J %J\n', steer.btm_of([4, 0, 8, 0]), steer.btm_of([4, 0, 0, 0]), steer.btm_of(null));
