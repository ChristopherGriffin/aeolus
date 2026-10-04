# 0061. Switching a network between its transports

- Status: Accepted
- Date: 2026-10-03
- Proposed by: Griff: step 2 of 0059, a network moves to its fallback when its primary fails, when set to, and only to one known to work; written up by Claude, and accepted with the merge of part 1
- Refines: 0018, 0020, 0022, 0054, 0059, 0060

## Context

- **Step 1 of 0059 tells whether a tunnel works.** Each AP probes its tunnels, and its verdicts (`up`, `down`, `unverified`) reach the manager within a poll.
- **Nothing acts on them yet:**
  - a network's fallback is rendered, but never used;
  - a VXLAN fallback's tunnel is never started (0054);
  - a VLAN fallback isn't probed.
- **0059 set the rules:**
  - each network chooses `report` (the default) or `automatic`;
  - the AP switches only to a transport that answers;
  - one transport at a time, break before make;
  - switching back follows 0022's failback and hold-down;
  - the switch doesn't restart the Wi-Fi.

  This record says how.

## Decision

### What a network sets

- **`transport.switching`:** `report` or `automatic`; unset, it is `report`. `report` probes and tells, and never moves the network. `automatic` also switches, under the rules below.
- **0022's fields apply as written:**
  - `transport.ha`: keep the standby started and probed, so a switch is a re-attach;
  - `transport.failback`: `revertive`, the default, or `equal`;
  - `transport.holddown`: seconds the primary must answer before a revertive switch back, default 300.
- **The config check holds** (0029):
  - `automatic` without a fallback;
  - a switching network whose tunnel's VNI another network or a tunnel port uses at the AP, since the tunnel will be taken in and out of the network.

### What the AP probes

- **Both transports of a network with a fallback:**
  - **A VXLAN transport** as in 0059 and 0060.
  - **A VLAN transport:**
    - sent tagged straight on the uplink, so no local port or Wi-Fi client sees it;
    - from the VLAN's own MAC (`06:…`, 0060), with a lease of its own;
    - asking the gateway that lease names, or the probe address set.
    - The answers come in with their tag taken off, and the VLAN in the packet's metadata, which the prober reads with `recvmsg`. Checked on OpenWrtnight: a discover and an ARP on VLAN 20 were answered, the VLAN read as 20.
- **Without HA mode, a VXLAN standby stays stopped** until the primary goes down. Then the AP starts it, probes it, and uses it only once it answers.

### How a switch is made

- **A network with a fallback gets a bridge of its own,** and its SSIDs stay on it whichever transport carries it:
  - its name is `br-n` and 8 hex digits made from the network's name, inside Linux's 15 characters;
  - the network's interface is on that bridge.
- **Neither transport is in that bridge in the rendered config.** The prober attaches the active one at run time, through netifd: `add_device` and `remove_device` on the network's interface, with `link-ext` false. With the default, netifd takes the device for one someone else manages and waits for word that it exists. For a device that already exists, that word never comes, so the device is never attached.
  - **A VXLAN transport is attached as its tunnel device itself,** which is that network's alone at the AP.
  - **A VLAN transport is attached through a veth pair:**
    - one end is a port of the uplink bridge, untagged in that VLAN;
    - the other end is what joins the network's bridge;
    - the installer adds `kmod-veth`.
    - netifd makes the pair from a `veth` device section, whose own end must be the VLAN end. That end keeps the pair alive, since releasing it deletes the pair; its peer is the network's end. Checked on OpenWrtnight: the VLAN end joined the uplink bridge untagged in VLAN 20, the way a VLAN network's Wi-Fi does, and stayed through a network reload.
- **The prober attaches the primary when it starts.** A device attached with `add_device` stays through a network reload; checked on OpenWrtnight. The prober still checks the bridge's members after one, and attaches the active transport again if it went.
  - The render check holds that neither transport is in the bridge's configured ports, so a reload can never leave both attached.
  - Which transport is active is state, not config (0020).
- **Break before make** (0018): the failed transport comes out of the bridge before the other goes in. A VLAN 50 primary and a VNI 50 fallback are one broadcast domain, so both at once would be the loop 0018 warns of.
- **Clients stay associated through a switch.** Their traffic moves with the bridge's member. On a fallback that reaches the same segment, a client keeps its address.

### When

- **Away from the primary:**
  - when its verdict turns `down` (three probes missed in a row, 0059);
  - only to a fallback whose verdict is `up`;
  - with HA mode, the fallback has been probed all along;
  - without it, the fallback is started and probed first. The network stays on the failed primary, down, until the fallback answers, or the primary comes back.
