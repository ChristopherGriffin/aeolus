# 0125. Removing a folder

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Claude, with two folders left in Griff's lab that nothing could take away: "PPSK lab (temporary)" in Services, made for a test, and "new office" in Locations, made to try prestaging. Griff picked it among the housekeeping
- Refines: 0005, 0032, 0038, 0076

## Context

- **A folder can be made, renamed and moved** (0076). An AP can be removed (0038). A folder could not.
- **A folder made by mistake, or for a test, stayed for good,** in the tree, in every list of where to move something, and in the change log's future.

## Decision

- **`remove-folder`** takes a folder out of its tree: `{kind, tree, node}`.
- **Only a folder that holds nothing:** no folder and no AP below it. What is in it is moved or removed first, each as its own change, so nothing goes that nobody named.
- **And only one nothing leans on:**
  - **a Services folder some Locations node offers stays,** whether the offer is set there or kept from before a break (0005): the answer names where;
  - **a Services folder with per-user keys stays** (0070);
  - **a Locations folder an AP template is made at stays** (0085).
- **The Org and the built-in folders stay** (0032): Landing Zone, and Sandbox in both trees.
- **What goes with it:** what was set at it, its locks, its baseline, and the roles granted on it. The change's effect lists what was set, as an AP's removal does.
- **Who may:** operator where the folder hangs, as for making one. Where roles are granted on it, admin on it, as for revoking them.
- **The page of a folder that holds nothing has Remove…,** beside Rename and Move. Its preview says what is set at it; once it is gone, the page is its parent's.

## Consequences

- **No AP's config changes,** as no AP was below it.
- **A removed folder cannot be brought back,** only made again: its ID is free, and what was set at it is in the change log.
- **The log still names it** in the changes that made and set it, by its ID.

## Not done here

- **Removing a folder with what is in it,** in one change.
- **An account's removal,** for one made by mistake.
