# Aeolus

Aeolus is the manager for an OpenWrt-based Wi-Fi system. It holds what each AP should run and what each AP reports it is running. It stays out of the data path, and the network keeps working without it.

**Status: running on Aeolus** (`https://aeolus.symtus.com:8443`, the latest `v*` tag). M1–M3 are done. M4, the AP contract, is done but for the DHCP listeners, which come after M5. The agent (M5, [`agent/`](agent)) runs on PumphouseAP, the lab AP: VLAN and VXLAN transports, switching between them, and with 0064, telling which VLANs reach the AP. The web UI (M6) edits and shows all of it. Decisions are recorded in [`docs/decisions`](docs/decisions) before any code depends on them.

## Build order

Each milestone ends in something checkable before the next starts.

1. **Resolution engine** ([`internal/hierarchy`](internal/hierarchy)): both trees, field-level inheritance, locks, Break Hierarchy, service assignment, per-AP resolution.
2. **Change log and versions** ([`internal/change`](internal/change), [`internal/changelog`](internal/changelog)): append-only log in SQLite, Org-wide sequence, per-AP version numbers.
3. **Admin API and identities** (0024–0028), in four parts:
   1. Field schema, validation and encrypted secrets ([`internal/schema`](internal/schema), [`internal/secret`](internal/secret)).
   2. Accounts, tokens and roles ([`internal/access`](internal/access), [`internal/change`](internal/change) `Authorize`).
   3. The HTTP API with preview ([`internal/api`](internal/api), [`cmd/aeolus`](cmd/aeolus), [API reference](docs/api.md)), running on Aeolus and updated with [`aeolus-update`](deploy/update.sh).
   4. The MCP adapter ([`internal/mcpadapter`](internal/mcpadapter), served at `/mcp`). Done when a change made through it shows up in the log under Claude's name.
