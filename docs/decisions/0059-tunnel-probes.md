# 0059. Tunnel probes, the loop guard, and switching transports

- Status: Proposed
- Date: 2026-10-03
- Proposed by: Griff: an agent on the AP that probes every tunnel, to keep it open and to prove it works with no client on it; it always reports, and switches transports when set to, but only to one known to work; written up by Claude, with the defaults Griff agreed
- Refines: 0018, 0020, 0022, 0054, 0058
- Resolves: 0020's open point on judging a VXLAN transport healthy

## Context

- **A quiet AP drops off the concentrator.** The Arista learns the AP's tunnel end only from traffic the AP sends (`vxlan flood vtep learned data-plane`), and forgets it when none comes. After that, nothing it floods reaches the AP. A client that is idle but still connected can't be reached from the wired side until it sends something itself.
  - OpenWrtnight's `aeolus_50` had sent and received nothing since it came up.
  - One 60-byte probe from the AP, and the Arista began flooding VNI 50 to it: 136 frames within seconds.
- **"Up" says little.** A tunnel's interface is up as soon as it is configured. It doesn't show whether the concentrator answers, or whether the VNI is mapped there. Today a network whose tunnel is broken just goes quiet.
- **Switching needs a verdict.** 0020 lets an AP move a network to its fallback on its own, and 0022 adds HA mode and failback, but nothing yet says a transport has failed, so nothing switches. A fallback is rendered, but never started (0054).
- **A tunnel port can close a loop that nothing breaks** (0058): STP doesn't cross a tunnel.

## Decision

### Probes

- **The agent probes every transport the AP runs, every interval:** each VXLAN tunnel, each VNI on it, and a network's VLAN fallback. A tunnel's probes double as its keepalive.
- **Two checks, reported separately,** so a failure says where it is:
  - **Underlay:** an ICMP echo to the concentrator's address, over the management interface. Says whether the AP can reach the concentrator at all.
  - **Overlay:** a probe inside the segment. Says whether traffic actually crosses the tunnel, or the VLAN, and comes back.
- **The overlay probe goes only toward the far side:**
  - A VXLAN probe is sent straight on the tunnel device `aeolus_<VNI>`, not on its bridge, so no Wi-Fi client or tunnel port sees it.
  - A VLAN probe is sent tagged straight on the uplink.
  - Only an answer that comes back in on that same device counts. A Wi-Fi client of this AP can't vouch for the tunnel.
- **What the overlay probe asks:**
  - **A probe address, if one is set:** an address on the segment, normally its gateway. It is asked by ARP from 0.0.0.0, as a host checks an address before using it, so the AP needs no address of its own on the segment.
    - On OpenWrtnight, the Arista's VLAN 50 address, 192.168.50.1, answers such a probe, and so does a host on that VLAN.
    - The first probe after a long silence went unanswered, while the Arista was still learning the AP; every probe after it was answered. That is one reason a single miss is not `down`.
  - **Otherwise, any IPv6 neighbor:** an echo to all-nodes (`ff02::1`) from a link-local address. Any host on the far side with IPv6 answers. Linux, macOS and phones answer by default (OpenWrtnight does too); an Arista SVI needs `ipv6 enable`.
- **The verdict for each transport:**
  - **`up`:** the overlay answered within the last three intervals.
  - **`down`:** the underlay doesn't answer, or the overlay answered before and has now missed three probes in a row.
  - **`unverified`:** the underlay answers, but nothing on the segment has ever answered. The AP can't tell broken from quiet. This is reported with the hint to set a probe address, and it never triggers a switch.
- **The fields:**
  - **`concentrators.<name>.probe_interval`:** seconds, 5 to 300, default 30, settable per tunnel. A VLAN fallback is probed every 30 seconds.
  - **`probe`:** the probe address, on a network's transport (`transport.primary.probe`, `transport.fallback.probe`) and on a tunnel port's VNI (`ports.<name>.vxlan.<vlan>.probe`). Where a network and a port use the same VNI, the AP asks every address set for it.
