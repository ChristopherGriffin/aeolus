# 0029. Field checks refuse changes; config checks report them

- Status: Proposed (awaiting Griff)
- Date: 2026-09-25
- Proposed by: Claude, while building M3
- Refines: 0008, 0027

## Context

Every field is set on its own (0012), and each write is one change (0026). Some rules tie fields together: a WPA2 network needs a passphrase, and a VXLAN transport needs a concentrator and a VNI. Building a network therefore passes through states that are incomplete, for example after its security is set and before its passphrase is.

## Decision

- **Field checks refuse.** A change is refused when a field it touches does not exist in the target tree, when a value is invalid for its field, or when a secret arrives unsealed.
- **Config checks report.** Whether an AP's whole assembled config is complete and consistent is a check result for that AP, shown before anything is published (0008), not a reason to refuse a change. An AP whose config does not pass is not sent a new one; it keeps running its last approved config.
- A change that would leave an AP unable to resolve at all, such as two service folders providing the same network, stays refused (M2).

## Consequences

- Admins can build a network step by step without being blocked halfway through.
- The preview (0026) should show which APs a change would leave with a failing config check.
