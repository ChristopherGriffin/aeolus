# 0064. Telling which VLANs reach an AP

- Status: Proposed
- Date: 2026-10-04
- Proposed by: Griff: 0020's rule that APs detect the VLANs on their trunk, and the manager flags a VLAN that is expected but missing; written up by Claude
- Refines: 0020, 0059, 0060, 0061
- Resolves: 0020's first open point, how APs detect VLANs

## Context

- **A VLAN missing on the switch port is a common fault, and a quiet one.**
  - The AP renders the VLAN and its Wi-Fi comes up.
  - Its clients then get no address, and nothing on the AP says why.
  - 0020 has the AP find out, and the manager say so.
- **The AP sees only the VLANs it carries.**
  - On a DSA switch such as OpenWrtnight's, the switch chip drops a tagged VLAN that the uplink's bridge doesn't carry, before the CPU sees it (0061).
  - Checked on 2026-10-04. The Arista's Et6 trunk carries every VLAN it has (`Trunking VLANs Enabled: ALL`), 30 and 1010 among them. Each has an SVI in OSPF, so a hello goes out every 10 seconds.
  - In 60 seconds the AP got none of them on `wan`. It got 4631 frames on VLAN 20, 2013 on VLAN 10, 1636 on VLAN 50 and 1825 untagged.
  - On hardware whose uplink isn't a switch-chip port, this may differ.
  - So the AP can't list what else the switch port carries.
  - It can tell, for each VLAN it carries, whether that VLAN reaches it.
- **Active probes already work on the uplink (0061).** A network's VLAN transport is probed tagged on the uplink, from the VLAN's own MAC. The answer's VLAN is read from `PACKET_AUXDATA`. But only VLAN transports of networks with a fallback are probed.
- **0020 gave three ways to tell, checked on OpenWrtnight on 2026-10-04:**
  - **Traffic heard.** In 40 seconds on `wan`, incoming broadcast and multicast frames numbered:
    - 1834 on VLAN 20;
    - 1324 on VLAN 10;
    - 1086 on VLAN 50;
    - 1178 untagged.

    That is 27 to 45 a second on every VLAN the uplink carries. A frame the AP sends shows as outgoing (`packet_type` 4), so it is told apart.
  - **LLDP.** The Arista sends LLDP every 30 seconds. It gives:
    - its name (homelab.symtus.com);
    - the port (Ethernet6, described as "Pumphouse OpenWrt");
    - its management address;
    - the port's native VLAN (Port VLAN ID 1).

    It sends no VLAN Name TLVs, so LLDP doesn't list the VLANs the port carries. A switch can be set to send them.
  - **A probe per VLAN:** as 0061 does. A DHCP discover and an ARP to the gateway were answered on VLANs 20 and 50.

## Decision

### Which VLANs

- **Each VLAN the AP's intent needs on its uplink is watched,** but for the management VLAN, which the agent reaches the manager on:
  - a network's VLAN transport, primary or fallback;
  - a port's VLANs: an access port's, and a trunk's untagged and tagged ones;
  - the VLAN a tunnel starts from (0063).
- The AP's own networks, which Aeolus doesn't manage, are not watched (VLAN 10 in the lab).

### How the AP tells

- **A probe on each VLAN,** as 0061's VLAN probes:
  - sent on the uplink, tagged as the uplink carries the VLAN;
  - from the VLAN's own MAC (0060), every 30 seconds;
  - each answer's VLAN read from `PACKET_AUXDATA`.
- **No lease, unless the VLAN carries a network with a fallback.** Taking an address on every VLAN, on every AP, would spend a pool for nothing. So the probe stops at the DHCP offer:
  - The offer shows that a DHCP server answers on the VLAN, and it names the gateway.
  - The probe then asks the gateway by ARP, from 0.0.0.0 as 0059's probes do. It sends a fresh discover every 10 intervals.
  - With no offer, it sends an echo to IPv6 all-nodes, from the link-local address the VLAN's MAC makes.
  - A VLAN transport of a network with a fallback keeps its lease (0061): the prober uses it to judge a switch.
- **What it hears.** One socket on the uplink takes the incoming broadcasts of every VLAN. A broadcast that comes in tagged with a VLAN shows that the switch port carries that VLAN, even when nothing answers the probe.
  - The filter keeps only broadcasts, so the prober reads ARP requests and DHCP discovers, not clients' traffic.
  - The prober notes when each VLAN was last heard.
- **LLDP, when the switch sends it.** The same socket takes LLDP frames. The prober keeps the newest: the switch's name, its port and the port's description, the native VLAN, and the VLANs it names, if it sends VLAN Name TLVs.

### What it reports, and what the manager shows

- **Each watched VLAN, in the state report's `vlan_probes` (0061),** with two new fields:
  - `heard_ago`: seconds since a frame on that VLAN came in, or null;
  - `lldp`: whether the switch's LLDP names the VLAN, when it names any.
- **The uplink's neighbour,** as `uplink_neighbor`: the switch's name, its port and the port's description, the native VLAN, the VLANs it names, and seconds since its last LLDP frame.
- **The unused `vlans` field goes.** It is the report's list of VLANs seen on the uplink, which no agent ever sent.
- **The manager judges each VLAN:**

  | Judgment | When | Means |
  |---|---|---|
  | **answering** | the probe's verdict is `up` | It reaches the AP. |
  | **heard** | no answer, but a frame came in on it within three intervals | It reaches the AP, but nothing answers the probe: no DHCP server, and no gateway that answers. |
  | **missing** | the switch's LLDP names its VLANs, and not this one | The switch port doesn't carry it. |
  | **silent** | none of the above, after three intervals | It may not be on the switch port. A VLAN with no DHCP, no gateway and no chatter looks the same, so the manager says "may". |

- **A warning, not a hold.** A missing or silent VLAN is a fault on the switch, not in the AP's config, so the config is sent as before.
  - The AP's overview warns, in one line for each VLAN: which VLAN, who needs it (the networks, ports and tunnels), and, from LLDP, which switch port to look at.
  - Interfaces › Ports shows the uplink's VLANs with their judgment, and its neighbour.

## Consequences

- **Every watched VLAN costs the AP:**
  - a probe socket;
  - a frame every 30 seconds;
  - a DHCP discover every 5 minutes.

  It costs no address.
- **The uplink's broadcasts are read by the prober,** at the rate the lab showed, tens a second. The lab check measures its CPU.
- **A switch that sends VLAN Name TLVs turns "silent" into "missing",** and a quiet VLAN it names is no longer silent.
- **The AP still can't see VLANs it doesn't carry.** Nothing here tells what else the switch port carries, but LLDP's list.

## Not now

- **Switching a single-transport VLAN network's clients away when its VLAN is missing.** That network has nowhere to go: only the manager's warning helps.
- **Setting the switch's VLANs.** Aeolus manages APs.
