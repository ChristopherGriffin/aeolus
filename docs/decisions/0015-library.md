# 0015. A global library of named definitions

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- The Org keeps a library of named, reusable definitions. It starts with VXLAN configurations.
- Folders pull a library entry in by name from a drop-down. The folder stores a reference, not a copy.
- Editing a library entry is one logged change. It re-versions every AP that uses that entry.
- The library is built so other kinds of reusable definitions can join it later (see 0016).

## Open

- Which folders offer the VXLAN drop-down: Services folders, Locations folders, or both.
