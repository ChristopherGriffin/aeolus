# 0076. An AP's name is its hostname

- Status: Accepted
- Date: 2026-10-05
- Proposed by: Griff: change an AP's hostname from the APs tab. Written up by Claude, and accepted with the merge
- Refines: 0032, 0040, 0075

## Context

- **An AP gets its name in Aeolus when it enrolls:** the hostname it reports (0032). OpenWrtnight and OfficeOpenWrt were named so, and their hostnames still match.
- **Nothing renames a node:** a folder or an AP keeps the name it was made with.
- **Aeolus never sets an AP's hostname,** so the two can drift apart. It is what the AP gives its DHCP server, and what shows in LLDP and logs.
- **The lab check, 2026-10-05,** on the pumphouse: setting `system.hostname` and running `reload_config`, as the agent does when it applies, renamed the AP at once. The name was put back, and no client dropped.

## Decision

- **An AP's name in Aeolus is its hostname,** one thing.
  - The composed config carries it, as `ap.hostname`, a part only the manager fills in.
  - The renderer writes it to the system section's `hostname`, and the render check holds the AP to it.
  - Where a name isn't a valid hostname, from before this, it's made into one: letters, digits and hyphens, at most 63, other characters turned into hyphens.
- **A new change, `rename`,** renames a folder or an AP in either tree. An operator of the node may make it.
  - An AP's new name must be a hostname: letters, digits and hyphens, 1 to 63 of them, starting and ending with a letter or digit. No other AP may have it, whatever its case.
  - A folder's name may be any 1 to 64 characters, no control characters.
  - Renaming an AP re-versions its config. The agent applies it: the system section changes, `reload_config` sets the hostname, and nothing else restarts.
- **The APs tab** offers **Rename…** beside each AP for someone who may change it, previewed and logged as other changes are.
- **A folder's page** offers **Rename…** beside its name, in both trees (Griff, 2026-10-05). Renaming a folder changes no AP's config.
- **An AP's own page** offers Rename… and Move… beside its name too (Griff, 2026-10-05).
- **A folder's page offers New folder…** (Griff, 2026-10-05): a folder inside it, by name, in either tree; not inside an isolated folder.
  - Its ID, which goes into addresses, is made from the name: lowercase letters, digits and hyphens, such as `north-wing`, with a number added where the tree has the ID already.
  - A new folder's ID and name are checked when it is made (`change.Guard`, the commit guard, not on replaying the log): an ID of 1 to 32 such characters, starting with a letter or digit, and a name as a folder's rename takes.
- **A folder's page offers Move… too** (Griff, 2026-10-05): into any other folder of its tree, nesting it there with everything inside it. The Org and isolated folders, such as Landing Zone, stay where they are, and a folder can't go inside itself.
  - What's set on the folder and inside it stays. What it inherited from its old place now comes from its new one, as inheritance always works.
  - A value the new place locks, set on the folder or inside it, gives way to the lock, and the preview lists each, with where it was set.
- **The APs tab offers Move…** beside each AP too (Griff, 2026-10-05): to any Locations folder outside Landing Zone, by its path from the Org, with the existing `move` change.
  - The preview says which of the AP's own settings the new folder's locks would drop.
  - It warns that the AP takes its new folder's settings, which restarts its Wi-Fi where its networks or radios differ.

## Consequences

- An AP's hostname follows its name in Aeolus, so a renamed AP shows up under its new name in DHCP leases, LLDP and logs.
- The rollout changes no hostname: the lab APs' names are their hostnames already.
- Folders and APs are renamed, and APs moved, from the UI, with no API calls.
