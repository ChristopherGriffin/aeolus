# 0102. A folder's Overview

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, as the dashboard Wi-Fi managers commonly open on (Griff, "create features common in these systems"); a step toward the dashboards Griff wants later, the client journey among them
- Refines: 0042, 0047, 0099

## Context

- **A folder's tabs each show one part:** interfaces, APs, networks, clients. None shows the whole at once.
- **A manager usually opens on the whole:** how many APs are up, how many clients, what is wrong, and where the load is.

## Decision

- **Every Locations folder, and the Org, opens on an Overview tab.** It covers the APs below it, from what the other tabs already read: the fleet, each AP's config view and last report, and the alerts (0099). It sets nothing.
- **Four tiles, each a link to the tab with more:**
  - APs up, of how many;
  - Wi-Fi clients, by band;
  - alerts, critical and warning;
  - configs in force, and how many are held.
- **Under them:**
  - the critical alerts, at most five;
  - the busiest APs, with each radio's channel and clients;
    - Without each radio's channel since 2026-10-12, at Griff's word: the channels are on the APs tab. A band says how many clients are on it.
  - the clients on each network, by band;
    - Folded since 2026-10-12, and left out where no client is on: the tile says how many, and by band. The tables under the day's usage (0108), its top clients and its APs, are folded there too.
  - the channels the APs use on each band, a channel shared by more than one AP marked, as their clients share its airtime.
    - Taken out on 2026-10-12, at Griff's word: the channels are on the APs tab, and the Overview is to say less.
- **It refreshes every 30 seconds,** as the live tabs do.

## Consequences

- Each AP's config view is fetched to draw it, as the APs and Clients tabs do. Fine for tens of APs; a fleet of thousands will want one summary call from the manager.
- History, such as clients over a day, needs the manager to keep counts over time. That is for the dashboards to come.
