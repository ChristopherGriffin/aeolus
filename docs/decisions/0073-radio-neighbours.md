# 0073. Radio resource management: APs as neighbours, channels by rating

- Status: Accepted
- Date: 2026-10-05
- Proposed by: Griff: each AP still picks its own channel, but with reliable scanning and collision avoidance. APs that hear each other become neighbours and talk over the wire. Each rates every channel after each scan and keeps the ratings like a link-state protocol's link costs. Better channels become more likely to be chosen over time. A neighbour's channel is blotted out, and sudden interference moves an AP to its next best channel. Three neighbours each, ideally at −70 dBm or better. A client that can't follow a move is accepted, with planned moves in off hours. Written up by Claude, and accepted with the control plane's merge
- Refines: 0045, 0050, 0071

## Context

- **0045 had each AP pick its own channel, expecting APs to spread out. They don't.**
  - On 2026-10-05 both lab APs were on 2.4 GHz channel 11. They hear each other at −73 dBm, all five SSIDs, both ways. So they share one channel's airtime, and 10 SSIDs' beacons, while 1 and 6 had no Aeolus AP.
  - On 5 GHz they were apart, on 149 and 157, and the pumphouse doesn't hear the office there.
- **Why: the AP's own channel choice is hostapd's ACS.**
  - It runs only when the radio starts, and samples each allowed channel for about a tenth of a second. The office AP measured channel 1 as 58% busy over 124 ms, and 6 as 12%.
  - It weighs how busy a channel is, not who is on it. An idle neighbour's beacons hardly register.
  - Each AP decides alone, at a different moment, and never looks again until its radio restarts.
  - On the office AP (mt76), some channels' surveys lack a noise floor, which ACS then leaves out.
- **usteer (0050) already shares some of this over the management LAN.**
  - Each AP broadcasts on UDP 16720 its networks (BSSID, SSID, frequency, noise, load, clients) and its clients.
  - It doesn't share who hears whom in the air, how good each channel is, or where an AP is about to move.
- **Aeolus runs without the cloud, and its APs keep working without the manager (0069).** Channel choice belongs on the APs too.

## Decision

Radio resource management (RRM) runs on the APs, as a link-state protocol does on routers. The manager sets policy and shows what the APs do, but isn't in the loop.

### Neighbours, found in the air

- **An AP finds the other Aeolus APs it hears.**
  - Each Aeolus AP's beacons carry a small vendor-specific element, through hostapd's `vendor_elements`: its AP ID and its management address.
  - An AP that hears such a beacon knows it as an Aeolus AP and knows how to reach it over the wire, without asking the manager.
- **The closest by RF become its neighbours:** the three strongest heard, on each band separately, however strong they are.
  - The aim is for each of the three to be heard at −70 dBm or better, which makes for seamless roaming. Power management, a later decision, will help get there.
  - Two APs can be neighbours on 2.4 GHz and not on 5 GHz, as the lab's are.
- **Neighbours keep in touch over the wire,** from management address to management address, as routers exchange hellos.
  - They exchange hellos at an interval. A neighbour silent for a dead interval is dropped.
  - A neighbour no longer heard in the air ages out.
  - The messages are signed with a key the manager gives the APs in each location, so nothing else on the management LAN can pose as a neighbour. Once an AP has the key, the exchange needs no manager.

### What neighbours share

- **Their radios:** each band's channel, width, power, noise and load.
- **Their channel ratings,** below.
- **Their clients,** as usteer shares them now.
- **Where each is about to move,** below.

### Listening on every channel

- **Each AP scans every channel it may use, periodically.**
  - It hears its own channel all the time.
  - It visits the others one at a time, briefly, spread out, at moments with little traffic, so its clients don't notice.
  - DFS channels are only listened to, never probed on.
- **Each scan measures, per channel:**
  - how busy the channel is with others' traffic, and its noise floor;
  - the foreign networks heard, and how strongly;
  - the neighbours on it.

