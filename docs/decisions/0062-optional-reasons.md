# 0062. A change needs no reason

- Status: Accepted
- Date: 2026-10-04
- Proposed by: Griff: typing a reason for every change is a chore, so the log should simply record the changes made, by whom and when; written up by Claude, and accepted with the merge
- Refines: 0026, 0031, 0042

## Context

- **The log already records what, who and when.** Each entry holds the change itself, its before and after, the account that made it, and the time (0009). The reason is one more column.
- **Two places insist on a reason:**
  - the web UI won't apply a change until one is typed (0042);
  - the MCP `make_change` tool refuses a change without one (0031).

  The API has always taken an empty reason. The Changes view leaves it out when it's empty, and the manager host's `aeolus token` fills in its own.
- **Most reasons repeat the change.** "Wider channels for the house" says no more than the entry's own "radio.5g.width = 40". Typing one for every change slows editing, and adds little that the log doesn't already show.

## Decision

- **A change needs no reason.** The log records what changed, who changed it and when, as it does now, and that is all a change requires.
- **A note can still be added, where the change doesn't speak for itself:**
  - In the web UI, the text field beside Apply becomes "Note (optional)". Apply works with it empty, and Enter in the field applies.
  - The MCP `make_change` tool takes `reason` as optional.
  - The API doesn't change: `reason` stays in `POST /v1/changes`, and may be empty or left out.
- **The log doesn't change:** it stays append-only, past entries keep their reasons, and a change without a note is stored with an empty one.
- **The Changes view** says each change is logged with who made it and when, and shows a note only where there is one, as it does now.

## Consequences

- **Editing in the web UI is preview, then Apply,** with nothing to type.
- **New entries carry a note only when someone writes one.** "Why" is no longer guaranteed to be in the log; the who, the when and the exact change still are.
- **An MCP client may leave the reason out.** Claude still adds a short note where a change isn't obvious from the change itself.
