# 0053. Ethernet ports, and an Interfaces tab

- Status: Proposed
- Date: 2026-10-03
- Proposed by: Griff: a section to configure the Ethernet ports, in an Interfaces tab that will also hold VXLAN tunnels; with VLANs per port and on/off first; written up by Claude
- Refines: 0006, 0039, 0040, 0047, 0048

## Decision

- **Ports are set by name** (`ports.<name>`, Locations tree), on a folder or AP:
  - `enabled`: off turns the port off;
  - `mode`: `access` carries one VLAN, untagged; `trunk` carries an untagged VLAN (0 for none) and tagged VLANs;
  - `untagged`, `tagged`: those VLANs.

  LACP (`mode: lacp`, `bond`) stays in the schema but is not applied yet: a config asking for it is held, with a reason (0029). `uplink` stays the agent's own setting (0040).
- **A port counts on an AP if it is in the AP's VLAN-filtering bridge,** the one its uplink is in. On OpenWrtnight that is `lan1`–`lan4` and `wan` in `br-lan`. A folder can set `lan2`, and an AP without a `lan2` in that bridge has nothing to apply. Aeolus never adds a port to the bridge.
- **The uplink port is left alone.** Its VLANs and whether it is on belong to the AP's management, so a setting for it is refused by the render check, with a reason.
- **On the AP, Aeolus owns only what it sets for each port** (0040):
  - **VLANs:** the port's entries in the bridge's `bridge-vlan` sections. Its old entries are removed. An access or trunk untagged VLAN gets `port:u*`, and each tagged VLAN gets `port:t`.
  - **The uplink carries them too:** each VLAN a port uses is tagged on the uplink as well, the way a network's VLAN is, so its traffic reaches the core. A missing `bridge-vlan` is added as `aeolus_vlan<id>`.
  - **On/off:** `enabled` on the port's own `device` section. If the AP has none, Aeolus adds `aeolus_port_<name>` to turn it off, and removes it to turn it on again.
- **A setting left unset leaves the port as it is,** as with a radio's channel. Aeolus does not put back what the AP had before it set the port.
  - A VLAN Aeolus added stays while a port is still on it, even once nothing asks for it, so that port is not cut off.
  - The schema says that unset, a port is on, unless Aeolus turned it off.
- **The checks:**
  - **Config check:** these are held, with a reason:
    - an access port with no untagged VLAN, or with tagged VLANs;
    - a VLAN both untagged and tagged;
    - VLANs set with no mode to carry them;
    - LACP.
  - **Render check:** each configured port's entries and `enabled` must match the intent, and the uplink must carry each VLAN tagged. A port the AP does not have is not judged.
- **The agent reports its ports** in each state report: each port in that bridge, in the bridge's order, whether it is up, whether it has a link and at what speed, and which is the uplink.
- **A new Interfaces tab sits after Networks,** on folders and APs:
  - **Ethernet** has a card for each port by name, with its on/off, mode and VLANs, and where each comes from.
  - **Edit** opens a form built from the schema, which shows only what the mode uses, and does not offer LACP.
  - The uplink port is shown, but not offered for editing.
  - **Set up another port** sets one no AP here has reported yet, such as for APs still to be adopted.
  - **Ports now** shows each AP's ports live: link and speed, beside what Aeolus sets there.
  - **Tunnels** will join Ethernet here (step 2 of the plan).
  - Hardware keeps Radios and Channels.
- **The schema description lists a list's items in place,** so the form reads tagged VLANs as numbers (0048).

## Consequences

- A wired device on an AP's spare port can be put on any VLAN the uplink can carry, from Aeolus.
- **Changing a port reloads the AP's network.** Wired clients on that port drop briefly. If the AP then cannot reach Aeolus, it puts its old settings back (0008).
- **Undoing a port setting does not restore the AP's own:** to put a port back on its first VLAN, set that.
- An agent older than this still renders the old way, so a config with port settings is refused on it until its agent is updated. It also reports no ports, and the Interfaces tab says so.