### Channel ratings, like link costs

- **After each scan, each channel's rating is updated,** as a cost: lower is better.
  - It's built from the busy time, the noise floor, and the foreign networks weighted by signal.
  - It's smoothed over time, so a channel that stays good keeps getting better and one bad sample doesn't sink it. Better channels become more likely to be chosen over time.
- **A channel a neighbour uses is blotted out** for that band.
  - On 5 GHz, a neighbour's block blots out every channel it covers at the AP's width.
- **The ratings are kept as a table,** like a link-state database, and reported to the manager.

### Choosing and moving

- **When its radio starts,** the AP takes its best-rated channel that isn't blotted out. This replaces hostapd's ACS.
- **Where neighbours use every channel,** as they easily can 2.4 GHz's three, the AP takes the one whose nearest user is furthest away by signal: the stronger of how each hears the other. Of those alike, the best rated (Griff, 2026-10-05).
- **While it runs, it moves only for a clear gain.** The best channel must beat the current one by a margin, and keep doing so for a while. A small or brief difference doesn't move it, so it doesn't hop between channels.
  - **Such a planned move waits for off hours,** a window the policy sets.
- **If interference suddenly appears on its channel,** for example the channel stays very busy for some seconds, it moves at once to its next best channel. The interference is disrupting clients already, so the move costs little more.
- **Collision avoidance:**
  - Before moving, the AP tells its neighbours where it's going, and waits a short hold time.
  - If a neighbour claims the same channel in that time, one of them, by a fixed tie-break, picks again.
  - **The tie-break:** the AP whose current channel rates worse moves first; where they rate alike, the one with the lower AP ID (Griff, 2026-10-05). The other waits, and once the first has moved, looks again: often it no longer needs to move.
  - A channel a neighbour has claimed is blotted out like one it uses.
- **A move announces itself to clients first,** with hostapd's channel switch announcement, so clients that support it follow without disconnecting. It doesn't restart the radio.
  - A client that can't follow has to find the AP again. That's accepted: planned moves happen in off hours, and sudden ones happen when interference is already disrupting clients.

### The manager: policy and view

- **What it sets** is what each band may use:
  - channels and widths (0044, 0045);
  - DFS allowed or avoided (0071);
  - whether RRM is on;
  - the off-hours window for planned moves;
  - its margins.

  An AP or folder can still pin a channel, which RRM then leaves alone.
- **What it shows,** from the state reports:
  - each AP's neighbours, with the signal both ways;
  - its channel ratings;
  - its moves, each with why.
- **Without the manager,** APs keep their neighbours and go on rating and moving with the policy they last had.

## Consequences

- **APs that hear each other spread out on their own,** and keep doing so as things change, not only at a restart.
- **Moves cost clients little:** a channel switch announcement where the client supports it, a reconnect where it doesn't, and planned moves only in off hours.
- **Off-channel listening costs a little airtime,** spread thin, and is held back while clients are busy.
- **A new piece runs on every AP,** with a port open on the management address to its neighbours.
- **What usteer does stays as it is.** Whether RRM later feeds usteer, or takes over its sharing, is open.

## Lab plan

Before building, on both lab APs (OfficeOpenWrt, mt76; OpenWrtnight, ath10k):

- **Vendor elements:** an AP's beacon element with its ID and address, read back from the other's scan.
- **Off-channel visits:** how long each driver is away for one channel, and what a busy client and an idle IoT device see.
- **Survey counters:** busy time and noise per channel, read repeatedly, and how often mt76 lacks the noise floor.
- **Channel switch announcement:** `ubus call hostapd.<bss> switch_chan` on both drivers. Which clients follow, and which have to find the AP again: a phone, a laptop, and the Espressif and Nest devices. This is to know, not a gate: clients that can't follow are accepted.
- **The wire:** UDP between the APs' management addresses, through their firewalls.

## As built: the control plane

The first part (v0.40.0): neighbours. Ratings and moves come next.

