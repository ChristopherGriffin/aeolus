# 0049. Multicast to unicast, per network

- Status: Accepted
- Date: 2026-10-02
- Proposed by: Griff, among the network settings for the Networks tab (0047); written up by Claude and accepted with the merge
- Refines: 0006, 0039, 0048

## Decision

- **A network can turn multicast-to-unicast on or off** (`network.*.multicast_to_unicast`).
  - **On:** the AP sends all multicast (ARP, IPv4, IPv6) to each client as unicast. This is hostapd's `multicast_to_unicast` on the network's interfaces.
  - **Off:** the AP converts none.
  - **Unset:** the option is left out, and OpenWrt's default holds. On 25.12, that converts only the multicast groups clients joined, which the bridge learns by IGMP/MLD snooping.
- **The agent renders it** on each of the network's interfaces. The render check holds it to the intent, including leaving it out when unset (0039).
- **It is edited on the Networks tab** like any network field (0048).

## Consequences

- Casting and mDNS discovery, which rely on multicast, can be made dependable on a busy network, at the cost of one copy per client.
- An AP still running an agent older than this setting renders without it. The render check then refuses that config, and the AP keeps what it runs until its agent is updated.
