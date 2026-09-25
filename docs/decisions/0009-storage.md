# 0009. Change log and conditions

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Context

Config changes and every observed condition must be kept on a time base, for change management and admin accountability.

## Decision

- **Change log, append-only.** Every change is one row: sequence number, actor, time, reason, target (folder or AP), before and after. The database refuses edits and deletes. The current desired config is derived from the log, so the config at any past moment can be reconstructed exactly. The rendered UCI and check result for each AP version (0008) are stored alongside.
- **Every actor has its own identity:** each person, each automation, and Claude. Nobody acts under someone else's name.
- **Conditions, time-series.** Observed state over time is kept in one table per kind of condition (radio, clients, health and so on), each with its own retention. The change log is kept forever.
- **Config and change log live in SQLite** on every tier. There is one implementation of the part that carries accountability.
- **Conditions sit behind a storage interface** with two backends: a time-series database on a server-hosted manager (engine not yet chosen), and short history kept mostly in RAM on an AP-hosted manager, because constant writes wear out flash (see 0010).

## Consequences

- "Who changed what, when and why" and "what was the config at time T" both have exact answers.
- Rollback is a new change (0007), never a rewrite of history.
