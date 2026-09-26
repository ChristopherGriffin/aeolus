# 0032. Landing Zone and Sandbox

- Status: Accepted
- Date: 2026-09-26
- Decided by: Griff

## Decision

- **No Org CA and no config signing for now.** AP identity works without them (0033). Both can come later (0016).
- **Landing Zone.** New APs arrive in a folder called Landing Zone. The folder remains unconfigured.
- **Sandbox.** A folder for testing configs.

## Details (proposed by Claude, accepted by Griff)

**Landing Zone**
- It is a built-in folder in the Locations tree that every Org has and nobody can delete.
- Nothing can be set or locked on it, and nothing inherits into it. It sits outside inheritance entirely.
- An AP in Landing Zone gets no config from the manager. It keeps running whatever it already runs, and it still reports its state, so it can be seen before anyone decides what to do with it.
- An enrolling AP lands there automatically (0033).
- Moving an AP out of Landing Zone into a real folder is **adoption**. It is a logged change that needs operator on the destination folder, and from then on the AP receives that folder's config.

**Sandbox**
- It is a built-in folder in both trees. The Locations Sandbox holds test APs; the Services Sandbox holds test networks.
- Apart from being built in, it is a normal folder. It inherits from the Org, so a test runs against the real settings around it, and it can Break Hierarchy if a test needs a clean slate.
- Later: a **promote** action that moves an override tested in Sandbox to the folder where it belongs, as one logged change.

## Consequences

- Enrolling an AP never changes what it runs. Only adoption, a human decision, does.
- An existing AP that is serving clients, such as PumphouseAP, can be enrolled safely. Before it is adopted anywhere, the destination must already provide the networks its clients depend on.
