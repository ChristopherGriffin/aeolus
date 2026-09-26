# 0028. The MCP adapter

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- The MCP adapter is part of the product and lives in this repo.
- It runs on the manager host as a thin client of the API (0003), with its own account and token (0024), so Claude's changes appear under Claude's name in the change log.
