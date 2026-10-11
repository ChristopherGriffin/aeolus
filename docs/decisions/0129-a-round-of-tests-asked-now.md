# 0129. A round of tests, asked now

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Claude, as what a test over the wire (0128) is most wanted for: a site just built, or just changed, checked before its clients find out
- Refines: 0123, 0128

## Context

- **Tests on a schedule (0123) come round slowly on purpose:** one thing every quarter of an hour at the most often, so a site of five networks takes over an hour for each AP.
- **Someone who has just built a site, or moved a VLAN, wants to know now:** does every network reach every AP, with an address, a gateway and DNS behind it?
- **A test by hand (0121) is one thing on one AP.** Twenty APs of five networks is a hundred forms.
- **Over the wire a test takes seconds and every AP can make it** (0128), so testing everything is a matter of minutes.

## Decision

- **A round can be asked for at a Locations folder, or at an AP:** every AP at or below it tests each thing it can, once, as soon as it can.
- **What an AP can test in a round:**
  - **through the air,** where it has a scan radio, what it would on a schedule (0123): each network a test can join, on itself and on the APs it hears well, on each band;
  - **over the wire,** where its agent can (0128), each network it offers, whether or not a folder asks for that on a schedule. Someone asked for this round.
- **It needs operator at that node.** It is refused where no AP there can test anything.
- **The manager asks the tests as it asks those on a schedule,** in its own name, one of an AP at a time, and none while one is open there, the AP has not called in, or it does not run its current config.
  - **What a round has not had asked since comes before the schedule's turn,** and waits for none: the next is asked as soon as the last is done.
  - **What of a round cannot be asked for now does not hold the schedule up:** a thing on an AP that has not said its BSSIDs yet, say. The schedule has its turn as if there were no round.
  - **Each thing is asked once.** One that fails is not tried again by the round; the schedule's own second try (0123) is the schedule's.
- **A round is worked at for an hour.** What is left of it then, as of an AP that never called in, is let go.
- **A later round at the same node takes the place of the earlier.** An AP below two rounds is in the later.
- **Rounds are kept in the manager's memory, not its log:** asking one changes nothing that is configured. A manager that restarts forgets a round under way, and whoever asked asks again.
- **A round's tests count as any others:** in what is tested, in the tests listed, and for what is failing and its alert (0123).
- **The Assurance tab has the button,** "Test everything now", and says how the round is going: how many things, how many tested, and when it is done. A thing counts by the round's own test of it, whatever the schedule asks after. Another round can be asked while one is under way, as after something was put right; where what is left cannot be asked for, the tab says so.
- **Who asked a round, and the name of the folder it was asked at, are said to one who may view that folder.** Someone who may view only what is below sees that a round is on, and how far it has got with what they may view.
- **An AP that can test only when asked is shown with the others:** one with no scan radio whose folders ask for no tests over the wire. It says it tests in a round.
- **`POST /v1/assurance/round`** asks one; `GET /v1/assurance` says how it goes, in `round`. Claude and other clients ask through `ask_assurance_round`.

## Consequences

- **A site is checked in about a minute a thing for each AP,** the APs side by side: five networks over the wire in five or six minutes, however many APs.
- **A round on a site with scan radios borrows them** for half a minute a thing (0121), one after another, for as long as the round lasts there.
- **A network that fails in a round is seen in the tab at once,** and is an alert only once it has failed twice running (0123): on a schedule, when its turn there comes, which for the round's last thing on an AP is two minutes later; with none, only if a second round finds it so.
- **Each test over the wire takes an address and gives it back** (0128): a round asks a DHCP server once for each AP on each network.
- **A manager restarted mid-round leaves it half done.** What was tested stays.

## Not done here

- **A round of some networks only, or of the wire only.**
- **A report of a round to keep or send:** what was tested where, and what passed.
- **Asking again what failed,** from the round's result.
