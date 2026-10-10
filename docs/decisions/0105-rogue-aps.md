# 0105. A network that takes one of Aeolus's SSIDs

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, as one of the features Wi-Fi managers commonly have that Aeolus lacked (Griff, "create features common in these systems")
- Refines: 0073, 0099, 0101

## Context

- **An AP broadcasting one of our SSIDs that is not ours is a rogue:** an evil twin, catching clients that trust the name, or an AP of the same name that someone set up apart.
- **Radio resource management already scans other channels** and reads every beacon heard (0073). It counted the other networks for a channel's rating, and dropped the rest.
- **On PumphouseAP, a scan held each network's BSSID and SSID** (2026-10-10).
  - Among them were the C-360's networks, `aeolus_50`, `test` and `Aeolus Lab`, from 30:86:2d:03:17:d0, 32:… and 36:…, which carry no advert: the C-360 runs no RRM, as its scan radio owns scanning (0081).
  - Only one network on a radio carries the advert in any case.
  - So a BSSID's own beacon cannot say whether it is Aeolus's.

## Decision

- **The RRM daemon keeps each network it hears that carries no advert,** on the channel it visits: its BSSID, its SSID (printable ASCII, anything else a `?`; hidden ones, empty or all zero bytes, not kept), band, channel and signal. What the manager would refuse, a channel it can't number or a signal outside -127 to 0 dBm, it leaves out, as one network the manager refused would cost the AP its whole report.
  - It keeps them for an hour, and the strongest 64 go in the AP's report as `rrm.others`.
  - A BSSID that shares its last five bytes with one that carried an advert is that radio's other network, and is left out. A radio makes its BSSIDs from one MAC, changing only the first byte.
- **Each AP reports its own BSSIDs (`bssids`),** its Wi-Fi interfaces' MACs, whether or not RRM runs on it.
- **A warning alert, `rogue`, comes from a network another AP heard** when it broadcasts the SSID of one of Aeolus's networks from a BSSID no AP named as its own. Aeolus's SSIDs are matched as an AP hears them, each byte not printable ASCII a `?`, so a stranger's `Café` is caught as `Caf??`. It is keyed by BSSID and says:
  - the network's BSSID and SSID;
  - its band and channel;
  - how strongly it was heard;
  - since when.
- **It is a warning, not critical:** a neighbour's AP of a common name is not an attack. ntfy and webhooks take only critical alerts unless set lower (0101).

## Consequences

- An AP whose own report has not come since this release may have its networks taken for strangers' until it does, within five minutes.
- A network on a channel the scanning radio does not visit goes unseen. So does one on a band no AP scans, as a radio on a DFS channel cannot scan (0073).
- Telling an evil twin from a neighbour's AP of the same name needs more than the name: whether it is on our wire (Griff's ARP probe per VLAN, airscan's), its security, its vendor. That is for later. A neighbour's can be marked known (0106).
