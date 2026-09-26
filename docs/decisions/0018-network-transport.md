# 0018. Network transport: primary and fallback

- Status: Accepted (open points below)
- Date: 2026-09-25
- Decided by: Griff
- Resolves: the open question in 0015 (where the VXLAN drop-down lives)

## Decision

- How a network's traffic gets to where it is going is part of the network's config in the Services tree.
- Each network has a **primary** transport and an optional **fallback** transport.
- Each of them is either a **VLAN** (a VLAN ID) or **VXLAN** (a tunnel to a concentrator). Any combination is allowed: VLAN then VXLAN, VXLAN then VLAN, or the same type twice.
- The concentrator maps VNI to VLAN. The manager configures APs only; it never configures the concentrator.
- A VXLAN transport consists of the concentrator's routable address, the VNI, the UDP port (default 4789) and the MTU, from which the TCP MSS clamp is derived. The tunnel's local end is the AP's own management address, which comes from the Locations tree.

## Consequences

- A network never has both of its transports active on the same AP at once. A local VLAN and a VNI that lands in the same VLAN at the concentrator form one broadcast domain, so running both would make a bridging loop.

## Open

- When the fallback applies: chosen per AP by the manager from the AP's port config (config time), or switched by the AP when the primary fails (run time), or both.
- Library shape: the library holds concentrators (address, UDP port, MTU) and each network sets its own VNI, or each library entry is a full VXLAN config including the VNI.
