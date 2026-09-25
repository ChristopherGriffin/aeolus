# 0002. Where the manager sits

- Status: Accepted
- Date: 2026-09-20
- Decided by: Griff

## Decision

- **Data plane: local on each AP.** Each AP forwards its own traffic (including local VXLAN VTEP termination). No traffic goes through the manager.
- **Control plane: distributed.** Neighboring APs coordinate peer to peer (roaming keys, RF coordination). It is not centralized.
- **Manager: desired and observed state.** The manager holds each AP's desired config, each AP's last reported observed state, and the difference between them. It is out of the data path and out of the live control path.
- **The network runs without the manager.** If the manager is down, APs keep their config, neighbors keep coordinating, and clients keep roaming. When the manager returns, edits and drift reconciliation resume.

## Consequences

- The manager is a desired-state reconciliation service, not a network appliance.
- Client state never lives in the manager.
