# 0119. An AP's own health, and what happened to it

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff: "we also need an AP health page with mem and proc, also the same kind of history for the AP as the client." Written up by Claude
- Refines: 0039, 0099, 0109

## Context

- **An AP said nothing of itself as a machine.** Its state report had its radios, clients, tunnels and uplink, and how long it had been up. Whether it was short of memory, pinned at full load or running hot was not there.
- **What happened to an AP was in four places,** none of them a history: its applies, the alert log (0109), what it was asked to do (0104), and whatever could be read out of comparing its state reports by eye. A restart showed only as an uptime that had gone down.
- **A client now has a history** (0118): each time it came online, and how. Griff asked for the same of an AP.

## Decision

### Health

- **Each state report says how the AP itself is doing,** as `health`:
  - how busy its processor was since its last report, in percent of all its cores, and how many it has;
  - its load over 1, 5 and 15 minutes;
  - its memory, total and available;
  - the space where its config is kept (`/overlay`), total and free;
  - the hottest of its temperature sensors;
  - how many processes it runs, and its connection table.
- **The manager keeps a row a report,** so a day's chart reads a few hundred rows, as usage does (0108). They are kept as long as the state reports are.
- **`GET /v1/aps/{ap}/health`** gives the latest, and the series over a day, or up to 30. Past 300 reports, each point is the worst of a run of them: the busiest, the least memory free, the hottest.
- **Alerts** (0099), from the latest report:
  - `memory-low`: under 10% of memory free, a warning; under 5%, critical;
  - `storage-low`: under 5% of storage free;
  - `cpu-busy`: 90% busy or more over the whole time since its last report;
  - `hot`: 95 °C or more.

### History

- **Each state report is compared with the one before, as it comes in,** and what changed is kept as an event:
  - a restart, with how long it had been up, and how long it was silent;
  - a silence of more than three reports that was not a restart;
  - another agent, or other firmware;
  - a radio that went down or came up, or moved channel or width;
  - an uplink on another switch or port;
  - its first report.
- **`GET /v1/aps/{ap}/timeline`** joins these with what was already kept: each config applied or not, each alert that began and ended, and each thing it was asked to do. Newest first, over a week, or up to 30 days.
- **Events are kept as long as clients' connections are** (0118): 90 days unless set.

### The page

- **An AP has a Health tab:** tiles for its processor, memory, storage, temperature, time up and what it runs; its processor and memory over a day or a week; and its history.

## Consequences

- **The processor's figure is an average over the time between two reports,** five minutes as a rule. A burst of a few seconds does not show. An AP's first report after its agent starts has none.
- **A restart is told by its uptime,** so one that takes less than two minutes between two reports a few seconds apart could be missed. Reports are five minutes apart.
- **What changed between two reports is all that is seen.** A radio that moved channel and back between them left no event.
- **History begins with this release.** What happened before is not made up from old reports.
- **The temperature is the hottest sensor's,** whichever it is: a radio's on the C-360, the processor's on the office AP. The alert's 95 °C is a guess at "too hot for any of them", to be set per kind of AP if it proves wrong.

## Not done here

- **Health across a folder's APs at a glance.**
- **Which process uses the memory or the processor.**
- **Memory and processor on the manager itself.**
