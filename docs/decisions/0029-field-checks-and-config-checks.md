# 0029. Field checks refuse changes; config checks report them

- Status: Accepted
- Date: 2026-09-25
- Proposed by: Claude, while building M3; accepted by Griff with the UI guard below
- Refines: 0008, 0027

## Context

Every field is set on its own (0012), and each write is one change (0026). Some rules tie fields together: a WPA2 network needs a passphrase, and a VXLAN transport needs a concentrator and a VNI. Building a network therefore passes through states that are incomplete, for example after its security is set and before its passphrase is.

## Decision

- **Field checks refuse.** A change is refused when a field it touches does not exist in the target tree, when a value is invalid for its field, or when a secret arrives unsealed.
- **Config checks report.** Whether an AP's whole assembled config is complete and consistent is a check result for that AP, shown before anything is published (0008), not a reason to refuse a change. An AP whose config does not pass is not sent a new one; it keeps running its last approved config.
- A change that would leave an AP unable to resolve at all, such as two service folders providing the same network, stays refused (M2).
- **UI guard (Griff).** The UI does not let an admin leave the page of anything they are editing while it breaks a rule that ties fields together. That covers every such rule in the schema, now and later, for example:
  - a network whose security needs a passphrase, without one of at least 8 characters;
  - a transport missing its VLAN, or its concentrator and VNI;
  - static management addressing without an address and gateway;
  - an LACP port without a bond.

  Changes are still saved one field at a time; the guard only keeps the admin on the page until everything there is filled in correctly.

## Consequences

- Admins can build a network step by step without being blocked halfway through.
- The preview (0026) should show which APs a change would leave with a failing config check.
- The API reports, for each object being edited, which of these rules are unmet, so the UI can enforce the guard.