- **The setting:** `rrm.enabled`, in Locations, off unless set. On, the renderer gives the agent's package a section, `aeolus_rrm`, naming the AP. Off, there is none. The render check holds the AP to it.
- **The key:**
  - The manager derives it from its own secret key (`secret.Box.Derive("rrm")`), so it never has to be stored, and it's the same for every AP.
  - It comes with each ready poll while RRM is on, beside the config, so it versions nothing.
  - The agent keeps it in `/etc/aeolus/rrm.key`, mode 0600, from before the config that needs it is applied. It removes it only once a config without it runs, so a refused or reverted config still has its key.
- **The daemon,** `aeolus-rrm`, runs beside the agent under procd. Its pure parts are `aeolus/rrm.uc`. It needs ucode's nl80211, digest and socket modules, which OpenWrt 25.12's image has, and which the installer now adds where missing.
- **The advert:**
  - It's a vendor element: OUI `02:ae:01`, from the locally assigned range, so no vendor's OUI or CID can be the same; then type 1, version 1, the AP's ID (6 bytes), its IPv4 management address and its port, 16730.
  - It goes on one network per radio, Aeolus's own where there is one, through hostapd's `set_vendor_elements`. No restart, and it's put on again when a radio restarts.
  - The daemon takes it off when it stops.
- **Listening:**
  - Each radio visits one channel every 15 seconds: 2.4 GHz's 1, 6 and 11, every 20 MHz 5 GHz channel the radio may use, and its own.
  - A visit elsewhere waits while the radio's own channel has been more than 60% busy, at most four times in a row.
  - **A visit only listens, and reads beacons only.** Both lab APs' drivers answer probes from a template that misses the element's changes. The OpenWrt One's answers lacked it after it was put on, and the R7800's still carried it after it was taken off. Beacons always carry the current one. Without a probe, a visit stays long enough to hear one.
  - Signals are smoothed (three parts old, one part new), and an AP not heard for 20 minutes is forgotten.
- **Neighbours:**
  - The three heard most strongly on each band, by name where alike, plus any AP whose signed hellos come.
  - Hellos every 10 seconds, UDP from management address to management address, port 16730.
  - Each is the HMAC-SHA256 under the key, then JSON: the AP, the time, a sequence number, its radios' bands, channels and widths, the APs it hears and how strongly, and whose hellos it gets.
  - A hello is refused unless its signature holds, its time is within 60 seconds of this AP's clock, and it's later than the last from the same AP. So a hello sent again is refused, and the APs need their clocks (0069).
  - A neighbour is up when hellos go both ways, and down after 40 seconds without one; one down for 15 minutes is forgotten.
- **What the AP reports:** in its state report's `rrm` (docs/api.md). The manager shows it under **Interfaces › Radios › Neighbours**, where RRM is turned on and off.
- **Not yet:** sharing clients. usteer still shares those.

## As built: channel ratings

The second part (v0.41.0). Moves come next.

- **Each visit rates the channel it visited,** as a cost, lower being better. It's built from three things:
  - **Busy:** the share of the visit others kept the channel busy, times 100.
    - Off the radio's channel, it's from the survey of the visit itself. Both lab drivers keep only the last visit's counts for a channel not in use: about 140 ms on ath10k and 170 ms on mt76.
    - On its own channel, it's from the survey's running counts since the last visit, less the AP's own sending, which both drivers count on the channel in use (`time_tx`). At the first visit the counts run from the radio's start; they're taken less its own sending too (v0.42.0; before, the first visit wasn't).
  - **Noise:** 2 for each dB of noise floor above −95 dBm, where the driver says. A noise floor outside −127 to −20 dBm is taken as unknown: ath10k gives 0 for its own 5 GHz channel. Elsewhere ath10k reports −102 to −108, so on the R7800 noise hardly tells channels apart.
  - **Other networks:** each adds up to 10, fully at −55 dBm or stronger and nothing at −95. On 2.4 GHz a network up to three channels away counts too, 6 dB weaker for each channel between. Other Aeolus APs' adverts aren't counted, but their other networks are.
