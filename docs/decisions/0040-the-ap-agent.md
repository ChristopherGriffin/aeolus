# 0040. The AP agent, v1

- Status: Accepted
- Date: 2026-09-26
- Proposed by: Claude, to start M5 on PumphouseAP; accepted by Griff
- Refines: 0008, 0020, 0022, 0033, 0038, 0039

## Context

PumphouseAP (Netgear R7800, OpenWrt 25.12.5) already has everything the agent needs in its default image:
- ucode, with its HTTPS client (`uclient`, TLS through mbedtls);
- the `uci`, `ubus`, `fs`, `uloop`, `digest`, `log` and `nl80211` modules.

Its network is a VLAN-filtering bridge. `br-lan` has `bridge-vlan` entries, management runs on untagged VLAN 1 (`br-lan.1`, by DHCP), and the uplink `wan` carries VLANs 10 and 20 tagged. It serves Sweet Spot (VLAN 20) and Sweet_Spot_IoT (VLAN 10), which Aeolus did not create.

## Decision

- **One ucode program**, run and restarted by procd (`/etc/init.d/aeolus`). It uses only modules in OpenWrt's default image, so nothing is installed from a feed (0008).
- **Its own settings are UCI**, in the `aeolus` package: the manager's URL and the uplink port, written at install.
  - Settings from intent that belong to the agent are rendered into this package like everything else, so the render check sees them (0039). That starts with `system.poll`, which moves from deferred to checked.
  - The manager's certificate (`/etc/aeolus/manager.crt`) and the AP token (`/etc/aeolus/token`, mode 0600) survive a sysupgrade (`/lib/upgrade/keep.d/aeolus`).
- **It trusts only the pinned certificate** for the manager (0033). No public CA can stand in for it.
- **Identity.** It enrolls with the MAC of the interface it reaches the manager through, which is also the MAC the DHCP server sees (0035). PumphouseAP's is a0:04:60:21:36:5e, on `br-lan.1`. Enrollment also sends:
  - every MAC, the hostname, model, board and OpenWrt version;
  - for each radio, its band and the channels and widths it supports. The manager can use these later to refuse intent a radio cannot carry (0008).
- **The loop:**
  - **No token:** enroll.
    - A `409` means a person must remove the old entry (0038), so it retries hourly.
    - A `503` means Landing Zone is full; it retries later.
  - **Poll** every `poll` seconds (60 until told otherwise, jittered so a fleet does not poll in step), with the version it runs.
    - `unassigned`: nothing to do.
    - `held`: log the problems and keep running what it has.
    - `ready`: render, check, apply, report.
    - A `401` means it was removed: it forgets its token, keeps its config and enrolls again.
  - **Report state** every 5 minutes and right after each apply (0039).
- **Rendering: Aeolus only touches what it owns.** This answers the open question in 0039.
  - The agent starts from the AP's current UCI, in a staging directory.
  - It changes only these, and it never edits or removes a section it did not create:
    - the radio options intent names (channel, width as `htmode`, `txpower`, `disabled`, country);
    - sections named `aeolus_`;
    - the time zone, NTP and syslog settings.
  - On PumphouseAP, Sweet Spot and Sweet_Spot_IoT stay exactly as they are.
  - When a network is no longer called for, its `aeolus_` sections are deleted.
- **VLAN transport on a VLAN-filtering bridge:**
  - A `bridge-vlan` for that VLAN with the uplink tagged. It reuses one that already exists (PumphouseAP has 10 and 20), so it never duplicates a VLAN.
  - An interface `aeolus_<network>` on `br-lan.<vlan>`, which the network's wifi-ifaces join.
  - APs with an older switch driver (swconfig) get an 802.1Q device on the uplink instead, when one needs it.
- **Apply with automatic revert (0008):**
  1. It backs up the packages it changes and moves the checked ones into `/etc/config`.
  2. It reloads what changed.
  3. It must reach the manager again within 90 seconds. If it can't, it restores the backup, reloads, and reports the apply as failed ("lost the manager; reverted") when it is back.
  4. It reports the apply (version, hash, whether it worked).
- **Install, v1:** `agent/install.sh`, run over SSH. It copies the program and init script, writes the `aeolus` package, installs the manager's certificate and starts the service. A package in a feed comes later.
- **The renderer is tested against the manager.**
  - Rendering is a pure function from intent and current UCI to new UCI, kept apart from the code that reads and writes files.
  - CI builds ucode, runs the renderer on test configs, and feeds its output to the Go render check (0039). A mismatch between agent and manager fails the build, not an AP.
- **M5 comes in two parts:**
  1. The full loop with VLAN transports, on PumphouseAP in Sandbox.
  2. On the AP itself: VXLAN, switching between transports and HA (0020, 0022), and detecting VLANs on the uplink (0020).

## Consequences

- A reload of Wi-Fi drops clients on the changed radios for a few seconds. On PumphouseAP that includes the pumphouse heater and controller on Sweet_Spot_IoT, so the first apply there waits for Griff's OK.
- An AP can run Aeolus networks beside hand-made ones. Taking over existing SSIDs is a later, deliberate step.
- Removing an AP from Aeolus leaves its network running as it was. The network does not depend on the manager (0002).

## Open

- SSH keys, rate limits and the management interface: rendered when the agent takes them on, with the ports decision.
- Health checks for VXLAN, and the VLAN detection method (0020): M5 part 2.
