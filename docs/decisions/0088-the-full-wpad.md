# 0088. The full wpad, so band steering and 802.11v work

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff: band steering and 802.11v are refused on every AP, though the C-360 can do them. Written up by Claude
- Refines: 0050, 0057, 0069

## Context

- **Every lab AP runs `wpad-basic-mbedtls`,** OpenWrt's default hostapd, the C-360 and PumphouseAP included (2026-10-09). It is built without 802.11v. Its ubus object has no `bss_transition_request`, so the agent reports `bss_transition: false`.
- **0057 holds such an AP's config when it asks for band steering or BSS transition,** because that hostapd refuses the radio's whole config over the one line. The hold protects the AP. The fix it names, a full wpad, was left to a person.
- **apk swaps them in one change:** `apk add wpad-mbedtls '!wpad-basic-mbedtls'` purges the basic one, upgrades `hostapd-common` and installs the full one. Dry-run on PumphouseAP, 2026-10-09.

## Decision

- **Aeolus installs the full wpad in place of `wpad-basic`,** with the same TLS library (mbedtls, openssl or wolfssl):
  - `aeolus-packages install` does it when the agent is set up;
  - `restore` does it again after a sysupgrade brings the image's basic one back.
  Under apk it is one change. Under opkg the basic one is removed first, and comes back should the full one not install.
- **Then hostapd is started, and the Wi-Fi brought up on it** (`/etc/init.d/wpad start`, `wifi`). Removing the basic wpad stops hostapd, and installing the full one does not start it again. Found on OfficeOpenWrt (2026-10-09): every network on it, Sweet Spot included, stayed down for about six minutes, until hostapd was started by hand. The Wi-Fi is off for a few seconds. The network is not restarted, so nothing else moves: on OfficeOpenWrt a network restart had also brought a new DHCP lease, 192.168.1.69 in place of .55.
- **The C-360's image ships `wpad-mbedtls`** (openwrt-arista ce3fb5b), so it needs no swap.
- **0057's hold stays** for an AP where the swap failed, or that has not reported since it.

## Consequences

- **Existing APs keep `wpad-basic`** until a sysupgrade, or until someone swaps it once. The agent runs `restore` only after a sysupgrade.
- **The full wpad is about 250 KiB larger,** small beside any AP Aeolus manages.
