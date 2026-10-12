# 0141. How long the network takes to answer, a VLAN at a time

- Status: Proposed
- Date: 2026-10-12
- Proposed by: Griff, who asked for a dashboard, "probably called Network", of how long each VLAN's DNS, DHCP server, AAA and gateway take to answer, with a graph of it over time, a VLAN at a time or all together. Written up by Claude
- Refines: 0140, 0130, 0121, 0128, 0114

## Context

- **The APs already time the network's own services.** A client test (0121, 0128), through the air or over the wire, times how long an address took, the gateway's answer, a name looked up, and a sign-in where the network has one; on a schedule that is every 15 minutes, on every network an AP can test. Every state report says how long each network's RADIUS server took to answer the AP (0114).
- **Each time was shown where it was made, and nowhere together.** A test's way online (0130) says its own times; Assurance says what a thing tested typically takes. Nothing said what a VLAN's DHCP server takes, over all the APs and networks that use it; nor how that moved over a week.
- **The tests themselves are kept 30 days,** each with every step it took. What they timed was counted again from them at every look.
- **Griff, 2026-10-12:** "eventually we need a dashboard (probably called Network) showing the time typically it takes to get from each vlan showing the average response time of the dns, dhcp server, AAA, Gateway etc. and a graph plotting it over time (pulldown vlans to see each and a global for all)". 0140 had left the trends of what is kept as events for later; this is the first of them.

## Decision

- **The manager keeps what was timed of each network's steps, by the hour:** for an AP, a network and a step, how many times it was timed, the sum of the times and the slowest, and how many times no answer came. It is a trend (0140), kept as long as the others.
  - **From a client test,** as its result comes: `dhcp`, from asking for an address to having one; `gateway`; `dns`; and `signin`, a whole 802.1X sign-in. A test that stopped at one of them counts there as no answer.
  - **From a state report:** `aaa`, the round trip of each network's RADIUS server, where it answered since the report before; and no answer where the AP finds it silent. One not yet asked says nothing.
  - **The tests still kept are counted in once,** where nothing of a test was ever counted, so the history begins with them. Of the RADIUS servers it begins with this manager.
- **`GET /v1/network` says it of the APs the caller may view,** below a Locations node where one is named, over up to a year:
  - **a segment at a time:** the networks are put together by the VLAN, or the tunnel, their primary transport has. A VLAN's DHCP server, gateway and names are its own, whichever network a test joined it by. A network on neither is said by itself;
  - for each step, how many times it was timed, its average and its slowest, and how many times no answer came; and the same of all together;
  - each step's average over the time, in at most 48 buckets of whole hours, of all or of one segment.
  - The MCP adapter has it as `get_network`.
- **A folder has a Network tab.** A tile a step: what it typically takes, its slowest, how often no answer came, and its line over the time. The time is chosen there, a day, a week, 30 days or a year, and the VLAN from a list, all of them unless one is picked. Under them, a line a VLAN with what each step takes on it.
- **Typical is the average,** Griff's word. An hour keeps a sum and a count, so the average of any stretch of hours is exact.

## Consequences

- **A slow DHCP server, or a gateway that has grown slow, shows as a line that rose,** on the VLAN it serves, without opening a test.
- **A VLAN is said only where it is tested.** With no tests on a schedule, only the RADIUS servers are timed.
- **A network's segment is read from Services as it is set now.** Moved to another VLAN, what was timed of it before is said under the new one.
- **The address a test waits for is the whole of getting one,** and not the DHCP server's first answer alone: over the wire a test also times the offer (0130), which is not kept here.
- **AAA is the RADIUS server's answer to the AP,** a few ms; a whole sign-in, many round trips and the client's own time, is kept beside it and shown where a network has one.
- **Averages hide a slow minute in a good hour.** The slowest of each hour is kept and said beside it.
- **The store's schema moves to 20.**

## Not decided

- **An AP's own way out,** a tab on the AP: its management address, its gateway, its names, its time and the internet, each with how long it took (Griff, the same day; the internet by a target set a folder at a time). The agent does not time these yet.
- **The same of real clients' own attempts** (0118): their addresses and gateways are timed too, and are not counted here.
- **What is too slow.** Nothing here is amber or red but an answer that did not come.
- **An alert** where a VLAN's service slows, or stops answering.
