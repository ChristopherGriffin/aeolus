# 0108. Usage over the last day

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, as one of the features Wi-Fi managers commonly have that Aeolus lacked (Griff, "create features common in these systems")
- Refines: 0040, 0066, 0102

## Context

- **A Wi-Fi manager's dashboard leads with how busy the network has been:** clients and traffic over the last day, by site and by AP. Meraki, UniFi and Omada all do. Aeolus showed only the clients each AP has now (0102).
- **Every state report already lists each client's data counters** (`rx_bytes`, `tx_bytes`) and how long it has been connected (0066), every five minutes (0040). The counters run from when the client joined, so what moved between two reports is how much they grew.
- **Charting a day from the reports themselves** would read a few hundred reports per AP for every chart.

## Decision

- **As each state report comes, the manager works out what its clients moved since the AP's report before,** and keeps one row: the AP, when, how many clients it had, and the bytes it sent them (`down`) and received from them (`up`).
  - A client in both reports counts how much its counters grew.
  - A client that joined since counts all it moved: it is not in the report before, or has been connected for less time than then, and joined within the interval.
  - A client whose counters went back without its joining again, as a 32-bit counter does when it wraps, counts nothing that time.
  - The rows are kept as long as the state reports are (`--keep-state-days`, 30 unless set).
- **`GET /v1/usage?under=&hours=` gives the APs the caller may view, over the last 24 hours unless set (at most 720).** It comes in at most 48 buckets of whole minutes, rounded up and at least five, the last ending after now.
  - Each bucket has the most clients each AP had at once, summed over the APs, and the bytes they moved.
  - With it come the totals, the most clients at once, and each AP's most clients and traffic, the busiest first.
  - The MCP adapter offers `get_usage`.
- **A Usage panel on a folder's Overview, and on an AP's,** draws the day:
  - traffic in bars, down and up stacked;
  - the most clients at once as a line;
  - each bar's numbers on hover;
  - the totals;
  - on a folder, its busiest APs by traffic.

## Consequences

- After a gap, a report more than three intervals (15 minutes) after the AP's last, as after an outage, only the clients that joined in the last interval count. The others' bytes from the gap are not counted, as they would all fall in one bucket, a spike that never was.
- Only the 256 clients each report holds count. A larger AP's usage is short.
- A bucket shows the busiest moment of each AP within it, not the network's: clients that moved between APs within the bucket may be counted on both.
- Usage by client, or by network, would need more than one row a report. That is for later.
