# 0110. The clients that moved the most

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, as one of the features Wi-Fi managers commonly have that Aeolus lacked (Griff, "create features common in these systems")
- Refines: 0108

## Context

- **Usage (0108) says how much the APs moved, not who moved it.** A day's traffic that doubled raises the question at once: which client?
- **Wi-Fi managers commonly list their top clients** beside the day's usage, as UniFi's dashboard does.
- **0108 already works out, at each report, what each client moved,** then keeps only the sum.

## Decision

- **The manager keeps what each client moved at each report,** one row for each that moved anything: the AP, when, its MAC, the host name it gave in DHCP if any, and its bytes down and up. The AP's own usage row is their sum. Both are written in one transaction, and trimmed with the state reports.
- **`GET /v1/usage` gives `clients` too:** the ten that moved the most over the span on the APs the caller may view. Each comes with its MAC, the last host name it gave, its bytes down and up, and the APs it moved them on. `get_usage` in the MCP adapter gives the same.
- **The Usage panel lists the top clients** under the chart: the ten on a folder, with the APs each used, and five on an AP.

## Consequences

- A row a client a report: ten clients on each of ten APs make about 29,000 rows a day, under a million over 30 days.
- A client with a private MAC that changes it counts as two.
- Usage by network, or a client's own day in its journey (0103), can be read from the same rows. That is for later.
