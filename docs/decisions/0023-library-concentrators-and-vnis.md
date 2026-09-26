# 0023. The library holds concentrators and their VNIs, scoped to locations

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff
- Supersedes: 0021

## Decision

- The library is global, at the Org. It holds **concentrators** (name, routable address, UDP port with default 4789, MTU), and each concentrator holds its **VNIs**, each with a label.
- VNIs are defined only in the library, never inside a network. On a network's transport, you pick a concentrator from a pull-down, then a VNI from that concentrator's list.
- A concentrator can be **limited to certain Location folders** (properties): it is offered and used only at those folders and below them. This covers split networks, where properties X, Y and Z reach one concentrator and Q, R and T reach another.

## Where scoping meets service assignment

Proposed by Claude, accepted by Griff with the M1 merge. A network lives in the Services tree, but a property is a Locations folder, and one service folder can be assigned to many locations. Proposed rules for where the two meet:

- **Pull-down.** A network's concentrator pull-down offers the concentrators available at every location its service folder is assigned to.
- **Per AP.** When the manager resolves an AP (M1), a transport whose concentrator is not available at the AP's location is left out of that AP's config, so the AP never tries a concentrator it cannot reach.
- **Check.** If that leaves a network with no usable transport at an AP, the manager's check flags it before anything is published. A network whose primary concentrator is unavailable at a property still works there when its fallback is.
