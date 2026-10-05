# 0073. Radio resource management: APs as neighbours, channels by rating

- Status: Proposed
- Date: 2026-10-05
- Proposed by: Griff: each AP still picks its own channel, but with reliable scanning and collision avoidance. APs that hear each other become neighbours and talk over the wire. Each rates every channel after each scan and keeps the ratings like a link-state protocol's link costs. Better channels become more likely to be chosen over time. A neighbour's channel is blotted out, and sudden interference moves an AP to its next best channel. Three neighbours each, ideally at −70 dBm or better. A client that can't follow a move is accepted, with planned moves in off hours. Written up by Claude
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
- **While it runs, it moves only for a clear gain.** The best channel must beat the current one by a margin, and keep doing so for a while. A small or brief difference doesn't move it, so it doesn't hop between channels.
  - **Such a planned move waits for off hours,** a window the policy sets.
- **If interference suddenly appears on its channel,** for example the channel stays very busy for some seconds, it moves at once to its next best channel. The interference is disrupting clients already, so the move costs little more.
- **Collision avoidance:**
  - Before moving, the AP tells its neighbours where it's going, and waits a short hold time.
  - If a neighbour claims the same channel in that time, one of them, by a fixed tie-break, picks again.
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
  - The agent keeps it in `/etc/aeolus/rrm.key`, mode 0600, and removes it when the poll no longer has it.
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

## Open

- **The hello and dead intervals.**
- **The rating's weights and smoothing,** and the margin and time a move needs.
- **The tie-break** for two APs claiming one channel.
- **The default off-hours window.**
- **Power management,** to be discussed later: transmit power that brings each AP's three neighbours to −70 dBm or better. Width stays Aeolus's setting.

## Until then

The lab's two APs share 2.4 GHz channel 11. Pinning one of them to 1 or 6 separates them now. Restarting that radio drops the Sweet_Spot_IoT devices for a few seconds, so it waits for Griff's word.
