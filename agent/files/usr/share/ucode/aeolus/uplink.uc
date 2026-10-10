// The AP's uplink as it runs (0096): its ports, which of them the AP's
// traffic takes, and the Ethernet ports under each. The renderer names the
// uplink, aeolus.agent.uplink. Where Aeolus took the uplink's bond apart,
// the bond's members are the uplink's ports, the first the primary, as
// aeolus_unbond keeps them, and the path out is whichever of them the
// bridge forwards on: a cable moved from eth0 to eth1 still reaches the
// manager.

'use strict';

import * as fs from 'fs';

function read(path) {
	let v = fs.readfile(path);
	return v == null ? null : trim(v);
}

function list(v) {
	return v == null ? [] : (type(v) == 'array' ? v : [v]);
}

const NAME = /^[A-Za-z0-9._-]{1,15}$/;

// ports is the uplink's ports, the primary first, as the UCI cursor c has
// them.
function ports(c) {
	let kept = filter(list(c.get('aeolus', 'aeolus_unbond', 'ports')), p => match(p, NAME));
	if (length(kept))
		return kept;
	let up = c.get('aeolus', 'agent', 'uplink');
	return up && match(up, NAME) ? [up] : [];
}

// path_out is the one of ports the AP's traffic takes now: the first with a
// link that its bridge forwards on, which spanning tree chose; else the first
// with a link; else the first.
function path_out(ports) {
	let linked = filter(ports, p => read(`/sys/class/net/${p}/carrier`) == '1');
	for (let p in linked) {
		let state = read(`/sys/class/net/${p}/brport/state`);
		if (state == null || state == '3')
			return p;
	}
	return linked[0] ?? ports[0];
}

// physical is the Ethernet ports under a port: a bond's members, else the
// port itself. Each hears its own switch port's LLDP.
function physical(port) {
	let slaves = read(`/sys/class/net/${port}/bonding/slaves`);
	if (slaves == null)
		return [port];
	return filter(split(slaves, /[ \t]+/), s => match(s, NAME));
}

export { ports, path_out, physical };
