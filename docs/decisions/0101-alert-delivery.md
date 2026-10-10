# 0101. Sending alerts

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, the step 0099 left next (Griff, "create features common in these systems")
- Refines: 0027, 0085, 0099

## Context

- **0099 lists what needs attention, but only for someone looking.** A manager earns its keep at night when it tells someone: an AP offline, a network with no transport.
- **Two ways cover most people.** An ntfy topic gives a push on a phone with no account. A webhook reaches anything else: a chat, a ticket, a script.

## Decision

- **`notify.*`, a Locations field set on the Org, a folder or an AP,** and inherited as any is. Like `templates.*`, it is the manager's own, left out of every AP's config. It holds:
  - `ntfy`: a topic URL;
  - `webhook`: a URL each alert is POSTed to as JSON, `{event, alert, at}`;
  - `severity`: the least severe sent, critical unless set;
  - `resolved`: also send when the cause is gone, on unless set off.
  - The URLs are sealed, as secrets are (0027): a topic is as good as a password, and a webhook URL often holds a token. They are opened only to send.
- **The manager looks every minute,** at every AP's alerts and where each one's go.
  - An alert is sent once it has lasted two minutes, so a blip of one poll is not. It is sent once while it lasts.
  - Its end is sent when the cause is gone. Then the same alert stays quiet for 15 minutes, so an AP coming and going does not page someone every minute.
  - At its first look after a start, the manager takes what is already alerting as sent, so a restart sends nothing anew. An end it sees afterwards is still sent.
- **ntfy gets the message as a push,** titled "Aeolus: <AP>", at priority 5 for critical, 4 for warning and 3 for info or resolved.
- **A send that fails is logged, not retried.** The next alert tries again.
- **The folder's System tab has an Alerts section,** with these fields.

## Consequences

- What was sent is kept in memory only. After a restart, an alert that began while the manager was down is sent once it has lasted two minutes. One that began and ended then is never sent.
- Email needs a mail server to send through, so it is left for later. A webhook to a mail gateway covers it until then.
- No endpoint is set anywhere yet: Griff picks where his go.
