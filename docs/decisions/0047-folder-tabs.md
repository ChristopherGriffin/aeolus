# 0047. A Locations page in tabs: Hardware, Networks, System

- Status: Accepted
- Date: 2026-10-02
- Proposed by: Griff: "folders on the left hand side, then tabs at the top … interface first (2.4, 5, 6) … one feeding into the other"; written up by Claude and accepted with the merge
- Refines: 0042, 0013, 0044

## Decision

- **A Locations folder's settings are in three tabs. An AP's page has the same tabs, after an Overview.**
  - **Hardware** has three parts:
    - **Radios:** one card per band, 2.4, 5 and 6 GHz. Each shows width, channel, power and whether the radio is on, with where each comes from. A band no AP here has a radio for is shown as such.
    - **Channels:** a live view of the channel, width and client count each AP last reported for each radio, beside what Aeolus sets. With automatic channels (0045), this is where the AP's choice shows.
    - **Ports:** the Ethernet port settings.
  - **Networks** lists the service folders that apply here, then each network they offer. Each network shows which radios it is on: the bands it asks for, marked where no radio here has that band. That is how the radios feed the networks.
  - **System** holds country, time, NTP, syslog, management and the agent's poll.
  - **Overview,** on an AP only, holds its latest check and apply, what it said when it enrolled, and its history.
- **The tab is part of the address,** for example `#/locations/sandbox/hardware/channels` or `#/aps/<ap>/networks`. A tab can be linked to and survives a reload.
- **Moving to another folder or AP in the tree keeps the tab,** so folders can be compared side by side.
- **Above the tabs stay:** the node's name, its overrides, its problems, and on an AP, its status and the revert button (0046). **Below them,** a folder still lists what it holds.
- **Services folders keep their single page.** Their settings are networks only.

## Next

- Networks will be editable in place on the Networks tab. Each edit changes the service folder the network comes from, and the preview names every AP it reaches (Griff's choice). The Services tree stays, so one network can be shared by many locations (0013).
- New settings will be added to the tab they belong in, starting with multicast-to-unicast on Networks. Band steering (usteer) needs a decision of its own first.
