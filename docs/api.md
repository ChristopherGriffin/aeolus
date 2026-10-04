# Aeolus API, v1

JSON over HTTPS (0026). A browser opening `/` gets the web UI (0042), which uses this same API with the person's own token; `/` asked for as JSON returns pointers. Every request except `/healthz` and `/v1/enroll` carries `Authorization: Bearer <token>`: an account's token, or on the AP routes an AP's. A node you cannot view answers `404`.

## Reads

| Request | Returns | Needs |
|---|---|---|
| `GET /healthz` | `{ok, seq}` | nothing |
| `GET /v1/whoami` | your account and grants | a token |
| `GET /v1/trees/{locations\|services}` | the nodes you can view, root first | viewer on each node |
| `GET /v1/trees/{tree}/nodes/{id}` | the node, its ancestry, your role there, every field with its value, `from` and `origin` (`self`, `inherited`, `locked`, `baseline`), the overrides menu (`in_effect`, `below`), `locks_above`, `problems`: the rules its config breaks (0029), for an enrolled AP the `facts` it sent (0038), and on Locations nodes `hardware`: the APs a setting here reaches, those that never reported their radios (`unknown`), and for each band the `widths` every one of them can use, each with `ok` and, if not, `why` (0044); a width some AP's fixed channel cannot carry comes with `auto`: set the channel to `auto` with it, which `moves` those APs (`id`, `name`, `from`) off their fixed channels; `radar` marks a width sure to put an AP on radar (DFS) channels (0045) | viewer |
| `GET /v1/aps` | every AP you can view: `id`, `name`, `ancestry`, `version`, `config` (`ready`, `held` or `unassigned`), `problems` (how many), `seen` (when, from where, the version it runs) and `in_sync` (0042) | viewer on each AP |
| `GET /v1/aps/{id}/config` | the AP's resolved Location fields and networks with origins, its service folders, its `version`, whether it is `unassigned` (in Landing Zone), the composed `document` it will receive (secrets sealed), `check: {ok, problems}`, and `condition`: when it was last `seen` and from where, the version it runs, `in_sync` (whether that is its current version), and its latest render `check`, `apply` and `state` report (0039) | viewer on the AP |
| `GET /v1/aps/{id}/history?limit=N` | the AP's recent render `checks` (each with the `uci` it sent, secrets shown as `<secret>`, 0041), `applies` and `states`, newest first (limit 1 to 200, default 20) | viewer on the AP |
| `GET /v1/library` | the concentrators, with their labeled VNIs and the Location folders they may be used at (0023). The library is shelved (0055): tunnels are Location fields, `concentrators.<name>.address`, `.port` and `.mtu`, and a network transport's `concentrator` names one. | a token |
| `GET /v1/schema` | the fields that can be set: `fields`, each as its JSON Schema with `x-aeolus-tree` (the tree it is set in) and `writeOnly` for secrets, by path with `*` for any name, a list's `items` described in place; and `names`, the pattern a name must match where a path takes any name, by the path before it. Clients build their inputs from it (0048). | a token |
| `GET /v1/changes?after=N&limit=M` | change-log entries after `N` (limit 1 to 1000, default 100) | viewer at the Org root of either tree |

Secrets are never returned: a secret value appears as `{"sealed": true}`, and token hashes are dropped.

## Writes

| Request | Body | Returns |
|---|---|---|
| `POST /v1/changes` | `{op, reason?}`: a note for the log, which records who made the change and when without one (0062) | the logged `change`, the APs it `reversioned`, and `checks` for any of them whose config now fails |
| `POST /v1/preview` | `{op}` | the `effect` (before, after, overrides a lock or move would remove), the APs it would re-version, their `checks`, and for a set or unset, how each field it touches would resolve at its node (`resolved`) afterwards (`value`, `from`, `origin`, or null if nothing would set it; 0046); nothing is recorded |
| `POST /v1/tokens` | `{account?, reason?}` | a new token for you or, as Org admin, for another account, shown once |
| `DELETE /v1/tokens/{id}` | | the logged revocation |

An `op` is one change, as the change log records it:

```json
{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.ssid", "value": "Sweet Spot"}
```

A `set` can instead carry `values`, several fields of one node set together, all or none, as one logged change (0045):

```json
{"kind": "set", "tree": "locations", "node": "sandbox", "values": {"radio.5g.width": 160, "radio.5g.channel": "auto"}}
```

An `unset` can likewise carry `paths`, several fields of one node unset together. Each must be set there, or none is unset (0046):

```json
{"kind": "unset", "tree": "locations", "node": "ap-a0046021365e", "paths": ["radio.5g.channel", "radio.5g.width"]}
```

