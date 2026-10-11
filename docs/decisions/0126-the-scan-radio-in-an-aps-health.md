# 0126. The scan radio in an AP's health

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Claude, after a night in which the lab C-360's scan radio went deaf nine times and nothing in Aeolus said so
- Refines: 0081, 0116, 0119, 0121

## Context

- **An AP's scan radio is another service's,** airscan's (0081). Aeolus leaves it alone.
- **Aeolus leans on it all the same:** radio resource management listens through it (0116), and so rates the channels with it; client tests borrow it (0121), and with them the tests on a schedule (0123).
- **It can go deaf.** The C-360's does after being a client on a DFS channel (0121): every look hears nothing, its own AP's networks among them, until its driver is bound again.
- **airscan 0.2 brings it back itself,** and says how often in its status. An older one does not.
- **Aeolus said nothing of any of it.** A deaf scan radio showed only as channel ratings growing old.

## Decision

- **An AP with a scan radio says how it is doing in each report:** `scan_radio`.
  - **what airscan is at:** between jobs, looking at clients or for networks, sweeping, lent, binding its driver again; `waiting` where it has no radio to work, `missing` where airscan does not answer;
  - **how many networks its latest look heard,** and how long ago that was;
  - **how often airscan bound its driver again,** how many of those did not bring it back, and how long ago the last was, where airscan says.
- **It is shown with the AP's health** (0119): a tile, Scan radio, with the networks heard, what airscan is at, and how often it was brought back.
- **It is an alert, a warning, only where nothing is putting it right:**
  - airscan does not answer;
  - airscan has no radio to work;
  - the latest look, in the last quarter of an hour, heard no network, and either binding the driver again did not help, or this airscan brings none back.
- **A deaf radio that airscan is bringing back is no alert.** It is seen to in half a minute, and the tile counts it.

## Consequences

- **A scan radio that needs a person is said to,** and why: radio resource management and client tests are blind until it is seen to.
- **How often a radio goes deaf can be read,** which says whether the cure is enough or the cause should be found.
- **An AP on an older agent says nothing of its scan radio,** and has no tile.

## Not done here

- **Each time the radio was brought back, in the AP's history.**
- **Binding the driver again from the manager,** as an action.
