# 0073. Radio resource management: APs as neighbours, channels by rating

- Status: Proposed
- Date: 2026-10-05
- Proposed by: Griff: each AP still picks its own channel, but with reliable scanning and collision avoidance. APs that hear each other become neighbours and talk over the wire. Each rates every channel after each scan and keeps the ratings like a link-state protocol's link costs. Better channels become more likely to be chosen over time. A neighbour's channel is blotted out, and sudden interference moves an AP to its next best channel. Written up by Claude
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
- **The closest by RF become its neighbours,** on each band separately: the strongest heard, above a threshold, up to a limit.
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
- **While it runs, it moves only for a clear gain.** The best channel must beat the current one by a margin, and keep doing so for a while. A small or brief difference doesn't move it, so it doesn't hop between channels.
- **If interference suddenly appears on its channel,** for example the channel stays very busy for some seconds, it moves at once to its next best channel.
- **Collision avoidance:**
  - Before moving, the AP tells its neighbours where it's going, and waits a short hold time.
  - If a neighbour claims the same channel in that time, one of them, by a fixed tie-break, picks again.
  - A channel a neighbour has claimed is blotted out like one it uses.
- **A move announces itself to clients first,** with hostapd's channel switch announcement, so clients that support it follow without disconnecting. The rest reconnect. It doesn't restart the radio.

### The manager: policy and view

- **What it sets** is what each band may use:
  - channels and widths (0044, 0045);
  - DFS allowed or avoided (0071);
  - whether RRM is on;
  - its thresholds and margins.

  An AP or folder can still pin a channel, which RRM then leaves alone.
- **What it shows,** from the state reports:
  - each AP's neighbours, with the signal both ways;
  - its channel ratings;
  - its moves, each with why.
- **Without the manager,** APs keep their neighbours and go on rating and moving with the policy they last had.

## Consequences

- **APs that hear each other spread out on their own,** and keep doing so as things change, not only at a restart.
- **Moves cost clients little:** a channel switch announcement where the client supports it, a reconnect where it doesn't.
- **Off-channel listening costs a little airtime,** spread thin, and is held back while clients are busy.
- **A new piece runs on every AP,** with a port open on the management address to its neighbours.
- **What usteer does stays as it is.** Whether RRM later feeds usteer, or takes over its sharing, is open.

## Lab plan

Before building, on both lab APs (OfficeOpenWrt, mt76; OpenWrtnight, ath10k):

- **Vendor elements:** an AP's beacon element with its ID and address, read back from the other's scan.
- **Off-channel visits:** how long each driver is away for one channel, and what a busy client and an idle IoT device see.
- **Survey counters:** busy time and noise per channel, read repeatedly, and how often mt76 lacks the noise floor.
- **Channel switch announcement:** `ubus call hostapd.<bss> switch_chan` on both drivers. Which clients follow, and which drop: a phone, a laptop, and the Espressif and Nest devices.
- **The wire:** UDP between the APs' management addresses, through their firewalls.

## Open

- **The neighbour threshold and limit,** and the hello and dead intervals.
- **The rating's weights and smoothing,** and the margin and time a move needs.
- **The tie-break** for two APs claiming one channel.
- **Whether RRM chooses width and power too.** For now, width stays Aeolus's setting. Transmit power control is the natural next part of RRM.

## Until then

The lab's two APs share 2.4 GHz channel 11. Pinning one of them to 1 or 6 separates them now. Restarting that radio drops the Sweet_Spot_IoT devices for a few seconds, so it waits for Griff's word.
