# 0087. Finding 6 GHz: neighbour reports everywhere, and 6 GHz channel sets

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff, after his phone stayed on the C-360's 5 GHz: a channel map for 6 GHz that works as the others do, with a preferred-scanning-channels-only option and a non-overlapping one, so 160 MHz radios don't pick clashing channels; RNR offered everywhere; Bands and Channels combined. Written up by Claude
- Refines: 0045, 0049, 0072, 0075, 0086

## Context

- **A phone finds a 6 GHz network in two ways only.** It hears about it in a Reduced Neighbor Report in a 2.4 or 5 GHz beacon, or it finds it on a preferred scanning channel (PSC: 5, 21, 37 and every 16th to 229), the only 6 GHz channels phones scan by themselves.
- **The C-360 (2026-10-09) did neither.** Its 6 GHz radio was on channel 1, not a PSC, and no network had `rnr`. A phone on 5 GHz never learned 6 GHz was there.
- **0075's channel sets stopped at 5 GHz:** "6 GHz isn't offered yet: no lab AP has a 6 GHz radio." The C-360 has one.
- **Two radios on PSCs of one block still clash.** At 160 MHz, 5 and 21 are both PSCs of the block 1–29. A radio on 5 and one on 21 take up the same 160 MHz.

## Decision

### A Reduced Neighbor Report everywhere

- **Every network Aeolus renders, on every band, has `rnr '1'`.** Its beacons and probe responses list the AP's other networks, on every band. OpenWrt runs every radio in one hostapd, so each sees the others. There is no setting: it is always on.

### 6 GHz channel sets

- **`radio.6g.channels`, as 0075 has 5 GHz's:** the channels an automatic 6 GHz channel may be, in whole blocks of the width, set on any folder or AP and inherited. Unset, every channel.
- **6 GHz's blocks run 1–233:** 40 MHz every 8, 80 every 16, 160 every 32. 320 MHz blocks come in two families that overlap by 160 MHz (1–61 and 33–93, and so on).
- **`radio.6g.psc`, preferred scanning channels only:** an automatic channel is one of those, in a whole block of the set. A block with none cannot be used: at 40 MHz, half of them.
- **`radio.6g.non_overlapping`, one channel a block:** the first usable channel of each whole block, the blocks apart. At 320 MHz that means one family. Radios on different channels then never share a block. With PSC only, at 160 MHz: 5, 37, 69, 101, 133, 165, 197, not also 21, 53 ….
- **Both off unless set.**
- **Held when nothing is left to go to:** no whole block, or none with a PSC where PSC only is on. A channel set by hand that is not a PSC, with PSC only on, is held too.
- **One rule:** `radio.Usable`, for the composer and the render check. The renderer has the same, and writes it as the radio's `channels`, which hostapd's ACS picks from when the radio starts.
- **Radio resource management does not move 6 GHz radios yet.** It rates only a 6 GHz radio's own channel. On the C-360 it is off anyway, as the AP has a scan radio (0081).

### One view for a band

- **Interfaces › Radios › Bands and channels:** each band's card (width, channel, power, on), and beside it its channel map. An old link to Channels lands there.
- **The 6 GHz map:**
  - draws 1–233 in its blocks;
  - underlines each PSC;
  - has a switch each for PSC only and one channel a block;
  - with either on, rings the channels the APs go to, and greys out the blocks they cannot use.
- **A map says when its set does nothing:** the set is for an automatic channel, so where the band's channel is set by hand, or not at all, it says so.

### Not new

- **802.11k already has its switch, per network:** Roaming and steering › Neighbor reports (11k). Band steering turns it on too (0050).

## Consequences

- **On the C-360, once this release runs:**
  - set Sandbox's 6 GHz channel to automatic, with PSC only and one channel a block;
  - its 6 GHz radio then picks one of 5, 37, 69, 101, 133, 165 or 197 when it starts;
  - its 2.4 and 5 GHz beacons point phones there.
- **Each switch changes the 6 GHz radio's config,** so the radio restarts where its channel is automatic. The preview says so.

## Open

- Radio resource management on 6 GHz: visiting the set's channels (or its PSCs), and moving radios between blocks.
- One channel a block on 5 GHz too, where 80 and 160 MHz blocks have four or eight primaries.
- hostapd's ACS on 6 GHz on ath11k has not been tried on the C-360 yet.
