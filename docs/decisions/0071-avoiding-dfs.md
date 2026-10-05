# 0071. Avoiding DFS channels, for now

- Status: Accepted
- Date: 2026-10-05
- Proposed by: Griff: keep the 5 GHz radios off DFS channels for now, without losing them for good, since DFS channels are a large part of the band. Written up by Claude, and accepted with the merge
- Refines: 0044, 0045

## Context

- **On 2026-10-05, automatic channels put both lab APs on DFS channels:**
  - OpenWrtnight on 60;
  - OfficeOpenWrt on 108.

  On 108, the office AP's two 5 GHz clients never came back. Many IoT devices and some laptops can't use DFS channels.
  - A DFS channel also waits 60 seconds for radar after every radio restart.
  - And clients don't send probes on it until they've heard a beacon, which hides 5 GHz clients from a band survey.
- **Griff pinned them,** office to 48 (change 45) and pumphouse to 36 (change 46), as temporary per-AP settings.
- **Pinning gives up automatic choice,** and has to be undone AP by AP.
- **OpenWrt can avoid DFS for automatic channels:** the wifi-device option `acs_exclude_dfs` (in OpenWrt 25.12's schema) has hostapd's channel choice skip DFS channels.
- **In the US, outside DFS** (`internal/radio`, 0045):
  - 20 MHz has 36–48 and 149–165;
  - 40 MHz has four blocks: 36/40, 44/48, 149/153, 157/161;
  - 80 MHz has two: 36–48 and 149–161;
  - 160 MHz has none: every 160 MHz block holds DFS channels.

## Decision

- **A new Location field, `radio.5g.dfs`:** `allow` (the default, as now) or `avoid`. It's set on a folder like any radio setting, and inherited, overridden and locked like them.
- **With `avoid` and channel `auto`,** the renderer sets `acs_exclude_dfs '1'` on the 5 GHz wifi-device. With `allow`, it takes the option out. The render check checks it.
- **`avoid` with a DFS channel set is a problem the config check holds:** "radio.5g.channel 108 is a DFS channel, but radio.5g.dfs is avoid". An explicit channel is never quietly ignored.
- **`avoid` with a 160 MHz width is a problem too,** since no 160 MHz block avoids DFS. The Hardware tab's width choices already mark radar (0045). With `avoid`, the ones that need it are shown as unavailable.
- **The 2.4 and 6 GHz bands have no DFS,** so there's no such field for them.

### For the lab now

- **Set `radio.5g.dfs` to `avoid` on Symtus,** and unset the two pins, changes 45 and 46. Both APs then choose their own channel again, outside DFS.
- **Getting DFS back later is one change:** `allow` on Symtus.

## Consequences

- **With `avoid`, automatic channels choose among four 40 MHz blocks,** or two at 80. Neighbouring APs share channels more.
- **DFS comes back for every AP with one change,** and per folder or per AP if wanted.

## Lab plan

- **On both APs, with `avoid` and channel auto:** check that a radio restart lands outside 52–144. Do it on OpenWrtnight (ath10k) and OfficeOpenWrt (mt76), whose drivers both pick channels with ACS. Then check that `allow` lets it pick DFS again.

## As built

- **The field:** `radio.5g.dfs`, `allow` or `avoid`, in the schema with `allow` as its default. Unset, the renderer leaves the AP's own setting alone, as with every radio field.
- **The renderer:**
  - With `avoid`, and a channel that is automatic (`auto`, or not set on the AP), it sets `acs_exclude_dfs '1'` on the 5 GHz wifi-device.
  - Otherwise it removes the option if it is on. An `acs_exclude_dfs '0'` the AP already has is left alone, so that `allow` changes nothing on an AP that never avoided DFS. The pumphouse AP has one from uCentral.
  - OpenWrt 25.12 passes the option to hostapd as is (`/usr/share/ucode/wifi/hostapd.uc`), on both lab APs.
- **The render check** holds the AP to it: `acs_exclude_dfs` on exactly when DFS is avoided and the channel is automatic. With `avoid`, a channel in UCI that uses DFS channels at the htmode's width is a problem, whoever set it.
- **The config check** holds two problems with `avoid`:
  - `radio.5g.width: every 160 MHz channel uses DFS channels, but radio.5g.dfs is avoid`;
  - `radio.5g.channel: channel 108 uses DFS channels, but radio.5g.dfs is avoid`, for a set channel, judged at the set width (48 is fine at 80 MHz, not at 160).
- **The Radios tab's 5 GHz card** has a "DFS channels" row under Channel: allowed or avoided, where it comes from, and "Avoid…" or "Allow…", previewed like any change, for those who may edit. With `avoid`, 160 MHz is offered as unavailable: "it needs DFS channels, which are avoided here".
- **A new render case,** `agent/test/cases/dfs`: automatic channels at 80 MHz with DFS avoided.

## Not now

- **Avoiding particular channels, rather than all of DFS:** OpenWrt's `channels` list does that, as the office AP's 2.4 GHz has 1, 6 and 11. It's a field of its own.
- **Countries other than the US,** whose DFS ranges differ. `internal/radio` is US and EU only.
