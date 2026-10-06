# 0075. Channel sets, and an APs tab

- Status: Accepted
- Date: 2026-10-05
- Proposed by: Griff: a folder's tabs gain an APs tab, holding what Interfaces › Radios › Channels shows now. Channels becomes where the channels an AP may go to are chosen: every real channel, shaded in the blocks the band's width makes, picked a block at a time, as sets the APs can jump to. Written up by Claude, and accepted with the merge
- Refines: 0045, 0071, 0072, 0073

## Context

- **What an automatic channel may be is fixed today:**
  - On 2.4 GHz, 1, 6 and 11: the renderer writes OpenWrt's `channels` option (0045).
  - On 5 GHz, any channel, or any outside DFS with `radio.5g.dfs` avoid (0071).
  - RRM (0073) moves radios among the same channels.
  - Nobody can keep the APs off a channel, say one a neighbour's network sits on, or keep 5 GHz to a few blocks.
- **Interfaces › Radios › Channels** lists each AP's channel now. That's about the APs, not about what's set.
- **The lab check, 2026-10-05,** on the pumphouse's 5 GHz radio (ath10k, VHT40, no clients):
  - With `channels '149 153'`, hostapd's ACS (`chanlist=149 153`) chose 153 at 40 MHz, inside the list.
  - With `channels '36 149'`, it chose 149 at 40 MHz, centred on 151, taking 153 though 153 wasn't listed. **hostapd checks only the primary channel against the list,** not the rest of the block.
  - So a list has to be kept in whole blocks, or a radio spills outside it. The radio was put back as it was.

## Decision

### An APs tab

- **A Locations folder's tabs:** Interfaces, APs, Networks, Clients, System.
- **APs shows each AP below,** with its radios as each last reported: band, channel, width, clients, and what Aeolus sets. That's what Interfaces › Radios › Channels showed.
  - Each AP's state, such as In sync, Out of sync or Held, is a column there. The Inside list at the foot of a Locations folder's page is gone: its APs are on the APs tab, and its folders in the tree beside it (Griff, 2026-10-05).
- An AP's own page keeps its Overview.
- **The tree on the left holds folders only** (Griff, 2026-10-05): a folder's APs are on its APs tab, and on an AP's page its folder is the one marked.

### Channel sets

- **Each band has a set of channels an automatic channel may be,** `radio.2g.channels` and `radio.5g.channels`: lists of 20 MHz channels, set on any folder or AP, and inherited.
  - Unset, 2.4 GHz is 1, 6 and 11, as now, and 5 GHz is every channel.
  - DFS avoided still keeps 5 GHz off 52–144, whatever the set holds.
  - A channel set for a radio is left as it is: the set is for automatic channels.
- **Chosen in whole blocks:**
  - On 5 GHz at 40 MHz or more, the renderer writes only the channels of blocks wholly in the set, at the radio's width. The lab check showed hostapd would otherwise take a block's other channels unlisted.
  - A config is held if no whole block is left, or none outside DFS while it's avoided: the radio would have nowhere to go.
- **The AP:**
  - `channels` limits hostapd's ACS when the radio starts.
  - RRM (0073) moves radios only to blocks wholly in the set.
  - On 2.4 GHz, RRM listens on the set's channels, not just 1, 6 and 11.
- **Interfaces › Radios › Channels becomes the channel map,** for each band:
  - every real channel, in order, with 5 GHz's three ranges apart;
  - shaded in the blocks of the band's width, alternately, so each 40, 80 or 160 MHz block reads as one;
  - a block a click: clicking any of its channels picks or drops the whole block;
  - DFS channels hatched, and dimmed while avoided; channels no block of the width includes, such as 165 at 80 MHz, dimmed;
  - a bracket labelled DFS over 52–144, one where the ranges sit on one line, split where they wrap (Griff, 2026-10-05);
  - the APs on each channel now, marked under it;
  - saved with a preview, as other changes are.
  - on 5 GHz, whether DFS channels are avoided (0071), under the channel set; it was on the band's card under Bands (Griff, 2026-10-05).

  The width is the band's width in force there, or, where none is set, the one most of the APs below report.

## Consequences

- An operator can keep APs to chosen channels, say 36–48 and 149–161, and RRM's moves stay inside them.
- Changing the set restarts that radio where its channel is automatic, as any wireless change does. The preview says so.
- Changing the width can leave fewer whole blocks in a set. Where none is left, the config is held, and the map shows why.
- 6 GHz isn't offered yet: no lab AP has a 6 GHz radio.
