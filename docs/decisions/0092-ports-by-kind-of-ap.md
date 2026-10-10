# 0092. Ports by kind of AP

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff. On Ethernet, a breakout by device type: each kind of AP in the folder on one line (the C-360 with its two ports, the OpenWrt One, the R7800 with four), an arrow opening its ports as cards. "Port continuity first, then the features." Written up by Claude
- Refines: 0053, 0058, 0085, 0091

## Context

- **A folder sets a port by name alone,** for every AP below with a port of that name (0053).
- **The lab's kinds share names** (2026-10-09):

  | Kind | Ports | Uplink |
  |---|---|---|
  | Arista C-360 | eth0, eth1 | both, bonded as bond0 by its image (LACP, layer 3+4), which is all it reports |
  | OpenWrt One | eth0, eth1 | eth0 |
  | Netgear R7800 | wan, lan1–lan4 | wan |

  So `ports.eth1` on a folder reaches the C-360's eth1, half of its uplink bond, and the One's spare eth1 alike. Sandbox's `ports.lan4` tunnel was already in the C-360's config, which has no lan4.

## Decision

- **A kind of AP's own settings: `boards.<board>.ports.<name>.<field>`,** a Locations field family, by board as OpenWrt names it.
  - It inherits like any field. An AP of that board takes it in place of the plain `ports.<name>.<field>` set on the same node or above, its template's included.
  - A plain one set closer to the AP, or locked above, stays.
  - It is checked against the schema as the plain field is. The manager folds it into each AP's own settings in `ResolveAP` and takes every `boards.` field out, so the agent and the render check see plain ports only.
  - Ports only, for now.
- **Ethernet is a line a kind of AP,** by board, below the Inherits from bar (0089, 0091):
  - The line says its model name, how many APs, its ports (the uplink first) and how many are set.
  - Its arrow opens a card a port: link, mode, what it carries and where from, and Edit, or Info on the uplink.
  - Edit's form, and Add a port's, open under the cards and set the kind's own fields on a folder, or the AP's own on its page.
  - A kind stays open across the page's redraws. One kind alone opens by itself.
- **Each AP's config view says its model,** beside its board, for the lines' names.
- **The C-360's bond is shown, not edited:** it is the uplink, which Aeolus leaves alone. Making LACP work (0053) is the next step, after this.
- **The safety net is the one every apply has:** an AP that can't reach the manager within 90 seconds of applying puts its old config back (0008, 0040).

## Consequences

- One kind's ports no longer reach another's of the same name, where they are set by kind. A plain setting still reaches all, and the plain ones already set stay as they are.
- A lock on a plain port field holds a kind's own below it too, though the kind's own is another path.