- **Each channel keeps two ratings:**
  - its lasting rating, which each visit moves by a tenth, so a channel earns it over many visits;
  - its rating now, which each visit moves by half.
- **A rating not refreshed for 30 minutes is dropped.**
- **Blotted out:** a channel is blotted out by a neighbour using it, at its width, on a band where that neighbour is among the three this AP hears best, or its hellos say it hears this AP. Its channel and width are its hellos'.
- **What the AP reports:** in its state report's `rrm.ratings` (docs/api.md). The manager shows them under **Interfaces › Radios › Ratings**: by AP and band, with the channel each radio is on, and who blots out the others.
- **The best on each band** is the AP's own pick, reported as `best` (v0.42.0, `rrm.pick`): the best rated no neighbour uses; or, where neighbours use them all, the one whose nearest user is furthest away, as above. A neighbour heard at no known strength counts as near. Since v0.43.0 it's where the radio would move, as below: at its width, and never a channel shared with radar.
- **A fix in v0.40.0:** ucode divides whole numbers to a whole number, so the check that holds a visit back while a radio's own channel is busy read 0 unless the channel was busy all the time. It now divides as decimals.

## As built: moves

The third part (v0.43.0).

- **The policy,** in Locations, beside `rrm.enabled`:
  - `rrm.moves`: on unless set off. Off, the APs rate channels but don't move.
  - `rrm.window`: when planned moves may happen, in the AP's local time (`system.tz`); 02:00–05:00 unless set. It may run past midnight.
  - `rrm.margin`: how much better, in rating points, a channel must rate than the radio's own for a move to it; 20 unless set.

  The renderer writes all three into the daemon's section, defaults included, and the render check holds the AP to them.
- **Which radios move:** those whose channel is automatic. A set channel is left alone, as is a radio whose own channel isn't rated yet.
- **Where to:** the AP's best on the band (`rrm.best_of`), at the radio's own width.
  - On 5 GHz at 40 MHz or more, it weighs blocks, not channels: a block rates as its worst channel, is blotted out where any of its channels is, and is entered on its best-rated channel.
  - Moves don't go to DFS channels, even where they're allowed: the radio would first have to listen for radar for a minute, off the air.
  - A move never goes from a free channel to a shared one.
- **Why it moves,** as the report says:
  - **shared:** a neighbour uses the radio's channel, and a free one is there, or, with all of them used, one whose nearest user is at least 6 dB further away;
  - **better:** another channel rates better by the margin, and is no more shared;
  - **interference:** for three visits in a row (about 45 seconds), others kept the radio's own channel more than half busy, and the target rates better than that by the margin;
  - **start:** within ten minutes of the radio starting, while its clients reconnect anyway, another channel ranks above its own, by any margin.
- **When:**
  - Shared and better are planned moves: their reason must hold for ten minutes, for the same target, and they wait for the window. Another target, or another reason, starts the ten minutes again (v0.43.1).
  - Interference moves at once, as do moves within ten minutes of the radio starting. A radio counts as started when its network interface is made anew, when it comes up while the daemon runs, or when the daemon starts within ten minutes of the AP booting. Turning RRM on doesn't count.
  - Before any move, every channel the radio visits must have been rated, and the daemon must have run for a minute, so it knows its neighbours' channels.
  - After a move, or a switch that failed, the radio stays put for 15 minutes.
- **Collision avoidance:**
  - The AP claims the target in its hellos (each radio's `to`, with its own block's rating as `cost`), in one sent at once, and waits 30 seconds.
  - It claims nothing while a neighbour's claim on the band is in its hellos, nor for 20 seconds after one ends.
  - Two claims at once go by the tie-break: the higher `cost` goes first, then the lower AP ID. The other yields and looks again later.
  - A neighbour's claimed channel is blotted out, as is one it uses.
  - Before moving, the AP checks again that the move is still worth it; if not, it withdraws.
