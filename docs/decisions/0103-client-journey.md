# 0103. A client's journey, first part

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Griff, a dashboard that follows each client coming online, stage by stage, and pinpoints every issue it has. This is a first part of it, written up by Claude
- Refines: 0039, 0065, 0066, 0067

## Context

- **Each AP's state report lists every Wi-Fi client on it** (0066): signal, rates, retries, how long it has been connected, its address, its DHCP verdict (0065) and its 802.11 features (0067).
- **The manager keeps these reports for 30 days** (0039).
- **A client's history is already there,** spread across the reports of whichever APs it was on.

## Decision

- **`internal/journey` builds one client's journey from those reports:**
  - **Sessions:** each on one AP, band and network, unbroken while it appears in reports no more than 12 minutes apart.
    - Each has its start: the first report less the time it says it had been connected, but never before the session it follows ended.
    - Each has its end, and its signal (lowest, highest, mean), mean rate, retries, address and DHCP verdict.
  - **Roams:** a session on another AP that starts within 12 minutes of the last.
  - **Issues:**
    - a mean signal under −75 dBm;
    - more than one frame in five sent again;
    - a mean rate under 30 Mbit/s on 5 or 6 GHz;
    - no DHCP, with no address, or a static address;
    - moving back and forth between two APs, four times or more within half an hour.
- **`GET /v1/clients/{mac}?hours=` gives it,** 24 hours unless set and at most 720, from the reports of the APs the caller may view. The database finds the reports that hold the MAC, so only those are read. The MCP adapter offers it as `get_client_journey`.
- **On the Clients tab, a client's name opens its journey above the table,** with the last week a click away.

## Consequences

- A session's edges are known only to within a report, every few minutes.
- Steps between reports go unseen: the association, the keys, DHCP's four messages. The journey Griff wants, each stage of coming online, needs the AP to record them as they happen: hostapd's events and the DHCP watch's (0065). That is the next part.