Kinds: `add-folder`, `add-ap`, `remove-ap` (0038), `move`, `set`, `unset`, `lock`, `unlock`, `break-hierarchy`, `assign-services`, `add-builtins`, `add-account`, `grant`, `revoke`, `revoke-token`, and for the library, which is shelved (0055), `set-concentrator` (`concentrator`, `value: {name, address, port, mtu, scope}`), `remove-concentrator`, `set-vni` (`concentrator`, `vni`, label in `name`) and `remove-vni` (0037). A tunnel is set like any Location field (`concentrators.<name>.address`, `.port`, `.mtu`), and so is what a tunnel port carries (0058): `ports.<name>.mode` `tunnel`, and `ports.<name>.vxlan.<vlan>.tunnel` and `.vni` for each VNI, where `<vlan>` is the VLAN it is tagged with on the port, or `untagged`. What the AP's prober does is set the same way (0059): `concentrators.<name>.probe_interval`, and a probe address, `.probe`, on a network's transport and on a tunnel port's VNI. Moving an AP out of Landing Zone (a node with `"isolated": true`) is adoption: it needs viewer on Landing Zone and operator on the destination (0032). `create-org` happens only through `aeolus init` on the manager host, and tokens are issued through `/v1/tokens`. Set values are checked against the field schema (`internal/schema/v1.json`), and secret values are sealed before they are logged (0027). Who may make which change is 0030.

## AP routes

What an AP uses (0033, 0038). An AP token starts `aeolusap1.` and works only here; account tokens do not work here.

