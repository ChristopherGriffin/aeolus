# 0120. DFS channels after a look

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff, on seeing the office AP on DFS channel 132 with no channel rated: "From boot the radio does not hit a DFS channel. It scans, then at some point shortly afterward makes a decision to hit a DFS channel if the coast is clear." And: "I would rather it not borrow an AP that is not itself." Written up by Claude
- Refines: 0071, 0073, 0116

## Context

- **With DFS allowed (0071), hostapd picks the 5 GHz channel when a radio starts,** from a few seconds' listening, and may pick a DFS one. The radio then waits a minute for radar before it serves anyone.
- **A radio on a DFS channel can't scan.** Linux keeps it there listening for radar and refuses every scan (the office AP on 108, 2026-10-06). Its ratings lapse after half an hour, and radio resource management (0073) then knows of no channel to move it to.
- **RRM never moves a radio to a DFS channel,** as the move would cost a radar check off the air.
- So the channel is chosen by the process that knows least, and the one that could choose well never gets to. On 2026-10-10 the office AP sat on 132 with no channel rated.
- **An AP with a scan radio (0116) is not blind there:** the scan radio looks for it.
- **What the lab's APs can do,** read on all three, 2026-10-10:
  - **Linux says how each DFS channel stands,** which `iw phy <phy> channels` prints: usable, available (checked), or unavailable (radar was found, and it is closed for half an hour), and how long its radar check takes: 60 s on every one of them in the US.
  - **hostapd's `switch_chan` takes `force`.** An announced switch to a DFS channel that has not been checked is refused. With `force`, hostapd then starts the radio's networks again on that channel, and they send nothing until the radar check is done.
  - **`acs_exclude_dfs` only keeps hostapd's own pick off DFS.** A forced move there is not held back by it.
  - **hostapd says when a radar check is done:** its `get_status` has the radio's state and frequency.

## Decision

### A third choice for `radio.5g.dfs`: `look-first`

- **`avoid`** keeps a radio off DFS, as now. **`allow`** lets hostapd start it on one, as now.
- **`look-first`:**
  - **The radio starts outside DFS,** as with `avoid`: the renderer sets `acs_exclude_dfs`.
  - **It looks at every 5 GHz channel from there,** DFS ones too. Listening on a DFS channel from another is allowed.
  - **RRM moves it to a DFS block that is clearly the best.** The daemon's section says it may (`dfs '1'`).
- **It applies to an automatic channel.** A channel set by hand is used as set.
- **It needs RRM moving radios.** Where RRM is off, or moves none, nothing would look: the radio stays off DFS, as with `avoid`.
- **A width or a set of channels that leaves the radio nowhere to start is refused,** as with `avoid`: 160 MHz, or a set (0075) with no whole block outside DFS.
- **`allow` stays the default, and means what it did.** Nothing changes on an AP until `look-first` is chosen for it.

### Which DFS block, and when

- **A DFS block may be gone to when:**
  - it is wholly in the radio's set of channels, where it has one;
  - each of its channels has been looked at twice;
  - Linux has none of them closed for radar found there;
  - none takes more than 60 s to check. Europe gives the channels weather radar uses (120 to 128) ten minutes, which no move is worth;
  - the radio did not leave it, or fail to get there, within a day.
- **It must be clearly the best.** It must beat where the radio is by RRM's own rules: a neighbour uses the radio's channel and the block is free, or it rates better by the margin (0073). And it must beat the best block outside DFS the same way: what a move that costs nothing gets the radio is not worth a minute's silence to better by less.
- **Interference is no reason to go to one.** A radio leaves that at once, for a channel it can use at once. Having just started is none either.
- **When:**
  - **at once, in the half hour after the radio starts.** The second look at every channel is done about a quarter of an hour in;
  - **otherwise in the window for moves (0073),** once the reason has held ten minutes.
- **The move is claimed in hellos first,** as any move is, so two neighbours do not go for one block.

### The move, and its radar check

