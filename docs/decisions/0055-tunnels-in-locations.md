# 0055. Tunnels are set in Locations; the library is shelved

- Status: Proposed
- Date: 2026-10-03
- Proposed by: Griff: create tunnels in the Interfaces tab of a folder or AP, and keep the library for later, as a way to copy them quickly; a network picks a tunnel; written up by Claude
- Supersedes: 0023 (where concentrators are defined)
- Refines: 0018, 0029, 0030, 0037, 0054

## Decision

- **A tunnel is a Location setting:** `concentrators.<name>`, set on a folder or AP.
  - **Its fields:** the far end's IP address (IPv4 or IPv6), its UDP port, and the MTU.
  - **Inherited like any setting (0012):** set on a folder, a tunnel reaches every AP below it, and a folder or AP below can change any of its fields.
  - **Split networks** come from this, without scopes. The same tunnel name can reach a different concentrator at each site, where 0023 needed a concentrator limited to certain folders.
- **The UI calls it a tunnel.** Interfaces › Tunnels, on a folder or AP:
  - adds a tunnel, starting at port 4789 and an MTU of 1450;
  - edits one, or puts it back to follow the folder above, field by field;
  - deletes one set there.
- **A network picks a tunnel.**
  - Its VXLAN transport names a tunnel (its `concentrator`, shown as Tunnel) and sets a VNI as a number.
  - The list offers the tunnels set where the network is being edited.
  - VNIs carry no labels for now.
- **Composing an AP's config:**
  - A transport whose tunnel is not set at the AP's location is left out there, and the fallback takes its place, as 0037 did for a concentrator that was not allowed there.
  - A network left with no transport is a problem.
  - The AP's config carries only the tunnels its networks use. An unfinished tunnel nothing uses holds no AP back; its folder's page shows it.
- **Nothing changes on the AP.**
  - The agent and the render check read the same config as under 0054.
  - So the names stay `concentrators` and `concentrator`, where the AP reads them.
- **Who may change a tunnel** is whoever may change that folder or AP (0030), as with ports. The library's rule of admin at the Services root (0037) does not apply to tunnels.
- **The library is shelved.**
  - Its page leaves the UI.
  - Its API and its changes stay, but nothing uses them, and a transport no longer has to name a library concentrator.
  - It may come back as a way to copy tunnels quickly.

## Consequences

- **More people can set tunnels.** An operator on a folder can set its tunnels, not only an Org admin.
- **Tunnels sit with their site.** Each one is shown and changed on the folder or AP it serves.
- **VNIs have no labels** until the library returns.
- **A tunnel name a site does not have** leaves that transport out there without a problem, as a concentrator not allowed there did. If it was the network's only transport, that is a problem, and the preview shows it.
