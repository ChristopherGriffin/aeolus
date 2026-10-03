# 0050. Band steering through usteer, installed with the agent

- Status: Accepted
- Date: 2026-10-02
- Proposed by: Griff: band steering on the Networks tab, with usteer installed "at the outset"; written up by Claude and accepted with the merge
- Refines: 0040, 0039, 0048

## Decision

- **A network can ask for band steering** (`network.*.band_steering`). Clients that can use a higher band are steered onto it.
- **Steering is done by usteer,** OpenWrt's steering daemon, through hostapd.
  - **The agent's installer adds it** (`apk add usteer`), so every AP has it from the start.
  - The agent itself still uses only what OpenWrt's default image has (0040). It never installs packages.
- **usteer steers nothing until Aeolus asks.**
  - usteer's own default steers every SSID, and installing a package starts its service. So the installer turns steering off at once: `band_steering_interval` is set to `0`.
  - From then on, Aeolus owns two of usteer's options, the way it owns the radio options it sets (0040):
    - `band_steering_interval` is `30000` (usteer's own default, in milliseconds) while any network on the AP asks for band steering, and `0` while none does;
    - `ssid_list` names exactly those networks' SSIDs.
  - The rest of usteer's configuration stays the AP's own, including the network it uses to talk to other APs (`lan` by default).
- **A network with band steering also gets 802.11k and 802.11v on its interfaces.** usteer uses them to tell a client where to go before it has to push the client off.
- **The render check holds all of this to the intent** (0039):
  - usteer's interval and SSID list;
  - 11k and 11v on steered networks;
  - when a network asks for steering on an AP without usteer, the check refuses the config and says to install usteer. The AP keeps what it runs.

## Consequences

- **Installing the agent now needs the AP to reach OpenWrt's package feeds,** for usteer. Without them, the installer says so and goes on. Everything but band steering works, and a network that asks for it is refused on that AP until usteer is installed.
- **A sysupgrade drops usteer,** as it does any added package. Its configuration is kept, but a network asking for band steering is refused on that AP until usteer is installed again.
- **APs installed before this need usteer installed by hand,** with its steering turned off the same way, plus the new agent.
- **Only band steering is turned on.** usteer's roaming and load balancing between APs stay at its defaults, which are off. Turning them on is a later decision.
