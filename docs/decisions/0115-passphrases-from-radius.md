# 0115. Each device's passphrase from the RADIUS server

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff, to get Aeolus "up to enterprise snuff"; third of the three steps after RADIUS VLANs (0111) and MAC authentication (0112). Written up by Claude
- Refines: 0070, 0078, 0112

## Context

- **Devices that can't sign in themselves share a passphrase.** Printers, sensors and TVs have no WPA Enterprise. On one passphrase, a device that leaks it, or leaves, means changing it on all of them.
- **Per-user keys (0070) answer that on the APs:** each key is its own passphrase, kept by the manager and sent to every AP. That needs no server, and suits a home or a small building.
- **A site with a RADIUS server or a NAC keeps its devices there already.** With MAC authentication (0112) the server says yes or no to each device, and may give it a VLAN. It can as well give the device's passphrase: RADIUS's `Tunnel-Password`, which PacketFence and others call DPSK or iPSK. One place then holds the device, its network and its key.
- **OpenWrt has it as `ppsk`** on a network with a passphrase and a RADIUS server. It has hostapd ask the server by the device's MAC (`macaddr_acl=2`) and require the passphrase in the answer (`wpa_psk_radius=2`). The network's own `key` is not given to hostapd at all.

## Decision

- **A network with MAC authentication may take each device's passphrase from the server:** `radius.passphrases`, off unless set.
  - The AP renders OpenWrt's `ppsk`, and writes no `key`.
  - A device joins with the passphrase the server holds for its MAC. A device the server gives none, or does not know, is refused.
  - The server's VLAN for the device (`radius.vlans`) works as with MAC authentication alone.
- **The network's own passphrase is not used,** and the schema no longer asks for one while this is on. One left set from before is kept, sealed, and ignored, so turning this off again needs nothing more.
- **Only WPA2 (`wpa2-psk`) is rendered.** The manager refuses it on any other security.
- **It is one way or the other.** A network with per-user keys can't be turned to it, and a network turned to it takes no keys: the manager refuses either change when it is made. A change log from before the rule, with both, still replays. It refuses VLANs offered to keys (`keys.vlans`) there too, and 802.11r.
- **The Networks tab shows it under RADIUS,** as "Passphrases from RADIUS", once MAC authentication is on for a WPA2 network. The network's own passphrase field goes away while it is on.

## Consequences

- **The server must know each device's MAC before it joins.** That is the server's or NAC's enrolment to run, not Aeolus's.
- **Private (random) MAC addresses defeat it,** as they do MAC authentication (0112): a phone shows a different MAC per network. This is for devices with a fixed one.
- **With the server down, no device can join,** new or returning: hostapd has no passphrase to check against. The RADIUS status and its alert (0114) say so. Per-user keys keep working through an outage; that is the trade.
- **A wrong passphrase and an unknown device look alike to the device:** it just fails to join. The server's own log tells them apart.

## Not done here

- **WPA3.** hostapd can use the server's passphrase for SAE, but OpenWrt then turns off the newer way of deriving the key (hash-to-element), which 6 GHz requires. It needs testing on real clients before it is offered.
- **Finding the key a device used without knowing its MAC** (0078): hostapd's `wpa_psk_radius=3`, with a server that tries the handshake against its keys, such as FreeRADIUS's `dpsk`. The lab's APs and server both have it. It would let per-user keys live on a server instead of the APs, and is a separate build.
- **802.11r with it.**
