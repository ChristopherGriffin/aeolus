# 0003. One API; operator access is separate

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- The manager exposes one HTTP API. Every client uses it: the UI, the CLI, the MCP adapter Claude uses, and any automation. No client gets a private path into the database.
- Operator access to the host (SSH, root on the Aeolus LXC) is for running the box, not for managing networks. Nothing in the product depends on it.

## Consequences

- The MCP server is an adapter over the API, not the API itself.
- Claude's product actions go through the API under Claude's own identity and appear in the change log like anyone else's (see 0009).
