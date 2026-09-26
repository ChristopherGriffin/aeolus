# 0017. A lock removes the overrides it hides

- Status: Proposed (awaiting Griff)
- Date: 2026-09-25
- Proposed by: Claude, while building M1
- Refines: 0005

## Context

A lock can be added above folders that already override the locked field, and a folder can be moved under a lock. If those overrides were kept, they would have no effect while the lock stands, then come back without warning after an unlock or a Break Hierarchy. A break could then change APs at the moment it happens, which 0005 rules out.

## Decision

- Adding a lock, or moving a folder under one, removes the overrides below that the lock would hide.
- The removed overrides are shown in the change preview before it is committed, and recorded in the change log with the change.
- Setting a value below a lock is refused, so hidden overrides never exist.

## Consequences

- An unlock or a break never resurrects old values.
- A Break Hierarchy is always zero-impact: nothing under the broken folder is hidden, so every value it copies is already the value in effect.
