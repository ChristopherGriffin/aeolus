# 0058. Ethernet ports on tunnels

- Status: Proposed
- Date: 2026-10-03
- Proposed by: Griff: attach an Ethernet port to VXLAN by tunnel and VNI, carrying several VNIs tagged; written up by Claude
- Refines: 0053, 0054, 0055, 0056, 0057

## Context

A port carries VLANs from the AP's uplink bridge (0053), and a network can travel over a tunnel to a concentrator (0054, 0055). This lets a port carry tunnels too, the standalone tunnels of 0053's plan: a wired device, or a switch, reaches the far end's segments with no Wi-Fi network involved.

## Decision

- **A port's mode can be `tunnel`.** It then carries VNIs instead of the uplink's VLANs, each one named under `ports.<name>.vxlan.<vlan>`:
  - **`<vlan>`** is the VLAN the VNI is tagged with on the wire, 1 to 4094, or `untagged` for at most one carried bare;
  - **`tunnel`** names a tunnel set where the AP is (0055);
  - **`vni`** is the VNI.

  Each mapping inherits like any setting, so a folder can map VLAN 50 to VNI 50 for every AP below it, and an AP can add more. In `tunnel` mode, the port's `untagged` and `tagged` VLANs do not apply.
- **On the AP, the port leaves the uplink bridge:**
  - Aeolus takes the port out of the uplink bridge's port list, and out of that bridge's `bridge-vlan` entries.
  - **The untagged VNI:** the port itself joins that tunnel's bridge `br-vx<VNI>`.
  - **Each tagged VNI:** an 802.1Q device on the port, `<port>.<vlan>`, joins that tunnel's bridge. Its section is `aeolus_port_<port>_<vlan>`, a name no network can take (0054).
  - **The tunnel and its bridge** are the ones a network's transport makes (0054), so a port and a Wi-Fi network on the same VNI share one segment.
  - **What Aeolus owns grows** (0040): a tunnel port's place in the uplink bridge's port list. Set back to `access` or `trunk`, the port returns to that bridge with those VLANs. Left unset, it stays as it is, as with any port setting (0053), and the tunnel and bridge it is on stay while it is on them.
- **The checks:**
  - **Config check (0029):** these are held, with a reason:
    - a tunnel port with no VNIs;
    - a tunnel not set where the AP is;
    - a VLAN that is not 1 to 4094;
    - one VNI on two VLANs of the same port;
    - a VNI reaching two tunnels at one AP;
    - a VNI that a network uses as its fallback, as that tunnel waits until switching starts it (0054).

    The uplink is refused as in 0053. 0056's uplink MTU and 0057's netifd check hold a tunnel port as they hold a network's tunnel.
  - **Render check (0039):**
    - the port is in no `bridge-vlan` of the uplink bridge, and out of its port list;
    - each VNI's tunnel is set as in 0054, and started;
    - the untagged port, or each 802.1Q device with the right `ifname` and `vid`, is in that tunnel's bridge.
- **The state report** lists a tunnel port after the uplink bridge's ports, as the agent finds it on a tunnel's bridge, itself or under an 802.1Q device Aeolus made, so Ports now still shows it (0053).
- **The UI:** the port editor adds Tunnel as a mode. Its VNIs are a table of VLAN, tunnel and VNI, edited a row at a time: add a row, change a row, or remove one. The port's card and Ports now show each mapping, such as "VLAN 50 → arista · VNI 50".

## Consequences

- A wired device or switch on a spare port reaches the concentrator's segments, tagged or untagged, without a Wi-Fi network.
- **A tunnel port can close a loop that nothing breaks.** If the concentrator bridges a VNI to a VLAN that also reaches the AP, by its uplink or another port, cabling the tunnel port back to that VLAN loops broadcasts through the concentrator. STP does not cross the tunnel, and Aeolus does not know the concentrator's VLAN for a VNI, so it cannot hold such a config; 0059's tunnel probes are to notice a loop and stop it.
- **Changing a tunnel port reloads the AP's network.** The automatic revert covers a change that cuts the AP off (0008, 0057). The renderer and the agent both change, so APs need the new agent files.
- **Traffic between the port and its tunnel crosses the AP's CPU.** The hardware switch does not bridge to a tunnel, so a busy wired device costs the AP more than one on a VLAN.
- **A port cannot carry local VLANs and tunnels at once,** because it can be in only one bridge.
