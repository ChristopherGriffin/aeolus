# 0013. Two trees: Locations and Services

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff
- Extends: 0004, 0005, 0012 (their rules apply within each tree)

## Decision

Under the Org there are two parallel trees. Both follow the same rules: folders to any depth, field-level inheritance, locks, Break Hierarchy, and permissions granted per folder.

- **Locations** describe where an AP is and its hardware: radio policy, system settings, and wired port configuration (trunk, access, LACP). APs are placed in the Locations tree.
- **Services** describe what an AP offers: networks (SSIDs) and their network portion (segments: VLAN number, VXLAN).
- Every object type belongs to exactly one tree, so the two trees cannot conflict.
- **Which service folders apply is a setting in the Locations tree.** It inherits like any other setting: every AP added under a location gets that location's services automatically, and a lower folder or a single AP can override it.
- Permissions can be granted per tree. For example, installers can manage Locations while a leasing office manages Services.

## Outside both trees

- **Neighbor links** are Org-level relationships between two APs. Editing one requires rights on both APs. Only links drawn by hand are config; neighbors the APs discover over the air are observed state.

## Open

- The UI for assigning services to locations. A mockup comes first.
