# 0142. An AP's own way out, a hop at a time

- Status: Proposed
- Date: 2026-10-12
- Proposed by: Griff, who asked for a tab under the AP with "the logical path out and its individual bits in each, Mgmt vlan IP > Router > internet with important stuff, like ntp and dns". Written up by Claude
- Refines: 0059, 0064, 0069, 0093, 0141

## Context

- **An AP works only while its own way out does.** It reaches the manager, its time and its names through one interface, by one gateway. When an AP misbehaves, whether that way is whole is the first thing asked, and the manager could not say.
- **Parts of it were known, in different places:** the uplink's speed and the switch port it is on (0093, 0064), the management VLAN, whether its clock is in step (0069). Whether its gateway answers, and how long its name servers take, were not: nothing on the AP asked.
- **The prober already asks such things of others:** the far ends of tunnels by ping (0059), RADIUS servers by a status request (0114), each every so often with how long the answer took.
- **Not every site has the internet,** and Aeolus runs without it (0069). Griff, asked what an AP should take for the internet: a target set a folder at a time.

## Decision

- **The prober asks along the AP's own way out, every 30 seconds:**
  - **the gateway** of the interface that holds its default route, by ping. Of several default routes, as with a second uplink kept for when the first fails, the one of the lowest metric, which the kernel takes;
  - **each name server** that interface was given, at most three, by a question for a name: the internet target's, where that is a name; else the manager's own; else a time server's. Where the AP is given none of them by name, the servers are not asked;
  - **the internet,** by ping, where the config names a target.
  - Each is `up` while it answers, with how long its last answer took; `silent` once three asks in a row went unanswered; `unknown` until it has been asked that long.
- **`system.internet` names the internet's target:** a host name or an IPv4 address, in Locations, inherited as any setting is. A name is held to what a name server can be asked for: labels of 1 to 63 letters, digits and hyphens, 253 in all. Nothing of the internet is asked unless it is set.
  - **A target that is a name is never looked up in a way that could hold the prober up.** Its address is read from the name servers' own answers to the question they are asked anyway. Until one gives an address it is `unresolved`. The first address given is kept while it answers, and another taken only once it has gone silent.
- **The agent says it in its state report,** as `path`, with how long the manager took to answer its last config poll. The manager keeps it as said; one not well said is left out, and the report kept.
- **It is at the head of the AP's Network Paths, under Interfaces** (Griff, 2026-10-12), above what that AP timed of its networks (0141). What follows said a tab of its own.
- **An AP has a Path tab.** Its way out as a path, a hop at a time: the AP's management address and VLAN, its uplink's speed and the switch port it is on, its gateway, and the internet; each coloured by whether it answers, with how long it took. Under it, what the AP leans on: each name server, its time (0069), and the manager.

## Consequences

- **Whether an AP's own way out is whole is seen at once,** and where it stops.
- **An AP sends three or four small packets every 30 seconds** more than before; to the internet only where a target is set.
- **A gateway that does not answer pings reads as silent** though it forwards. So does an internet target that does not.
- **What the name servers take is for one name,** asked again and again: mostly from their cache. It says the server answers, and little of how long a name not yet known takes.
- **The manager's time is a whole request,** with the connection made for it: tens of ms where the gateway's is a few.
- **Only the latest is known.** These times are not kept by the hour yet (0141 keeps a network's, as a client would find them).
- **It comes with the agent:** an AP shows it once its agent has updated itself (0079), and where its prober runs.

## Not decided

- **The same kept over time,** and drawn as 0141's are: an AP's gateway, name servers and internet by the hour.
- **An alert** where an AP's gateway or its name servers go silent.
- **IPv6.** The way out is the IPv4 one.
- **More than one question,** such as a name of the site's own beside one of the internet's.
