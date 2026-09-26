# 0041. Keeping rendered UCI without its secrets

- Status: Accepted
- Date: 2026-09-26
- Proposed by: Claude, correcting a conflict between 0008 and 0039; accepted by Griff
- Supersedes: 0039's rule that rendered UCI is not kept

## Context

0008 says the manager stores the exact rendered UCI each AP ran, for every version. That makes a later fleet-wide preview and diffs possible. 0039 dropped that and kept only a hash, because rendered UCI carries passphrases in plain text (0027). The two conflict, and 0039 did not say it was overriding 0008.

## Decision

- The manager keeps the UCI from every render check, with secret values replaced by `<secret>`.
- **What counts as secret:**
  - the options that hold keys and passwords (`key`, `sae_password`, `password`, `auth_secret`, `private_key`, `preshared_key`);
  - any value equal to a secret in that AP's config, wherever it appears.
- The hash stays the SHA-256 of the exact UCI the AP sent. An apply is still matched to its check by that hash (0039).
- The kept UCI shows in the AP's history, like the rest of the check.

## Consequences

- What each AP ran can be read back and compared across versions, without the conditions store holding a readable passphrase.
- A kept copy cannot be re-applied as is: its secrets are gone. It is a record, not a backup.
