# 0123. Service assurance: client tests on a schedule

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff, asked what should follow the client test (0121): tests on a schedule with an alert, and "a service assurance module tab in Aeolus", to be put right together after. Written up by Claude
- Refines: 0099, 0104, 0121

## Context

- **A client test says whether a network works from a client's side** (0121): an AP with a scan radio joins it as a client would and times each step. A person asks for one, from the AP's page.
- **Nobody asks until someone complains.** A network that gives no address, on one AP, on one band, fails everyone there until then.
- **Alerts are what the manager works out from what it has** (0099): it stores none, and each ends when its cause does.
- **With an airscan that lends its radio a test takes half a minute** and costs airscan no scan or sweep (0121), so tests can be many.

## Decision

### The schedule

- **A folder says how often its APs test: `assurance.tests.every`,** in minutes: 15, 30, 60, 180, 360, 720 or 1440. Unset, or 0, none is asked on a schedule. 0 says so below a folder that sets one.
- **`assurance.tests.speed`** says whether those tests measure speed too, against the speed server (`system.speed_server`, 0121). Off unless set: a speed measurement fills the air for ten seconds.
- **They are the manager's settings,** as where alerts go is (0101): never sent to an AP, and no new version of its config.
- **The manager asks,** each minute looking at what is due, in its own name (`aeolus`), through the action a person's test is (0104, 0121). Only an AP with a scan radio is asked.
- **An AP is asked for one test at a time, and none while:**
  - a test of it is open, by hand or on the schedule;
  - it has not called in for three polls;
  - it does not run its current config, as a test of an AP that is changing says little.

### What is tested

- **One thing each time: a network, on an AP, on a band.**
  - **The networks** are the AP's own that a test can join (0121).
  - **The APs** are itself, and each AP it keeps as a radio neighbour (0073) and hears at -75 dBm or better on the band. Weaker, a test fails for the distance and says nothing of the service.
  - **The bands** are those the network is on, on that AP.
- **The thing tested longest ago is next,** so each comes round in turn, and a new one is first.
- **A thing that fails is tried again in two minutes,** not in its turn: one failure may be one bad try.
- **A thing that failed twice running is failing.** It is then tested every other time, turn about with the rest, so its mending is seen within two rounds of the schedule.
- **A test that was never made says nothing:** one the AP did not take up, or that never had the scan radio. Only a test that looked for the network counts, for failing as for passing.

### The alert

- **A network that is failing on an AP is a warning on that AP** (0099): the AP it was joined on, not the one that tried. It says the network, the band, which AP's scan radio tried it, where the latest test stopped and why, and since when.
- **Tests asked by hand count as those on the schedule do.**
- **It ends when a test of the same thing passes.** It also ends three rounds of the schedule and an hour after the last test of it, or an hour after it where the AP is asked for none: a verdict nobody is checking is not kept up.
- **It goes where the folder's alerts go** (0101), as any other.

### What is shown

- **A Locations folder has an Assurance tab:**
  - **the schedule,** with each AP that has a scan radio, how often it is asked, how many things it tests in turn and so how long a round is; and its editor: how often, with speed or not, and the speed server;
  - **what is tested:** each thing an AP is asked for in turn, with how many of its tests of the day passed, when the last was, and a chip for the last or for "failing"; the failing first. A line opens to its latest test, drawn as on the AP's page (0121);
  - **the tests,** the latest fifty, by hand or on the schedule.
- **`GET /v1/assurance`** gives the same, and Claude and other clients read it through `get_assurance`.
- **An AP's page keeps its own client tests** (0121), where one is asked by hand.

### Kept

- **A client test is kept thirty days,** by hand or on the schedule: every fifteen minutes they are a hundred a day for each AP, each with every step it took.

## Consequences

- **A network that stops working on one AP and band is known within a round of the schedule,** and said twice before it is an alert.
- **The round grows with what there is to test.** An AP with three networks, on itself and one neighbour, on two bands, has twelve things: at fifteen minutes, three hours a round.
- **The AP has no scan radio for half a minute each time,** two or three where airscan must be stopped (0121): radio resource management hears nothing new for that long.
- **A test on a DFS channel costs the scan radio a driver restart** each time (0121).
- **Each test is a client on the network:** a DHCP lease, a line in the client history of the AP it joined (0118), under the scan radio's own MAC and the name `aeolus-test`.
- **Only what an AP with a scan radio can reach is tested.** A site with none has an empty tab, which says so.
- **Networks that sign clients in by 802.1X are not tested** (0121).

## Tried

- **In the manager's tests:** the first asked of an AP on itself, the next on the AP it hears well, never on one heard at -82 dBm; a failure tried again after two minutes and not after one; an alert after the second, on the AP it failed on, gone when it passed; the failing thing tested every other time; speed only when asked for; nothing asked of an AP not heard from.

## Not done here

- **Networks that sign clients in by 802.1X,** which need a user for the test (0121).
- **A test's speed held against what the network should give,** and an alert when it falls.
- **How a thing did over a week:** only the counts of the time asked for are shown.
- **An AP's own Assurance tab.**
- **Tests of the uplink alone,** from an AP with no scan radio: its gateway, DNS and the speed server over the wire.
