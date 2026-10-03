# 0045. APs pick their own channels, and a width never waits on one

- Status: Proposed
- Date: 2026-10-02
- Proposed by: Griff, after the first width change from the UI: offer 160 MHz when the radios can do it, and let the APs pick their channels, since they are best placed to see which ones are free; written up by Claude
- Refines: 0044, 0026, 0029

## Decision

- **APs pick their own channels.** Aeolus sets each band's channel to `auto` for the whole Org.
  - Each AP scans when its radio starts and picks the quietest channel that can carry its width.
  - Several APs in one folder then spread out instead of sharing a channel.
- **On 2.4 GHz, an automatic channel is one of 1, 6 and 11,** the only ones that do not overlap. The agent renders them as the radio's `channels` list. Wherever Aeolus sets the channel, it owns that list: the list exists only for automatic 2.4 GHz, and the render check holds the agent to that (0039).
- **A width is offered whenever every AP's radio can do it.** The channel an AP is on no longer limits what a folder offers (0044 had it do so).
- **If an AP's fixed channel cannot carry the width, the same change sets the channel to `auto`.**
  - It is set on the same folder or AP as the width. A folder's setting reaches every AP it covers, so the folder stays uniform (0044).
  - The preview names each AP that leaves a fixed channel.
- **Some settings stop the change. The width is then not offered, and the reason names what stops it:**
  - an AP or folder below sets its own channel that cannot carry the width;
  - a lock above holds the channel.

  A setting made below is someone's explicit choice, so Aeolus does not undo it quietly.
- **One change can set several fields of one node.** A set carries either one `path` and `value`, or `values`, a map of paths to values.
  - All the values apply, or none do.
  - It is still one log entry with one reason (0026), and the APs get one new config version, so the radio restarts once.
- **A width that is sure to put a radio on radar channels says so.**
  - In the US and EU, 5 GHz channels 52–144 are shared with radar (DFS), and every 160 MHz block includes some of them.
  - After the change, the radio listens for radar for about a minute before it transmits, and moves off by itself if it hears any.
  - The offer marks such a width, and the preview warns before anything is recorded.
  - At 80 MHz or less, an automatic channel may or may not be a radar channel. That is the AP's choice.
- **The config check holds a fixed channel and a width that Aeolus sets and that do not fit** (0029). A preview then shows the problem before anything is recorded. The render check still catches a fixed channel the AP set itself (0044).

## Consequences

- **The AP picks once, when its radio starts:** at boot, or when a new config restarts it. If a neighbour takes the channel later, the AP stays until its next restart. Re-picking on a schedule is a later decision.
- **Aeolus learns which channel an AP picked from its state report (0039).** The AP page shows it.
- **Turning on automatic channels restarts both radios on every AP.** On 2.4 GHz the AP may leave the channel it is on, and its clients reconnect.
- **Choosing 160 MHz always means radar channels in the US,** so the radio is off the air for about a minute after each restart while it listens.
- **To pin an AP or folder to one channel, set a number there.** A width that channel cannot carry is then not offered below it.
