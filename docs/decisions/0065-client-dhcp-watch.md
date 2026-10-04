# 0065. Watching clients' DHCP

- Status: Proposed
- Date: 2026-10-04
- Proposed by: Griff: the AP should tell when a client doesn't use DHCP, when DHCP is asked for and nothing answers, and when a request gets answers from more than one server. Written up by Claude
- Refines: 0035
- Resolves: 0035's open points, for v1: what is detected, and where expected servers are declared (nowhere, yet)

## Context

- **0035 has the AP watch DHCP and report what it sees,** as the half of DHCP observation that works where nothing relays DHCP to the manager. It left open what to detect, and with what thresholds.
- **Every Wi-Fi client's DHCP crosses the AP's Wi-Fi interface for its network:** the client's request comes in there, and the server's answer goes out there. The AP's own frames are told apart as before (0064).
  - On OpenWrtnight, `network.wireless status` names each interface's network: `phy0-ap1` and `phy1-ap2` carry Aeolus Lab (`aeolus_lab`), for example.
- **A wired client's DHCP isn't seen reliably.** Between two switch ports, unicast answers are switched in hardware, and never reach the CPU.
- **The AP knows when each client joined.** nl80211's station dump gives each client's connected time. The prober can ask for it with ucode's nl80211 module, which OpenWrt installs; checked on OpenWrtnight.

## Decision

### What the AP watches

- **Each Wi-Fi interface of an Aeolus network,** with a packet socket whose filter keeps only:
  - DHCP: IPv4 UDP to or from ports 67 and 68;
  - ARP, in from a client: how a client that doesn't use DHCP shows its address.
- **For each client on each network,** the prober keeps:
  - when it joined;
  - its last DHCP request, with its kind and transaction ID;
  - the answers to that request: each server's ID (option 54) and MAC, and the address offered;
  - the address it uses, from its DHCP ack or its ARP.
- The AP's own networks, which Aeolus doesn't manage, are not watched, as in 0064. Wired clients are left for later.

### What it finds

1. **A client that doesn't use DHCP.**
   - It joined while the prober watched, and asked nothing by DHCP within 60 seconds.
   - If it shows an address, it is reported with it: a static address, or one leased before it roamed in.
   - If it shows none, it is reported as having no address. That client is stuck.
   - A client that joined before the prober started isn't judged until it asks, or joins again.
2. **DHCP asked for, and nothing answered.**
   - A request is unanswered when no offer, ack or nak comes for its transaction ID within 10 seconds.
   - Each network's count of unanswered requests over the last 10 minutes is reported, with the clients that made them.
   - When every request in that time went unanswered, **DHCP isn't answering on the network**.
3. **More than one server answering.**
   - Two answers to one request from different servers, told by server ID, or by source address without one, are a duplicate. A server sending the same answer twice is just a retransmission.
   - Each network's servers are reported: ID, MAC, how many answers, and when last seen.
   - Each duplicate is reported with its servers and when it happened. A second server on a network is often a rogue one.
   - Which server is expected isn't declared anywhere yet. Any second server is reported, and the manager leaves it to the person to tell which.

### What it reports, and what the manager shows

- **The state report's `dhcp`:** for each Aeolus network on the AP, its servers, its unanswered requests, its duplicates, and its clients without DHCP. Lists are capped.
- **A change in what was found sends a report at once,** as the prober's verdicts do (0059): a network that stops answering, a second server, or a stuck client.
- **The AP's overview warns, one line each:**
  - "On Aeolus Lab, DHCP isn't answering: 3 requests from 2 clients in 10 minutes, none answered."
  - "On Aeolus Lab, two DHCP servers answer: 192.168.20.254 (28:e7:…) and 192.168.20.99 (f2:0d:…)."
  - "On Aeolus Lab, a client has no address: it joined 2 minutes ago and asked nothing by DHCP."
- **The Networks view's panel for each AP shows each network's DHCP compactly:** its servers, and its clients without DHCP with their addresses. A client with a static address is shown there, not warned about.

## Consequences

- **Little load.** A client asks for DHCP when it joins, and then every few hours. Its ARP comes with its traffic, but the filter keeps only ARP, and only what comes in from clients.
- **Clients' MACs and addresses reach the manager,** in the AP's state report, kept as long as reports are (0039).
- **Roaming clients:** a client roaming in from another AP may not ask for DHCP again. It is reported with the address it shows, as information, not a warning.

## Lab checks, to do

- A phone on Aeolus Lab: the four messages (discover, offer, request, ack) seen on its Wi-Fi interface, in and out, with the server's ID.
- A client on a test network on VLAN 999, which the switch doesn't carry: its requests reported as unanswered, and the network as not answering.
- A second server: by frames in the prober's tests. A real one on a test VLAN, only if Griff wants one.
