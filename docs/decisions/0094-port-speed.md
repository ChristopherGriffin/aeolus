# 0094. A port's speed

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff. A folder's port, as a template, should say its speed: Auto, or 1GbE or 5GbE if set that way. Written up by Claude
- Refines: 0053, 0092, 0093

## Context

- **netifd takes a device's `speed`, `duplex` and `autoneg`** (strings in /sbin/netifd on the C-360, 2026-10-09).
- **With `speed` set, netifd advertises only that speed** to the far end, of the port's own link modes (`system_set_ethtool_settings`, netifd master). It turns autonegotiation off only where `autoneg` is set off. With `duplex` set full, it advertises only full duplex.
- **Without `duplex`, a speed is advertised in both duplexes.** Fixed and half, a link at that speed would be one no switch port wants.

## Decision

- **`ports.<name>.speed`:** `auto`, or 10, 100, 1000, 2500, 5000 or 10000 Mbit/s. Set per kind of AP it is `boards.<board>.ports.<name>.speed` (0092), folded as the rest.
- **The agent puts a fixed speed on the port's device section,** with `duplex '1'`, leaving autonegotiation on. Both ends then agree on that speed alone.
  - The section is the port's own where it has one, with what else it holds left as it is.
  - Otherwise it is Aeolus's `aeolus_port_<name>`, which also holds the port's off (0053).
  - `auto` takes the speed and duplex off. A section of Aeolus's own with nothing left goes.
  - Unset leaves the port as it is.
- **Never on the uplink:** a port that is the uplink is refused as before, and a bond's members are not ports in the uplink's bridge, so they are never set.
- **The render check** wants exactly that speed, with full duplex, on the port's device sections, or none for `auto`.
- **On Ethernet, a port's form has Speed:** Auto, 10M up to 10G. It is limited to what the port can go where its APs report it (0093): faster ones are shown, greyed, with "the port goes to 1G".
  - A folder's jack, and its details, say the speed set ("Auto", "1GbE") beside what the port can go.
  - An AP's details say the speed where one is set.
  - Previews and the change log say "Speed: 1G".
- **The form builder offers a field that is one of a few words or numbers** (`oneOf` of consts and enums) as a pulldown.

## Consequences

- A wrong speed can cost a port its link. That is why it is never set on the uplink. A port Aeolus's apply cuts off from the manager goes back within 90 seconds, as any apply does (0008, 0040).
- Half duplex, and turning autonegotiation off, are left out: no lab port needs them.
