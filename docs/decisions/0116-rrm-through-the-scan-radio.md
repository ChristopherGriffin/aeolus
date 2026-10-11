# 0116. Radio resource management listens through the scan radio

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff, "the APs don't seem to be neighbouring with the 360 any more". Written up by Claude
- Refines: 0073, 0077, 0105
- Supersedes: 0081's rule that radio resource management stays off on an AP with a scan radio, and settles its open item

## Context

- **On an AP with a scan radio, the serving radios never scan** (0081): all scanning is the scan radio's, which airscan runs.
- **Radio resource management listened by scanning from the serving radios** (0073), so 0081 turned it off on such an AP altogether. The C-360 then put no advert in its beacons and sent no hellos. The office and pumphouse APs neighboured with each other, and the C-360 stood alone on the neighbour map.
- **Only the listening needs a scan.** The advert goes in the AP's own beacons, and hellos go over the wire.
- **The scan radio already hears everything RRM listens for.** Read on the C-360, 2026-10-10:
  - `ubus call airscan bss` lists every network in the air with its BSSID, band, channel and signal, refreshed every 5 minutes: 39 networks;
  - the scan radio's own survey (`NL80211_CMD_GET_SURVEY`) has each channel's active and busy time and noise, 98 frequencies.
  What it does not pass on is the Aeolus advert a beacon carries.

## Decision

- **RRM is on again on an AP with a scan radio.** The renderer writes the daemon's section as on any AP, with `scan 'airscan'`. The render check wants that option exactly where the AP has a scan radio.
- **There the serving radios still never scan.** Each only reads its own channel's counts, which sends nothing.
- **The daemon takes in what the scan radio heard,** asking airscan every 30 seconds and taking each of its looks in once:
  - **Neighbours:** a neighbour's radio is known by its BSSID, which every AP's hellos now carry for each radio. A radio makes its networks' BSSIDs from one MAC, changing only the first byte, so the last five bytes mark them all. The strongest is how well the AP is heard on that band.
  - **Other networks** go to the manager as before (0105), and into the channels' ratings: on 2.4 GHz those up to three channels away count too, 6 dB weaker each.
  - **Each channel's busy share and noise** are the scan radio's survey of it. The AP's own channels are rated from the serving radios' own counts where they have them, since the scan radio hears this AP's own sending as someone else's.
- **Everything after the listening is unchanged:** the three strongest per band, hellos, ratings, moves in the policy's window, and power control (0077).
- **A look over 15 minutes old is not taken in.** The daemon says so once in the log, and what it knows ages as it does for a radio that can't scan.

## Consequences

- **An AP with a scan radio learns of a neighbour from the neighbour's first hello,** not from the air: the neighbour hears its advert and writes to it. Tried on the C-360: both lab APs were up within a minute.
- **Two APs with scan radios do not find each other,** as neither hears the other's advert. That needs airscan to pass on the adverts it hears, which is airscan's to add. The lab has one such AP.
- **A neighbour running an agent from before this sends no BSSIDs,** so it is a neighbour by its hellos, with how well it is heard left blank, until it updates.
- **A radio on a DFS channel is no longer deaf there.** Linux refuses scans from an AP that must listen for radar; the scan radio is not that AP.
- **Ratings refresh every 5 minutes,** with airscan's looks, where a scanning radio gets round its channels in 3 to 6.
- **The scan radio's busy share and a serving radio's are two instruments.** They are compared channel against channel on one AP, and an AP's own channel is measured by the serving radio. If they prove to read differently enough to cause moves, the own channel's rating is the place to look.

## Not done here

- **Saying on the neighbour pages that an AP listens through its scan radio.**
- **Using airscan's spectral data** for the ratings.
