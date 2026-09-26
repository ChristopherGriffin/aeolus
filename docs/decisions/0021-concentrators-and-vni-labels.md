# 0021. Concentrators in the library, labeled VNIs

- Status: Accepted (open point below)
- Date: 2026-09-25
- Decided by: Griff
- Resolves: the second open question in 0018 (library shape)

## Decision

- The library holds **concentrators**: name, routable address, UDP port (default 4789) and MTU.
- A network's VXLAN transport picks a concentrator and a VNI.
- Entering a new VNI asks for a **label**. The label and VNI are saved together, and from then on appear as a pull-down when picking a VNI for other networks at that location.

## Open

- The scope of the label list: per concentrator (every network tunneling to that concentrator sees its labeled VNIs), or per folder (visible at the folder where the label was made and below it).
