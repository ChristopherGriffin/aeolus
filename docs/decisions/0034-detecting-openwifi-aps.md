# 0034. Detecting unconfigured OpenWiFi APs

- Status: Accepted
- Date: 2026-09-26
- Decided by: Griff

## Context

OpenWiFi (uCentral) APs run no Aeolus agent, so they never enroll (0033). They find their gateway through DHCP option 224 (0033's constraint).

## Decision

Two sources of evidence, used together:

- **DHCP leases** (through the DHCP server's API, Technitium here). Lease MACs and hostnames are matched against OpenWiFi hardware vendors. This answers *who and where*, for every scope, and changes nothing on the network. A match is a **possible** OpenWiFi AP.
- **Listening.** In DHCP scopes chosen by an admin, and only where no OpenWiFi gateway serves APs, option 224 points at a listener on the manager. An unconfigured OpenWiFi AP in such a scope tries to connect to it, and the manager records the attempt. Only an unconfigured AP looking for a gateway does this, so a knock that matches a lease makes it a **confirmed** unconfigured OpenWiFi AP.
- The two are not redundant: leases find every device, including OpenWiFi APs already bound to another gateway, while a knock proves the device is unconfigured.
- Detected APs appear beside Landing Zone as "detected, not enrolled". Adopting one means turning off its uCentral client and installing the Aeolus agent, after which it enrolls like any other AP (0033).
- **LLDP** from switch neighbor tables is an optional later source. It needs a path into every switch.
