# 0035. DHCP observation: vendor-neutral detection and network watch

- Status: Accepted (details marked open)
- Date: 2026-09-26
- Decided by: Griff
- Supersedes: the DHCP-lease method in 0034 (the listener on option 224 stays)

## Context

0034 read DHCP leases through the DHCP server's API. That ties Aeolus to one DHCP server product and needs a credential for it; not everyone runs Technitium.

## Decision

Aeolus observes DHCP itself, in two ways, and uses both:

- **APs listening.** The Aeolus agent watches DHCP on every VLAN its AP carries and reports what it sees (0020). This covers networks with no DHCP relay, which is why both ways are needed.
- **Relay copies.** Where a site relays DHCP, the relay gets Aeolus as an extra helper address. The manager listens passively, never answers, and records which subnet each request came from.

What the observations are used for:

- **Detecting OpenWiFi APs.** An OpenWiFi AP's DHCP request asks for options 43, 60, 138 and 224 and usually carries a vendor class. That fingerprint makes a **possible** OpenWiFi AP. A knock on the option 224 listener (0034) makes it **confirmed** unconfigured.
- **Watching the network (Griff).** The same listening detects:
  - **rogue DHCP servers**, meaning offers from servers not expected on that VLAN;
  - **IP-address attacks**, such as ARP spoofing of a gateway, duplicate IPs, and DHCP starvation.

  These are reported as observed conditions (0009) and shown as alerts.
- **No dependency on any DHCP server's API,** and no credential for one.

## Open

- Which address attacks v1 detects, and their thresholds. For clients' DHCP, 0065: a client not using it, requests unanswered, and more than one server answering.
- Where the expected DHCP servers per VLAN are declared (a field in the schema).
- Whether APs later act on what they see, for example by dropping rogue DHCP offers on the wireless side, or only report it.

## Consequences

- Detection moves mostly into the AP agent (M5). The manager side, the relay listener and the option 224 listener, is small.
