# 0098. WPA Enterprise, against a RADIUS server

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, as the biggest of the features Wi-Fi managers commonly have that Aeolus lacked (Griff, "create features common in these systems")
- Refines: 0018, 0070, 0086, 0088

## Context

- **Every network Aeolus made shared one passphrase,** or a per-user key (0070). Business Wi-Fi signs each client in as itself, by 802.1X against a RADIUS server: WPA2- or WPA3-Enterprise.
- **OpenWrt names them `wpa2`, `wpa3` and `wpa3-mixed`.** The server and secret go in `auth_server`, `auth_port` and `auth_secret`, accounting in the `acct_*` options, and the NAS-Identifier in `nasid`.
- **hostapd must be built with EAP.** The full wpad is, and is what Aeolus installs (0088); wpad-basic is not. OpenWrt's hostapd says what it was built with: `hostapd -veap` exits 0 with EAP and 1 without. All three APs say 0 (2026-10-10).
- **6 GHz takes WPA3 only (0086).**

## Decision

- **`network.<id>.security` adds three modes:** `wpa2-enterprise`, `wpa3-enterprise` and `wpa2-wpa3-enterprise`.
  - The last is WPA2/WPA3 mixed (`wpa3-mixed`) on 2.4 and 5 GHz.
  - On 6 GHz, both WPA3 modes are WPA3-Enterprise alone, and WPA2-Enterprise is not offered there.
- **`network.<id>.radius`:**
  - `auth_server` and `auth_secret`, which are needed, and `auth_port` (1812 unless set);
  - `acct_server`, with `acct_port` (1813) and `acct_secret` (the sign-in one unless set);
  - `nas_id`.
  - The secrets are sealed, as every secret is (0027), and blanked in the UCI the manager keeps (0041).
- **Each AP asks the server itself, from its own address.** The server must know every AP as a client, or a subnet of them.
- **The agent reports whether hostapd has EAP.** The renderer refuses an enterprise network where it does not, so the AP keeps what it runs.
- **The manager refuses three cases:**
  - an enterprise network without its server or secret;
  - one with per-user keys, which are passphrases;
  - one with 802.11r, whose key holders for 802.1X are not rendered yet.
- **The render check wants:** the mode as above, and exactly the RADIUS options of the network on each of its Wi-Fi interfaces, the secrets matching. Another network has none of them.
- **In the network editor,** a RADIUS section shows for the enterprise modes, and the passphrase only for the PSK ones.

## Consequences

- Dynamic VLANs from RADIUS (Tunnel-Private-Group-ID), and 802.11r for enterprise networks, are left for later. Both need what per-user keys' VLANs have (0082): one per network, and the key holders.
- A RADIUS server that does not answer leaves an enterprise network's clients unable to join. The AP stays up, so no revert catches it. A later probe could ask the server.
