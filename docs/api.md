# Aeolus API, v1

JSON over HTTPS (0026). Every request except `/healthz` carries `Authorization: Bearer <token>`. A node you cannot view answers `404`.

## Reads

| Request | Returns | Needs |
|---|---|---|
| `GET /healthz` | `{ok, seq}` | nothing |
| `GET /v1/whoami` | your account and grants | a token |
| `GET /v1/trees/{locations\|services}` | the nodes you can view, root first | viewer on each node |
| `GET /v1/trees/{tree}/nodes/{id}` | the node, its ancestry, your role there, every field with its value, `from` and `origin` (`self`, `inherited`, `locked`, `baseline`), the overrides menu (`in_effect`, `below`), `locks_above`, and `problems`: the rules its config breaks (0029) | viewer |
| `GET /v1/aps/{id}/config` | the AP's resolved Location fields and networks with origins, its service folders, its `version`, and `check: {ok, problems}` | viewer on the AP |
| `GET /v1/changes?after=N&limit=M` | change-log entries after `N` (limit 1 to 1000, default 100) | viewer at the Org root of either tree |

Secrets are never returned: a secret value appears as `{"sealed": true}`, and token hashes are dropped.

## Writes

| Request | Body | Returns |
|---|---|---|
| `POST /v1/changes` | `{op, reason}` | the logged `change`, the APs it `reversioned`, and `checks` for any of them whose config now fails |
| `POST /v1/preview` | `{op}` | the `effect` (before, after, overrides a lock or move would remove), the APs it would re-version, and their `checks`; nothing is recorded |
| `POST /v1/tokens` | `{account?, reason?}` | a new token for you or, as Org admin, for another account, shown once |
| `DELETE /v1/tokens/{id}` | | the logged revocation |

An `op` is one change, as the change log records it:

```json
{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.ssid", "value": "Sweet Spot"}
```

Kinds: `add-folder`, `add-ap`, `move`, `set`, `unset`, `lock`, `unlock`, `break-hierarchy`, `assign-services`, `add-builtins`, `add-account`, `grant`, `revoke`, `revoke-token`. Moving an AP out of Landing Zone (a node with `"isolated": true`) is adoption: it needs viewer on Landing Zone and operator on the destination (0032). `create-org` happens only through `aeolus init` on the manager host, and tokens are issued through `/v1/tokens`. Set values are checked against the field schema (`internal/schema/v1.json`), and secret values are sealed before they are logged (0027). Who may make which change is 0030.

## MCP

`/mcp` serves the API as MCP tools (streamable HTTP): `whoami`, `list_tree`, `get_node`, `get_ap_config`, `list_changes`, `preview_change` and `make_change`. Each request carries the caller's own token, and the tools call the API with it, so changes are logged under the caller's name (0031). `make_change` requires a reason.

## Errors

`{"error": "..."}` with `400` (invalid change or value), `401` (no or bad token), `403` (not permitted), `404` (not found or not visible), `409` (conflicts with the current state: a lock, an AP that would stop resolving, something that already exists).
