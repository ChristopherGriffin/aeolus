# 0008. AP renders, manager checks, AP applies

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

The AP turns intent into UCI, but it applies nothing until the manager has checked the result and answered OK:

1. The AP sees a higher number (0007) and pulls its resolved intent.
2. The AP renders UCI locally. Nothing is applied yet.
3. The AP sends the rendered UCI to the manager.
4. The manager checks it and answers OK or a list of errors.
5. On OK, the AP applies with auto-revert: if the new config cuts it off from the manager, it restores the previous config on its own. It then reports the result.

Further rules:

- The manager sends each AP its **fully resolved** intent. Inheritance, locks and breaks are worked out on the manager; the AP never sees the tree.
- APs report their hardware capabilities when they join, so the manager can reject intent a radio cannot support before publishing it.
- The check confirms that the UCI is well-formed, that it matches the intent (the manager reads the UCI back and compares), and that it respects locks.
- An OK is bound to the intent number and a hash of the rendered UCI. If the AP later renders the same intent differently, for example after a firmware upgrade, it needs a fresh OK before applying.
- The OK is automatic when the checks pass. An optional hold for human approval exists for now and may be dropped.
- The manager stores the exact rendered UCI each AP ran, for every version.
- The AP-side agent is written in ucode, which is built into OpenWrt, and needs no extra packages.

## Consequences

- While the manager is down there are no new changes, but APs keep running their last approved config, including across reboots.
- Fleet-wide preview (every affected AP's diff before approval) and canary rollout (approve one AP first) come from the staged renders.
