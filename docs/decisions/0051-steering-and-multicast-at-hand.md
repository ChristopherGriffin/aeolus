# 0051. Band steering and multicast at hand, and what usteer does

- Status: Accepted
- Date: 2026-10-02
- Proposed by: Griff: toggles on each network, and band steering status per AP; written up by Claude and accepted with the merge
- Refines: 0039, 0048, 0049, 0050

## Decision

- **Each network card on the Networks tab has its own controls for these two settings.** The full Edit form is not needed for them.
  - **Band steering is a switch.**
    - Turning it on sets it in the network's service folder.
    - Turning it off unsets it there, so the folder follows what is above, or sets it off there if the value came from above.
  - **Multicast to unicast is a three-way choice:**
    - **OpenWrt default** unsets it, and is only offered where this folder sets it;
    - **All** sets it on;
    - **None** sets it off.
  - **Each control shows what is in force,** and where it comes from when that is another folder. Clicking it opens the usual preview, with a reason and Apply (0042); the control changes only once the change is recorded.
  - **The band steering preview warns** when an AP it reaches has said it has no usteer, since that AP would refuse the change (0050).
  - Controls are disabled for a viewer, and where the value is locked above.
- **The agent reports what usteer is doing,** in each state report (`steering`, 0039):
  - whether usteer is installed (`/sbin/usteerd`) and running;
  - its live `band_steering_interval` and `ssid_list`;
  - for each SSID on each band, its clients now, and the clients usteer has moved off it and onto it since usteer started (its `roam_events`).
  - The manager checks the shape of this (bands, counts, sizes) like the rest of the report.
- **The Networks tab shows "Band steering on each AP".** For each AP the tab reaches, it shows:
  - whether usteer runs;
  - whether it steers, and which networks;
  - for those networks, the clients on each band and the moves;
  - when that was reported.

## Consequences

- Whether band steering works, and for which networks, can be seen without logging in to an AP.
- The move counts restart when usteer restarts, for example at each Wi-Fi reload. They show steering happening, not a history of it.
- An agent older than this reports no `steering`. The tab says so for that AP.
