# 0019. Server tier first; election later

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff
- Supersedes: in 0010, "both tiers are tested from day one" and "election is designed now"

## Decision

- The manager is built and proven on the server tier first: the Aeolus container. It is ported to APs afterwards.
- Election (an AP chosen among eligible APs to host the manager) is set aside. It may turn out not to be practical.
- When election is picked up, eligibility weighs uplink port speed, CPU and memory.
- Still in force from 0010: one Go codebase that cross-compiles for AP hardware. CI keeps building for AP targets so that nothing is added that would block the port, such as a C dependency. This is a build check only; there is no AP-tier testing until the port.

## Consequences

- The AP-tier backend for conditions (0009) and replication with Raft (0010) are not built until the port.
- Interfaces for tier-specific parts are added when the port needs them, not ahead of time.
