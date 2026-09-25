# Aeolus

Aeolus is the manager for an OpenWrt-based Wi-Fi system. It holds what each AP should run and what each AP reports it is running. It stays out of the data path, and the network keeps working without it.

**Status: architecture.** There is no code yet. Decisions are recorded in [`docs/decisions`](docs/decisions) before any code depends on them.

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

## Open questions

- Intent model contents: the full field list for each object type.
- UI for assigning services to locations (mockup first).
- Which folders offer the VXLAN library drop-down (0015).
- Key delivery details (0014).
- Whether intent gets a narrow raw-UCI escape hatch.
- AP identity and enrollment; how config is signed.
- Time-series engine for the server-hosted manager.
- Human hold on the OK step: keep or drop.
- MSPs working across several Orgs (proposed as permissions, not a level above Org).
- Rejoining a broken folder to its parent's inheritance.
- Election consensus details.
