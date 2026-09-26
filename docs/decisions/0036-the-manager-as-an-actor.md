# 0036. The manager as an actor

- Status: Accepted
- Date: 2026-09-26
- Proposed by: Claude, while building M4; accepted by Griff with the part 1 merge
- Refines: 0009, 0024

## Context

Some changes are made by no person. Creating the built-in folders (0032) happens when the manager starts, and recording an enrolling AP (0033) is triggered by a device with no account. The change log still needs an actor for each of them (0009).

## Decision

- The manager logs such changes under the reserved actor **`aeolus`**. No account can be created with that name, so nobody can hold a token for it or act as it through the API.
- The manager may do exactly two things in its own name:
  - **create the built-in folders** (Landing Zone and both Sandboxes), which it does at startup when any are missing;
  - **add an AP into Landing Zone** when that AP enrolls.
- Everything else under the `aeolus` name is refused.
- A person with admin at the root of both trees can also create missing built-ins.

## Consequences

- Every change still has a named actor, and "the manager did it" is distinguishable from anything a person did.
- An Org created before Landing Zone existed gets its built-ins through one logged change, so older log entries replay unchanged.
