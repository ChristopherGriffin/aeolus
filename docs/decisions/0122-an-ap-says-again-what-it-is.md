# 0122. An AP says again what it is

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Claude, on the lab's C-360 having no Client tests on its page (0121): the manager did not know it had a scan radio
- Refines: 0033, 0040, 0081

## Context

- **An AP says what it is once, when it enrolls** (0033): its MACs, hostname, model, board, OpenWrt release, and its radios, each with its band, the modes it can do, whether it can make AP/VLAN interfaces (0082) and whether another service owns it (0081).
- **The manager keeps that for good.** It is what a person sees when adopting the AP, and more since:
  - which template an AP takes, by its board (0085);
  - what its radios can do, which the manager checks a width or an 802.11 generation against (0089);
  - whether it has a scan radio, which a client test needs (0121).
- **What an AP is changes.**
  - A scan radio is given to airscan after the AP enrolled. The lab's C-360 was so: on 2026-10-11 the agent on it said `radio2` was a scan radio, and the manager, told once, when it enrolled, had it as an ordinary 6 GHz one. So the AP's page offered no client test.
  - A newer driver or board file gives a radio modes it lacked.
  - OpenWrt is upgraded. A radio card is added.
- **The only way to tell the manager was to remove the AP and enroll it again,** which loses what was set on it.

## Decision

- **The agent looks at what the AP is every five minutes,** and where that differs from what it last said, says it again: `POST /v1/ap/facts`, with what it would enroll with.
  - **It says only what two looks running agree on.** A radio that is restarting has no modes for a moment, and is not a radio that lost them.
  - **After it starts it looks again within a minute,** so a new agent's first word comes soon.
- **The manager records it where it differs from what it has,** as a change of kind `facts`, in its own name, as an enrollment is (0036). The same said again is no change.
  - **It must be the AP's own:** under its token, with the MAC it enrolled with.
  - **Where it enrolled from stays.** The address in its facts is where it first called from, not where it calls from now.
  - **Nothing else of the AP changes:** its place, its name and its token stay.
  - **Its config becomes a new version,** though no setting changed: the manager checks it against the radios the AP now says it has, and the AP renders it for them.
  - **A board no template is for gets its template,** as an adopted AP's does (0085).
- **What an AP is may change once in ten minutes.** Sooner, the manager answers `429`, and the agent says it again at its next look. So a radio that comes and goes fills no log.
- **No person makes this change.** `facts` through `POST /v1/changes` is refused, as `enroll` is.
- **The AP's page says which radio is a scan radio,** with its radios.
- **A scan radio's modes are not its band's.** Now that the manager can learn of one after the fact, every check of what a band's radios can do leaves aside a radio another service owns (0081): the width a folder offers and the width an AP is refused, as the 802.11 generations and the per-user keys' VLANs already did.

## Consequences

- **The manager's picture of an AP follows the AP,** within about ten minutes: its template, the checks of its settings, and what its page offers.
- **An AP takes its config up again each time it says it is something else:** a render, the manager's check, and an apply that changes nothing where the config comes out the same. Where a setting was refused for what a radio could not do, or the board is another, it comes out different.
- **The change log has an entry each time,** by `aeolus`: "the AP said what it is again". A rename by a person (0076) is followed by one, as the hostname is among the facts.
- **An AP on an older agent says nothing again,** and stays as it enrolled. An older manager answers `404`, and the agent does not ask it again until it starts again.
- **The lab's C-360 gets its Client tests** once its agent has said `radio2` is a scan radio.

## Not done here

- **Showing what changed** in the change log's entry, field by field.
- **An alert when an AP loses a radio.**
