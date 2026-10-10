# 0112. MAC authentication, for devices that cannot sign in

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Griff, "a radius server and maybe opennac to get this thing up to enterprise snuff". Written up by Claude
- Refines: 0098, 0100, 0111

## Context

- **WPA Enterprise signs each client in as itself (0098),** and the RADIUS server can then put it in a VLAN or disconnect it (0111). Printers, sensors, plugs and cameras cannot do 802.1X. They join with a passphrase, and nothing says which device is which.
- **A NAC handles those by MAC address.** The AP asks the RADIUS server about each device's MAC before it lets it on, and the server answers yes, no, or yes in this VLAN. NACs call it MAC authentication, or MAB.
- **OpenWrt does this for any network that is not WPA Enterprise and has a RADIUS server** (ap.uc, read on OfficeOpenWrt, 2026-10-10):
  - `iface_ppsk` gives hostapd the server and `macaddr_acl=2`, for open, OWE, WPA2 and WPA3 networks alike. `psk2` counts as `psk` there (iface.uc).
  - A VLAN from the server is possible on the passphrase kinds (`vlan_possible`), not on open or OWE ones.
  - Accounting goes to an accounting server on any kind of network.
- **Two things do not fit with it:**
  - A deny list on the same network (`blocked`, 0100) makes OpenWrt write `macaddr_acl=0` after the `2`, and hostapd takes the last, so the server would not be asked.
  - OpenWrt hands hostapd a Disconnect secret only for WPA Enterprise.

## Decision

- **`radius.mac_auth` turns MAC authentication on, for a network that is not WPA Enterprise.** The network's `radius.auth_server` and secret are then rendered on its Wi-Fi, as are accounting and the NAS-Identifier where set, and hostapd asks the server about each device by its MAC: twelve hex digits, in lower case, as both name and password.
- **The server may put a device in a VLAN the network offers, `radius.vlans`,** as on WPA Enterprise (0111): `r<vlan>`, beside the per-user keys' `k<vlan>` (0070).
  - `radius.vlan_required` refuses a device given none.
  - A VLAN is offered to the keys or to the server, not both.
  - An open or OWE network offers none.
- **Compose refuses what would not work:**
  - MAC authentication without a server and secret;
  - MAC authentication on WPA Enterprise, which signs each client in already;
  - MAC authentication together with `blocked`: a device is refused at the server instead;
  - `radius.das` on any network that is not WPA Enterprise;
  - `radius.vlans` where nothing asks the server.
- **In the network form, MAC authentication is a tick under RADIUS** on a network that is not WPA Enterprise. Ticked, it shows the server's fields. The Disconnect fields show for WPA Enterprise only.

## Consequences

- **A device still needs the passphrase.** The server decides on top of it: an unknown MAC with the right passphrase is refused.
- **A MAC can be copied.** MAC authentication sorts devices into VLANs. It does not prove what they are. A NAC adds profiling to that, and the VLAN keeps a copied MAC to what that device could reach.
- **A device with a private (random) MAC changes it for each network,** so it must be known to the server by the one it uses here.
- **A device whose VLAN the server changes stays where it is until it joins again.** Without disconnects on these networks, that is when the device next reconnects. The Reconnect action (0107) does it by hand.
- **Per-device passphrases from the server** (OpenWrt's `ppsk`, hostapd's `wpa_psk_radius`) are the next step on the same path. 0078 scoped the kind that looks a key up during the handshake.