- **The move:** hostapd's `switch_chan` on one of the radio's networks, which moves all of them, announced for 10 beacons, about a second. No restart.
- **What the AP reports:** in its state report's `rrm.moves`, its last 16 moves, each with its band, from and to, why, and what came of it: announced, moved, yielded (to which AP), withdrawn or failed. The manager shows them under **Interfaces › Radios › Ratings**. The daemon logs each one too.
- **Across a restart of the daemon,** as when the agent is updated, it keeps its ratings and its moves: it reads them back from its last results.
- **Not yet:** ACS still picks the channel a radio starts on; RRM moves it within its first minutes, as above. Giving the radio its pick before it starts is a later step.

## Lab checks, 2026-10-05

- **The element, by hand:** set with `set_vendor_elements` on the pumphouse's "Aeolus Lab" (ath10k), the office (mt76) read it back through nl80211 byte for byte, at −73 dBm, and the other way round.
- **The OpenWrt One's `iw`** is a cut-down build that prints few elements, which is why the daemon reads nl80211, not `iw`'s text.
- **UDP** from 192.168.1.45 to 192.168.1.38:16730 went through; both APs' management is in the `lan` zone, which accepts.
- **A one-channel visit took** 160–220 ms on ath10k and 280–490 ms on mt76, and no client dropped: three IoT devices on the pumphouse's 2.4 GHz and nine on the office's.
- **The daemon, run beside the agent** with a throwaway key for six minutes:
  - Both APs advertised on 2.4 and 5 GHz.
  - They became neighbours 50 seconds after starting, up both ways, hearing each other at −71 and −73 dBm on 2.4 GHz, each knowing the other's channels: 11 and 149 for the office, 11 and 157 for the pumphouse.
  - They don't hear each other on 5 GHz.
  - No client dropped.
  - Its first run showed the probe-answer template: the pumphouse found the office over the wire, one-way, before it heard it in the air.
  - **After it stopped,** both beacons were clean, but the pumphouse's probe answers still carried the element, even after `update_beacon` and with the office's scan cache flushed. So visits became listen-only, reading beacons alone. They stay so until that radio restarts, and are ignored.
- **The second run, listen-only:**
  - The office ran alone first, for a minute: it heard no one, as the pumphouse's beacons were clean.
  - With both running, they were up within 30 seconds of the pumphouse starting, at −72 and −74 dBm on 2.4 GHz.
  - Listening for a full beacon interval also caught what probing hadn't: the office hears the pumphouse faintly on 5 GHz, at −91 dBm.
  - No client dropped.

- **The ratings, run beside the agent** for six minutes on 2026-10-05:
  - **2.4 GHz, both APs:** channel 1 rated worst, with six networks and up to 29% busy (the office 83, the pumphouse 40); channel 6 best (27 and 19).
    - Channel 11, where both APs are, was blotted out on each by the other. On the office it rated 33, counting the pumphouse's four other networks at −73 dBm.
  - **5 GHz:** quiet, 0 to 12, but for the R7800's own channel 157.
    - Its driver gave a noise floor of 0 there, which read as 190 points of noise: such a value is now taken as unknown.
    - It also counts its own sending as busy, 36% with no 5 GHz client.
    - On the R7800, the office's block, 149 and 153, was blotted out.
  - No client dropped.

- **Channel switch announcements,** with `ubus call hostapd.<bss> switch_chan`, announced for 10 beacons:
  - The pumphouse's 2.4 GHz (ath10k), 11 to 6 at 20 MHz: all five networks moved with the one call, and its four clients followed without dropping. Their connected times ran on, and each sent within seconds.
  - The office's 5 GHz (mt76), 149 to 36 at 40 MHz (HE): both clients followed.
  - The office went on hearing the pumphouse's advert on its new channel, at −70 dBm, and its hellos gave the new channel. 11 was then free on the office, and its best.
