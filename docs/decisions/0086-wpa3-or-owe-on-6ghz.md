# 0086. 6 GHz takes WPA3 or OWE only

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff: WPA2 is not legal on 6 GHz networks, and APs shouldn't be able to do it. Written up by Claude
- Refines: 0049, 0081

## Context

- **6 GHz admits WPA3-Personal (SAE, with protected management frames and hash-to-element) and OWE only.** WPA2 and open networks are not allowed there, and neither is WPA2/WPA3 transition on a 6 GHz BSS.
- **Aeolus offered every network on every band unless its `bands` said otherwise.** On the C-360 (2026-10-09), `aeolus_50`, a WPA2 network, was rendered onto the 6 GHz radio. OpenWrt's wifi scripts quietly made that BSS SAE-only (`wpa_key_mgmt=SAE`, `ieee80211w=2`, `sae_pwe=1`), so nothing illegal went on the air. But Aeolus asked for a WPA2 network on 6 GHz, and a WPA2-only device saw the SSID on 6 GHz as something it could not join.

## Decision

- **A WPA2 or open network is never offered on 6 GHz.** With its bands unset, it is on the other bands only, and nothing is said. Named on 6 GHz by its bands, its config is held with the reason (`network.<id>: 6 GHz takes WPA3 or OWE only, not wpa2-psk; ...`). The agent's renderer refuses it too, should a config ever reach it.
- **A network in WPA2/WPA3 transition is WPA3 alone on 6 GHz:** `sae` there, `sae-mixed` on 2.4 and 5 GHz, under one SSID.
- **`wpa3-sae` and `owe` are offered on 6 GHz as on any band.**
- **One rule, in one place for the manager** (`radio.SixGHzEncryption`), read by the composer's check, the render check and the per-user keys' VLAN check (0082), and the same table in the agent's renderer. The UI shows a network's 6 GHz chip as "not offered" or "not allowed", with why.

## Consequences

- **On the C-360, `aeolus_50` leaves the 6 GHz radio** once a manager and agent with this release run: one fewer BSS there. `test ssid` (WPA2/WPA3) stays on 6 GHz, as WPA3.
- **To have a WPA2 network on 6 GHz, make it `wpa2-wpa3`:** WPA2 devices keep 2.4 and 5 GHz, and WPA3 devices get 6 GHz too.

## Open

- **Getting clients onto 6 GHz at all** (found the same day): the C-360's 6 GHz radio was on channel 1, which phones do not scan (they scan the preferred scanning channels, 5, 21, 37 and so on), and the 2.4 and 5 GHz networks carry no Reduced Neighbor Report (`rnr`). A phone on 5 GHz never learns 6 GHz is there. Radio resource management should pick only preferred scanning channels on 6 GHz, and a network on 6 GHz should be advertised in its 2.4 and 5 GHz beacons.
