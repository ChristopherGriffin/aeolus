# 0068. The manager's DHCP listeners

- Status: Proposed
- Date: 2026-10-04
- Proposed by: Griff: start on 0035, the manager's side of DHCP observation (build order 4.4). Written up by Claude
- Refines: 0034, 0035

## Context

- **0035 has the manager listen two ways,** besides what the APs watch (0065):
  - **relay copies:** a site's DHCP relay gets Aeolus as an extra helper address, and the manager records which subnet each request came from;
  - **an option 224 listener** (0034): in DHCP scopes an admin chooses, option 224 points at the manager. An unconfigured OpenWiFi AP there tries to connect, which confirms what it is.
- **What a relay sends a helper:** only the clients' requests, each with the relay's address on the client's subnet (`giaddr`), and option 82 if the relay adds it. The servers' answers go back through the relay, not to its helpers. So relay copies can't show rogue servers; the APs' watch (0065) can.
- **What an OpenWiFi AP does with option 224,** read in OpenWiFi's AP source (`wlan-ap`, `feeds/tip/cloud_discovery`, as of February 2026):
  - The option is a string, `server[:port]`, with port 15002 by default.
  - Since November 2025, the AP first enrolls with OpenWiFi's public certificate server (`est.certificates.open-lan.org`, or one a CAA record of the server's name points to), using its factory certificate. Only if that works does it start its uCentral client, which connects to the server over TLS for a WebSocket, checking the server's certificate and name.
  - **So an AP knocks only if it reached the internet and enrolled,** and then rejects any certificate Aeolus shows. Its first TLS message, with the name it asked for, still arrives.
- **0033's constraint stands:** Aeolus's own APs never find the manager by options 43, 60, 138 or 224. The 224 listener exists only to catch OpenWiFi APs, in scopes an admin points at it.
- **The manager runs as the user `aeolus`, with no capabilities.** UDP port 67, where relays send, needs `CAP_NET_BIND_SERVICE`. Nothing listens on 67 on the manager host now.

## Decision

### The relay listener

- **The manager listens on UDP port 67 and never sends.** It takes only relayed requests, those with a `giaddr`. Broadcasts from the manager's own segment are counted as ignored, and not kept.
- **For each request it keeps:**
  - the subnet, by `giaddr`, and the relay it came from;
  - the client's MAC;
  - its host name, vendor class and parameter list, as the APs read them (0067);
  - the address it asks for or renews (option 50, or `ciaddr`);
  - option 82's circuit and remote IDs, when the relay adds them.
- **Each client is kept by subnet and MAC,** with when it was first and last seen, and the maker and guess the APs' clients get (0067). It is kept for 7 days after it was last seen.
- **What it finds:**
  - **A possible OpenWiFi AP:** a client whose parameter list asks for 138 and 224.
  - **A burst of new clients:** more than 64 MACs new to a subnet within a minute. It's one sign of DHCP starvation. 64 is a starting point; 0035 left the threshold open.
- **For each subnet:** the requests of the last 10 minutes, the clients seen, the new ones, and any finding.

### The option 224 listener

- **The manager listens on TCP port 15002,** with TLS and its own certificate. It never speaks uCentral.
- **Each connection is a knock.** It records:
  - the source address and time;
  - the name the client asked for (SNI) and its TLS versions;
  - the client's certificate subject, should it ever send one. An OpenWiFi AP won't, having rejected Aeolus's certificate first.

  Then it closes the connection.
- **A knock is matched to a device by its address,** from the relay copies (what the client asked for or renewed) or from an AP's clients report.
  - A matched knock makes the device a **confirmed** unconfigured OpenWiFi AP, with its MAC, maker and subnet.
  - An unmatched knock stays a knock from that address.
- **Knocks are kept for 30 days,** at most 1,000, the oldest dropped first.

### Where it shows

- **The APs page gets a "Detected, not enrolled" list** (0034): possible and confirmed OpenWiFi APs, each with its MAC, maker, subnet, the evidence, and when last seen. It also lists unmatched knocks.
- **The Org's Networks view gets a "DHCP from relays" panel,** beside 0065's "DHCP on each AP". For each subnet it shows the relay, the requests of the last 10 minutes, the clients seen and new, and any burst.
- **Both read from new admin API routes,** and the MCP adapter passes them through (0031).

