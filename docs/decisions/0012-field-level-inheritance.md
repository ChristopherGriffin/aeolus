# 0012. Inheritance works per field

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff
- Refines: 0004, 0005

## Decision

- Every field of every object has an address, for example `network/staff/security`. Inheritance, overrides and locks work on individual fields, not whole objects.
- An AP's value for a field comes from the first folder at or above it that sets that field. The walk stops at a Break Hierarchy folder or at the Org.
- A lock on a field beats any value set below it.
- Turning off an inherited object in one place is a field override (`enabled: false`), not a deletion.
- "Revert to inherited" deletes the override. Like every change, it is logged.

## UI requirements

- Every value shows where it comes from: set here, inherited from a named folder, or locked by a named folder.
- A folder or AP has an overrides menu (on click or mouse-over) that lists only the overrides made there. Each entry links to the page where the override was made.

## Consequences

- Changing one field lower down never freezes a copy of the rest of the object, so later changes above keep flowing down.
