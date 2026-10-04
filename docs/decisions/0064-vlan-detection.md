# 0064. Telling which VLANs reach an AP

- Status: Proposed
- Date: 2026-10-04
- Proposed by: Griff: 0020's rule that APs detect the VLANs on their trunk, and the manager flags a VLAN that is expected but missing. Since an AP can't see a VLAN it doesn't carry, it tells when one it carries, for a port or an SSID, has no frames coming in from the uplink: that VLAN isn't on the switch port. Written up by Claude
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

- **Each VLAN the AP's intent needs on its uplink is watched:**
  - a network's VLAN transport, primary or fallback;
  - a port's VLANs on this AP: an access port's, and a trunk's untagged and tagged ones;
  - the VLAN a tunnel starts from (0063).
- **The management VLAN gets no exception.** The render check can't tell which it is. It is watched only when the intent puts something on it, and it is always heard.
- **The AP's own networks, which Aeolus doesn't manage, are not watched** (VLAN 10 in the lab).
- **The renderer writes the list into the prober's plan:** a `watch` section, `aeolus_watch<N>`, for each VLAN the uplink carries. It says whether the VLAN is tagged there, and the AP's MAC on it (0060).

### How the AP tells

- **A VLAN is there if frames come in on it.**
  - The prober listens on the uplink for incoming frames tagged with the VLAN, or untagged for a VLAN the uplink carries untagged.
  - Such a frame came from the switch. The AP's own frames, its clients' included, leave as outgoing (`packet_type` 4), and don't count.
  - The socket's filter keeps:
    - broadcasts and multicasts: ARP, DHCP, OSPF, mDNS, IPv6 neighbour discovery;
    - frames to the AP's own MACs on the VLANs (0060), where the answers to a nudge come.

    Clients' unicast traffic is not read.
  - It listens in spells: once a minute, until every watched VLAN has been heard, and for at most 10 seconds. On busy VLANs, such as the lab's, a spell is over in well under a second.
- **A quiet VLAN is nudged before it is called silent.** A VLAN that wasn't heard in a spell gets three frames in the next one, from its own MAC:
  - a DHCP discover;
  - an ARP, from 0.0.0.0 as 0059's probes ask, to the gateway the VLAN's last DHCP answer named;
  - an echo to IPv6 all-nodes, from the link-local address its MAC makes.

  An answer comes in on the VLAN, so it counts as heard. No lease is taken.
- **A VLAN not heard for three minutes running, nudged meanwhile, is silent.** One heard within the last three minutes is present. One watched for less than three minutes is not known yet.
- **The VLAN transports of networks with a fallback are probed as before** (0061), with their leases, as switching needs. Their answers count as heard too.
- **LLDP, when the switch sends it.** The same socket takes LLDP frames. The prober keeps the newest:
  - the switch's name;
  - its port, and the port's description;
  - the native VLAN;
  - the VLANs it names, if it sends VLAN Name TLVs.

### What it reports, and what the manager shows

- **The state report's `uplink_vlans`:** each watched VLAN with:
  - whether the uplink carries it tagged;
  - its verdict (`present`, `silent` or `unknown`);
  - seconds since it was last heard.

  The unused `vlans` field goes. It was the report's list of VLANs seen on the uplink, which no agent ever sent.
- **The uplink's neighbour, `uplink_neighbor`:**
  - the switch's name;
  - its port, and the port's description;
  - the native VLAN;
  - the VLANs it names;
  - seconds since its last LLDP frame.
- **The manager judges each VLAN, with LLDP where it helps:**

  | Judgment | When | Says |
  |---|---|---|
  | **present** | heard within three minutes | — |
  | **missing** | silent, and the switch's LLDP names VLANs but not this one | The switch port doesn't carry VLAN N. |
  | **silent** | silent, and LLDP names no VLANs | Nothing comes in on VLAN N: it most likely isn't on the switch port. |
  | **quiet** | silent, but the switch's LLDP names it | The switch port carries VLAN N, but nothing on it answers. |

- **A warning, not a hold.** A missing VLAN is a fault on the switch, not in the AP's config, so the config is sent as before.
  - The AP's overview warns, one line for each such VLAN. The line says which VLAN, who needs it (the networks, ports and tunnels), and, from LLDP, which switch port to look at.
  - Interfaces › Ethernet shows, on the uplink's row, its neighbour and each VLAN's judgment.

## Consequences

- **Watching costs the AP little:**
  - one socket on the uplink;
  - a spell of at most 10 seconds a minute;
  - three frames a minute for each quiet VLAN.

  It takes no address, and sends nothing on a VLAN that is busy.
- **A missing VLAN shows within about three minutes** of the AP carrying it, or of its going.
- **A VLAN with nothing on it off the AP looks silent,** as there is nothing to hear and nothing to answer. Its clients can't reach anything off the AP anyway, so the warning holds.
- **The AP still can't see a VLAN it doesn't carry.** Nothing here tells what else the switch port carries, but LLDP's list.

## Not now

- **Checking a VLAN before anything uses it,** by carrying it on the uplink alone, with no port or SSID. Untested: whether the AP then sees it.
- **Moving a single-transport VLAN network's clients when its VLAN is missing.** That network has nowhere to go. Only the manager's warning helps.
- **Setting the switch's VLANs.** Aeolus manages APs.

## Lab checks

On OpenWrtnight on 2026-10-04, with the new prober and probe.uc installed by hand and four watches added to its plan by hand. Afterwards the v0.29.0 prober and the plan as rendered were put back.

- **VLANs 20 and 50,** which the AP carries and the switch sends, were present at the first spell, 5 seconds after the prober started.
- **VLANs 30 and 999 turned silent three minutes in, to the second,** as nothing came in on them. The AP doesn't carry 30, and 999 is on neither side.
  - At the next spell each got its nudge, tagged: a DHCP discover and an echo to IPv6 all-nodes.
  - No ARP went out, rightly, as neither had a DHCP answer naming a gateway.
- **LLDP:** with `lldp tlv transmit vlan-name` set on the Arista, the prober logged that the uplink is on homelab.symtus.com, port Ethernet6, which carries VLANs 1, 10, 20, 30, 50 and 1010. So the manager would judge 999 missing, and 30 quiet.
- **The new agent's state report,** run read-only, carried `uplink_vlans` and `uplink_neighbor`, and its ports no longer listed a veth end.
- **CPU,** in clock ticks over about 25 seconds:

  | Prober | Ticks |
  |---|---|
  | v0.29.0 | 6 |
  | the new one, watching VLANs 20 and 50 | 6 to 7 |
  | the new one, with the two silent VLANs as well | 18 |

  A silent VLAN keeps each spell open its full 10 seconds, reading the uplink's broadcasts, about half a percent of a core.
