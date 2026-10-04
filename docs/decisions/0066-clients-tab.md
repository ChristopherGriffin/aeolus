# 0066. A Clients tab

- Status: Proposed
- Date: 2026-10-04
- Proposed by: Griff: a Clients tab beside Hardware, Networks, Interfaces and System. The menus may be nested or moved later; a tab is fine for now. Written up by Claude
- Refines: 0039, 0042, 0065

## Context

- **The manager shows how many clients each radio and each SSID has** (0039, 0051), but not who they are, how they connect, or how they're doing.
- **The AP knows a good deal about each Wi-Fi client,** checked on OpenWrtnight:
  - **hostapd** gives its signal, its rates, and bytes and packets each way;
  - **nl80211's station dump** gives its connected time, how long since it was last heard, its signal and average signal, its receive and transmit rates with MCS, its retries and failures, and the throughput it should get;
  - **the DHCP watch (0065)** gives, on Aeolus's networks, the address each client uses and whether it got it by DHCP. The host name a client sends with its DHCP request (options 12 and 81) can be kept too.

## Decision

### Where

- **A Clients tab on an AP's page,** after Networks: Overview, Hardware, Networks, Clients, Interfaces, System.
- **The same tab on a folder's page,** listing the clients of every AP below it, with an AP column.
- When the menus are reorganised later, the tab moves with them.

### What it shows

- **One row per client, newest first:**

  | Column | What |
  |---|---|
  | **Client** | the host name it gave, else its MAC; its MAC beneath |
  | **Network** | the SSID it is on |
  | **Band** | the band, and its signal |
  | **Rate** | its rates each way |
  | **Address** | the address it uses |
  | **Connected** | how long it has been joined |
  | **Data** | its data each way |
  | **DHCP** | a chip: by DHCP, static, no address, or not known yet |

- **Filters** by network and by AP, and a search box for a name, MAC or address.
- **Sorted** by any column.
- **Compact rows,** as the UI's density work asks: no banners, one line a client but its MAC.

### What the AP reports

- **The state report's `clients`:** each Wi-Fi client on every SSID the AP broadcasts, the AP's own included, as they are its clients too. For each:
  - its MAC;
  - the network (Aeolus's ID, or none for the AP's own) and the SSID;
  - the radio's band;
  - its signal;
  - its rates each way (Mbit/s, with MCS);
  - its data and packets each way;
  - its retries;
  - its connected and inactive times;
  - its address and host name;
  - its DHCP status.

  At most 256 clients an AP.
- **Address, host name and DHCP status come only from Aeolus's networks,** where the DHCP watch runs (0065). On the AP's own networks they are blank.
- **When it is sent:**
  - A client joining or leaving sends a state report within a poll, about a minute, as a finding does.
  - Signal, rates and data are as fresh as the last report: at most five minutes old.
  - The tab says when each AP last reported.

## Consequences

- **Clients' MACs, host names and addresses reach the manager** in every state report, kept as long as reports are (0039). Phones' private MACs are shown as they are.
- **State reports grow:** a few hundred bytes a client.
- **No client history yet:** where a client was before, or how its signal moved over time. The time-series question in the README's open questions is where that belongs.

## Not now

- Acting on a client: disconnecting it, or blocking it.
- Clients on wired ports.
