# Aeolus

Aeolus is the manager for an OpenWrt-based Wi-Fi system. It holds what each AP should run and what each AP reports it is running. It stays out of the data path, and the network keeps working without it.

**Status: M1, the resolution engine.** Decisions are recorded in [`docs/decisions`](docs/decisions) before any code depends on them.

## Build order

Each milestone ends in something checkable before the next starts.

1. **Resolution engine** ([`internal/hierarchy`](internal/hierarchy)): both trees, field-level inheritance, locks, Break Hierarchy, service assignment, per-AP resolution.
2. **Change log and versions**: append-only log in SQLite, Org-wide sequence, per-AP version numbers.
3. **Admin API and identities**: every actor has its own identity; CLI and MCP are clients of the API.
4. **AP contract**: poll, rendered-UCI report, check, OK, apply result; AP identity and enrollment.
5. **ucode agent on a lab AP**: the full loop, with drift shown.
6. **Key channel** (0014).

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
| [0017](docs/decisions/0017-locks-remove-hidden-overrides.md) | A lock removes the overrides it hides (proposed) |
| [0018](docs/decisions/0018-network-transport.md) | Network transport: primary and fallback |
| [0019](docs/decisions/0019-server-tier-first.md) | Server tier first; election later |
| [0020](docs/decisions/0020-aps-act-on-their-own.md) | APs act on their own: transport choice and VLAN detection |
| [0021](docs/decisions/0021-concentrators-and-vni-labels.md) | Concentrators in the library, labeled VNIs |

## Open questions

- Intent model contents: the full field list for each object type.
- UI for assigning services to locations (mockup first).
- VLAN detection method, VXLAN health check and hold-down for switching back (0020).
- Scope of VNI labels: per concentrator or per folder (0021).
- Key delivery details (0014).
- Whether intent gets a narrow raw-UCI escape hatch.
- AP identity and enrollment; how config is signed.
- Time-series engine for the server-hosted manager.
- Human hold on the OK step: keep or drop.
- MSPs working across several Orgs (proposed as permissions, not a level above Org).
- Rejoining a broken folder to its parent's inheritance.
- Election: set aside until the port to APs (0019).
