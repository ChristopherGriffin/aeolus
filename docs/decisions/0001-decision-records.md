# 0001. Decision records and terminology

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Context

Aeolus is built deliberately: architecture is settled before code, and code is written to the settled architecture. Trial-and-error builds cruft.

## Decision

- Every architectural decision is written in `docs/decisions/` before code depends on it.
- One decision per file, numbered, never renumbered.
- A decision that changes gets a new record that supersedes the old one. The old record is marked `Superseded by NNNN`, not edited away.
- Terminology: the config authority is the **manager**. That is its only name in code, docs, API and UI.

## Consequences

- Every piece of code can point at the record that justifies it.
- Changes of direction stay visible in history.
