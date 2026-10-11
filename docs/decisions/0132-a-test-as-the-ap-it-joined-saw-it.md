# 0132. A test, as the AP it joined saw it

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Claude, from the morning it was needed: tests in a round failed at their gateway, and what told whose fault it was lay in another record, a click away and unread
- Refines: 0118, 0121, 0130

## Context

- **A test through the air (0121) says what the client's side found:** it asked for its gateway, and nothing answered.
- **The AP it joined recorded the same attempt from the other side (0118),** as it records every client's: what it saw the client ask for, and what it saw come back. The test's client is a client like any other to that AP.
- **The two together say where the fault lies.** On 2026-10-11 three tests failed at their gateway and at DNS. The APs' records had the client asking for its gateway three times, and no answer coming back through the AP: the frames reached the AP, and the network behind it did not answer. So it was not the air, and not the scan radio. A capture at the AP's uplink then showed the switch delivering nothing to that client.
- **The record was there, and nobody was shown it.** A test opened has a link to the client's journey, which lists every attempt that client ever made; the one that is this test's is among them, to be found by its time.

## Decision

- **A test through the air has, with what it found, the same attempt as the AP it joined recorded it:** `seen`, with that AP, when it began, how it went, where it stopped and why, and its way online a point at a time (0130).
- **Which attempt is the test's:** the latest by the test's client on the AP that owns the BSSID it joined, begun between when the test was asked and when it was done, give or take five seconds, as the two are by different clocks.
- **It is read with the test, and kept nowhere,** as the way is. A test from before this has it, for as long as the AP's record of the attempt is kept (0118). An AP that says what another saw is not believed.
- **It is said only to one who may view the AP the test joined.** A test names that AP to no one else (0121), and says nothing of what it recorded.
- **There is none** for a test over the wire, which joins nothing; where the BSSID is no AP's here; or where the AP recorded no attempt, as of a test that never reached it, or an AP whose agent records none.
- **The page draws it under the test's own way:** "As the office AP recorded it", how it went in a line, and its way as the test's is drawn. The link to the client's journey stays.
- **In the API:** `detail.seen` on a client test, wherever a test is listed.

## Consequences

- **A test that hears nothing back says which side of the AP the silence is on.** The AP saw the asking and no answer: behind the AP. The AP saw nothing of the asking: the air, or the scan radio. The AP saw the answer go out, and the test did not hear it: the air again, the other way.
- **Each test listed costs one more look** in the attempts kept, by its client and AP, which are indexed. A page of fifty tests is fifty looks.
- **Two clocks decide which attempt is whose.** Tests a few seconds apart on the same AP could be given each other's; a round leaves about a minute between them.
- **Not in this:** the two ways drawn as one path, the client's steps and the AP's interleaved; and the same for a test over the wire, whose other side is the switch.
