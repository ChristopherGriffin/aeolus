# 0060. Each AP takes an address on the segments it probes

- Status: Proposed
- Date: 2026-10-03
- Proposed by: Griff: rather than a probe address set by hand, the AP pulls an address on each segment and probes from it, with a MAC of its own that says which AP and which segment it is; written up by Claude
- Refines: 0059

## Context

- **0059's probes ask an address someone sets** (`probe`, normally the segment's gateway), or else any IPv6 host. Nothing on VNI 50 answers IPv6, so VNI 50 was `unverified` until 192.168.50.1 was set by hand (change 35). Every new segment needs the same, and the setting goes stale when the gateway moves.
- **The segments mostly have DHCP.** On OpenWrtnight, a DHCP discover sent straight into `aeolus_50` from 02:21:36:5e:00:50 was answered at once by 192.168.50.254, offering 192.168.50.6, router 192.168.50.1, for 24 hours. The same on VLAN 20, sent tagged on the uplink from 06:21:36:5e:00:20, was answered by 192.168.20.254, router 192.168.20.1. Each offer names the gateway the probes want.

## Decision

- **The AP takes a DHCP lease on each segment it probes,** from a MAC of its own for that segment. The lease is a probe in its own right: an answer from the segment's DHCP server shows traffic crosses the tunnel both ways.
- **The MAC says which AP, and which segment:**
  - **The first byte** marks the address as one the AP made up (locally administered), and the kind of segment:
    - `02` for a VNI, its number written in decimal;
    - `06` for a VLAN;
    - `0a` for a VNI above 9999, its number in hex.
  - **The next three bytes** are the last three of the AP's own MAC.
  - **The last two bytes** are the segment's number, written so it reads as the number.

  OpenWrtnight (`a0:04:60:21:36:5e`) is `02:21:36:5e:00:50` on VNI 50, and `06:21:36:5e:00:20` on VLAN 20. It asks with a host name to match, such as `OpenWrtnight-vni50`. A DHCP server's lease list then shows each AP's presence on each segment, by name and by MAC.
- **Two segments can't share a MAC at an AP:**
  - **A tunnel's bridge takes its segment's MAC as its own,** rendered and render-checked. So the answers to it are delivered at the AP rather than flooded to the segment's Wi-Fi clients.
    - On OpenWrtnight, giving `aeolus_50`'s bridge the MAC through the config made netifd cycle the bridge's ports and hostapd reload its settings, with no network taken down. No client dropped, the IoT clients on 2.4 GHz included.
    - A VLAN's MAC has no bridge of its own to take it, so answers to it reach the uplink bridge as an unknown address, and it floods them to that VLAN's local ports: one small frame per probe.
  - **A config that would give two segments one MAC at an AP is held.** That only happens with VNIs above 9999 that agree in their last 16 bits.
- **The address is the prober's, never the AP's:**
  - The prober runs DHCP itself, over the same packet socket on the tunnel device that its probes use.
  - The kernel never holds the address. So nothing on the AP can be reached from a client segment, the segment gets no route, and the AP's own traffic is never drawn into a tunnel, even when a segment shares the manager's subnet.
  - It renews at half the lease, as any client does, and releases the lease when the tunnel goes.
- **What the prober asks, in order:**
  1. a probe address, where one is set: it stays, to override;
  2. the gateway the lease names;
  3. with a lease but no gateway, the DHCP server itself;
  4. with no lease, IPv6 all-nodes, as now.

  Once the AP holds an address, its ARP probes come from that address rather than 0.0.0.0.
- **The verdicts don't change** (0059). The state report adds, for each tunnel, its lease: the address, the server, the gateway, and how long it has left. The Tunnels view shows them.
- **VLAN segments,** probed once switching is built (0061), take a lease the same way, from their `06` MAC.

## Consequences

- **A new segment needs no setting:** VNI 50's probe address can go, since the lease names the same gateway.
- **Each AP takes one lease per segment it probes.** 20 APs on three tunnels take 60 addresses. A segment without DHCP works as before: with a probe address, or IPv6.
- **The probe's first answer waits for the lease.** Until then the prober asks from 0.0.0.0, as now.
- **The tunnel's bridge changes its MAC once,** when this is applied. Its Wi-Fi clients don't notice; its IPv6 link-local address changes with it.

