# 0026. API shape

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- The API is JSON over HTTPS (0003).
- **Reads:** the trees, a node's values with where each comes from, the overrides menu (0012), an AP's resolved config, and the change log.
- **Writes:** each write is one change with a reason, and becomes one change-log entry.
- **Preview:** a call that takes a change and returns its effect, which APs it would re-version, and what a lock or move would remove (0017), without committing anything.
