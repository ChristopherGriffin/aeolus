// Renders one test case and prints the result as `uci export` text, for the
// manager's contract test (internal/rendercheck, 0040). With "clamp" after
// the case, it prints the MSS clamp's file made from it instead (0054).
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/render.uc agent/test/cases/sandbox.json

'use strict';

import * as fs from 'fs';
import { render, clamp, PACKAGES } from 'aeolus.render';
import { text } from 'aeolus.uciexport';

let path = ARGV[0];
let c = json(fs.readfile(path));
let dir = match(path, /^(.*)\/cases\//)?.[1] ?? '.';
let current = json(fs.readfile(dir + '/' + c.current));

// A leftover the renderer must remove.
for (let name in c.stale ?? [])
	current.wireless[name] = { '.anonymous': false, '.type': 'wifi-iface', '.name': name, '.index': 99, device: 'radio0', mode: 'ap', ssid: 'Gone' };

// Sections the AP has beyond the fixture, such as ones Aeolus made earlier.
for (let pkg in c.extra ?? {})
	for (let name in c.extra[pkg])
		current[pkg][name] = c.extra[pkg][name];

let out = render(c.intent, current, c.facts);
for (let e in out.errors)
	warn('render: ' + e + '\n');
if (ARGV[1] == 'clamp')
	print(clamp(out.config.network));
else
	print(join('\n', map(PACKAGES, p => text(p, out.config[p]))));
