# 0016. Built open for later features

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- New features arrive as new object types, or as new fields with defaults. Existing objects are not reshaped.
- The intent schema is versioned. Each AP reports the schema version it understands, and the manager never sends it anything newer.
- Out of scope for now, but not ruled out: IoT radios (Thread, BLE), MQTT forwarding, spectrum scan.

## Consequences

- Hierarchy rules (0004, 0005, 0012, 0013) are the foundation and are settled first.