- **Back,** with `revertive`, once the primary has answered for `holddown` seconds without a miss. With `equal`, the network stays put until the fallback fails in turn.
- **Never to a transport that is `down` or `unverified`.** The AP reports that it could not switch, and why.
- **A switch is logged** on the AP and reported: from which transport to which, why, and when.

### What the manager shows

- **Each network's transports, on each AP:**
  - which transport is active;
  - each transport's verdict;
  - the last switch and its reason.

  This uses the state report's `transports`, which gains `unverified` and the last switch.
- **The Networks and Tunnels views say so.** A network that has switched to its fallback is a warning on the AP's overview.
- **The network editor offers switching beside the transports.** Choosing `automatic` shows HA mode, failback and hold-down.

## Consequences

- **A network with a fallback is rendered differently** (a bridge of its own), so applying this reloads the network once and restarts that network's Wi-Fi once on each AP, after which switching never does.
- **Each switching network costs an AP a bridge, and a veth pair for a VLAN transport,** and its fallback a DHCP lease (0060).
- **Report mode costs nothing new** but the VLAN fallback's probe and lease.
- **0058's rule that a port can't carry a network's fallback VNI stays.**

## Lab checks

- **Attaching the veth's network end at run time, and taking it out:** checked on OpenWrtnight on 2026-10-04, with a test veth pair and two test bridges, so nothing reached a real segment. The pair's configured end (`lv20`) was in one bridge, and its other end (`lv20n`) was attached to the second bridge and taken out again.
  - With `add_device`'s default `link-ext`, nothing happened: netifd listed `lv20n` as a member, but not present, and left it down and out of the bridge.
  - With `link-ext` false, `lv20n` joined the bridge and came up.
  - Taking it out left the pair alone, as the configured end holds it: both ends kept their device indexes.
  - Attached again, it stayed through a network reload.
- **A switch, break before make,** between that veth end and a VXLAN tunnel device (a dynamic interface on VNI 999, which the Arista doesn't map). The bridge held one of them at a time, both ways. Both devices kept their indexes, and the tunnel stayed up.
- **The three IoT clients stayed associated through it all,** across two network reloads.

## As built: part 1, report mode

Built in two parts. Part 1 renders a network with a fallback as described above, attaches its primary, and probes and reports both transports. Nothing switches until part 2, which brings `transport.switching`, HA mode, failback and hold-down.

- **Names:** a network's hash is FNV-1a of its name, as 8 hex digits.
  - Its bridge is `br-n<hash>`, in a section `aeolus_n<hash>`.
  - Its veth pairs are in sections `aeolus_n<hash>_p` and `_f`, with the VLAN end `av<p|f><hash>` and the network end `an<p|f><hash>`.
  - Network names `n` and 8 hex digits, alone or followed by a hyphen, are reserved.
- **The render:**
  - The network's bridge takes the primary's segment MAC (0060).
  - A tunnel of such a network has no bridge of its own, and the MSS clamp follows it into the network's bridge.
  - The veth's VLAN end is a port of the uplink's bridge, untagged in its VLAN. Aeolus takes these ends out of every bridge and VLAN at each render, and puts back those still wanted.
  - The prober's plan is a `switch` section, `aeolus_n<hash>`, naming the bridge, each transport's device, and its VNI or VLAN.
  - Each VLAN transport gets a `probe` section, `aeolus_vlan<N>`: on the uplink, `tagged` as the uplink carries the VLAN, from the VLAN's MAC, every 30 seconds.
- **Held:**
  - an AP whose prober can't run (no ucode-mod-socket);
  - a VLAN transport on an AP without kmod-veth, which the installer now adds;
  - a VNI a port carries that is a tunnel of a network with a fallback.
- **The prober:**
  - It attaches with `add_device` and `link-ext` false. It looks every 2 seconds, so whatever took the primary out, a reload or a tunnel made again, is put right at once, and it takes the fallback out first if it finds it in.
  - The agent sends it a SIGHUP after each apply, which has it read its plan at once.
  - A VLAN probe sends its frames tagged on the uplink and reads the answers' VLAN from `PACKET_AUXDATA`. A VLAN the uplink's bridge doesn't carry can't be probed, as the switch chip drops it; the render probes only VLANs the uplink carries.
  - It reports each network's active transport and both verdicts. The agent sends them as the state report's `transports`, with the VLAN probes as `vlan_probes`.
- **Checked on OpenWrtnight,** with the prober run by hand:
  - VLAN 20 probed tagged on `wan` leased 192.168.20.80 and found the gateway in 6 ms, at about 0.15% CPU.
  - On a test bridge with two test tunnels, the prober took out a fallback it found in the bridge, put the primary in, put it back 3 seconds after it was taken out by hand, and swapped the two within a second of a changed plan and a SIGHUP.
