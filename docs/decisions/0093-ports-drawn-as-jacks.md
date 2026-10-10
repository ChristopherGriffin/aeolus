# 0093. Ports drawn as jacks, bonds broken out, and what the far end offers

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff, on 0092's Ethernet. bond0 is two 10 GbE ports, on 2.5 GbE switch ports, and should be shown broken out. Ports should look like Ethernet ports rather than cards: on a folder, white configured and grey unconfigured or shut down; on an AP, green active, white configured but inactive, grey unconfigured or shut down (red, later, for a locked MAC). An AP's Ethernet should say what is known of its PoE. Written up by Claude
- Refines: 0053, 0064, 0092

## Context

- **The C-360 reported one port, bond0 (5000F),** the uplink its image bonds of eth0 and eth1 (LACP, 802.3ad, aggregator 1).
- **netifd knows more than that.** Each member is up at 2500F. Its `link-supported` reaches 10000baseT, and the switch's `link-partner-advertising` stops at 2500baseT: a 10 GbE port on a 2.5 GbE switch port (2026-10-09).
- **The switch's LLDP already says it powers the AP:** class 4, 40 W allocated and asked, on the signal pair. That is in each report (0064).

## Decision

- **The agent reports more of each port:**
  - `max`, the fastest it can go, and `partner_max`, the fastest the far end offers it, in Mbit/s, from netifd's link modes;
  - for a bond, `bond`: its mode, the aggregator in use, and each member, with its own link, `max` and `partner_max`, MII status and aggregator.

  The manager takes and checks these. It must be deployed before the agents send them, since it refuses fields it doesn't know. A release does that: the manager updates first, then the APs take the bundle from it.
- **Ethernet draws each port as an RJ45 jack,** its name and link under it:
  - on a folder: white where set (the uplink and a bond's members included), grey where not set, or off;
  - on an AP: green with a link, white where set but without one, grey where not set, or off.

  ↑ marks the uplink, ⚡ PoE where the switch says it powers the AP, and ! a loop.
- **A bond is its members' jacks together,** under its name, its mode (LACP) and its speed. Its details list each member as its port and the speed it synced at, "10GbE port/2.5G sync" (Griff, 2026-10-09), with what the far end offers on hover, and mark a member outside the aggregate.
- **A jack selects its port.** Its details open under the jacks: link, as port and sync where the AP says how fast the port can go, mode, what it carries and where from, and on an AP's uplink the switch's power ("powered by homelab.symtus.com Ethernet13 · class 4 · 40 W allocated · 40 W asked · signal pair"), with Edit, or Info on the uplink. The last jack, "+", adds a port.
- **The jacks are built as SVG elements:** the UI never sets `innerHTML`.

## Consequences

- An AP that runs an older agent shows its ports as before, without members or what the far end offers.
- Red, for a port locked to a MAC, is left for when Aeolus does that.
