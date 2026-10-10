# 0109. What alerted, and for how long

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, as one of the features Wi-Fi managers commonly have that Aeolus lacked (Griff, "create features common in these systems")
- Refines: 0099, 0101

## Context

- **Alerts are worked out on each look from what the manager has (0099),** so one ends when its cause does. That is right for what needs attention now. It leaves nothing to say what happened while nobody was looking: an AP that dropped for twenty minutes at 3 a.m., or a tunnel that went down and came back.
- **The notifier sees each alert begin and end (0101),** but only for a folder with somewhere to send it, and it keeps nothing.
- **Wi-Fi managers keep an event log** of what alerted, when, and for how long: Meraki's event log, UniFi's alerts history.

## Decision

- **The manager keeps an alert log** in its conditions database: each alert's AP, key, kind, severity and message, when it began, and when it ended.
  - The loop that sends alerts (0101) keeps it, every minute, whatever the folders' `notify.*` say.
  - An alert is logged once it has lasted two minutes (`alerts.Hold`), as one that would be sent. It begins when it was first seen, or at its own `since` where that is earlier, as an AP's last report is for one gone quiet. A shorter blip is not logged.
  - Its end is logged at the first look it is gone.
  - A manager that starts again takes up the alerts its log has open. Those still alerting go on; the others end at its first look, which may be later than they did.
  - The log is kept as long as the state reports are (`--keep-state-days`).
- **`GET /v1/alerts/history?under=&hours=`** gives the alerts that lasted at some time in the last 24 hours unless set (at most 720), newest first, at most 500, on the APs the caller may view. The MCP adapter offers `list_alert_history`.
- **The Alerts tab shows a History panel** below the alerts now: each alert of the last day, its AP and message, when it began, and how long it lasted, or that it lasts still.

## Consequences

- An alert's message is the one it had when logged. One whose message changes as it goes on keeps its first.
- While the manager is down nothing is logged. An alert that began and ended in that time is not there.
- The log is what the manager saw, not each AP's own account. A history the AP keeps of itself, such as its own log, is for later.
