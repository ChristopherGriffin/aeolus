# 0061. Switching a network between its transports

- Status: Proposed
- Date: 2026-10-03
- Proposed by: Griff: step 2 of 0059, a network moves to its fallback when its primary fails, when set to, and only to one known to work; written up by Claude
- Refines: 0018, 0020, 0022, 0054, 0059, 0060

## Context

- **Step 1 of 0059 tells whether a tunnel works.** Each AP probes its tunnels, and its verdicts (`up`, `down`, `unverified`) reach the manager within a poll.
- **Nothing acts on them yet:**
  - a network's fallback is rendered, but never used;
  - a VXLAN fallback's tunnel is never started (0054);
  - a VLAN fallback isn't probed.
- **0059 set the rules:**
  - each network chooses `report` (the default) or `automatic`;
  - the AP switches only to a transport that answers;
  - one transport at a time, break before make;
  - switching back follows 0022's failback and hold-down;
  - the switch doesn't restart the Wi-Fi.

  This record says how.

## Decision

### What a network sets

- **`transport.switching`:** `report` or `automatic`; unset, it is `report`. `report` probes and tells, and never moves the network. `automatic` also switches, under the rules below.
- **0022's fields apply as written:**
  - `transport.ha`: keep the standby started and probed, so a switch is a re-attach;
  - `transport.failback`: `revertive`, the default, or `equal`;
  - `transport.holddown`: seconds the primary must answer before a revertive switch back, default 300.
- **The config check holds** (0029):
  - `automatic` without a fallback;
  - a switching network whose tunnel's VNI another network or a tunnel port uses at the AP, since the tunnel will be taken in and out of the network.

### What the AP probes

- **Both transports of a network with a fallback:**
  - **A VXLAN transport** as in 0059 and 0060.
  - **A VLAN transport:**
    - sent tagged straight on the uplink, so no local port or Wi-Fi client sees it;
    - from the VLAN's own MAC (`06:…`, 0060), with a lease of its own;
    - asking the gateway that lease names, or the probe address set.
- **Without HA mode, a VXLAN standby stays stopped** until the primary goes down. Then the AP starts it, probes it, and uses it only once it answers.

### How a switch is made

- **A network with a fallback gets a bridge of its own,** and its SSIDs stay on it whichever transport carries it:
  - its name is `br-n` and 8 hex digits made from the network's name, inside Linux's 15 characters;
  - the network's interface is on that bridge.
- **Neither transport is in that bridge in the rendered config.** The prober attaches the active one at run time, through netifd: `add_device` and `remove_device` on the network's interface.
  - **A VXLAN transport is attached as its tunnel device itself,** which is that network's alone at the AP.
  - **A VLAN transport is attached through a veth pair:**
    - one end is a port of the uplink bridge, untagged in that VLAN;
    - the other end is what joins the network's bridge;
    - the installer adds `kmod-veth`.
- **The prober attaches the primary when it starts,** and attaches the active transport again after any network reload.
  - The render check holds that neither transport is in the bridge's configured ports, so a reload can never leave both attached.
  - Which transport is active is state, not config (0020).
- **Break before make** (0018): the failed transport comes out of the bridge before the other goes in. A VLAN 50 primary and a VNI 50 fallback are one broadcast domain, so both at once would be the loop 0018 warns of.
- **Clients stay associated through a switch.** Their traffic moves with the bridge's member. On a fallback that reaches the same segment, a client keeps its address.

### When

- **Away from the primary:**
  - when its verdict turns `down` (three probes missed in a row, 0059);
  - only to a fallback whose verdict is `up`;
  - with HA mode, the fallback has been probed all along;
  - without it, the fallback is started and probed first. The network stays on the failed primary, down, until the fallback answers, or the primary comes back.
- **Back,** with `revertive`, once the primary has answered for `holddown` seconds without a miss. With `equal`, the network stays put until the fallback fails in turn.
- **Never to a transport that is `down` or `unverified`.** The AP reports that it could not switch, and why.
- **A switch is logged** on the AP and reported: from which transport to which, why, and when.

### What the manager shows

- **Each network's transports, on each AP:**
  - which transport is active;
  - each transport's verdict;
  - the last switch and its reason.

  This uses the state report's `transports`, which gains `unverified` and the last switch.
- **The Networks and Tunnels views say so.** A network that has switched to its fallback is a warning on the AP's overview.
- **The network editor offers switching beside the transports.** Choosing `automatic` shows HA mode, failback and hold-down.

## Consequences

- **A network with a fallback is rendered differently** (a bridge of its own), so applying this reloads the network once and restarts that network's Wi-Fi once on each AP, after which switching never does.
- **Each switching network costs an AP a bridge, and a veth pair for a VLAN transport,** and its fallback a DHCP lease (0060).
- **Report mode costs nothing new** but the VLAN fallback's probe and lease.
- **0058's rule that a port can't carry a network's fallback VNI stays.**

## Open

- **Three things to check on OpenWrtnight before it's built:**
  - **Hot-plugged members across a reload:** does netifd keep a device added with `add_device` through a network reload? If not, the prober re-attaches on netifd's reload event.
  - **VLAN probes on the uplink:** can a packet socket on the uplink port send a tagged probe, and see the tagged answer with its VLAN? The tag may come in the packet's metadata rather than its bytes.
  - **veth pairs:** do `kmod-veth` and netifd's `veth` device make the pair, and does its uplink end join the uplink bridge's VLAN?
