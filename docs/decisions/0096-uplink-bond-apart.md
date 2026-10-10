# 0096. The uplink's bond, kept or taken apart

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff. Unbond bond0, and turn the C-360's ports back into single Ethernet ports. eth0 defaults to primary, but the AP finds the path out itself and labels it, so the AP still works if someone plugs into the other port. Written up by Claude
- Refines: 0053, 0064, 0093, 0095

## Context

- **The C-360's image makes its uplink an LACP bond.** `c360bond` is bond0, of eth0 and eth1: 802.3ad, fast LACP, layer 3+4 hashing, MII every 100 ms. bond0 is the only Ethernet port of br-lan, and carries VLAN 20, its management, tagged (`c360vlan20`). Aeolus's own VLANs ride `bond0:t` too.
- **The switch sees a port-channel.** Its two ports, Ethernet13 and Ethernet14 on homelab, are in it. Each member hears its own switch port's LLDP. With one LLDP socket on bond0, the prober's uplink neighbour flipped between the two every 30 seconds.
- **Two ports in one bridge to the same network loop,** unless spanning tree blocks one (0095).
- **The AP's uplink was one name, `aeolus.agent.uplink`.** The renderer tags VLANs on it. The prober probes and watches VLANs on it, and reads LLDP there. The agent reports it.

## Decision

- **`uplink.bond`, true or false.** It is a template field. Per kind of AP it is `boards.<board>.uplink.bond`, folded as the rest (0092). True, or unset, keeps the image's bond, and puts back one Aeolus took apart. Only false acts.
- **False takes the bond apart where the uplink is a bond, as the renderer's first step.**
  - Each member takes the bond's place in the bridge, the bridge's ports and every bridge-vlan entry alike: `bond0:t` becomes `eth0:t` and `eth1:t`. The image's own entries are included, so every VLAN the bond carried, management too, is on both ports.
  - The bond's section is kept as it was, every option, its name and place, in `aeolus.aeolus_unbond`, and taken out of the network.
  - `aeolus.agent.uplink` becomes the first member, the primary.
  - Put back, the section returns as it was, the bond takes its members' place again, and the uplink is the bond's.
- **The uplink is one or more ports.**
  - The renderer tags every VLAN it adds on each of them.
  - It refuses port settings on any of them, as on the uplink.
- **The bond stays where taking it apart is unsafe, and the renderer says why:**
  - without spanning tree on and running (ustp, 0095), since two ports to one switch would loop;
  - where the bond is more than one bridge's port, since something else uses bond0.
- **An uplink that is no bond is left alone.** A folder can set it false for every kind.
- **The manager refuses false without `uplink.stp` true.** It is a compose problem, shown in the preview.
- **The render check, for false:**
  - the bond gone from the network, kept in `aeolus_unbond`;
  - each member in the bridge, and not the bond;
  - the first member the uplink;
  - spanning tree on;
  - every VLAN on the bridge on each member alike, so none is lost when the other is the path out.
  - For true or unset, none kept.
- **The path out is found as the AP runs (`aeolus.uplink`, shared by the agent and the prober).** Of the uplink's ports, it is the first with a link that the bridge forwards on, which spanning tree chose. Else it is the first with a link, else the first. A cable moved from eth0 to eth1 still reaches the manager. The prober's VLAN probes and watches on the uplink are made on the path out, and follow it within 10 seconds.
- **LLDP is read on each Ethernet port under the uplink:** a bond's members, or the ports taken apart.
  - Each says its own switch port, and the log speaks of each port, so the neighbour no longer flips.
  - The uplink's neighbour is the path out's, or a bond's first member's that hears one.
- **The state report:**
  - both ports `uplink`;
  - the path out `path`;
  - each port, and a bond's members, with the switch and switch port its LLDP names (`neighbor`).
  - The uplink port's counters are the path out's.
- **On Ethernet, a kind whose uplink is a bond has an Uplink line** under its jacks: LACP bond, or Two ports, apart, with spanning tree, with Edit.
  - Taking it apart turns spanning tree on in the same change, where it is not on already.
  - The confirm warns of the switch: plain trunks, or LACP fallback to individual ports with a timeout well under 90 seconds, and no BPDU guard.
  - On an AP, the path out's jack says "path out", with ↑, and the other "standby", or "blocked", in amber. Each port's details give its switch port.
- **On an AP's page, what a folder sets for the AP's board shows as the AP's own,** as the manager folds it. That applies to its ports too, which 0092 left showing only plain settings.

## Consequences

- **The switch must be ready first.** A port-channel that waits for LACP keeps both ports down. The AP then cannot reach the manager, and puts the bond back after 90 seconds, its apply's revert (0008). On Arista, `port-channel lacp fallback individual`, with `port-channel lacp fallback timeout` well under 90, lets each port come up on its own, on its own interface's switchport settings. Those must then carry the same VLANs as the port-channel.
- **Two ports apart carry no more than one.** Spanning tree blocks the second cable to one switch, which is the point: it is a standby, not more bandwidth. LACP stays the way to use both.
- **Taking the bond apart, or putting it back, changes the AP's uplink,** which Aeolus otherwise never touches. It is set only by a person, and the revert covers a mistake.
