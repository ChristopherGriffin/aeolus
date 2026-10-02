# 0045. A width the channel cannot carry moves the channel

- Status: Proposed
- Date: 2026-10-02
- Proposed by: Griff, after the first width change from the UI; written up by Claude
- Refines: 0044, 0026, 0029

## Decision

- **A width is offered whenever every AP's radio can do it.** The channel an AP is on no longer limits what a folder offers (0044 had it do so).
- **If an AP's channel cannot carry the width, the same change moves the channel.**
  - The channel is set on the same folder or AP as the width, to the lowest channel that can carry it. On 5 GHz that is 36 for 40, 80 and 160 MHz.
  - Moving a folder's channel moves every AP the folder reaches, not only the ones that needed it, so the folder stays uniform (0044).
  - The preview names each AP that moves, and the channel it moves from.
- **One change can set several fields of one node.** A set carries either one `path` and `value`, or `values`, a map of paths to values.
  - All the values apply, or none do.
  - It is still one log entry with one reason (0026), and the APs get one new config version, so the radio restarts once.
- **Some settings stop a move, and then the width is not offered. The reason names what stops it:**
  - an AP or folder below sets its own channel that cannot carry the width;
  - a lock above holds the channel.

  A setting made below is someone's explicit choice, so Aeolus does not undo it quietly.
- **A width that puts a radio on radar channels says so.**
  - In the US and EU, 5 GHz channels 52–144 are shared with radar (DFS), and every 160 MHz block includes some of them.
  - After the change, the radio listens for radar for about a minute before it transmits, and moves off by itself if it hears any.
  - The offer marks such a width, and the preview warns before anything is recorded.
- **The config check holds a channel and width that do not fit, when Aeolus sets both** (0029). A preview then shows the problem before anything is recorded. The render check still catches a channel the AP picked itself (0044).

## Why 36

- Each 5 GHz width's first block starts at channel 36.
- Channel 36 is allowed indoors in the US and EU.
- At 40 and 80 MHz, a radio on 36 avoids radar channels altogether.
- Putting a folder's APs on different channels is a separate decision. It matters once a folder has more than one AP.

## Consequences

- Choosing 160 MHz on Sandbox moves OpenWrtnight's 5 GHz radio from channel 149 to 36. The radio is off the air for about a minute while it listens for radar.
- Aeolus now sets the channel where it moved it. To let the AP pick its own channel again, unset the channel there.
- Going back to a narrower width leaves the channel where it is, since 36 carries every width.
- If radar is heard, the AP moves itself and reports the channel it moved to. Aeolus does not undo the move. The radio returns to Aeolus's channel the next time it restarts with a new config.