### On the manager host

- **The service unit gains `CAP_NET_BIND_SERVICE`,** both ambient and as its only bounding capability, so it can bind port 67. Nothing else changes, and `aeolus-update` installs the unit.
- **Both listeners are on by default.** `AEOLUS_RELAY_LISTEN` and `AEOLUS_KNOCK_LISTEN` in `serve.env` move them, and `off` turns them off.

## Consequences

- **A site points its relays and its 224 scopes at Aeolus,** which is a change on their side. Aeolus changes nothing on the network.
- **Relay copies see wired clients too,** which the APs' watch (0065) can't. They don't see answers.
- **An OpenWiFi AP behind a site without internet access never knocks.** It can only be found as possible, by its fingerprint.

## Lab plan

- **Relay:** Griff adds `ip helper-address 192.168.20.60` to the Arista's VLAN 50 interface. VLAN 50's own server, on the segment, keeps answering. The prober's leases on VNI 50 and VLAN 50, renewed every few minutes, then show up as relayed requests from 192.168.50.0/24.
- **Option 224:** there's no OpenWiFi AP in the lab. A TLS connection from OpenWrtnight to port 15002, naming the manager, stands in. A real AP would also need the internet, to enroll first.

## As built

- **`internal/dhcpwatch`:**
  - `Parse` reads a relayed request: BOOTP or DHCP, with a relay address, from a client with a 6-byte MAC.
    - It refuses answers, unrelayed requests, unknown message types and options that run past the end.
    - Host names and vendor classes are kept only as printable text of at most 64 bytes, a trailing NUL dropped.
    - Option 82's IDs are kept as text when printable, else as hex, at most 32 bytes.
  - **The book** keeps clients by subnet and MAC, and counts requests and new clients by the second, so a flood takes no more room than a trickle.
    - At most 256 subnets, and 4,096 clients a subnet; beyond that, new clients are counted and not kept.
    - **A burst counts only discovers from clients new to the subnet.** A relay newly pointed at the manager copies every renewal on the subnet, from clients new to the manager, and those aren't a burst.
  - **Saving:**
    - Clients are saved every half minute and when the manager stops.
    - Knocks are saved at once.
    - Everything is trimmed hourly.
- **The conditions store** gains `relay_clients` and `knocks` (schema version 3).
- **The knock listener:**
  - It runs at most 16 handshakes at once, each with 10 seconds to finish.
  - It reads the client hello's name and versions, and asks for, but never checks, a client certificate.
  - A source's knocks for the same name within a minute are counted as one.
- **`serve`:**
  - `newServer` returns a start function. The listeners start with the API and stop before the store closes.
  - **The unit gains `CAP_NET_BIND_SERVICE`,** ambient and bounding, and nothing else.
- **The API:**
  - `GET /v1/dhcp/relayed` and `GET /v1/detected` need a viewer's role at the Locations root.
  - The MCP adapter adds `get_relayed_dhcp` and `list_detected`.
- **The UI:**
  - The APs page lists "Detected, not enrolled" when there's anything to list.
  - The Org's Networks tab shows "DHCP from relays", with each subnet's clients by kind, and its findings: a burst, and possible OpenWiFi APs.
- **Checked:**
  - **Tests:** parsing, the book's counts, bursts, saving and trimming, both listeners over real sockets, and `serve` starting them and saving what they heard.
  - **The API:** a client possible by its fingerprint and confirmed by a knock, a Wi-Fi client confirmed through its AP's report, and an unmatched knock.
  - **The harness:** both views, with an Edgecore AP's fingerprint and knock, and a burst of 80 discovers.
- **The lab plan above waits for the release.**

## Not now

- **Sending option 224, or any change to a site's DHCP:** the admin sets it in the scopes they choose (0034).
- **Rogue servers seen by the manager:** relays don't copy answers; the APs see them (0065).
- **Adopting a detected AP** (0034): turning off its uCentral client and installing the agent.
- **Dismissing a detection,** and declaring each subnet's expected servers (0035).
