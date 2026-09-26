# 0031. The MCP adapter holds no credentials

- Status: Proposed (awaiting Griff)
- Date: 2026-09-25
- Proposed by: Claude, while building M3
- Refines: 0028

## Context

0028 has the MCP adapter run on the manager host with its own account and token. If the adapter held a token itself, anyone who could reach its endpoint would act under that token's name.

## Decision

- `aeolus serve` serves the MCP adapter at `/mcp`, on the API's HTTPS port and certificate. There is no separate service.
- **Pass-through.** Every MCP request must carry the caller's own Aeolus token. The adapter makes its API calls with that same token, through the API's HTTP handler in the same process. A request without a working token is refused before any MCP handling.
- The adapter holds no credentials and reaches the state only through the API (0003).
- Each MCP client uses its own account and token (0024). Claude's token lives in the Claude Desktop config, like the other MCP tokens there; a future agent such as Hermes gets its own account and appears in the change log under its own name.
- `make_change` requires a reason, so every change made through MCP says why.

## Consequences

- The token file `aeolus init -mcp-account` writes is a hand-over: its token is copied into the client's config once, and the file can then be deleted.
- MCP clients must trust the manager's certificate. Until the Org has its own CA (M4), the self-signed certificate is given to the client, for example through `NODE_EXTRA_CA_CERTS` for `mcp-remote`.
