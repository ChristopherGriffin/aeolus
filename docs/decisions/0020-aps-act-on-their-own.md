# 0020. APs act on their own: transport choice and VLAN detection

- Status: Accepted (open points below)
- Date: 2026-09-25
- Decided by: Griff
- Resolves: the first open question in 0018 (when the fallback applies)

## Decision

- **APs do almost everything themselves.** The manager holds intent, checks rendered config, and shows what APs report. It does not make run-time decisions for them (consistent with 0002).
- **Transport choice is the AP's.** For each network, the AP runs the primary transport when it works at that AP, otherwise the fallback, and switches on its own when the active one fails.
- **One transport at a time.** An AP tears down a network's active transport before bringing up the other, so both are never up together (the bridging loop in 0018).
- **Switching is state, not config.** The rendered config (0008) carries both transports. Which one is active is observed state the AP reports, so a switch needs no OK from the manager.
- **APs detect the VLANs on their trunk** and report them to the manager. The manager compares them with the VLANs each AP's networks need and flags a VLAN that is expected but missing, which usually means it is not configured on the switch port.

## Open

- ~~How APs detect VLANs. Candidates: LLDP from the switch where it advertises VLANs; an active probe per expected VLAN (a DHCP discover, or ARP to the gateway); tagged traffic seen on the uplink as supporting evidence. A quiet VLAN shows no traffic, so absence of traffic alone does not prove a VLAN is missing.~~ Resolved by 0064: frames coming in on each VLAN the AP carries, a nudge for a quiet one, and the switch's LLDP.
- How an AP judges a VXLAN transport healthy (a reachability check to the concentrator).
- ~~Switching back to the primary.~~ Resolved by 0022: HA mode, with revertive (hold-down) or equal-weight failback.