| Request | Body | Returns |
|---|---|---|
| `POST /v1/enroll` | no token; `{mac, macs?, hostname?, model?, board?, openwrt?, radios?}` | `201` with the AP's `ap` ID (`ap-` and its MAC's hex digits), `name` and `token`, shown once. The AP lands in Landing Zone. `409` if that ID is already known; `503` if Landing Zone is full (250). |
| `GET /v1/ap/config` | `If-None-Match: "<version it runs>"`, except on the agent's first poll after it starts, which asks for the config whole so that an updated agent renders it again | `304` if unchanged; otherwise `{ap, version, state}`, where `state` is `unassigned` (in Landing Zone), `held` (with `problems`; keep running what you have) or `ready` (with `config`, secrets included, and the version as the `ETag`) |
| `POST /v1/ap/render` | `{version, uci}`: the rendered UCI as `uci export` text | `{result, problems, version, hash}`: `ok` (apply it), `refused` (the UCI is malformed, misses the intent, or the config is held) or `stale` (poll again). The rules are 0039 and `internal/rendercheck`. |
| `POST /v1/ap/applied` | `{version, hash, ok, error?}` after each apply attempt | `{recorded, checked}`: whether an `ok` check covered that version and hash |
| `POST /v1/ap/state` | `{version, uptime, openwrt?, radios?: [{radio, band, channel, width, clients}], transports?: {network: {active, primary, fallback, last_switch?: {from, to, why, ago}, cannot_switch?}}, steering?: {installed, running, interval, ssids?, bss?: [{ssid, band, clients, steered_away, steered_in}], bss_transition?}, ports?: [{name, up, carrier, speed?, uplink?}], vxlan?: {installed, loaded?, clamp, prober?, uplink_mtu?, tunnels?: [{vni, peer, port, mtu, from_vlan?, from_address?, up, standby?, probe?: {verdict, interval, asks?, underlay, underlay_ms, from?, rtt_ms, answered_ago, lease?: {address, server?, router?, expires_in}}}], loops?: [{port, device, vni?, came_in?, ago}]}, vlan_probes?: [{vlan, tagged, probe}], uplink_vlans?: [{vlan, tagged, verdict, heard_ago}], uplink_neighbor?: {chassis?, chassis_kind?, system?, system_description?, port?, port_kind?, port_description?, ttl?, capabilities?, enabled_capabilities?, management?: [{address, interface, interface_kind}], native_vlan?, vlans?, vlan_names?, protocol_vlans?, protocols?, aggregation?: {capable, enabled, port}, max_frame?, mac_phy?: {autoneg_supported, autoneg_enabled, advertised, mau, mau_name?}, power?: {pse, supported, enabled, pair?, class?, requested_w?, allocated_w?}, med?: {capabilities?, class?, policies?, inventory?, location?, power_w?}, other?: [{type, oui?, subtype?, data}], ago}, uplink_port?: {name, mac, mtu, carrier_changes, vlans?: [{vlan, tagged}], management_vlan?, rx_bytes, tx_bytes, rx_packets, tx_packets, rx_errors, tx_errors, rx_dropped, tx_dropped}, dhcp?: {network: {servers: [{id, mac, answers, ago}], answered, unanswered, unanswered_clients, duplicates: [{servers, client, ago}], without: [{mac, joined_ago, address}]}}}` | `{recorded}`. `dhcp` is what the AP saw of its Wi-Fi clients' DHCP on each Aeolus network (0065): the servers that answered anyone on the segment, by server ID and MAC, with how many answers and when last; how many of the clients' own requests in the last 10 minutes were answered and how many not, and which clients made those; the requests more than one server answered; and the clients that asked nothing within 60 seconds of joining, each with the address it shows, or null. `uplink_vlans` are the VLANs the AP carries on its uplink for its intent (0064), at most 64, each with whether a frame came in on it from the switch: `present` (within three minutes), `silent` (none for three minutes, though nudged), or `unknown` (watched for less than that); `heard_ago` is seconds since the last. `uplink_neighbor` is all the switch's LLDP says, as the prober reads it: the base TLVs, 802.1's, 802.3's and LLDP-MED's, with what it doesn't know as hex in `other`, and seconds since it last said so. `uplink_port` is the uplink itself: its name, MAC and MTU, how often its link has come and gone, the VLANs it carries as the network config has them, the management VLAN, and its counters. `transports` are the networks with a fallback (0061): which transport the AP has in the network's own bridge, `active` (`primary`, `fallback` or `none`), and each transport's health, the prober's verdict on it (`up`, `down`, `unverified`, `unknown`, or `off` for a VXLAN fallback that waits, stopped). With automatic switching, `last_switch` is the network's last move between its transports, from which to which, why, and seconds ago; `cannot_switch` says why a move that is due cannot be made, such as a fallback that does not answer. `vlan_probes` are the prober's probes of those networks' VLAN transports, made on the uplink, `tagged` or not as the uplink carries the VLAN, from the VLAN's own MAC; each `probe` reads as a tunnel's does, its `underlay` being whether the uplink has a link. `steering` is what usteer is doing (0051): `interval` is its band steering interval in ms, 0 when off; `steered_away` and `steered_in` count the clients it moved off and onto each SSID's band since it started. `ports` are the Ethernet ports in the bridge the uplink is in (0053), but for the networks' veth ends (0061), then those on tunnels, which have left it (0058), at most 32: `speed` is Mbit/s and duplex, such as `1000F`, while it has a link. `vxlan` is the AP's tunnels (0054), at most 64: `installed` and `clamp` say whether `vxlan` and `kmod-nft-bridge` are installed, `uplink_mtu` is what the AP's uplink carries now, which a tunnel's MTU plus its headers must fit (0056), `loaded` whether netifd has loaded vxlan, and `steering.bss_transition` whether hostapd has 802.11v; a config asking for what the AP reported it lacks is held (0057), and a fallback's tunnel is `standby` until the AP needs it, or always started in HA mode (0061). A tunnel that starts from a VLAN of the uplink (0063) says which, `from_vlan`, and the AP's address there, `from_address`, which it has only once that VLAN's DHCP answers. `prober` says whether ucode-mod-socket, which the prober needs, is installed. A tunnel's `probe` is what the prober found (0059): its `verdict` (`up`, `down`, `unverified`, `unknown` or `off`), its interval, the addresses it `asks`, whether the concentrator answers a ping (`underlay`, null until known) and in how many ms, and what on the segment last answered (`from`), in how many ms and how many seconds ago. Its `lease` is the IPv4 address the AP leased on the segment with its own MAC there (0060), the DHCP server and gateway that gave it, and the seconds it has left; the probes ask the override probe addresses, else that gateway, else that server, and come from that address. `loops` are the tunnel ports the loop guard took off their tunnels: the device whose frame came back, its VNI, the device it came back in on, and seconds ago. |

Every AP request updates when it was last seen. An AP in Landing Zone can only poll: its checks and reports answer `409` and are not recorded. The UCI from each check is kept with its secrets blanked (0041); a stale one is not kept. The agent that uses these routes is in [`agent/`](../agent).

## MCP

`/mcp` serves the API as MCP tools (streamable HTTP): `whoami`, `list_tree`, `get_node`, `get_ap_config`, `get_ap_history`, `get_library`, `list_changes`, `preview_change` and `make_change`. Each request carries the caller's own token, and the tools call the API with it, so changes are logged under the caller's name (0031). `make_change` takes a reason as an optional note (0062).

## Errors

`{"error": "..."}` with `400` (invalid change or value), `401` (no or bad token), `403` (not permitted), `404` (not found or not visible), `409` (conflicts with the current state: a lock, an AP that would stop resolving, something that already exists), `503` (Landing Zone is full).
