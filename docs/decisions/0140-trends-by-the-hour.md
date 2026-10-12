# 0140. Trends: what the reports came to, kept by the hour

- Status: Proposed
- Date: 2026-10-12
- Proposed by: Griff, who asked whether Aeolus should take in InfluxDB or another time-series database, and chose this in its place. Written up by Claude
- Refines: 0108, 0110, 0119, 0039

## Context

- **What a state report says is kept a row a report:** an AP's clients and traffic (0108), what each client moved (0110), and the AP's own health (0119). The rows go when the reports do, after 30 days (`--keep-state-days`).
- **So nothing can be said of more than 30 days.** Whether an AP is busier than it was in the spring, or its memory has been filling since an update, has no answer. Griff has said of the times of a client's steps that "later we will trend it"; the same is wanted of usage and health.
- **The time a graph is of is to be chosen at the graph.** Griff, the same day: "last 24, 7 day 1 month, at minimum". The usage chart was of a day and no other; an AP's processor and memory of a day or a week.
- **Keeping the rows longer costs most where it tells least.** What each client moved is a row a client a report: about 7,000 rows a day on the lab's three APs.
- **Griff asked whether a time-series database should come in.** The lab's store was measured on 2026-10-12, 86 MB:
  - the state reports themselves are 74 MB of it;
  - what each client moved is 3 MB, and the rest of the series under 1 MB together.
- **The series are small, and a time-series database would not hold the reports.** It would be a second service to install, update, back up and secure, beside a manager that is one program with its state in two directories, and that runs without the internet (0069).
  - Most of what Aeolus keeps is events: attempts (0118), roams (0136), askings (0139), alerts (0109). They are joined to clients, APs and networks, and shown only of the APs one may view. A relational store does that.
  - Clients' MACs, many of them private and changing, are what such a database handles worst.
- **What it would give that is missing is two things:** keeping for long, and counting many rows down to few.

## Decision

- **The manager keeps trends,** in its own store:
  - an AP's clients and traffic an hour at a time: how many reports, the most clients at one of them, and what they moved;
  - an AP's health an hour at a time: the worst a report in it said, the busiest processor and load, the least memory and storage free, the hottest, the most processes and clients;
  - what each client moved on each AP a day at a time, with the host name it last gave there that day.
- **A report is counted in as it comes,** in the transaction that keeps its own rows. Nothing is read back to make a trend, and the two never differ.
- **The trends are kept 400 days,** unless `--keep-trend-days` or `AEOLUS_KEEP_TREND_DAYS` says otherwise: a year, and enough of the one before to set a month beside the same month.
- **The rows already kept are counted in as the tables are made,** so the trends begin as far back as the store does.
- **`GET /v1/usage` and `GET /v1/aps/{ap}/health` take up to 8760 hours,** a year.
  - Over two days or less they are read as before, from the reports' own rows.
  - Over more they are read from the trends. Usage is in at most 48 buckets of whole hours. Its clients are counted from the reports' own rows where those still reach back to the span's start; where they do not, by whole days, from the first wholly in the span. An AP's health is the worst of each hour, and of a run of hours where there are more than 300.
  - The MCP adapter's `get_usage` takes as long.
- **The usage chart, and an AP's processor and memory, are of a time chosen there:** a day, a week, 30 days or a year. What a node's usage was last drawn over is kept over the page's redraw. An AP's history is of a day, a week or 30 days.
- **Its words are Griff's:** what went to a client is Down and what came from it Up, and the APs a client was on are under AP.
- **No time-series database comes in.**

## Consequences

- **A year of an AP is two rows an hour:** 8,760 of its usage and as many of its health. A client is a row a day on each AP it used.
- **The year says little at first.** It is as long as the store's rows were when this came: on the lab, since late September. It fills as time passes.
- **Over a span longer than the reports' own rows are kept, the clients' totals may lack up to a day of the chart's:** they are counted from the start of a day, in UTC, and never hold more than the chart does.
- **An hour's clients are the most at one report in it.** A bucket of many hours has the most of each AP in any of its hours, summed over the APs, as a bucket of many reports had.
- **Over 30 days the chart's buckets are 15 hours, and were whole minutes before:** the same 48, on whole hours.
- **An hour is not kept more finely than its worst.** What an AP's processor was at 14:25 on a day two months ago is not known; that it reached 90% in that hour is.
- **The store's schema moves to 19,** and does not move back.

## Not decided

- **The reports themselves,** most of the store. A client's journey (0103) reads them by searching each for its MAC; kept by client, they could be found by index and the reports let go sooner.
- **Trends of what is kept as events:** the times of an attempt's steps (0130), roams (0136), askings (0139). They are kept 90 days and counted as they are read.
- **A way out for others' dashboards,** such as a `/metrics` page, for one who runs a time-series database already.
- **Days counted down to months,** should a year of client-days grow large on a big site.
