# 0056. A tunnel's MTU: default or custom, and only what the uplink carries

- Status: Proposed
- Date: 2026-10-03
- Proposed by: Griff: a tunnel's MTU is the default or custom, and a higher one is allowed only if the AP's port carries it now, or else Aeolus says it cannot; written up by Claude
- Refines: 0018, 0054, 0055

## Context

VXLAN adds 50 bytes to every frame over IPv4, and 70 over IPv6. On a path that carries 1500-byte packets, a tunnel can carry frames of at most 1450 bytes, so the AP clamps TCP to fit (0054).

A path that carries jumbo frames can take a tunnel of 1500 or more, so clients' full-size frames pass whole. But the AP's own uplink must carry those packets too. VXLAN endpoints seldom put fragments back together, so a packet too big for the uplink is lost.

## Decision

- **A tunnel's MTU is the default or custom.**
  - **Left unset, it is the default:** 1450 when the far end's address is IPv4, and 1430 when it is IPv6. That is what a 1500-byte path carries.
  - **Set, it is custom,** and is used as set. At 1500 or more, the AP does not clamp TCP (0054).
  - The AP's config always carries the MTU in force, so the AP and the render check are unchanged.
- **A tunnel's MTU must fit the AP's uplink as it is now.** Its packets are the MTU plus 50 bytes (70 over IPv6), and the uplink must carry them.
  - **What the uplink carries:** the agent reports the MTU of the device it reaches the manager through, in each state report.
  - **Too big is held at once (0029).** A tunnel the uplink cannot carry is held, by what the AP last reported, with a reason such as "tunnel arista's MTU of 1500 needs 1550 on the AP's uplink, which carries 1500 now". That shows when the change is previewed, on the AP's page, and in the AP's poll, which keeps what it runs.
  - **The render check holds it too,** from the uplink's MTU as the AP's rendered config sets it: the interface's own `mtu`, else its device's, else the bridge it is a VLAN on, else 1500. That also covers an AP whose agent does not report the MTU yet.
- **Aeolus does not change the uplink's MTU.** Raising it is done on the AP, and the AP's next state report lets the tunnel through. The uplink carries the AP's management, so Aeolus leaves it alone (0053).
- **The Interfaces tab** shows an unset MTU as "default, 1450" (or 1430), and a tunnel's MTU can go back to the default.

## Consequences

- A tunnel works on any standard path with no MTU set.
- A jumbo tunnel needs the whole path, the AP's uplink included, raised first. Until then it is refused with the numbers, rather than losing large packets.
- An AP whose state report is older than its uplink change is judged by the old MTU until it reports again, within five minutes.
