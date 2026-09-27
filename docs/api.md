# Aeolus API, v1

JSON over HTTPS (0026). A browser opening `/` gets the web UI (0042), which uses this same API with the person's own token; `/` asked for as JSON returns pointers. Every request except `/healthz` and `/v1/enroll` carries `Authorization: Bearer <token>`: an account's token, or on the AP routes an AP's. A node you cannot view answers `404`.

## Reads

| Request | Returns | Needs |
|---|---|---|
| `GET /healthz` | `{ok, seq}` | nothing |
| `GET /v1/whoami` | your account and grants | a token |
| `GET /v1/trees/{locations\|services}` | the nodes you can view, root first | viewer on each node |
| `GET /v1/trees/{tree}/nodes/{id}` | the node, its ancestry, your role there, every field with its value, `from` and `origin` (`self`, `inherited`, `locked`, `baseline`), the overrides menu (`in_effect`, `below`), `locks_above`, `problems`: the rules its config breaks (0029), and for an enrolled AP the `facts` it sent (0038) | viewer |
| `GET /v1/aps` | every AP you can view: `id`, `name`, `ancestry`, `version`, `config` (`ready`, `held` or `unassigned`), `problems` (how many), `seen` (when, from where, the version it runs) and `in_sync` (0042) | viewer on each AP |
| `GET /v1/aps/{id}/config` | the AP's resolved Location fields and networks with origins, its service folders, its `version`, whether it is `unassigned` (in Landing Zone), the composed `document` it will receive (secrets sealed), `check: {ok, problems}`, and `condition`: when it was last `seen` and from where, the version it runs, `in_sync` (whether that is its current version), and its latest render `check`, `apply` and `state` report (0039) | viewer on the AP |
| `GET /v1/aps/{id}/history?limit=N` | the AP's recent render `checks` (each with the `uci` it sent, secrets shown as `<secret>`, 0041), `applies` and `states`, newest first (limit 1 to 200, default 20) | viewer on the AP |
| `GET /v1/library` | the concentrators, with their labeled VNIs and the Location folders they may be used at (0023) | a token |
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

Kinds: `add-folder`, `add-ap`, `remove-ap` (0038), `move`, `set`, `unset`, `lock`, `unlock`, `break-hierarchy`, `assign-services`, `add-builtins`, `add-account`, `grant`, `revoke`, `revoke-token`, and for the library `set-concentrator` (`concentrator`, `value: {name, address, port, mtu, scope}`), `remove-concentrator`, `set-vni` (`concentrator`, `vni`, label in `name`) and `remove-vni` (0037). Moving an AP out of Landing Zone (a node with `"isolated": true`) is adoption: it needs viewer on Landing Zone and operator on the destination (0032). `create-org` happens only through `aeolus init` on the manager host, and tokens are issued through `/v1/tokens`. Set values are checked against the field schema (`internal/schema/v1.json`), and secret values are sealed before they are logged (0027). Who may make which change is 0030.

## AP routes

What an AP uses (0033, 0038). An AP token starts `aeolusap1.` and works only here; account tokens do not work here.

| Request | Body | Returns |
|---|---|---|
| `POST /v1/enroll` | no token; `{mac, macs?, hostname?, model?, board?, openwrt?, radios?}` | `201` with the AP's `ap` ID (`ap-` and its MAC's hex digits), `name` and `token`, shown once. The AP lands in Landing Zone. `409` if that ID is already known; `503` if Landing Zone is full (250). |
| `GET /v1/ap/config` | `If-None-Match: "<version it runs>"` | `304` if unchanged; otherwise `{ap, version, state}`, where `state` is `unassigned` (in Landing Zone), `held` (with `problems`; keep running what you have) or `ready` (with `config`, secrets included, and the version as the `ETag`) |
| `POST /v1/ap/render` | `{version, uci}`: the rendered UCI as `uci export` text | `{result, problems, version, hash}`: `ok` (apply it), `refused` (the UCI is malformed, misses the intent, or the config is held) or `stale` (poll again). The rules are 0039 and `internal/rendercheck`. |
| `POST /v1/ap/applied` | `{version, hash, ok, error?}` after each apply attempt | `{recorded, checked}`: whether an `ok` check covered that version and hash |
| `POST /v1/ap/state` | `{version, uptime, openwrt?, radios?: [{radio, band, channel, width, clients}], vlans?: [IDs seen on the uplink], transports?: {network: {active, primary, fallback}}}` | `{recorded}`. `active` is `primary`, `fallback` or `none`; health is `up`, `down` or `unknown`. |

Every AP request updates when it was last seen. An AP in Landing Zone can only poll: its checks and reports answer `409` and are not recorded. The UCI from each check is kept with its secrets blanked (0041); a stale one is not kept. The agent that uses these routes is in [`agent/`](../agent).

## MCP

`/mcp` serves the API as MCP tools (streamable HTTP): `whoami`, `list_tree`, `get_node`, `get_ap_config`, `get_ap_history`, `get_library`, `list_changes`, `preview_change` and `make_change`. Each request carries the caller's own token, and the tools call the API with it, so changes are logged under the caller's name (0031). `make_change` requires a reason.

## Errors

`{"error": "..."}` with `400` (invalid change or value), `401` (no or bad token), `403` (not permitted), `404` (not found or not visible), `409` (conflicts with the current state: a lock, an AP that would stop resolving, something that already exists), `503` (Landing Zone is full).
