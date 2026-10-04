# 0063. A tunnel can start from any VLAN on the uplink

- Status: Proposed
- Date: 2026-10-04
- Proposed by: Griff: a tunnel can start from the management VLAN or from any VLAN the uplink's trunk carries, such as VLAN 20, and end on a concentrator that maps its VNI (55, say) to a VLAN of its own. The management VLAN is the default; otherwise the VLAN is given. Written up by Claude.
- Refines: 0054, 0055

## Context

- **Today every tunnel starts from the AP's management interface** (0054). The tunnel's `tunlink` is that interface, and the tunnel's local end is the AP's address on it. On OpenWrtnight that is `lan`, the uplink's untagged VLAN, 192.168.1.38.
- **That isn't always the right path:**
  - a site may keep a VLAN for tunnels;
  - the concentrator may be reachable from one VLAN and not from the management VLAN;
  - the management VLAN may be one that shouldn't carry client traffic.
- **OpenWrt's vxlan protocol can start a tunnel anywhere the AP has an address.** It sends the tunnel out of `tunlink`'s device, from `tunlink`'s address. So the AP needs an interface with an address on the chosen VLAN.
- **The far end's VNI is unaffected.** A concentrator mapping VNI 55 to its own VLAN 55 is its business. The VNI is already set per transport and per tunnel port.

## Decision

- **A tunnel says where it starts.** This is a new field on the tunnel, `concentrators.<name>.underlay_vlan`. Like the tunnel's other fields, it is set in Locations and inherits down (0055).
  - Unset, the default: the management VLAN, as now.
  - A VLAN number: the tunnel starts from that VLAN on the uplink.
- **In the UI**, the tunnel's editor (Interfaces › Tunnels) gains "Starts from": a radio button for "the management VLAN" (the default), or "VLAN" with a box for its number.
- **On the AP, the VLAN gets an interface of Aeolus's own,** `aeolus_vlan<N>_tunnels`:
  - It sits on the uplink's bridge at that VLAN, which is tagged on the uplink as a network's VLAN is.
  - Its address comes from DHCP.
  - Network names `vlan<number>-…` are reserved, as `vlan<number>` and `port-…` already are, so no network's interface can take the name.
  - The tunnels that start there name it as their `tunlink`. They then go out tagged on that VLAN, from the AP's address there.
- **Only the tunnels use that VLAN's gateway.**
  - The interface's routes go in a routing table of their own, with a rule for traffic from its address.
  - The AP's own traffic stays on the management VLAN: the manager, DNS, NTP and the agent's polls.
- **The AP can't be reached through that VLAN.**
  - The interface goes in a firewall zone of its own that rejects input and forwarding. It lets in only the tunnels' UDP from their concentrators, and DHCP's answers.
  - So a VLAN that also carries Wi-Fi clients, such as VLAN 20 in the lab, gives them no way in.
- **The rest works as now, from that VLAN:**
  - the MTU check against the uplink (0056), and the MSS clamp;
  - the prober, which pings the concentrator from the VLAN's address, so "the concentrator answers" is about the path the tunnel takes (0059);
  - the probes on the tunnel and its lease on the segment (0060).
- **Several tunnels can start from one VLAN.** They share its interface. When no tunnel starts there any more, the interface, its zone, its table and its rule go.
- **Held, or reported:**
  - an AP whose uplink isn't a trunk it can tag: the config is held, as for a VLAN network;
  - no DHCP answer on that VLAN: the tunnel can't start. The state report says "no address on VLAN 20", and the verdict is `down`, with the concentrator unreachable.

  A static address per AP can come later, if a site needs it.

## Consequences

- **The concentrator sees the AP at a new address,** its address on that VLAN. A concentrator that lists its peers (a flood list or static peers) must list that one too, and 0054's advice on reserved addresses applies to that VLAN's DHCP. One that learns its peers, like the lab's Arista, does that itself.
- **Each AP takes one more lease** for each VLAN its tunnels start from.
- **Changing where a tunnel starts** re-creates its device, with one network reload. The prober follows the new device (v0.26.1).
- **The AP holds an address on that VLAN.** Unlike the probe leases of 0060, this one is the kernel's, because the tunnel needs it. The zone is what keeps that VLAN's clients out.

## Lab checks before building

On OpenWrtnight, set up by hand and undone afterwards, with no change to its networks:

1. **An interface on VLAN 20 with DHCP and its own routing table.** Check that it gets an address from 192.168.20.254, and that the AP's default route and its traffic to the manager stay on `lan`.
2. **A test tunnel on a VNI the Arista maps but the AP doesn't use** (VNI 55, if the Arista maps it to VLAN 55), with `tunlink` on that interface. Check that:
   - its packets leave tagged on VLAN 20, from the AP's VLAN 20 address, through 192.168.20.1;
   - the Arista answers, and learns the new VTEP address;
   - the prober's DHCP and probes cross the tunnel.
3. **The zone.** Check that from VLAN 20, nothing on the AP answers except the tunnel.
