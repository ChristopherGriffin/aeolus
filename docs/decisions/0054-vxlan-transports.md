# 0054. VXLAN transports on the AP

- Status: Accepted
- Date: 2026-10-03
- Proposed by: Griff: VXLAN tunnels for networks, vendor neutral, so anything can terminate them; written up by Claude and accepted with the merge
- Refines: 0018, 0020, 0022, 0023, 0037, 0039, 0040, 0053

## Context

A network's transport can already be VXLAN in the schema: a concentrator from the library and a VNI (0018, 0023). The manager composes it, leaving out a concentrator that is not allowed at the AP's location (0037), and the render check has a first rule for it (0039). The agent has refused it until now ("VXLAN transports come in M5 part 2"). This is step 2 of the Interfaces plan in 0053.

OpenWrt builds a VXLAN tunnel through the `vxlan` package's netifd handler. Its Linux device takes the name of its interface section, and Linux allows 15 characters. A kernel keeps one VXLAN device per VNI and UDP port.

## Decision

- **The agent renders a VXLAN transport as a tunnel to the concentrator, bridged with the network's Wi-Fi:**
  - **The tunnel:** an interface `aeolus_<VNI>` with proto `vxlan`, or `vxlan6` for an IPv6 concentrator. It has:
    - the concentrator's address as its peer, and its port;
    - the VNI as `vid`;
    - the concentrator's MTU;
    - `tunlink` set to the AP's management interface.

    Its device has the same name, which fits in 15 characters for every VNI.
  - **Its own bridge:** `aeolus_<VNI>_br`, named `br-vx<VNI>`, with the tunnel as its only wired port. It is kept even while the tunnel is down, so the Wi-Fi can join it.
  - **The network's interface** `aeolus_<network>` sits on that bridge, and its Wi-Fi joins it, as with a VLAN (0040).
  - **The tunnel never shares a bridge with the uplink.** A VNI that lands on a VLAN the uplink also carries cannot loop through the AP (0018).
- **The tunnel's local end is the AP's address on its management interface,** whatever that is now: assigned by DHCP or static. This refines 0018, which took it from the Locations tree.
- **The primary runs, and the fallback waits,** until switching is built (0020, 0022):
  - the network's interface uses its primary transport;
  - a VXLAN fallback is rendered but not started;
  - a VLAN fallback's `bridge-vlan` is rendered, as now;
  - neither is attached. A network whose primary tunnel cannot reach its concentrator is down at that AP until switching comes.
- **One VNI, one network, per AP.** Tunnels are named by VNI, and Linux allows one per VNI and port. The config check (0029) holds:
  - two networks on the same VNI at an AP, which would join them into one;
  - a network whose primary and fallback use the same VNI, at two concentrators. Switching can later move one tunnel between them;
  - a concentrator whose address is a name, not an IP address, at an AP that uses it. A tunnel needs an address, and names do not resolve reliably on an AP (0040).
- **A network's name starts with a letter, and is not `vlan<number>` or `port-<anything>`.**
  - Those names are kept for Aeolus's own sections on the AP (`aeolus_<VNI>`, `aeolus_vlan<id>`, `aeolus_port_<name>`), which then never collide with a network's.
  - The schema refuses other names. Every existing network's name fits, and the UI already starts a name with a letter.
- **MTU and MSS:**
  - **The problem:** below 1500, a Wi-Fi client's full-size frames cannot cross the tunnel's bridge, and a bridge cannot tell the client so.
  - **TCP:** the agent clamps TCP MSS on each such bridge to the MTU less 40 for IPv4, and less 60 for IPv6. It does so in an nftables bridge table of its own, made from the rendered tunnels, which fw4 loads through a firewall include, `aeolus_clamp`.
  - **The package:** the clamp needs `kmod-nft-bridge`. A tunnel below 1500 on an AP without it is refused.
  - **Other traffic:** larger non-TCP packets still drop. Where it can, the network's DHCP server should give clients the MTU (option 26). That is outside Aeolus.
- **The firewall lets each tunnel in.** A rule `aeolus_vxlan_<VNI>` accepts the tunnel's UDP port from its concentrator's address, on the management interface's zone. A zone that rejects input by default then does not drop it.
- **What Aeolus owns grows** (0040): sections named `aeolus_` in the `firewall` package, which joins the packages the agent renders.
- **The installer adds `vxlan` and `kmod-nft-bridge`,** about 150 KB with their kernel modules.
  - As with usteer and snmpd (0050, 0052), the agent never installs packages.
  - On an AP without `vxlan`, a config asking for a tunnel is refused, and the AP reports what it lacks.
- **The render check (0039) holds:**
  - **the tunnel:**
    - its proto for the address's family;
    - its peer, port, VNI and MTU;
    - a `tunlink` naming an interface that exists;
    - started for a primary, and not for a fallback;
  - **its bridge:** it carries the tunnel, and the network's interface is on it;
  - **the clamp and the firewall rule:** the include whenever the MTU is below 1500, and the rule.
- **The agent reports its tunnels** in each state report. For each:
  - its network, and whether it is the primary or the fallback;
  - the concentrator's address, the port, the VNI and the MTU;
  - whether it is up, standing by, or missing.

  A network's transport health stays `unknown` until health checks come with switching.
- **The UI:**
  - **Interfaces › Tunnels** shows each AP's tunnels live, with the concentrator's name from the library.
  - **A network's VXLAN transport is picked from lists,** no longer typed in: a concentrator available where it is being edited, then a VNI from that concentrator's labeled list (0023). The preview shows any AP that cannot use it.
- **Each later step gets its own decision:**
  - switching between transports, with HA mode and health checks (0020, 0022);
  - standalone tunnels carrying a port or a VLAN (step 3);
  - editing the library in the UI (step 4).

## Consequences

- **The path in between needs no VLAN.** A network can reach its concentrator over any routed path, and nothing between them needs to carry its VLAN.
- **The concentrator must accept each AP as a peer.** Aeolus never configures it (0018), and anything that terminates VXLAN will do.
  - Many VXLAN endpoints list their peers: a flood list or static peers. An AP whose management address comes from DHCP should then have a reserved address.
  - Others learn their peers from the traffic itself, and need no list. The lab's Arista does, with `vxlan flood vtep learned data-plane`.
- **Changing a VXLAN network reloads the AP's network and Wi-Fi.** The automatic revert covers a change that cuts the AP off (0008).
- **APs installed before this need `vxlan` and `kmod-nft-bridge` installed by hand,** plus the new agent.
- **The lab:** OpenWrtnight's tunnels end at the Arista at 1.1.1.2, port 4789, which maps VNIs 10, 20 and 50 to VLANs 10, 20 and 50. Its library entry has an MTU of 1450, as the AP's own uplink carries 1500.
