# 0113. Who a client signed in as

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff, "are you putting in corresponding sections into aeolus for these?". Written up by Claude
- Refines: 0066, 0098

## Context

- **On WPA Enterprise each client signs in as itself (0098),** yet the Clients tab showed only its MAC and the host name it gave in DHCP. Who a device belongs to is the first thing asked of an enterprise network.
- **hostapd knows.** Its control socket answers `STA <mac>` with `dot1xAuthSessionUserName`: the name the RADIUS server's Access-Accept gave, or else the identity the client showed. With PEAP the client shows only its outer identity, often `anonymous`, so the server has to send the real name back. The lab FreeRADIUS does (0111).
- **OpenWrt's hostapd says it nowhere on ubus,** and ships no `hostapd_cli`. Read on the C-360, 2026-10-10: `get_clients` has no such field. The control sockets are there, one per network, at `/var/run/hostapd/<bss>`. hostapd answers as its own user, `network`, to the asking socket's path.

## Decision

- **The prober asks hostapd who each client on Aeolus's networks signed in as,** once a session: a client newly seen, or seen to have joined again.
  - It sends `STA <mac>` from a socket of its own, `/var/run/aeolus/hostapd-ctl`, which hostapd may write to, and waits 300 ms.
  - A client that signed in with no name, as on a passphrase network, has none.
- **The name goes in the state report as the client's `user`:** at most 64 characters, anything not printable ASCII a `?`. The manager holds it to that.
- **The Clients tab shows it under the client,** "signed in as", and the search box finds by it.

## Consequences

- **The name is what the RADIUS server chose to send.** A server that sends none leaves the outer identity, which may be `anonymous`.
- **A user name is personal data.** It is kept with the state reports, as long as they are (`--keep-state-days`), and shown to whoever may view the AP.
- **The client's journey (0103) could say who it was** from the same reports. That is not done here.
