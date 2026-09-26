# 0037. Library rules: who edits it, and references

- Status: Accepted
- Date: 2026-09-26
- Proposed by: Claude, while building M4; accepted by Griff with the part 2 merge
- Refines: 0023, 0029, 0030

## Decision

- **Editing.** Changing the library (concentrators and their VNIs) needs **admin at the Services root**, because every network can use it.
- **Reading.** Any account may read the library. It is what the transport pull-downs offer, so a folder-level operator needs it.
- **No dangling references.**
  - A network transport cannot be set to a concentrator the library does not have; the change is refused.
  - A concentrator, or a VNI, that a network still uses cannot be removed. The refusal lists the folders that use it, including a VNI set one folder below where its concentrator is chosen.
- **A VNI missing from its concentrator is a config problem, not a refusal (0029).** The concentrator and the VNI can be set in different folders, so they are only checked together where they meet: on each AP's composed config and on each node's page.
- **Definitions** are checked against the schema's `libraryConcentrator` (address, port 1–65535, MTU 1280–9000, scope folders that exist). VNIs are added and relabeled with their own change (`set-vni`), never inside a definition.
- **Composing an AP's config.**
  - A VXLAN transport whose concentrator is not allowed at the AP's location is left out.
  - If that removes the primary, the fallback takes its place.
  - A network left with no transport is a problem.
  - The concentrators an AP uses are attached with the address, port and MTU it needs.
