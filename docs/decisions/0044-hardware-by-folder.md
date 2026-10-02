# 0044. Radio hardware is set by folder, within what every AP can do

- Status: Accepted
- Date: 2026-10-02
- Proposed by: Griff, while starting to edit PumphouseAP's channel width; written up by Claude and accepted with the merge
- Refines: 0008, 0012, 0029, 0042

## Decision

- **Hardware settings belong on folders,** so every AP in a folder runs the same radio settings. Channel width is the first; channel and power follow the same rule.
- **A folder offers only what every AP it reaches can do.** With four APs where only one can do 160 MHz, the folder offers 80 MHz at most.
  - "Reaches" means every AP below the folder, except those below a break or in Landing Zone, where the folder's setting does not apply (0005, 0032).
  - What an AP can do comes from what it reported when it enrolled (0033).
  - An AP that never reported its radios is listed and not counted.
- **On 5 GHz, the channel limits the width too.** A width is offered only where the channel each AP is on can carry it. That is the channel Aeolus sets, or else the one the AP last reported. For example, 160 MHz needs a channel from 36–64 or 100–128.
- **One AP can differ, as its own custom setting.** It is set on that AP, overriding the folder (0012), and limited only by that AP's radios. The UI marks it as custom.
- **The manager works out what to offer** and shows it on each Locations node (`hardware`), so the UI, Claude and any other client offer the same choices. Behind that, two checks catch anything that slips through:
  - an AP's config with a width its radio cannot do is held (0029);
  - a rendered radio whose channel cannot carry its width is refused by the render check (0039), whoever set the channel.

## Consequences

- A folder's hardware settings never take an AP beyond what it can do. A new, less capable AP joining a folder is held until its config fits, and its page says why.
- A width that would leave a radio off the air while the AP still reaches the manager, which no automatic revert would catch, never gets applied.
- The channel groups are the ones that hold in the US and EU. The newer 165–177 groups are left out until an AP needs them.
