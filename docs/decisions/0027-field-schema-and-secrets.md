# 0027. Field schema and secrets

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- The v1 field list lives in one schema file in the repo: the fields from the UI mockup plus the transport, HA, concentrator and VNI decisions (0018–0023). The AP renderer will use the same file (0006, 0016).
- Every `set` is checked against the schema before it is committed.
- **Secrets.** Secret fields, such as Wi-Fi passphrases, are encrypted before they enter the change log, with a key held only on the manager host. The API never returns a secret in plain text; the manager decrypts it only when building an AP's config.

## Consequences

- The change log keeps secrets forever (0009) without keeping them readable. Anyone with a copy of the database but not the key cannot read them.
