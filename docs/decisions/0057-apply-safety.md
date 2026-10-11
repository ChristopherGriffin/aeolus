# 0057. An apply keeps the Wi-Fi up, and what an AP lacks is held

- Status: Accepted
- Date: 2026-10-03
- Proposed by: Claude, after OpenWrtnight's Wi-Fi went down for five minutes; Griff asked for the fixes, and accepted them with the merge
- Refines: 0008, 0039, 0040, 0050, 0054, 0056

## Context

On 2026-10-03, adding a network on OpenWrtnight made hostapd restart both radios from scratch. Its config held `bss_transition` (802.11v), which band steering had put on another network that morning (0050). This AP's hostapd, `wpad-basic-mbedtls`, has no 802.11v, so it refused the whole config, and every network on the AP, Sweet Spot and Sweet_Spot_IoT included, was down from 18:34 to 18:39. The apply was kept, because the agent only checked that it could still reach the manager (0008), and that worked over the wire.

The same apply rendered a VXLAN tunnel that never came up: `vxlan` was installed, but netifd loads its protocols only when it starts, and it had not been restarted.

## Decision

- **An apply that changes the Wi-Fi is kept only if the Wi-Fi comes back.**
  - After the AP reaches the manager again, it waits up to 150 seconds, a radar check (DFS) included, for every network that was up before to be up again: each interface netifd lists for a radio that is on.
  - If one is not, the AP puts its old config back and reports the apply as failed. The report names the networks and hostapd's last errors, such as "Line 148: unknown configuration item 'bss_transition'".
  - A network already down before the apply does not count against it.
  - Where netifd cannot list them, before the apply or after it, the AP counts instead: the AP networks its config has on radios that are on, less the AP interfaces that are up. More missing than before the apply fails it. Both counts are made the same way, so an interface that is no AP network and was already down cannot stand in for an AP network that failed.
- **Band steering and BSS transition need 802.11v:**
  - **The agent asks the running hostapd.** A BSS's ubus object offers `bss_transition_request` only when hostapd has 802.11v. It reports that with its steering state, and uses it when rendering. With no BSS running, it is not known.
  - **Where hostapd lacks it,** the agent leaves `bss_transition` out and says why, and the render check refuses the config.
  - **The manager holds it sooner.** Band steering or BSS transition on an AP that reported no 802.11v is held at once, in the preview and in the poll, with the fix: a full wpad, such as `wpad-mbedtls`.
- **VXLAN needs netifd to have loaded it:**
  - **The agent asks netifd** which protocols it knows, reports whether `vxlan` is loaded, and renders tunnels only if it is.
  - **The manager holds a tunnel** on an AP that reported `vxlan` not loaded, with the fix: restart the AP's network.
  - **The installer restarts the network** when it installed `vxlan`. It does this at the end, in the background, ignoring the hangup a dropped SSH session sends, so the restart always finishes.

## Consequences

- A change that breaks the Wi-Fi now undoes itself within about four minutes: 90 seconds to reach the manager, then up to 150 for the Wi-Fi.
- An apply that changes the Wi-Fi can take up to that long before it is reported.
- Band steering on an AP with `wpad-basic` is refused until a full wpad is installed. Installing one replaces `wpad-basic`, and restarts the Wi-Fi.
- What an AP has not reported yet is still caught, by the render check and by the apply.