4. **AP contract** (0032–0039), in four parts:
   1. Landing Zone and Sandbox built in; adoption; the manager as an actor.
   2. The concentrator library (0023) and per-AP filtering ([`internal/library`](internal/library), [`internal/compose`](internal/compose)).
   3. AP endpoints ([AP routes](docs/api.md#ap-routes)): enroll and the config poll (0038); the render check, apply and state reports, and the conditions store (0039, [`internal/rendercheck`](internal/rendercheck), [`internal/conditions`](internal/conditions)).
   4. The manager side of DHCP observation (0035, 0068): the relay listener and the option 224 listener.
5. **The AP agent** (0040), on PumphouseAP, in two parts:
   1. The full loop in Sandbox: install, enroll, poll, render, check, apply with automatic revert, reports; VLAN transports. The renderer is tested against the manager's check in CI.
   2. On the AP itself: VXLAN, switching between transports and HA (0020, 0022), and VLAN detection on the uplink.
6. **Web UI** (0042, [`internal/ui`](internal/ui)), from the mockup, served by Aeolus at `/`:
   1. Read-only: both trees with values and where they come from, overrides, locks and problems; each AP's sync, reports and history; Landing Zone; all APs; the library; the change log.
   2. Editing, each change previewed and logged with who made it and when (a note is optional, 0062), and the page guard (0029). Started with radio width, set by folder within what every AP can do, or per AP as a custom setting (0044); APs pick their own channels, and a width their channel cannot carry sets it to automatic (0045).
7. **Key channel** (0014).

## On the manager host

- `aeolus-update` builds and installs the newest release, and restarts the service.
- `journalctl -u aeolus -f` follows the manager's log.
- **The feed cache** (0069) is in `/var/lib/aeolus/feeds`, up to 2 GB. APs fetch OpenWrt's packages through it, so the manager is the only one that reaches OpenWrt's server, once for each file. Deleting the directory, with the service stopped, empties it.
- A lost token (0043):
  ```sh
  systemctl stop aeolus
  aeolus token -account griff -revoke-others
  systemctl start aeolus
  ```

## Pull requests

- **CI (`test`)** is one job: lint, the tests with ucode, and the builds for AP hardware. It runs on **talos** (CT 119 on powermox, 192.168.20.87 on VLAN 20), a self-hosted runner, which uses no GitHub minutes. Aeolus manages it over SSH with `/root/.ssh/talos_ed25519`.
  - It runs once per push to a pull request. One that changes only documentation passes in seconds.
  - It runs on `main` only when the workflow or Go's modules change, or when run by hand (`gh workflow run test.yml`) to make the caches pull requests start from again. It doesn't run on release tags.
- **The `merge-when-green` label,** put on when Griff approves, merges a pull request once `test` has passed on its head commit, if the head holds the tip of `main`.
  - The label's job marks the head it went on approved, with a `merge-when-green` commit status, and only an approved head is merged. A push after the label is never merged, and the push takes the label off.
  - The green run merges it at its end; if the label goes on later, its own job does.
  - Pull requests come from this repository's own branches; one from a fork is merged by hand.

## Decisions

| # | Decision |
|---|----------|
| [0001](docs/decisions/0001-decision-records.md) | Decision records and terminology |
| [0002](docs/decisions/0002-planes.md) | Where the manager sits |
| [0003](docs/decisions/0003-one-api.md) | One API; operator access is separate |
| [0004](docs/decisions/0004-hierarchy.md) | Hierarchy: Org, then folders to any depth |
| [0005](docs/decisions/0005-locks-and-break-hierarchy.md) | Locks and Break Hierarchy |
| [0006](docs/decisions/0006-intent-model.md) | Config is intent, not raw UCI |
| [0007](docs/decisions/0007-agent-contract.md) | APs poll; version numbers only go up |
| [0008](docs/decisions/0008-render-check-apply.md) | AP renders, manager checks, AP applies |
| [0009](docs/decisions/0009-storage.md) | Change log and conditions |
| [0010](docs/decisions/0010-two-tiers.md) | One codebase, two tiers |
| [0011](docs/decisions/0011-code-and-deployment.md) | Code and deployment |
| [0012](docs/decisions/0012-field-level-inheritance.md) | Inheritance works per field |
| [0013](docs/decisions/0013-locations-and-services.md) | Two trees: Locations and Services |
| [0014](docs/decisions/0014-per-user-keys.md) | Per-user keys have their own channel |
| [0015](docs/decisions/0015-library.md) | A global library of named definitions |
| [0016](docs/decisions/0016-open-for-extension.md) | Built open for later features |
| [0017](docs/decisions/0017-locks-remove-hidden-overrides.md) | A lock removes the overrides it hides |
| [0018](docs/decisions/0018-network-transport.md) | Network transport: primary and fallback |
| [0019](docs/decisions/0019-server-tier-first.md) | Server tier first; election later |
| [0020](docs/decisions/0020-aps-act-on-their-own.md) | APs act on their own: transport choice and VLAN detection |
| [0021](docs/decisions/0021-concentrators-and-vni-labels.md) | ~~Concentrators in the library, labeled VNIs~~ (superseded by 0023) |
| [0022](docs/decisions/0022-transport-ha-mode.md) | Transport HA mode and failback |
| [0023](docs/decisions/0023-library-concentrators-and-vnis.md) | The library holds concentrators and their VNIs, scoped to locations |
| [0024](docs/decisions/0024-identities-and-sign-in.md) | Identities and sign-in |
| [0025](docs/decisions/0025-permissions.md) | Permissions: roles on folders |
| [0026](docs/decisions/0026-api-shape.md) | API shape |
| [0027](docs/decisions/0027-field-schema-and-secrets.md) | Field schema and secrets |
| [0028](docs/decisions/0028-mcp-adapter.md) | The MCP adapter |
| [0029](docs/decisions/0029-field-checks-and-config-checks.md) | Field checks refuse changes; config checks report them |
| [0030](docs/decisions/0030-who-may-change-what.md) | Who may make which change |
| [0031](docs/decisions/0031-mcp-pass-through.md) | The MCP adapter holds no credentials |
| [0032](docs/decisions/0032-landing-zone-and-sandbox.md) | Landing Zone and Sandbox |
| [0033](docs/decisions/0033-ap-enrollment-and-identity.md) | AP enrollment and identity without a CA |
| [0034](docs/decisions/0034-detecting-openwifi-aps.md) | Detecting unconfigured OpenWiFi APs (partly superseded by 0035) |
| [0035](docs/decisions/0035-dhcp-observation.md) | DHCP observation: vendor-neutral detection and network watch |
| [0036](docs/decisions/0036-the-manager-as-an-actor.md) | The manager as an actor |
| [0037](docs/decisions/0037-library-rules.md) | Library rules: who edits it, and references |
| [0038](docs/decisions/0038-enrollment-and-the-poll.md) | Enrollment and the config poll in detail |
| [0039](docs/decisions/0039-render-check-and-reports.md) | The render check, AP reports and the conditions store |
| [0040](docs/decisions/0040-the-ap-agent.md) | The AP agent, v1 |
| [0041](docs/decisions/0041-keeping-rendered-uci.md) | Keeping rendered UCI without its secrets |
| [0042](docs/decisions/0042-the-web-ui.md) | The web UI |
| [0043](docs/decisions/0043-getting-back-in.md) | Getting back in when a token is lost |
| [0044](docs/decisions/0044-hardware-by-folder.md) | Radio hardware is set by folder, within what every AP can do |
| [0045](docs/decisions/0045-aps-pick-their-channels.md) | APs pick their own channels, and a width never waits on one |
| [0046](docs/decisions/0046-following-the-folder-again.md) | Following the folder again |
| [0047](docs/decisions/0047-folder-tabs.md) | A Locations page in tabs: Hardware, Networks, System |
| [0048](docs/decisions/0048-editing-networks-where-they-are-shown.md) | Editing networks where they are shown |
| [0049](docs/decisions/0049-multicast-to-unicast.md) | Multicast to unicast, per network |
| [0050](docs/decisions/0050-band-steering.md) | Band steering through usteer, installed with the agent |
| [0051](docs/decisions/0051-steering-and-multicast-at-hand.md) | Band steering and multicast at hand, and what usteer does |
| [0052](docs/decisions/0052-snmp.md) | SNMP through net-snmp, set like any system setting |
| [0053](docs/decisions/0053-ethernet-ports.md) | Ethernet ports, and an Interfaces tab |
| [0054](docs/decisions/0054-vxlan-transports.md) | VXLAN transports on the AP |
| [0055](docs/decisions/0055-tunnels-in-locations.md) | Tunnels are set in Locations; the library is shelved |
| [0056](docs/decisions/0056-tunnel-mtu.md) | A tunnel's MTU: default or custom, and only what the uplink carries |
| [0057](docs/decisions/0057-apply-safety.md) | An apply keeps the Wi-Fi up, and what an AP lacks is held |
| [0058](docs/decisions/0058-tunnel-ports.md) | Ethernet ports on tunnels |
| [0059](docs/decisions/0059-tunnel-probes.md) | Tunnel probes, the loop guard, and switching transports (step 1 built: probes, keepalive and the loop guard) |
| [0060](docs/decisions/0060-probe-addresses.md) | Each AP takes an address on the segments it probes |
| [0061](docs/decisions/0061-switching-transports.md) | Switching a network between its transports |
| [0062](docs/decisions/0062-optional-reasons.md) | A change needs no reason |
| [0063](docs/decisions/0063-tunnel-start-vlan.md) | A tunnel can start from any VLAN on the uplink |
| [0064](docs/decisions/0064-vlan-detection.md) | Telling which VLANs reach an AP |
| [0065](docs/decisions/0065-client-dhcp-watch.md) | Watching clients' DHCP |
| [0066](docs/decisions/0066-clients-tab.md) | A Clients tab |
| [0067](docs/decisions/0067-client-identity.md) | Who each client is, and how well it connects |
| [0068](docs/decisions/0068-manager-dhcp-listeners.md) | The manager's DHCP listeners |
| [0069](docs/decisions/0069-running-without-the-internet.md) | Running without the internet |
| [0070](docs/decisions/0070-per-user-keys-built.md) | Per-user keys, as built on OpenWrt |
| [0071](docs/decisions/0071-avoiding-dfs.md) | Avoiding DFS channels, for now |
| [0072](docs/decisions/0072-one-interfaces-tab.md) | One Interfaces tab: radios, Ethernet and tunnels |
| [0073](docs/decisions/0073-radio-neighbours.md) | Radio resource management: APs as neighbours, channels by rating |
| [0074](docs/decisions/0074-time-zones.md) | Time zones from a list |
| [0075](docs/decisions/0075-channel-sets.md) | Channel sets, and an APs tab (proposed) |

## Open questions

- Intent model contents: the full field list for each object type.
- UI for assigning services to locations (mockup first).
- VLAN detection method (0020). The VXLAN health check is 0059.
- Key delivery details (0014).
- Whether intent gets a narrow raw-UCI escape hatch.
- Config signing and an Org CA are set aside (0032).
- Time-series engine for the server-hosted manager.
- Human hold on the OK step: keep or drop.
- MSPs working across several Orgs (proposed as permissions, not a level above Org).
- Rejoining a broken folder to its parent's inheritance.
- Election: set aside until the port to APs (0019).
