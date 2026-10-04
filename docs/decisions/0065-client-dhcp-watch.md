# 0065. Watching clients' DHCP

- Status: Accepted
- Date: 2026-10-04
- Proposed by: Griff: the AP should tell when a client doesn't use DHCP, when DHCP is asked for and nothing answers, and when a request gets answers from more than one server. Written up by Claude, and accepted with the merge
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

## As built

- **The prober refreshes the interfaces every 10 seconds, and after each apply:**
  - The Wi-Fi interfaces of Aeolus's networks are found from `network.wireless status`, with one socket each.
  - Each interface's stations are read from nl80211, which also says when each joined.
  - An AP without ucode's nl80211 module still watches DHCP. It just doesn't judge which clients don't use it.
- **A request counts only when it comes in from a client.** Found live on OpenWrtnight: the bridge floods the segment's broadcast DHCP out to the Wi-Fi clients too. So with no client on aeolus_50, the AP's own probes' leases and other hosts' requests crossed its interfaces, going out.
  - An answer still shows its server, whoever it answers on the segment, so each network lists its servers even before a client asks.
  - Only a client's own requests count as answered or unanswered, or as answered by more than one server.
- **Judged every 5 seconds, when the prober writes its results:**
  - a request waits 10 seconds for an answer;
  - requests are counted over 10 minutes;
  - servers and duplicates are remembered for an hour;
  - a client gets 60 seconds to ask.
- **Moving between radios isn't joining** (v0.34.1). A client that moves between the AP's radios on one network, or rejoins within 15 seconds of when a scan last saw it, keeps its lease and needn't ask again. Its stay is judged from when it first joined, and it isn't judged while off them. One that rejoins later is watched afresh, even if it was back before a scan missed it. The scans are 10 seconds apart, so a move of a second or two always counts as staying.
  - Found with Griff's phone on 2026-10-04 (0067): it joined Aeolus Lab on 5 GHz and got its lease, then moved to 2.4 GHz a minute later. v0.34.0 took the move for a new join and reported "asked nothing in 60 s, and uses 192.168.20.81": static.
  - A request is counted as since joining when it comes up to 2 seconds before the join as nl80211 gives it. That is to the second, and the client asks within a second of joining.
  - A client roaming in from another AP still looks static: this AP can't know it asked elsewhere. With more than one AP, the manager will need to look across them.
- **Logged once each:** a network where nothing answers, a second server answering one request, and a client that asked nothing.
- **The agent sends a report at once** when a network stops or starts answering, when a duplicate appears, or when a client without DHCP comes or goes.
- **The UI:**
  - the AP's overview gets a warning line for each problem;
  - the Networks view gets a "DHCP on each AP" panel, listing each network's servers, its requests of the last 10 minutes, and its clients without DHCP.

## Lab checks

- **On OpenWrtnight on 2026-10-04,** with the new prober run by hand:
  - It opened sockets on the six Wi-Fi interfaces of Aeolus Lab, tedt and aeolus_50, and on none of the AP's own.
  - On aeolus_50, with no clients, it listed the segment's server, 192.168.50.254, from answers flooded to the Wi-Fi interfaces, and counted no client requests.
- **probe.out** covers:
  - the parser on a discover, a renewal, an offer and an ARP;
  - the ARP reader;
  - the new filter's jumps;
  - each judgment's limits.
- **With v0.32.0, and Griff's phone on Aeolus Lab, on 2026-10-04:**
  - **It joined** on the 5 GHz radio. Its request was answered by 192.168.20.254, VLAN 20's server, which was listed. The phone wasn't counted among the clients without DHCP. It joined with a private MAC, which the AP reports as it is.
  - **DHCP answers to Aeolus Lab's interfaces were dropped** by a temporary nft rule in the bridge's forward hook, instead of a test network on VLAN 999, so no SSID changed. The phone tried to join three times.
    - The rule dropped 96 answers.
    - The prober counted 6 unanswered requests and none answered, all from the phone. It logged that nothing answers at 20:05:11.
    - The manager's 20:05:39 report carried it, for the overview's warning.
  - **With the rule gone,** the phone joined again and got its answer. The prober logged that DHCP is answering again at 20:07:46, and the warning cleared.
  - Other hosts' answers on VLAN 20, flooded to tedt's interfaces, listed the same server for tedt, with no client requests counted. That is as built.
- **Not checked live:**
  - **Two servers answering:** only by the prober's test frames, as a second server on VLAN 20 would have reached Griff's own network.
  - **A client that doesn't use DHCP:** a device with a static address joining Aeolus Lab should show as static within a minute.