- **The daemon asks hostapd to switch with `force`.** hostapd starts the radio's networks again on the channel. They are silent while it listens for radar, 60 s, and the radio's clients go to another band or AP for that time.
- **The move is `checking` until hostapd says the radio is up on the channel,** then `moved`.
- **A radio not there three minutes on did not get there:** the move is `failed`.
  - Where hostapd took it elsewhere, as when radar was found while it listened, it stays where hostapd put it.
  - Where its networks are not up at all, the daemon has netifd start the radio again. It then starts outside DFS, as at any start.
- **Its networks made anew by its own radar check are not a start,** so it does not move again for having just started.

### On a DFS channel

- **An AP with a scan radio goes on as before.** It sees every channel through that radio, and moves by the same rules.
- **Any other radio can't scan there,** so it knows two things:
  - **its own channel,** which it rates every five minutes by its own counts: how busy others kept it, and its noise. They need no scan. The other networks it heard there before it came are counted as still there;
  - **where it came from:** the best block outside DFS when it left, rated as it was then. Who uses it now is known from the neighbours' hellos.
- **It steps off, to where it came from,** by RRM's own rules:
  - at once, when interference fills its channel;
  - in the window, when its channel has grown worse than that one was by the margin, or a neighbour has taken its channel and that one is free.
- **Stepping off is an announced switch,** which clients follow without reconnecting: a channel outside DFS needs no radar check.
- **From there it looks at every channel again,** and may go to another DFS block by the rules above. The one it left is left alone for a day.
- **Where it came from is kept across the daemon's restarts,** as an agent update is. A radio that does not know has the lowest block outside DFS in its set.

### Radar

- **When radar is found on its channel, hostapd moves the radio at once.** That is hostapd's, and the law's; the daemon does nothing to it.
- **The daemon notes it:** a radio that left a DFS channel without being moved, where Linux now has that channel closed. It is in the AP's moves as `radar`, and the block is left alone for a day.

### What is shown

- **Radios, 5 GHz, DFS channels:** Avoid, Look first or Allow.
- **Moves** name `radar` as a reason, and show a move to a DFS channel as listening for radar until it is made.
- **A radio on a DFS channel it went to** is no longer shown as one that can't scan and whose ratings age. It says where the radio steps off to, and when.

## Consequences

- **The channel is chosen by what has looked.** A radio is never on a DFS channel that nothing rated.
- **A radio that starts is serving at once,** and may go silent for a minute about a quarter of an hour later. Before, with DFS allowed, the minute came before it served at all. Any restart of its Wi-Fi counts as a start, a config that changes the radio among them.
- **Only what the AP itself measured decides.** No AP's ratings are another's.
- **A radio on a DFS channel does not hear a better channel appear.** It leaves when its own grows worse, which is what matters to its clients. It is not taken off to look on a schedule: that would cost a minute's silence each time it went back.
- **Its own channel's rating there is a little kind to it:** a network that came after the radio did is counted only by the airtime it takes.
- **2.4 GHz stays up through a radar check,** and a client that can't use DFS channels at all keeps to it, or to another AP. With `look-first` as with `allow`, such clients lose that AP's 5 GHz while it is on a DFS channel (0071).

## Not tried on an AP yet

- **The forced move and its radar check have not been run on the lab's APs.** They take a radio's 5 GHz clients off it for a minute, so that is for Griff to start: set `radio.5g.dfs` to `look-first` on one AP. What was read, not run: hostapd's source for the forced switch, its `get_status`, `iw`'s words for each channel on all three APs, and `switch_chan`'s `force` on each.
- **Checked without one:** the rules as `agent/test/rrm.uc` has them, the renderer's case `dfs-look-first`, the config check, the render check and the report's check.

## Not done here

- **Looking again on a schedule,** as once a week in the window, for a radio whose DFS channel stays good.
- **A neighbour's ratings as a hint** for a radio that can't scan, weighted by how strongly the two hear each other. Griff would rather an AP not borrow another's, and asked whether direct neighbours might; left out.
- **Making `allow` mean this wherever RRM moves radios.** Once it has run in the lab, there may be no reason to keep the old `allow` there.
- **Countries other than the US and those of Europe,** whose DFS channels differ (0071).
