# 0107. Reconnecting one client

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, as one of the features Wi-Fi managers commonly have that Aeolus lacked (Griff, "create features common in these systems")
- Refines: 0066, 0100, 0104

## Context

- **A client stuck on a weak AP, or holding a stale address,** usually comes right once it joins again. Restarting the AP's Wi-Fi (0104) drops every client to fix one; Block (0100) keeps it off for good.
- **Wi-Fi managers commonly have Reconnect beside a client:** UniFi and Omada among them. The AP deauthenticates the client, which joins again at once, roaming to a better AP if there is one.
- **hostapd's ubus `del_client` does this** (`{addr, reason, deauth, ban_time}`, read from PumphouseAP with `ubus -v list`, 2026-10-10). It answers 0 whether or not the client is there, so the agent looks first with `get_clients`.

## Decision

- **An action may be of one thing of the AP's, its `target`.** `disconnect` is of a client, its MAC the target. The others take none.
  - The database's rule is one open action of a kind for each target, so two clients can be reconnected at once, but one client is not asked twice.
  - Migration 6 adds the column and the wider rule.
- **`POST /v1/aps/{ap}/actions {kind: disconnect, target}`** asks it, with operator on the AP, as the other actions need. The AP is handed the target with the kind.
- **The agent's `disconnect`** finds the client on each of hostapd's objects (`hostapd.*`, not netifd's `network.wireless`, which may be lost: 0105). It deauthenticates the client from each with reason 1 (unspecified) and no ban, and says which networks it was on, or that it is not connected there.
- **In the Clients tab, each client has Reconnect,** beside Block, for someone who may act on its AP. It asks without a preview: the client only drops and joins again. The AP's Actions panel lists it as Reconnect with the MAC.
- **The MCP adapter's `ap_action` takes `target`.**

## Consequences

- A client that roamed to another AP before the action ran is not there, and the action fails, saying so.
- An agent older than this one says it cannot disconnect, and the action fails.
- A client may join the same AP again. Steering it elsewhere is for band steering and 802.11v (0050).
