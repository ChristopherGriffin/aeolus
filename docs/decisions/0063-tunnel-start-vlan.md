# 0063. A tunnel can start from any VLAN on the uplink

- Status: Accepted
- Date: 2026-10-04
- Proposed by: Griff: a tunnel can start from the management VLAN or from any VLAN the uplink's trunk carries, such as VLAN 20, and end on a concentrator that maps its VNI (55, say) to a VLAN of its own. The management VLAN is the default; otherwise the VLAN is given. Written up by Claude, and accepted with the merge.
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
  - Unset, the default: the management VLAN, as now. 0 says the same, so a folder or AP can choose the management VLAN over a VLAN set above it.
  - A VLAN number, 1 to 4094: the tunnel starts from that VLAN on the uplink.
- **In the UI**, the tunnel's editor (Interfaces › Tunnels) gains "Starts from": a radio button for "the management VLAN" (the default, saved as 0), or "VLAN" with a box for its number. The Tunnels table shows, under each tunnel's state, the VLAN it starts from and the AP's address there, or that it has none yet.
- **On the AP, the VLAN gets an interface of Aeolus's own,** `aeolus_vlan<N>_tunnels`:
  - It sits on the uplink's bridge at that VLAN, which is tagged on the uplink as a network's VLAN is.
  - Its address comes from DHCP.
  - Network names `vlan<number>-…` are reserved, as `vlan<number>` and `port-…` already are, so no network's interface can take the name.
  - The tunnels that start there name it as their `tunlink`. They then go out tagged on that VLAN, from the AP's address there.
- **Only the tunnels use that VLAN's gateway.**
  - The interface's routes go in a routing table of its own (netifd's `ip4table`). netifd adds the rules: one for traffic from its address, one for traffic to its subnet, and one for the AP's own traffic that comes after the main table.
  - The AP's own traffic stays on the management VLAN (DNS, NTP, the default route), except to that VLAN's own subnet, which the AP then reaches directly, as any host on it does. In the lab, the manager at 192.168.20.60 is on VLAN 20, so the agent would reach it on VLAN 20 while a tunnel starts there.
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
  - a VLAN that is the AP's management VLAN: held, since tunnels start from it anyway;
  - a tunnel to an IPv6 concentrator: held for now. Its start would need an IPv6 address and routes of its own on that VLAN;
  - no DHCP answer on that VLAN: the tunnel can't start. The state report says "no address on VLAN 20", and the verdict is `down`, with the concentrator unreachable.

  A static address per AP can come later, if a site needs it.

## Consequences

- **The concentrator sees the AP at a new address,** its address on that VLAN. A concentrator that lists its peers (a flood list or static peers) must list that one too, and 0054's advice on reserved addresses applies to that VLAN's DHCP. One that learns its peers, like the lab's Arista, does that itself.
- **Each AP takes one more lease** for each VLAN its tunnels start from.
- **Changing where a tunnel starts** re-creates its device, with one network reload. The prober follows the new device (v0.26.1).
- **The AP holds an address on that VLAN.** Unlike the probe leases of 0060, this one is the kernel's, because the tunnel needs it. The zone is what keeps that VLAN's clients out.

## Lab checks

Run on OpenWrtnight on 2026-10-04 with temporary interfaces only (`ubus call network add_dynamic`), with nothing in its config changed, and undone afterwards. They used VNI 10 in place of 55, which the Arista doesn't map yet. VNI 10 is mapped, and only lan4 used it on the AP, with nothing plugged in.

1. **An interface on VLAN 20 with DHCP and `ip4table` 120.**
   - It leased 192.168.20.74 from 192.168.20.254. Table 120 got the default route via 192.168.20.1, and the main table didn't change.
   - netifd added three rules: from 192.168.20.74 to table 120, to 192.168.20.0/24 to table 120, and the AP's own traffic (`iif lo`) to table 120 after the main table.
   - The second rule moved the AP's traffic to its VLAN 20 neighbours onto VLAN 20, the manager included; the agent still reached it. Everything else stayed on `lan`.
2. **A test tunnel for VNI 10 with `tunlink` on that interface,** in place of the AP's own VNI 10 tunnel.
   - The kernel refuses a second VXLAN device for a VNI and port that one already uses. So a tunnel that changes where it starts is re-created, not added alongside.
   - With the local address left as any (`0.0.0.0`, as now), the kernel chose 192.168.20.74 by itself. The tunnel left on VLAN 20 to 192.168.20.1 (the Arista, `28:e7:1d:ca:29:13`), through the third rule, since the main table's default route is on `lan`.
   - The Arista learned the new address at once and sent VNI 10 to it. It kept sending to the old one (192.168.1.38) too, until that ages out.
   - The prober, run by hand on the test tunnel, leased 192.168.10.235 (the same lease, from VNI 10's segment MAC), asked the lease's gateway 192.168.10.1, and found it `up` at 1.2 ms. A ping to the concentrator from 192.168.20.74 took 0.5 ms.
3. **Nothing on the AP answers on VLAN 20.**
   - From the manager host (192.168.20.60, on VLAN 20), the AP's 192.168.20.74 rejected a ping, and TCP 22, 80, 443 and 8443. Its management address still answered SSH.
   - The AP's firewall already rejects input from interfaces outside its zones. The build's zone makes that explicit, with the one rule the test needed: the tunnel's UDP 4789 from its concentrator, on that VLAN.
4. **Putting things back re-created VNI 10's tunnel device,** and the v0.26.1 prober followed it on its own: VNI 10 `up` again, at 0% CPU.

## As built

- **The interface:** `aeolus_vlan<N>_tunnels` is proto `dhcp` on `<uplink bridge>.<N>`, with `ip4table` 1000 + N and `peerdns 0`. It is in the zone `aeolus_ul`, which rejects input and forwarding. The tunnels' rules let them in from their concentrators on that zone.
- **The render check** wants all of that, the tunnel's `tunlink` on that interface, and its VLAN carried. A tunnel with no start VLAN must not start from such an interface.
- **The prober** pings each concentrator with a raw socket bound to the device its tunnel starts from (`SO_BINDTODEVICE`), so the ping takes the tunnel's path. An answer counts only on the socket its ping went out on.
- **Checked on OpenWrtnight,** with a dynamic interface on VLAN 20 (`ip4table` 1020) and the prober run by hand for two test tunnels to 1.1.1.2, one starting from VLAN 20 and one from the management VLAN:
  - each one's pings left on its own VLAN (from 192.168.20.76 and from 192.168.1.38), and each got its own answers;
  - with the VLAN 20 interface gone, the first lost the concentrator and the second kept it.
- **A new agent test case,** `underlay`, renders two networks over two concentrators that both start from VLAN 30, and removes an earlier VLAN 40 interface. It was rendered on the AP, and the render check passes it.
