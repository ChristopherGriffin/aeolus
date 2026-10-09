# 0082. Per-user keys' VLANs need AP/VLAN interfaces

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff: a radio that cannot carry a network's per-user VLANs should be caught before the config reaches the AP, not by the apply's revert; and a setting meant for some models must not take the others down. Written up by Claude
- Refines: 0070, 0057

## Context

- **A key's VLAN is an AP/VLAN interface.** For each VLAN a network offers its per-user keys (0070), the renderer adds a `wifi-vlan`. hostapd then makes an AP/VLAN interface, `<bss>-k<vlan>`, for the clients of the keys that land there.
- **Not every driver makes them.** The Arista C-360's four radios (ath11k: the IPQ8074's two, a QCN9074 and a QCN9072) list only managed, AP, monitor and mesh point among their interface modes. The other APs here list AP/VLAN: OfficeOpenWrt (mt76, MT798x) and Pumphouse-AP (ath10k, Netgear R7800).
- **Without them, hostapd fails the whole radio.** On a C-360 in a folder with a keyed network (2026-10-09, version 82), hostapd logged `Failed to create interface phy1-ap2-k50: -95 (Not supported)`, `VLAN initialization failed` and `Interface initialization failed`, for every BSS on each radio the network was on. The keyed network's neighbours went down with it. 150 seconds later the agent reverted the apply (0057): "the Wi-Fi did not come back within 150 seconds: ... (hostapd: VLAN initialization failed.; Interface initialization failed); reverted".
- **The renderer already refuses what hostapd would fail on.** It refuses BSS transition on an AP whose hostapd lacks 802.11v (0057): there, the line makes hostapd refuse its whole config.

## Decision

- **The agent reports, for each radio, whether its driver makes AP/VLAN interfaces:** `ap_vlan`, read from `iw phy <phy> info` ("Supported interface modes"). It is `null` when iw is missing or lists no modes. It goes in the facts the AP enrolls with, and in the facts the renderer is given. The enrollment facts also say which radios another service owns (`reserved`, 0081).
- **The renderer refuses a network whose keys offer VLANs on a radio without AP/VLAN interfaces:** "`network.<id>.keys.vlans: <radios> cannot put clients in VLANs of their own (the driver has no AP/VLAN interfaces); offer the network on other bands, or give its keys no VLANs`". As with every render error, the agent applies nothing and reports it, so the AP stays on the config it runs. A radio another service owns (0081), or one the network's bands leave out, does not count.
- **The manager refuses it too, when the AP's enrollment facts say so.** The same problem is among the config's problems before the AP polls (`keyVLANProblems`, beside the radio-width check of 0008). So a change that would put keyed VLANs on such a radio is shown with it, in its preview, as an AP whose config then fails its check. An AP that enrolled before it reported `ap_vlan` is not judged by the manager; its agent still refuses.
- **The render contract's cases can say which errors they expect.** A case with `"errors"` must make the renderer report exactly those (`TestAgentRendersItsOutput`, where ucode is installed). `agent/test/cases/keys-no-ap-vlan` is `keys` on an AP whose radio0 has no AP/VLAN interfaces: it renders the same, and is refused.

## Consequences

- **On a C-360, a network with keyed VLANs is refused until it is limited to bands it can carry.** None of its radios can, so in practice the network is not offered there, by its folders, or with its keys given no VLANs. Every other network on the AP keeps running.
- **A model's limits take effect without a setting per model.** The AP says what its radios can do, and the same folder settings fit every AP that can carry them. The ports an AP does not have are already left out the same way (0053).

## Open

- **Facts are what the AP said when it enrolled.** An AP enrolled before this decision reports no `ap_vlan`, and nothing refreshes facts after a driver or image change. The agent could send them again with its state report.
- **Keys without VLANs on such a radio.** The keys could still authenticate there, their clients landing in the network's own VLAN. That would quietly join clients meant to be apart, so it is not done.
