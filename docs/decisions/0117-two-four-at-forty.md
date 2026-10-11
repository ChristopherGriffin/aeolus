# 0117. 2.4 GHz at 40 MHz, and channels that do not overlap

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff: with 2.4 GHz set to 40 MHz the channel map "does not properly reidentify the channels", and it should have a non-overlapping option, "one channel per block I guess". Written up by Claude
- Refines: 0045, 0075, 0087

## Context

- **5 and 6 GHz channels come in fixed blocks.** At 40 MHz and up the map draws the blocks, a set is picked in whole blocks, and 6 GHz can be kept to one channel a block (0087).
- **2.4 GHz has no blocks.** Its channels are 5 MHz apart and 20 MHz wide, so neighbours overlap. The map drew each as one cell at any width, as if a 40 MHz radio took one channel.
- **At 40 MHz a 2.4 GHz channel is joined with another, four away.** Which one is OpenWrt's to say (`wifi/hostapd.uc`, read on the office AP, 2026-10-10):
  - a channel that is set: the one four above for 1 to 6, the one four below from 7 up;
  - an automatic channel: always the one above, so hostapd can start only on a channel that has one four above it.
  Radio resource management moves a radio by the first rule, though it joined 7 with 11.
- **Unset, an automatic 2.4 GHz channel is 1, 6 or 11** (0045), the three that do not overlap at 20 MHz. A set of one's own (0075) had no such help.

## Decision

- **A 2.4 GHz channel's pair at 40 MHz is the one four above for 1 to 6, and four below from 7 up,** everywhere: the map, the manager's rule (`radio.Pair2G`), the AP's renderer, and radio resource management, which now joins 7 with 3 as OpenWrt does.
- **The map shows the pair.** At 40 MHz:
  - each channel's cell says which channel it is joined with;
  - the channels joined with the ones picked are drawn as taken;
  - a channel set by hand lights both itself and its pair;
  - an AP is drawn on both channels its radio takes.
- **`radio.2g.non_overlapping` keeps an automatic channel to those of the set that do not overlap at the width,** taken from the lowest:
  - at 20 MHz, five channels apart: of 1 to 11, that is 1, 6 and 11;
  - at 40 MHz, their middles eight channels apart: of 1 to 11, channel 1 alone; of 1 to 13, 1 and 13.
  The map marks the channels the APs go to, as it does on 6 GHz.
- **The AP's list of channels is that:** the renderer writes it to `channels`, and the render check works it out the same way.
- **The Bands tab offers it on 2.4 GHz** as "Only channels that do not overlap".

## Consequences

- **At 40 MHz with it on, every AP that can only use 1 to 11 is on channel 1.** That is all the room 2.4 GHz has at that width: two 40 MHz channels there share 10 MHz at the least. The count on the map says one channel, which is the point of showing it.
- **Unset channels at 20 MHz are unchanged:** 1, 6 and 11, with it on or off.
- **A set at 40 MHz is still the channels a radio may be on, not the ones it may take up.** A set of 1, 6 and 11 lets a radio take 5 and 10 as well. On 5 GHz a set is picked in whole blocks instead; 2.4 GHz has none to pick.
- **hostapd starts an automatic channel only where the channel above is free to join.** With 1 to 11 allowed, that is 1 to 7. Channels 8 to 11 are reached only by a move.
- **The manager does not know which channels a country allows at 40 MHz.** It goes by 1 to 13; the AP's own regulatory rules drop what it may not use.
