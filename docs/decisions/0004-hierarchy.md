# 0004. Hierarchy: Org, then folders to any depth

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- The tree starts with an **Org** at the root. Under it, **folders** nest to any depth.
- All folders are the same kind of object. A folder may carry a label such as Building, Floor or Unit, but the label has no behavior.
- An AP can be placed in any folder, not only the lowest ones.
- Settings inherit downward. The setting closest to the AP wins, and a setting on the AP itself wins over all folders, unless a lock applies (see 0005).
- Permissions follow the same tree: a role granted on a folder covers everything below it.
- Moving a folder is a change like any other. It is logged, and before it is committed the manager shows how many APs it re-versions.

## Consequences

- There is no fixed MSP / property / building / floor / unit schema. Any shape of organization fits.
