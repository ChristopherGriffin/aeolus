# 0007. APs poll; version numbers only go up

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- **APs poll the manager.** The manager never pushes. Polling needs no inbound holes, so it also works behind CGNAT.
- **Every config carries a number.** If the manager's number for an AP is higher than the one the AP has, the AP pulls its whole config again. There are no partial updates.
- **One sequence per Org.** Numbers come from the change log's sequence (see 0009). An AP's number is the latest change that altered that AP's resolved config. A change at the Org bumps every AP; a change in one folder bumps only the APs under it.
- **Numbers only go up.** A rollback is a new change that restores earlier settings, with a new, higher number.
- **Check and download are one request.** The AP sends the number it has (HTTP conditional GET, `If-None-Match`). The manager answers `304 Not Modified` or the full config.
- **Every poll reports back.** The AP reports the version it is running, the result of its last apply, and basic health.

## Open

- How an AP proves its identity, and how it enrolls.
- How config is signed.
