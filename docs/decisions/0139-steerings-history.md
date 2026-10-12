# 0139. Steering's history

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff, who asked that Aeolus report whether steering is working or failing (0138). Written up by Claude
- Refines: 0138, 0039, 0118

## Context

- **An AP says what its steering asked of clients (0138):** for each steered network a count, by each way, of what was asked and of each thing that came of it, and the last fifty askings, each followed to its end. That is in its state report (`steer`), of which the manager kept the last.
- **The count is since the AP's steering started.** It starts again at every update of the agent, at every restart of the AP, and whenever the daemon is restarted. On the day steering was released the lab's APs were updated twice within the hour, and each count went back to nothing.
- **So the manager could not say whether steering works** over a day or a month, nor for a network over all its APs, nor which clients will not be moved. 0138 named this as a later step.
- **The askings themselves are in every report,** until an AP has fifty newer: at a report a minute, an asking is said many times over.
- **An AP's clock is not always right when it says a time.** OpenWrt starts with the time of its newest file, and sets its clock from the network some seconds after: by then its clients are joining again, and a hold or a suggestion may be made, and ended, with the old time on it.

## Decision

- **The manager keeps each asking,** as its AP said it, once: told from any other by its AP, when the AP said it was, the client, the way and what came of it. A report that says it again adds nothing.
  - They are kept as long as attempts are (0118: 90 days unless `--keep-connection-days` says otherwise), and go with them.
- **When an asking was is its AP's word, where that can be so.** An AP's steering holds its askings in memory: none it says is from before it last started. One said to be from before the AP itself started, by the uptime in the same report, or from more than ten minutes ahead, was said by a clock not yet set: it is of this run, and is taken to be of when the manager first heard of it.
  - **One first heard of more than a week after it was made is not kept.** By then it is one already kept; or one kept, and since let go with its time, which an AP with few askings may still be saying. An AP out of touch with the manager for more than a week loses its older askings so.
- **`GET /v1/steering` says what steering came to** on the APs the caller may view, below a Locations node where one is named, over a time of up to 30 days:
  - for each network, by each way, how many askings there were and how many came to each end, as an AP's own count has them (0138); and the same an AP at a time;
  - the twenty clients asked most, each with how often it was asked, moved, said no and came back;
  - the latest hundred askings.
  - A client moved up, by a suggestion or a hold, that came back down within five minutes is said by its AP in a record of its own: a `back`, counted as that, and as no asking. A weak client that came straight back is the end of its own asking.
  - The tests' own clients (0121) are left out, as from roaming (0136). An AP's steering no longer asks them (0138), and did for a day.
  - The MCP adapter has it as `get_steering`.
- **The Networks tab's Steering panel is of a time:** a day, a week or 30 days, chosen there. A line a network, and for each way that is on, or asked something in that time, how many of those asked then moved: green where half or more did.
  - The rest is a click down: the same an AP at a time, where more than one asked; the clients asked most, each opening its journey (0103); and the latest askings.
  - Which ways are on, and where steering is not running, is as each AP last reported, as before.
- **Nothing changes on the APs.** The history is made from what they already say.

## Consequences

- **Whether steering works can be said for a network, over a time that outlasts an update.** "Suggest: 9 of 12 moved up" is of the last day or month, on all the APs of a folder.
- **A client that will not be moved shows.** One asked many times that says no each time is at the top of the clients asked most.
- **The history begins with this manager.** At its first report after, an AP's last fifty askings of the past week are taken, with their times.
- **An AP that makes more than fifty askings between two reports loses the oldest of them** to the history: its own count has them still. At a report a minute that is more steering than a busy AP does.
- **A clock that was wrong and plausible is believed.** An asking said with a time within the AP's uptime, and not ahead, is kept with that time, right or not; as an attempt is (0118).
- **The count an AP keeps (0138) is no longer shown.** It is still in its report, and says the same of the time since its steering started.

## Not decided

- **A place for steering on the Overview,** beside roaming (0136): a tile that says whether steering works, where a folder has it on.
- **An alert** where a way of steering fails for most of the clients asked, over a day.
- **What a client's own page says of it:** the askings made of one client, among its attempts.