- **How the agent does it:**
  - A small ucode process, the prober, runs under procd beside the agent, so probes keep their rhythm while the agent is applying a config.
  - It sends raw frames through ucode's socket module, `ucode-mod-socket`, which the installer adds, as it added `kmod-nft-bridge` (0054). OpenWrtnight has `ping` but no `arping`.
  - A packet socket bound to `aeolus_50` sends a frame straight into the tunnel, and sees only what comes back in on it. The answer comes in addressed to the bridge, not to the tunnel device, and it still counts. Checked on OpenWrtnight.
  - It writes its latest results to `/var/run/aeolus/`, and the agent puts them in each state report.

### The loop guard

- **On each tunnel port, every 2 seconds,** the prober sends a small frame of its own: a locally administered multicast address, IEEE's local experimental EtherType `0x88B5`, and a nonce of its own.
- **If that frame comes back in on any other device** (the tunnel, the uplink, or another port), the segment loops.
- **Then the agent turns that tunnel port off** and reports which VNI looped and where the frame came back.
- **The port stays off until its settings change or the agent restarts.** Turning it back on by itself would restart the storm. The Interfaces view says so.
- **Wi-Fi networks get no guard.** A client can't bridge two SSIDs together in normal use.

### Switching

- **Each network sets `transport.switching`:**
  - **`report`, the default:** the AP probes and reports, and never moves the network by itself. Updating Aeolus never starts a network switching on its own; set `automatic` where you want it.
  - **`automatic`:** the AP also switches, under the rules below. This is 0020's "the AP switches on its own", now chosen per network.
- **A down transport is always reported,** in either mode, and whether or not the network has a fallback.
- **The AP switches only to a transport that answers:**
  - With HA mode on (0022), the standby is started and probed all along, but not attached, so the AP knows before switching that it works.
  - Without HA mode, the AP starts the standby when the active transport goes down. It attaches the standby only once it answers.
  - The AP never switches to a standby that is down or unverified. It reports that it couldn't.
- **One transport at a time, break before make** (0018, 0020). The AP detaches the failed transport before it attaches the other. A VLAN 50 primary and a VNI 50 fallback are one broadcast domain, so the two attached together would be exactly the loop in 0018.
- **Switching back** follows 0022's `failback`:
  - **Revertive:** back to the primary once it has answered steadily for `holddown` seconds, default 300.
  - **Equal:** stay on the active transport until it fails.
- **How the switch is done without restarting the Wi-Fi:**
  - A network with a fallback gets a bridge of its own, which its SSIDs stay on.
  - Neither transport is in that bridge in the config Aeolus renders. The agent attaches the active one at run time, through netifd's `add_device` and `remove_device` on the network's interface, which OpenWrtnight's netifd has.
  - After any reload, the agent attaches the active transport again.
  - So clients stay associated through a switch. The render check holds that neither transport is in the bridge's configured ports, so both can never be attached at once.
- **Which transport is active is state, not config** (0020). The AP reports it, and the manager shows it, with the reason for the last switch.
- **0058's rule that a port can't carry a network's fallback VNI** stays as it is.

### What the manager shows

- **The Tunnels view, for each transport:** the underlay result, the overlay verdict, which address answered and its round trip, when it last answered, and whether it is active or standing by.
- **Also shown:** a tunnel port turned off by the loop guard, and the last switch, with its reason.
- **A transport that is down** shows as a warning on the AP's overview too. It is a condition the AP reports, not a config problem, so it holds nothing.

## Consequences

- **The concentrator keeps the AP in its flood list,** so idle clients stay reachable from the wired side.
- **A broken tunnel names its broken half:**
  - "can't reach 1.1.1.2" is the underlay;
  - "1.1.1.2 answers, VNI 50 doesn't" is the VNI, most likely not mapped at the concentrator.
- **Each probe is a few small frames per VNI per interval.** Nothing goes to Wi-Fi clients. The loop guard adds one small frame every 2 seconds per tunnel port.
- **APs need the new agent files, and `ucode-mod-socket`.**
- **A network with a fallback changes how it is rendered** (a bridge of its own), and so needs one network reload on each AP.
- **Built in two steps:**
  1. Probes, keepalive, the loop guard and the reporting.
  2. Switching.

  Step 1 is useful on its own, and step 2 needs step 1's verdicts.

## Open

- Whether the Arista's VLAN 50 interface has IPv6 on, so it answers the all-nodes echo when no probe address is set. Checked while building step 1; until then, VNI 50's probe address is 192.168.50.1.
