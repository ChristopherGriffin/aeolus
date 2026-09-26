# 0025. Permissions: roles on folders

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- Roles are granted on folders and flow down within their tree (0004, 0013). A role on a Locations folder says nothing about the Services tree, and the other way round.
- Three roles to start:
  - **viewer:** can see.
  - **operator:** can change settings.
  - **admin:** can also lock, Break Hierarchy, and manage accounts and roles.
- Breaking hierarchy requires admin at the level where the escaped locks were set (0005).
- Permissions keep flowing down through a Break Hierarchy (0005).
