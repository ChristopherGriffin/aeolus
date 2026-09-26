# 0038. Enrollment and the config poll in detail

- Status: Accepted
- Date: 2026-09-26
- Proposed by: Claude, while building M4 part 3; accepted by Griff with the part 3a merge
- Refines: 0007, 0029, 0033, 0036

## Decision

- **An AP's ID comes from its MAC.**
  - The agent sends the MAC it is known by, and the manager names the AP `ap-` plus the twelve hex digits, for example `ap-bcff4d85717a`.
  - Its display name is its hostname.
  - The same hardware always gets the same ID, and DHCP observation (0035) can match what it sees against enrolled APs.
- **First enrollment wins.**
  - An ID the manager already knows, in Landing Zone or adopted, cannot enroll again. The request is refused (`409`), and the first AP's token keeps working.
  - So nobody can take over an AP by enrolling with its MAC, or swap the device behind a Landing Zone entry just before a person adopts it.
  - An AP that lost its token (a factory reset) can enroll again only after a person removes it. It then lands in Landing Zone and is adopted again.
- **What enrollment records.**
  - Enrollment is one change, `enroll`, made by the manager (0036). It holds the AP in Landing Zone, its token's ID and hash, and the facts it sent (MACs, hostname, model, board, OpenWrt version, radios) with the address it came from.
  - The reason reads "enrolled from" and that address.
  - The facts appear on the AP's page, for the person deciding whether to adopt it.
  - Accounts cannot make this change. The manager's second power in 0036 is this change; it no longer uses `add-ap`.
- **Removing an AP.**
  - `remove-ap` takes an AP out of the Locations tree, with what was set on it, revokes its token and forgets its facts.
  - It needs operator on the AP. The log keeps everything it removed.
- **Landing Zone holds at most 250 APs.** Enrollment needs no token, so this bounds what anonymous callers can add to the log. When it is full, enrollment answers `503` until a person adopts or removes some.
- **AP tokens are their own kind.** They start `aeolusap1.` and work only on the AP routes (`/v1/ap/...`). Account tokens do not work there.
- **The poll.** The AP calls `GET /v1/ap/config` with the version it runs in `If-None-Match`, and gets one of:
  - `304`, when that is still its version;
  - `unassigned`, while it is in Landing Zone, with no config;
  - `held`, with the problems, when its config breaks rules (0029). There is no config, and the AP keeps running what it has.
  - `ready`, with the config, secrets opened (0027), and the version as the `ETag`.

  Only a ready config carries a version, so an AP only ever holds the version of a config it was sent.
- **A version covers everything the config is built from (0007).**
  - That means the AP's resolved fields, and what it takes from the library: where its concentrators are, whether it may use them where it is, and whether they have its VNIs.
  - A new address for a concentrator re-versions the APs that use it, and no others.

## Consequences

- Anyone on the network can still fill Landing Zone and block real enrollments until a person clears it. The damage stops at 250 log entries. Per-source limits can come later if that happens.
- Re-adopting a reset AP loses the values set on the AP itself; the log still shows them. Re-keying a replaced or reset AP in place can come later.

## Open

- Which MAC the agent sends: settled with the agent in M5.
- Renaming APs and folders: there is no change kind for it yet.
