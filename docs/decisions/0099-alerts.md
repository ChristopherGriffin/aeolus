# 0099. Alerts: what needs attention, across the fleet

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, as one of the features Wi-Fi managers commonly have that Aeolus lacked (Griff, "create features common in these systems"); a first step toward the client journey Griff wants
- Refines: 0039, 0040, 0057, 0059, 0061, 0064, 0065, 0079

## Context

- **The manager knows a great deal about each AP:**
  - when it last called;
  - whether its config is held, refused or put back;
  - the agent it runs;
  - what its own reports say of tunnels, transports, the uplink's VLANs, DHCP and the clock.
- **Until now, finding trouble meant opening each AP.** Its page has banners for some of it.
- **A person wants one list, most urgent first.**

## Decision

- **`internal/alerts` turns what the manager has of one AP into alerts.** Each has a severity, a kind, a message, and since when where that is known. It stores nothing: an alert ends when its cause does.
- **Critical: clients are, or will be, without service.**
  - Offline: silent for three of its polls and half a minute, and never under five minutes.
  - Its current version refused by the render check, or not applied and put back.
  - netifd's lost `network.wireless` object.
  - A network with no transport in its bridge.
  - A loop the loop guard found.
- **Warning: a person should look.**
  - A config the manager holds, with its first problem.
  - An agent update failed or rolled back.
  - A tunnel down, unless it is a standby.
  - A network on its fallback, or that cannot switch.
  - A VLAN silent on the uplink.
  - DHCP that nothing answers.
  - Never seen.
  - Behind: running an older version for three polls, with none of the above to say why.
- **Info: worth knowing.**
  - Waiting in Landing Zone.
  - The clock not synced.
- **A state report older than three of its intervals (15 minutes)** says nothing. Offline speaks for an AP gone quiet.
- **`GET /v1/alerts`, with `?under=` a Locations node,** gives the alerts of the APs the caller may view, most urgent first, then by AP, with counts. The MCP adapter offers it as `list_alerts`.
- **Every Locations folder has an Alerts tab,** for the APs below it, each alert a link to its AP. It refreshes every 30 seconds.

## Consequences

- Alerts are seen, not sent. Delivery is the next step: a webhook, ntfy or email for critical alerts, with a hold-off so a flapping AP does not page someone every minute. It needs a place to remember what was sent.
- The rules are a first set. More follow as reports grow, such as a radio stuck on a DFS channel it cannot scan from, or a client that cannot get an address.
