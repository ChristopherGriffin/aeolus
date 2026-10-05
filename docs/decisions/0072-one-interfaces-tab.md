# 0072. One Interfaces tab: radios, Ethernet and tunnels

- Status: Accepted
- Date: 2026-10-05
- Proposed by: Griff: fold the Hardware and Interfaces tabs together, since radios, Ethernet and VXLAN are all interfaces, with channels as a sub-menu under radios. Written up by Claude, and accepted with the merge
- Refines: 0047, 0053, 0055

## Context

- **A Locations folder's page has two tabs for the AP's interfaces:**
  - **Hardware** (0047), with Radios, a card per band, and Channels, the live view of what each AP picked;
  - **Interfaces** (0053, 0055), with Ethernet and Tunnels.
  - The folder's tabs are Hardware, Networks, Clients, Interfaces and System. An AP's have Overview first.
- **They are one kind of thing.** A radio, an Ethernet port and a VXLAN tunnel are each an interface of the AP, which networks are carried on. Splitting them makes people remember which tab holds which, and puts two tabs where one would do. Griff finds the UI redundant, and this is part of that.
- **Channels is about the radios.** It shows the channel and width each AP's radios are on, beside what Aeolus sets.

## Decision

- **One Interfaces tab replaces Hardware and Interfaces.** Its parts, in this order:
  - **Radios:** the band cards, as now.
  - **Ethernet:** the port cards and "Ports now", as now.
  - **Tunnels:** the tunnels set here, as now.
- **Channels is under Radios.** While Radios is open, a smaller row under it offers **Bands** (the cards, the default) and **Channels** (the live view).
- **The tabs become:**
  - on a folder: Interfaces, Networks, Clients, System;
  - on an AP: Overview, Interfaces, Networks, Clients, System.
  - Interfaces comes first, as Hardware did, so the page still reads from the interfaces to the networks carried on them (0047).
- **Addresses:**
  - `…/interfaces/radios`, `…/interfaces/radios/channels`, `…/interfaces/ethernet` and `…/interfaces/tunnels`;
  - Interfaces alone opens on Radios. Today it opens on Ethernet.
  - The old addresses still work and lead to the new ones: `…/hardware` and `…/hardware/radios` to Radios, and `…/hardware/channels` to Channels. A link or bookmark from before keeps working.
- **Moving to another folder or AP keeps the part and the view,** as it keeps the tab now (0047).
- **What refreshes stays the same:** Channels, Ethernet and Tunnels every 30 seconds, while they're shown.
- **Nothing else changes:** the cards, the editors, the previews and the API. It's the UI's arrangement only.

## Consequences

- **One tab fewer on every Locations page.** Everything about an AP's radios, ports and tunnels is in one place.
- **Ethernet and Tunnels are one more click away** when the tab opens on Radios, as Channels already is.
- **Links inside the UI** to Hardware move to Interfaces › Radios. An AP's Overview links to Interfaces › Tunnels and Ethernet already.

## Checks

- In the UI harness, on a folder and an AP:
  - each part and view shows, and a reload keeps it;
  - the old Hardware addresses lead to the new ones;
  - moving between nodes keeps the part and view;
  - Channels, Ethernet and Tunnels refresh, and Bands does not.

## As built

- **Interfaces' parts** are Radios, Ethernet and Tunnels (`interfaces.js`). Radios has a third, smaller row of tabs for its views, Bands and Channels.
- **The addresses go one level deeper:** a node's address can carry a tab, a part and a view. Bands also has an address of its own, `…/interfaces/radios/bands`.
- **An old Hardware address is read as the new one,** and the new one is put in its place in the address bar, without a reload. It works on folders and APs alike.
- **In the UI harness:**
  - `#/locations/sandbox/hardware/channels` became `…/interfaces/radios/channels`, and `…/hardware` became `…/interfaces/radios`;
  - an AP's `…/hardware/channels` did the same;
  - Interfaces alone opened on Radios › Bands;
  - Ethernet and Tunnels showed as before;
  - the tree's links kept `interfaces/radios/channels` on every node;
  - Bands didn't redraw in 34 seconds, and Channels redrew once.

## Not now

- **The wider redesign** of the UI's density, which Griff has in mind. This is a step toward it, not the whole.
- **Where System, Networks and Clients sit,** and the AP's Overview, are unchanged.
