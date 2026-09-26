# 0022. Transport HA mode and failback

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff
- Extends: 0020 (resolves its open point on switching back)

## Decision

- A network's transports can run in **HA mode**: both the primary and the fallback stay established and health-checked, and the AP switches only after the active one is marked bad. Without HA mode, the standby is brought up only when needed.
- In HA mode, "one transport at a time" (0020) means one transport is attached to the network and carries its traffic. The standby is up and monitored but not attached, so switching is a re-attach, not a tunnel setup.
- **Failback** is set per network, one of:
  - **Revertive:** return to the primary once it has been healthy again for a set hold-down time.
  - **Equal weight:** no preference. Stay on whichever transport is active until it fails.
- HA mode and failback are ordinary network fields, so they inherit and can be overridden like any other (0012).

## Consequences

- Both transports never carry the same network's traffic at once, including in equal weight, because that would form a bridging loop (0018). True active/active belongs to the concentrators: an MLAG pair sharing one VTEP address appears to the AP as a single tunnel.
- The AP reports the health of both transports, not only the active one, so the manager can show a standby that has gone bad before it is needed.
