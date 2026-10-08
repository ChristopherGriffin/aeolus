# 0081. Radios another service owns

- Status: Proposed
- Date: 2026-10-07
- Proposed by: Claude, for the Arista C-360, under Griff's rule (2026-10-07): an AP with a third radio that serves no clients does all its scanning on that radio, and its serving radios never scan; an AP without one may scan from its serving radios
- Refines: 0040

## Context

- **Griff's rule on scanning is conditional.** Where an AP has a radio that serves no clients, the serving radios never scan or go off channel. Where it has none, they may, as RRM does today (0073).
- **The C-360 has four radios, two of them on 6 GHz.** radio3, a QCN9074, serves clients. radio2, a second QCN9074 that covers 2.4, 5 and 6 GHz, is a dedicated scan radio: `airscan` runs the AP's spectrum, BSS and client surveys on it, and it never serves.
- **airscan takes its radio out of netifd's hands.** It sets `disabled '1'` on that wifi-device and marks it with `option airscan '1'`.
- **The agent renders per band.** `radio.6g` would apply to both 6 GHz radios, turning the scan radio on, and each network would get a wifi-iface on it. That breaks airscan and serves clients from the scan radio. The manager's render check would demand the same: settings and a wifi-iface on every radio of a band.

## Decision

- **A wifi-device marked `airscan '1'` is reserved: Aeolus leaves it alone altogether.** The agent's renderer gives it no radio settings and no wifi-iface. The render check leaves it out of the radios it checks (`rendercheck.Reserved`), so both halves of the contract agree.
- **A test case pins it.** `agent/test/cases/c360` is a C-360's own wireless, system, firewall and dhcp config, with its network shown as a VLAN-filtering bridge on the uplink `eth0`. The scan radio must come out exactly as it went in, with no network on it (`TestAgentLeavesReservedRadiosAlone`).
- **An empty `snmpd` package means snmpd isn't installed.** The agent exports `package snmpd` with no sections on an AP without snmpd, as on the C-360's image, and the check had refused that ("there is no general section"), which would have held every config there.

## Consequences

- **On a C-360, `radio.6g` means radio3,** and a network on 6 GHz is broadcast from radio3 only. airscan keeps the scan radio.
- **The mark is airscan's own.** Another service that wants a radio to itself can use the same mark, or this decision grows a second one.

## Open

- **Radio resource management (0073) scans from the serving radios' own BSSes** (`NL80211_CMD_TRIGGER_SCAN` on each BSS). That stays right on an AP without a scan radio. On one with a reserved scan radio, the rule forbids it: there RRM should take its neighbours from airscan's BSS survey (`ubus call airscan bss`) instead. Until it does, RRM should stay off on such an AP; nothing enforces that yet.
- **The agent reports the reserved radio among the AP's radios** at enrollment. The manager could show it as the scan radio, and airscan's surveys beside it.
- **The C-360's management network is an 802.1Q device in a plain bridge** (`br-vlan20` on `eth0.20`), which the renderer does not render VLANs on yet (0040). A C-360 that Aeolus is to put networks on needs its uplink in a VLAN-filtering bridge, as in the test case, or the renderer to learn 802.1Q devices.