- **Moves, by the daemon,** run on both APs with an all-day window and the live RRM paused, both put back on 2.4 GHz channel 11 (one Espressif device dropped at that switch, and was back within a second):
  - **2.4 GHz:** both found 11 shared, and rated 6 best. The office's reason held ten minutes first. It claimed 6 at 18:46:02 and moved at 18:46:32, its nine clients following.
  - The pumphouse saw the claim, 6 blotted out by the office, and claimed nothing. Once the office had moved, 11 was free there and its best, so it stayed: one move settled it for both.
  - **5 GHz:** the pumphouse's block, 157 and 161, rated 35, against 0 for 44 and 48. It claimed 44 (better) at 18:47:15 and moved at 18:47:45, to VHT40 there.

## When netifd loses `network.wireless` (2026-10-06)

- **What happened:** the office AP's netifd lost its `network.wireless` ubus object while it ran on. netifd, ubusd and hostapd never restarted.
  - In OpenWrt 25.12, `/lib/netifd/wireless.uc` publishes that object from ucode once, at start. If netifd's ubus connection is lost and made again, its C objects come back, and this one doesn't.
  - The likely cause was 0078's 4,096-key load check, which made the object's status huge while RRM asked for it every 10 s. The log didn't reach back far enough to show it.
- **The effect:** the radios ran and clients joined, but RRM saw no radios. It had no advert, scans or ratings, made no moves and didn't control power.
  - The two APs stayed neighbours by hellos alone, until both daemons restarted together with v0.49.1's fleet update. The office's report had no radios or clients, and the Wi-Fi check after an apply passed without looking.
  - A network restart brought it back. The office then advertised, rated and found the pumphouse within seconds. ACS had put its 5 GHz on the pumphouse's 44, and RRM moved it to 36 at once, as a radio that had just started.
- **Now:**
  - The agent reports `wireless_missing`, and doesn't take the missing object for no Wi-Fi: an apply logs that it couldn't check the Wi-Fi.
  - RRM logs when the object goes and when it comes back.
  - The fleet view flags the AP ("Radios unseen"), and the neighbours table says why the AP hears no one.
  - Nothing restarts the network by itself, as that drops the AP's Wi-Fi.

## A radio on a DFS channel can't scan (2026-10-06)

- **What happened:** with DFS allowed, ACS put the office's 5 GHz on channel 108. Every scan there was refused: `iw dev phy1-ap0 scan trigger freq 5240 ap-force` gave "Resource busy (-16)", while 2.4 GHz scanned as usual. Linux keeps an AP on a DFS channel listening for radar.
- **The effect:** RRM's 5 GHz visits failed silently. Its 5 GHz ratings stopped at the move, and lapse after `RATED_FOR` (30 minutes). It couldn't hear the pumphouse on 5 GHz, though it still had the pumphouse's channel from its hellos.
- **Now:** after three refused visits in a row, RRM says so.
  - It logs that the radio can't scan, and why: on a DFS channel, or refused otherwise. It logs again once a scan works.
  - It reports `cannot_scan`, and the ratings and neighbours tables say which radio can't scan and why.
- **Griff put My House back on `radio.5g.dfs avoid`** (change 79), so RRM keeps sight of 5 GHz. A site that allows DFS gets this blind spot on any radio that lands on a DFS channel.

## Open

- **The hello and dead intervals.**
- **The rating's weights and smoothing,** and the margin, hold and wait times a move needs.
- **A channel busy with what the radio can't decode:** the R7800's own channel 157 was 36% busy over ten seconds, while it sent 1.3% and received nothing it could decode. That's energy from interference, or from traffic on overlapping channels, and it counts against the channel, rightly.
- **The default off-hours window,** 02:00–05:00 for now.
- **How often an interference move** is right: a client sending a lot also keeps the channel busy, and a move doesn't help it.
- **Power management,** to be discussed later: transmit power that brings each AP's three neighbours to −70 dBm or better. Width stays Aeolus's setting.
