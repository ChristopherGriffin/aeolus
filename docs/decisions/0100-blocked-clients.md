# 0100. Blocking a client from a network

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, one of the features Wi-Fi managers commonly have that Aeolus lacked (Griff, "create features common in these systems")
- Refines: 0048, 0066, 0067

## Context

- **A manager can usually block a client:** a lost phone, a misbehaving device, someone who should not be on the network. It does so from where the client is seen.
- **hostapd refuses clients by MAC** with OpenWrt's `macfilter 'deny'` and `maclist` on a Wi-Fi interface.

## Decision

- **`network.<id>.blocked`:** the MACs refused on the network, on every band, at most 1024.
  - The renderer writes them on each of the network's Wi-Fi interfaces, in lower case and in order.
  - With none, there is no MAC filter at all.
- **The render check** wants exactly that list, denied, and no filter without one.
- **On the Clients tab, each client on an Aeolus network has Block.** It adds the client's MAC to the network's list in one change, previewed first.
  - The list grows where it is set, or where the network is if it is set nowhere.
  - A client blocked shows as blocked.
  - A client with a private MAC is warned of: it may pick a new MAC and join again.
- **The network editor offers the list from the schema,** one MAC a line. Unblocking is taking a MAC out there.

## Consequences

- Blocking by MAC stops a careless device, not a determined one: a MAC can be changed. Per-user keys (0070), or WPA Enterprise (0098), revoke a person's access for good.
- A block reloads the Wi-Fi of each AP that offers the network, as any change to its interfaces does.
