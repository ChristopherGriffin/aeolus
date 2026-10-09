# 0085. The library holds AP templates, and only those

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff: hardware profiles, so every C-360 is set as one thing, every C-260 as another, every AX6000 or whatever as another. A site with two or three kinds of OpenWrt AP has each set as it comes online. This should be the library's only use; tunnels stay in Interfaces. A new kind of AP makes a template in the library, and templates are applied at any folder level: one location's without 6 GHz, another's with it. In a folder, the library shows the templates offered there; at another level, others. Written up by Claude
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

### The library is AP templates

- **A template is a named set of Locations settings for one or more boards.** It names its boards by OpenWrt's board name and holds settings as a folder does, field by field. It can't hold names or tunnels: tunnels stay in Interfaces › Tunnels (0055), and networks stay in Services.
- **Templates hold choices, not capabilities.** What an AP can do, it reports: its bands and widths (0039), AP/VLAN interfaces (0082), and a radio another service owns (0081). The manager goes by those reports. A template that copied them would go stale when firmware changes.
- **A template is made at a level, the Org or a Locations folder, and offered there and below,** as a setting reaches the folders below it (0012). Opened in a folder, the library lists the templates offered there: the folder's own, then those of the folders above, nearest first, then the Org's. A site can keep templates of its own that other sites never see. A name is unique among the templates a folder is offered.
- **Who may make or edit one is whoever may change its level** (0030): admin at the Locations root for the Org's, as for any setting there.
- **A new kind of AP makes its template.** When the first AP of a board that no template fits, built in or not, is adopted, the manager adds one at the Org: named for the AP's model (`Arista C-360`), for that board, and the Org's default for it. It holds no settings yet, so it changes nothing until someone fills it in. It shows what the AP reported (its radios, bands and ports) to fill it in by. Any AP can also be made into a template by hand, from Landing Zone too, to get one ready before adopting.
- **Each board has one default template, at the Org.** Every adopted AP of that board takes it.
- **A board may have several templates, and any folder may pick one of those it is offered,** by reference, as 0015 has it. Its APs of that board below take that template instead of the default, unless a folder below picks again. For example, one location gets `C-360, 6 GHz` and another `C-360, no 6 GHz`. The second runs dual 5 GHz, so its third radio serves 5 GHz instead of standing idle. That is the choice a folder's own `radio.6g.enabled` can't make, since it only turns 6 GHz off.
- **Where a value comes from, weakest first:** the baseline, the AP's template, its folders, the AP itself. A folder's setting beats a template because the site decides. A lock above stops what is set below it, as before (0012). The UI shows a template's value as from "template <name>".
- **Editing a template is one logged change** that re-versions every AP that takes it, as 0015 has it.

### Built-in templates

- **Aeolus ships templates for the hardware it knows,** beginning with the Arista C-360. Its choices:
  - eth0 as the uplink;
  - the 6 GHz radio plan, with dual 5 GHz as the other.
  That radio2 is the scan radio is not the template's to say: the AP reports it (0081).
- **A built-in template can't be edited.** It is the default until the Org copies it and makes the copy the default. A release can then improve its built-in templates without overwriting anyone's choices.

### Coming online

- **A template applies the moment its AP is adopted.** Adoption stays a person's act, or the AP's MAC was registered beforehand (Open). A board alone never adopts an AP: anyone could plug in a C-360 and receive the networks' passphrases.
- **Templates are made at adoption, not at enrollment.** Enrolling needs no token (0033), so whatever an AP in Landing Zone says of itself is anyone's to say. A library filled from enrollment would take as many templates as made-up boards were sent. Adoption is a person's say that the AP is real.

### Concentrators leave the library

- **The library's concentrators are retired.** The change log keeps replaying their old entries, so a log from before 0055 still opens. The API refuses new ones. The Library page, and the MCP tool `get_library`, show templates.

## Consequences

- **A site with a few kinds of AP sets each kind once.** A new AP of a known kind comes up right once adopted, with nothing set on it alone.
- **One more place a value can come from.** It is shown, as every origin is, and it is the weakest after the baseline. A folder or AP setting always wins over it.
- **A board-specific setting,** such as the C-360's radio plan, needs a setting Aeolus renders only on boards that have it. Today the radio plan lives in the C-360 image's own `arista-c360-radio` package, outside Aeolus.

## Open

- **Registering APs before they arrive:** MACs or serials that adopt themselves into a folder. This needs its own decision, since it changes what adoption means (0033).
- **The board-specific settings themselves,** the C-360's radio plan first, and how the agent renders a setting only on the boards it fits.
- **Whether a template may also set a network's bands on its boards.** For example, a 6 GHz network offered only on the boards that serve 6 GHz. That is a Services setting, which templates do not hold today.
