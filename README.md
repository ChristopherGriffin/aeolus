# Aeolus

Aeolus is the manager for an OpenWrt-based Wi-Fi system. It holds what each AP should run and what each AP reports it is running. It stays out of the data path, and the network keeps working without it.

**Status: running on Aeolus** (`https://aeolus.symtus.com:8443`, the latest `v*` tag). M1–M3 are done. M4, the AP contract, is under way: Landing Zone, Sandbox, adoption and the concentrator library are in; APs enroll, poll for their config, have their rendered UCI checked against intent, and report what they applied and their state. The agent (M5, [`agent/`](agent)) is next on PumphouseAP, ahead of the DHCP listeners, so the whole loop runs on a real AP sooner. PumphouseAP is the lab AP. Decisions are recorded in [`docs/decisions`](docs/decisions) before any code depends on them.

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
   4. The manager side of DHCP observation (0035): relay listener and option 224 listener. Built after M5.
5. **The AP agent** (0040), on PumphouseAP, in two parts:
   1. The full loop in Sandbox: install, enroll, poll, render, check, apply with automatic revert, reports; VLAN transports. The renderer is tested against the manager's check in CI.
   2. On the AP itself: VXLAN, switching between transports and HA (0020, 0022), and VLAN detection on the uplink.
6. **Web UI** (0042, [`internal/ui`](internal/ui)), from the mockup, served by Aeolus at `/`:
   1. Read-only: both trees with values and where they come from, overrides, locks and problems; each AP's sync, reports and history; Landing Zone; all APs; the library; the change log.
   2. Editing, each change previewed and made with a reason, and the page guard (0029). Started with radio width, set by folder within what every AP can do, or per AP as a custom setting (0044); APs pick their own channels, and a width their channel cannot carry sets it to automatic (0045).
7. **Key channel** (0014).

## On the manager host

- `aeolus-update` builds and installs the newest release, and restarts the service.
- `journalctl -u aeolus -f` follows the manager's log.
- A lost token (0043):
  ```sh
  systemctl stop aeolus
  aeolus token -account griff -revoke-others
  systemctl start aeolus
  ```

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

## Open questions

- Intent model contents: the full field list for each object type.
- UI for assigning services to locations (mockup first).
- VLAN detection method and VXLAN health check (0020).
- Key delivery details (0014).
- Whether intent gets a narrow raw-UCI escape hatch.
- Config signing and an Org CA are set aside (0032).
- Time-series engine for the server-hosted manager.
- Human hold on the OK step: keep or drop.
- MSPs working across several Orgs (proposed as permissions, not a level above Org).
- Rejoining a broken folder to its parent's inheritance.
- Election: set aside until the port to APs (0019).
