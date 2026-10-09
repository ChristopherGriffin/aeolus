# 0091. Interfaces › Ethernet in one table

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff ("it's all over the place"), on a mockup Claude drew. Written up by Claude
- Refines: 0053, 0058, 0064, 0089

## Context

- **Ethernet showed the same ports twice, in two shapes.** A grid of cards, one a port, listed each set field with "Set here" and "Follow …" beside it, and most said only "Not set". Below it, a separate "Set up another port" button and form, then a "Ports now" table of every AP's ports, with an Info toggle on the uplink.

## Decision

- **The same bar as Bands (0089),** shared by both views:
  - "Inherits from", the folder directly above, and an arrow to its Ethernet.
  - Customize, or Customized here with Inherit again.
  - A node that sets no port itself shows its ports greyed out, without Edit or Add, until Customize is pressed. Customizing pauses the page's redraw, as an open form does.
- **One table, a row a port, the uplink first.** Columns: Port, Link, Mode, Carries.
  - **Link** is the AP's own on its page. On a folder it reads "2 of 3 up · 1 Gbit/s", with each AP on hover.
  - **Carries:** an access port's VLAN, a trunk's untagged and tagged VLANs, or a tunnel port's VNIs (0058). Where the value is set when not here, as "from Symtus", and on a folder, how many APs below set the port their own way.
  - **Edit** opens the port's form under its row, as it was, with Follow above it where the node sets the port.
  - **The uplink** says the AP's management is left alone and has Info, all the AP knows of it (0064), in place of Edit.
- **Add a port** is below the table, for a port no AP has reported yet.
- **"Ports now" goes:** its link and uplink detail are in the table's rows.

## Consequences

- One place shows a port's state and its settings. The "Set here" and "Follow" labels on each field are gone; Follow lives in the port's form.
