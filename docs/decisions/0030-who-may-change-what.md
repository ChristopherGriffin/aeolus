# 0030. Who may make which change

- Status: Proposed (awaiting Griff)
- Date: 2026-09-25
- Proposed by: Claude, while building M3
- Refines: 0024, 0025

## Decision

Every change is authorized at the moment it is committed, against the live state, so a role revoked a second earlier is already gone.

| Change | Needs |
|---|---|
| Set or revert a field | operator on the node |
| Add a folder or AP | operator on its parent |
| Move a node | operator on the node and on its new parent |
| Assign service folders | operator on the Locations node, and at least viewer on each service folder |
| Lock or unlock | admin on the node |
| Break Hierarchy | admin on the folder, and admin on every folder whose locks it escapes (0005) |
| Add an account | admin on at least one folder |
| Grant or revoke a role | admin on the folder the role is on |
| Issue or revoke a token | the token's own account, or admin at the Org root |

- **Roles are granted on folders, not APs.**
- **Bootstrap.** Creating the Org names its first account, which becomes admin at the root of both trees in the same change. There is never a moment without an admin, and no window for someone else to claim it.
- **Tokens.**
  - Format: `aeolus1.<id>.<secret>`, where the secret is 32 random bytes.
  - Only the ID and a SHA-256 hash of the secret are recorded.
  - The plain token is shown once, when it is issued.
  - A revoked token stays revoked.

## Open

- Removing or disabling accounts (revoking all of an account's tokens covers it for now).
