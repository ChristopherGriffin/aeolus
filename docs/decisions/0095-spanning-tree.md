# 0095. Spanning tree on the AP's bridge

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff. APs should act like switches, with a media converter attached; spanning tree could run all the time, the bond or not, with an off/on for the rare case. Written up by Claude
- Refines: 0053, 0092, 0093

## Context

- **Unbonding the C-360's ports puts eth0 and eth1 in one bridge.** Two cables to the same switch would loop it, unless something blocks one of them. A switch does that with spanning tree.
- **netifd runs a bridge's spanning tree on `stp '1'`.** It hands it to ustp, the RSTP daemon, over ubus, unless `stp_kernel` is set; the kernel's `bridge-stp` helper falls back to the kernel's own STP where ustp is not there (netifd `bridge.c`; ustp's `bridge-stp`, 2026-10-09).
- **The kernel's own STP is 802.1D, with no edge ports.** Every port, each Wi-Fi network's included, waits twice the forward delay before it forwards, every time it comes up. RSTP's edge ports forward at once, and ustp makes a port that hears no BPDUs an edge port by itself (AutoEdge).
- **ustp is in OpenWrt's feed, not in its images.**
- **ustp gives every bridge the default priority, 32768.** It does not read netifd's `priority`, which sets only the kernel's, unused with ustp. The AP cannot be made a bridge that never wins the root.
- **A switch port with BPDU guard shuts itself** when the AP's first BPDU comes. The AP is then cut off until someone opens the port again, and no revert on the AP brings it back.

## Decision

- **`uplink.stp`, true or false,** turns spanning tree on or off on the bridge the AP's uplink is in. Set per kind of AP it is `boards.<board>.uplink.stp` (0092), folded as the ports are. It is a template field (0085). Unset leaves the bridge as it is.
- **On, the agent renders `stp '1'` and `stp_proto 'rstp'`** on the uplink's bridge device; off, it takes those two away. A bridge priority or timers the AP has set itself stay.
- **Without ustp, the agent installs it,** through the manager's feed cache (0069), with `aeolus-packages add ustp`:
  - in the background, tried again hourly while it will not install;
  - kept on the AP's package list, so a sysupgrade brings it back;
  - ustpd enabled and started, and the agent started again, to render with it.
- **Until ustp is in, the renderer leaves the bridge as it is and says why.** The render check wants `stp` and `stp_proto` as set, so the config is refused, and the AP keeps what it runs. The kernel's STP, with no edge ports, is never what the AP runs.
- **The state report says each port's state** where the bridge runs spanning tree: `forwarding`, `blocking`, `learning`, `listening` or `disabled`, from the bridge's `brif/<port>/state`.
- **On Ethernet, each kind of AP has a Spanning tree line** under its jacks: On, RSTP; Off; or not set, and where it comes from, with Edit.
  - Its form is On or Off, with Follow for one set here.
  - Turning it on warns of BPDU guard, and that the AP installs ustp first. It also says the core switch should have the lower priority.
  - On an AP, the line also says what the AP runs: every port forwarding, or which are blocked. A folder, as a template, says nothing of its APs' state.
  - A blocked port's jack is amber, with ⊘ and "blocked" under it, and its details say so.

## Consequences

- With the core switch at the default priority, the root is whichever bridge has the lowest MAC, which may be an AP. That is a loop-free tree still, but a poor one. A network's core should have its priority set lower anyway.
- The bond goes on working with spanning tree on: bond0 is one port of the bridge.
- A config that asks for spanning tree waits on an AP whose feed cannot give it ustp, its other changes with it, until ustp installs.
- Stage 2, two separate ports in place of the C-360's bond, builds on this. The uplink label then follows the forwarding port.
