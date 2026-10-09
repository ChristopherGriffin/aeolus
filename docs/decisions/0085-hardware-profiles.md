# 0085. The library holds hardware profiles, and only those

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff: hardware profiles, so every C-360 is set as one thing, every C-260 as another, every AX6000 or whatever as another. A site with two or three kinds of OpenWrt AP has each set as it comes online. This should be the library's only use; tunnels stay in Interfaces. Written up by Claude
- Supersedes: 0015 and 0023 (what the library holds)
- Refines: 0012, 0033, 0037, 0055

## Context

- **Some settings follow the hardware, not the place:**
  - which radio plan a C-360 runs (6 GHz, or dual 5 GHz);
  - which bands to serve;
  - which port is which;
  - power caps;
  - LEDs.
  Today these go on folders, which hold APs of every kind, or on each AP. Change 82, a C-360's own underlay override, was one such: 0084 took it away, but the kind remains.
- **The library was made for named definitions that folders refer to** (0015): defined once for the Org; one edit re-versions every AP that uses it. It held concentrators (0023), until tunnels became Location settings and the library was shelved (0055). Nothing uses it, and the manager's library is empty (2026-10-09).
- **Every AP reports its board when it enrolls** (0033), as OpenWrt names it: `arista,c360`. Marketing names don't tell boards apart: "AX6000" is a speed class many boards share.
- **Adoption is a person's act.** An AP gets nothing until adopted, because its config carries the networks' secrets (0033).

## Decision

### The library is hardware profiles

- **A profile is a named set of Locations settings for one or more boards.** It names its boards by OpenWrt's board name and holds settings as a folder does, field by field. It can't hold names or tunnels: tunnels stay in Interfaces › Tunnels (0055), and networks stay in Services.
- **Profiles hold choices, not capabilities.** What an AP can do, it reports: its bands and widths (0039), AP/VLAN interfaces (0082), and a radio another service owns (0081). The manager goes by those reports. A profile that copied them would go stale when firmware changes.
- **Each board has one default profile, at the Org.** Every adopted AP of that board takes it.
- **A folder may pick another profile for a board,** by reference, as 0015 has it. Its APs of that board below take that profile instead, unless a folder below picks again. For example, "C-360s in the barn run dual 5 GHz".
- **Where a value comes from, weakest first:** the baseline, the AP's profile, its folders, the AP itself. A folder's setting beats a profile because the site decides. A lock above stops what is set below it, as before (0012). The UI shows a profile's value as from "profile <name>".
- **Editing a profile is one logged change** that re-versions every AP that takes it, as 0015 has it. Who may edit: admin at the Locations root, since it reaches every site.

### Built-in profiles

- **Aeolus ships profiles for the hardware it knows,** beginning with the Arista C-360. That profile knows:
  - eth0 is the uplink;
  - radio2 is airscan's scan radio;
  - the radio plan can be 6 GHz or dual 5 GHz.
- **A built-in profile can't be edited.** It is the default until the Org copies it and makes the copy the default. A release can then improve its built-in profiles without overwriting anyone's choices.

### Coming online

- **A profile applies the moment its AP is adopted.** Adoption stays a person's act, or the AP's MAC was registered beforehand (Open). A board alone never adopts an AP: anyone could plug in a C-360 and receive the networks' passphrases.

### Concentrators leave the library

- **The library's concentrators are retired.** The change log keeps replaying their old entries, so a log from before 0055 still opens. The API refuses new ones. The Library page, and the MCP tool `get_library`, show profiles.

## Consequences

- **A site with a few kinds of AP sets each kind once.** A new AP of a known kind comes up right once adopted, with nothing set on it alone.
- **One more place a value can come from.** It is shown, as every origin is, and it is the weakest after the baseline. A folder or AP setting always wins over it.
- **A board-specific setting,** such as the C-360's radio plan, needs a setting Aeolus renders only on boards that have it. Today the radio plan lives in the C-360 image's own `arista-c360-radio` package, outside Aeolus.

## Open

- **Registering APs before they arrive:** MACs or serials that adopt themselves into a folder. This needs its own decision, since it changes what adoption means (0033).
- **The board-specific settings themselves,** the C-360's radio plan first, and how the agent renders a setting only on the boards it fits.
- **Whether a profile may also set a network's bands on its boards.** For example, a 6 GHz network offered only on the boards that serve 6 GHz. That is a Services setting, which profiles do not hold today.
