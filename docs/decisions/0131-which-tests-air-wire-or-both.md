# 0131. Which tests: through the air, over the wire, or both

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff. There should be test types: wired, wireless, both. Written up by Claude
- Refines: 0123, 0128, 0129

## Context

- **There are two kinds of client test.** Through the air (0121), an AP with a scan radio joins a network as a client would. Over the wire (0128), any AP asks a network for an address, its gateway and a name from its own uplink, and uses no radio.
- **A schedule (0123) could ask for the first alone, or for both:** `assurance.tests.wire` added the second to the first. It could not ask for the second alone.
- **A round asked by hand (0129) was always both.** Someone who has just moved a VLAN wants the wire, now, and not to borrow every scan radio for half a minute a thing to get it.
- **A scan radio is not free:** while a test has it, airscan has not. A site may want its networks watched over the wire all day, and its scan radios left to airscan.

## Decision

- **`assurance.tests.over` says which tests a folder's APs are asked for on a schedule:** `air`, `wire` or `both`. Unset, through the air, as before.
  - **`air`:** an AP with a scan radio tests through the air. One with none tests nothing.
  - **`wire`:** every AP whose agent can tests over the wire. A scan radio is not used.
  - **`both`:** each AP all it can, its tests through the air first.
- **`assurance.tests.wire`, the setting before, stays in the schema,** as the log has changes that set it. On, it says both where `over` is not set; `over` set says which whatever it is. The page sets `over`, and shows `wire` where a folder has it still, to be taken away.
- **A round says which tests for itself:** `over` in `POST /v1/assurance/round`, `air`, `wire` or `both`, which it is unless said. Whatever the folders ask for on a schedule, as a round is someone asking now.
  - It is refused where no AP there can make a test of the kind asked.
  - What a round is of is said with how it is going, `round.over`, and its count is of those tests.
- **The Assurance tab has the choice beside "Test everything now":** Both, Wireless, Wired. A kind no AP there can make cannot be chosen. The browser keeps what was chosen last.
- **An AP's own page keeps its one button,** a round of both: its form beside it asks for one test of either kind.
- **Each AP that tests says which tests its folders ask of it,** `over`, in `GET /v1/assurance`, and the tab says it in words: one with a scan radio asked for the wire alone says so.
- **Claude and other clients ask a round of one kind through `ask_assurance_round`,** with `over`.

## Consequences

- **A site can be watched over the wire alone,** every network on every AP, with no scan radio borrowed and none needed.
- **A thing tested the other way before is listed still,** as what was tested and is on the schedule no longer, until its tests age out of the time shown.
- **Both is two tests of a network, not one test two ways:** each with its own way online (0130), side by side in what is tested.
- **Not in this:** one test that is both, the wire's steps and the air's as one path from the client to the gateway.
