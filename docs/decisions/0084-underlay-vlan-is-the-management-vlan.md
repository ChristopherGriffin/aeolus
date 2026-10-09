# 0084. A tunnel's underlay VLAN may be the AP's management VLAN

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff, after an inherited setting broke the C-360: individualize settings en masse, or let an AP ignore one that doesn't fit it. Written up by Claude
- Refines: 0063

## Context

- **A concentrator's `underlay_vlan` is set once, on a folder** (0063). Every AP below it starts its tunnels from that VLAN of its uplink, with an interface of Aeolus's own that takes an address there by DHCP.
- **APs below one folder are not all managed on the same VLAN.** My House sets `concentrators.arista.underlay_vlan = 20` for APs managed on VLAN 1. The C-360 is managed on VLAN 20 itself (`vlan20`, on `br-lan.20`).
- **The agent refused that:** "VLAN 20 is the management VLAN here, which tunnels start from anyway; set it to the management VLAN". The C-360's config was held. Change 82 set `underlay_vlan = 0` on that AP alone, as a per-AP override.

## Decision

- **On the VLAN an AP is managed on, its tunnels start from its management interface,** as with no underlay VLAN. The tunnel's `tunlink` is the management interface, and its firewall rule's source is the management interface's zone. Aeolus makes no interface of its own there: a second DHCP client on that VLAN, from the same MAC, would share the management interface's address (0063's `from_shared`).
- **"Managed on" means the management interface is that VLAN of the uplink's bridge**, `<bridge>.<vlan>`. The agent knows its management interface; the manager's render check, which sees only the UCI, takes any interface on that VLAN of the uplink's bridge that is not Aeolus's own.
- **Elsewhere nothing changes:** an AP managed on another VLAN starts from Aeolus's own interface on the one named.

## Consequences

- **One folder value fits APs managed on different VLANs.** The C-360's override (change 82) can be unset once a manager with this release runs; until then the override keeps it working.
- **The path is the same either way:** the tunnel leaves by VLAN 20. Only which interface holds the address differs, and on the management VLAN the AP has one already.
- **Tested:** the case `underlay-management` (a C-360 managed on VLAN 20, its folder's tunnels starting from VLAN 20) renders `tunlink 'vlan20'`, the rule from zone `lan`, and no `aeolus_vlan20_tunnels`, without errors. The manager's check accepts it, and still wants Aeolus's own interface when the AP's interface on that VLAN is not on the uplink's bridge.
