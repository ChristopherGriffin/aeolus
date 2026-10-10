# 0097. Who may join, and how often radios beacon

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, filling in what Wi-Fi managers commonly offer and Aeolus did not, at Griff's ask ("create features common in these systems that we might be missing")
- Refines: 0045, 0077, 0089

## Context

- **Common controls Aeolus lacked:**
  - a minimum signal, so a client far off joins a nearer AP instead of holding this one at its slowest rates;
  - a limit on clients;
  - the DTIM period, which trades sleeping clients' batteries against multicast delay;
  - the beacon interval.
- **OpenWrt has each one.**
  - `rssi_reject_assoc_rssi` and `beacon_int` go on the radio.
  - `maxassoc` and `dtim_period` go on the Wi-Fi interface. In 25.12, `maxassoc` is an alias of `max_num_sta`.
  - The script-based releases before 25.12 take the same names.

## Decision

- **`radio.<band>.min_signal`:** off, or −95 to −50 dBm. The radio refuses to let a client heard weaker than that join (`rssi_reject_assoc_rssi`). Probes are still answered, so a client can still see the network and choose another AP. off takes it away.
- **`radio.<band>.beacon_interval`:** 40 to 1000 TU (`beacon_int`). 100 is OpenWrt's default.
- **`network.<id>.max_clients`:** 1 to 2007 (`maxassoc`), on each of the network's Wi-Fi interfaces, one a band.
- **`network.<id>.dtim`:** 1 to 255 (`dtim_period`).
- **Unset:**
  - the radio settings leave the radio as it is, as its other settings do;
  - the network settings are left out of the interface, so OpenWrt's own hold.
- **The render check** wants exactly each one set, and the network ones absent when unset.
- **On Bands,** each band has a Minimum signal row and a Beacon interval row. The network editor offers the network settings from the schema, as it does every field (0048).

## Consequences

- A minimum signal set too high leaves a far client with no AP at all. −75 dBm suits most offices; −80 suits a house.
- Turning a radio setting back to OpenWrt's default means setting it so: off, or 100 TU. Unset leaves what the AP has.
