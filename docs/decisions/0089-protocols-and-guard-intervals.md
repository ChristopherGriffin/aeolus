# 0089. Protocols and guard intervals for each band, and Bands and channels in one panel

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff. He asked for each band's panel to say which 802.11 protocols clients may use, as tick boxes, with a guard interval to suit each kind. He also asked for Bands and channels in one panel: radio on top left, the width top right, the channel and power as pulldowns under the map, one channel in manual, and DFS as a tick that allows it. Written up by Claude
- Refines: 0044, 0045, 0071, 0075, 0087

## Context

- **OpenWrt has three options for the protocols a radio serves:**
  - `legacy_rates` lets 802.11b clients in. It is off by default, so 2.4 GHz is 802.11g and newer.
  - `require_mode` (`n`, `ac` or `ax`) sets the least a client must support. hostapd turns it into `require_ht`, `require_vht` or `require_he`.
  - `htmode` caps the newest generation, by its family: HT, VHT, HE, EHT, or NOHT for none.
- **OpenWrt 24.10 can require only 802.11n or 802.11ac:** its `hostapd.sh` knows `n` and `ac`, nothing else. 25.12's `hostapd.uc` knows `ax` too (GateOpenWrt, 24.10.5, and OfficeOpenWrt, 25.12.2, 2026-10-09).
- **The short guard interval for 802.11n and 802.11ac** is a capability: `short_gi_20`, `_40`, `_80` and `_160`, all on by default.
- **802.11ax has no guard-interval option in OpenWrt.** The radio's rate control picks 0.8, 1.6 or 3.2 µs frame by frame. iw can hold an interface to one: `iw dev <if> set bitrates he-gi-<2.4|5|6> <0.8|1.6|3.2>`. The interface keeps it until hostapd makes the interface anew. On the C-360 (ath11k, 2026-10-09) a phone on 6 GHz went from HE-GI 0 to HE-GI 1 at once, and back when cleared, with no reconnect.

## Decision

- **`radio.<band>.modes`** is the 802.11 generations clients may use, oldest to newest:
  - 2.4 GHz: b, g, n, ax, be;
  - 5 GHz: a, n, ac, ax, be;
  - 6 GHz: ax, be.

  The list is one unbroken run. The oldest becomes `require_mode` where it is n, ac or ax, and never on 6 GHz, where every client is 802.11ax. 802.11b turns `legacy_rates` on, and on 2.4 GHz only. The newest caps the htmode family: a radio runs the best family it can up to the cap, and NOHT for b, g or a alone. A width wider than the cap's family goes (HT 40, VHT and HE 160, EHT 320) is refused when set. When the radio's own width is too wide, it is narrowed. Unset, nothing changes: each AP keeps OpenWrt's defaults.
- **The config check refuses:**
  - a gap in the list;
  - 802.11be alone, which OpenWrt cannot require;
  - a width wider than the newest's family goes;
  - an oldest some AP's radio can't serve, since no client could join it;
  - 802.11ax as the oldest on an AP before OpenWrt 25.12.
- **`radio.2g.short_gi` and `radio.5g.short_gi`:** false sets `short_gi_*` to 0, the long guard interval (800 ns) for 802.11n and 802.11ac. True is OpenWrt's default, and the options go.
- **`radio.<band>.he_gi`** is the 802.11ax guard interval, in ns: 800, 1600 or 3200, or `auto`, the radio's own choice.
  - The renderer writes it, for each band the AP serves, into the agent's package: section `aeolus_gi`, option `he_gi_<band>`, in µs as iw takes it.
  - `aeolus-gi` sets it with iw on each AP interface that is up. It runs after each apply and with each state report, and keeps what it holds in `/var/run/aeolus/gi/`, the guard interval with the interface's index. A band set back to `auto` is cleared.
  - The net hotplug (`/etc/hotplug.d/net/60-aeolus-gi`) runs it for an interface hostapd makes anew, once hostapd says its network is up (90 seconds at most, for a radar check).
  - Changing it restarts no radio.
- **Interfaces › Radios › Bands and Channels is one panel a band:**
  - the radio's on/off top left, the width top right;
  - tick boxes for DFS channels (ticked allows them) and, on 6 GHz, preferred scanning channels and one channel a block;
  - the map;
  - then the channel (automatic or manual: manual picks one channel on the map), the power, the protocols as tick boxes, and the guard intervals: "802.11n/ac" short or long, and "802.11ax" automatic or 0.8, 1.6 or 3.2 µs, each shown only while its kind is allowed.

  A tick that would leave a gap fills it. Unticking any but the newest raises the oldest. The last tick stays. Unset, the boxes show what any AP serves, except 802.11b. 802.11be shows only where an AP serves it. Widths the newest can't carry are disabled, and a set one narrows. Save is disabled, with the reason, when an AP can't serve the oldest. Everything saves as one change, through the preview.
- **On a folder, a value nothing sets says nothing** (Griff, 2026-10-09). No "each AP's own" appears beside it, and its pulldown shows a dash. On an AP's page it still says "its own". A folder's channel map has no dots for the channels APs are on now; an AP's map still marks its own. Each guard interval pulldown marks the usual value "(default)": short, 400 ns, for 802.11n/ac, which OpenWrt advertises wherever the radio can, and 0.8 µs for 802.11ax.
- **The hardware view** gives each band's `modes`: each generation, `ok` where every AP serves it, `any` where one does, and `why`.

## Consequences

- **A forced 802.11ax guard interval holds only for frames the AP sends,** and only between `aeolus-gi`'s runs. An interface hostapd makes anew runs on the radio's own choice for the seconds before the hotplug sets it again. If the hotplug misses one, the next state report (five minutes) catches it.
- **On 24.10 APs, 802.11ax can't be the least a client must support.** Such a folder needs another oldest, or the AP's config is held.
- **Unset fields leave the AP as it is,** as everywhere else: setting `modes` and unsetting it later leaves `require_mode` and `legacy_rates` where they were.
