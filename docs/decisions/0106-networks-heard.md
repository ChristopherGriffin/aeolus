# 0106. The networks the APs hear, and rogues marked known

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, as one of the features Wi-Fi managers commonly have that Aeolus lacked (Griff, "create features common in these systems")
- Refines: 0073, 0105

## Context

- **Each AP with radio resource management on reports the networks it hears** (`rrm.others`, 0105), and the manager raises a `rogue` alert for one that takes an Aeolus SSID. Nothing showed the rest: what is on the air around an AP, how strongly, and which AP hears it.
- **Wi-Fi managers commonly show this.** Meraki calls it Air Marshal, UniFi lists neighbouring APs, and Mist its rogue and neighbour APs. Each lets a person mark a same-named network as a neighbour's, so it stops alerting.
- **0105 left that for later:** a neighbour's AP of the same name is no attack, and its alert would never end.

## Decision

- **`GET /v1/airspace?under=` lists the networks the APs the caller may view hear.** Each appears once, by BSSID, with every AP that hears it, the strongest first, and when it was last heard. Each is one of four kinds:
  - `rogue`: one of Aeolus's SSIDs from a BSSID no AP names as its own (0105);
  - `known`: such a BSSID that the hearing AP's folders name in `rogues.known`;
  - `aeolus`: one of the APs' own, heard by an AP it is not a radio neighbour of, such as the C-360, which runs no RRM (0081);
  - `other`: any other network.
- **The list's order:** rogues first, then known, the APs' own and the others, each the strongest heard first. A network that is a rogue to any AP that hears it is a rogue, even where another AP's folders know it.
- **The MCP adapter offers `list_airspace`.**
- **`rogues.known` is a Locations field:** a list of BSSIDs, inherited as any list is, and the manager's own, as `notify.*` is (0101).
  - It is never sent to APs, and changing it re-versions none.
  - A BSSID it names raises no `rogue` alert from the APs below.
- **Radios › Networks heard, on a folder or an AP,** shows the list: rogues and the like in a table, the others folded away below it, with counts by kind.
  - A rogue has **Mark known…**, which adds its BSSID to `rogues.known` where the field is set, else on the page's node, in one change previewed first.
  - A known one has **Forget…**, where that list names it.
  - The buttons show only to someone who may change that node (0030).

## Consequences

- Only APs with Neighbours on listen. An AP without RRM hears nothing here, though its own BSSIDs still tell its networks from strangers'.
- A neighbour's AP that changes its BSSID, as some do on a reboot, is a rogue again until it is marked known again.
- Marking a network known takes the person's word for it. Telling an evil twin from a neighbour's AP is still for later: whether it is on our wire, its security, its vendor (0105).
