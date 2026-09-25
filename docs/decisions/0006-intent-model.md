# 0006. Config is intent, not raw UCI

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- Config is expressed as intent: networks, VLANs, radios, neighbor links and similar objects, not raw UCI.
- The manager stores and resolves intent. APs turn intent into UCI themselves (see 0008).
- The contents of the intent model (which objects exist and their fields) are the next architecture topic and will get their own record.

## Consequences

- The manager is not tied to one OpenWrt config layout or one kind of hardware.
- Open: whether intent gets a narrow raw-UCI escape hatch.
