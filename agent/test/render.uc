// Renders one test case and prints the result as `uci export` text, for the
// manager's contract test (internal/rendercheck, 0040).
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/render.uc agent/test/cases/sandbox.json

'use strict';

import * as fs from 'fs';
import { render, PACKAGES } from 'aeolus.render';
import { text } from 'aeolus.uciexport';

let path = ARGV[0];
let c = json(fs.readfile(path));
let dir = match(path, /^(.*)\/cases\//)?.[1] ?? '.';
let current = json(fs.readfile(dir + '/' + c.current));

// A leftover the renderer must remove.
for (let name in c.stale ?? [])
	current.wireless[name] = { '.anonymous': false, '.type': 'wifi-iface', '.name': name, '.index': 99, device: 'radio0', mode: 'ap', ssid: 'Gone' };

let out = render(c.intent, current, c.facts);
for (let e in out.errors)
	warn('render: ' + e + '\n');
print(join('\n', map(PACKAGES, p => text(p, out.config[p]))) + '\n');
