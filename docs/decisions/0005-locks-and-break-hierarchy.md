# 0005. Locks and Break Hierarchy

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Context

Upper folders need to enforce settings (for example, an org-wide security policy). A sub-org also needs to be able to grow into two or three configurations organically without rewriting the whole hierarchy.

## Decision

- A folder can **lock** a setting. Below a lock, no folder or AP can override it.
- There are two ways out from under a lock:
  1. **Move** the folder under a different parent. It then inherits the new parent's settings and locks.
  2. Set **Break Hierarchy** on the folder. The folder becomes the start of a new inheritance branch: locks from above no longer apply, and it owns its configuration and the inheritance below it. The UI shows broken folders in a different color.
- Breaking requires rights at the level where the lock was set. Rights on the folder alone are not enough.
- A broken folder starts from a copy of what it was inheriting at the moment of the break. No AP changes when the break happens. The branch diverges only through later, logged changes.
- Break Hierarchy affects configuration only. Permissions keep flowing down through a break, so admins above still see and manage the branch.

## Consequences

- Lock enforcement cannot be escaped by someone who only controls a sub-folder.
- Resolving an AP's config walks up the tree until it reaches the Org or a broken folder.
