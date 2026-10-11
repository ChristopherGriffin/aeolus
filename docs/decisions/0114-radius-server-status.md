# 0114. Whether the RADIUS server answers

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff, "are you putting in corresponding sections into aeolus for these?". Written up by Claude
- Refines: 0059, 0098, 0099, 0112

## Context

- **A network that signs clients in against a RADIUS server (0098, 0112) is only as good as that server's answer.** With the server down, out of reach, or not knowing the AP or its secret, no one signs in. Nothing in Aeolus said so: the AP was online, its config applied, and the network was on the air.
- **It shows up badly from the client's side.** A phone says "can't connect" or asks for the password again, which looks like a wrong password.
- **RADIUS has a question for this:** Status-Server (RFC 5997). A server that takes it answers a request signed with the shared secret, so an answer shows three things at once: the server is up, the AP reaches it, and the two agree on the secret. FreeRADIUS takes it by default. Some servers, Windows NPS among them, do not.
- **hostapd counts what the server answered its clients' sign-ins with:** its control socket answers `MIB` with the requests, accepts, rejects, challenges and timeouts since the network was started. The prober already asks that socket who signed in (0113).

## Decision

- **The prober asks each RADIUS server for its status every 30 seconds,** one request per server and port, however many networks use it. The servers are the ones `/etc/config/wireless` names on Aeolus's networks.
  - The request carries a Message-Authenticator made with the network's secret. An answer counts only if it fits the last request and the secret.
  - Any answer that fits counts, an Access-Reject too: the server is there and knows the AP.
- **It reads hostapd's counts as often,** summed over the network's radios, and notes when the count of answers grew and when the count of timeouts did.
- **Each network's server gets one of four verdicts:**
  - **up:** it answered a status request, or a client's sign-in, in the last 95 seconds; or it answers status requests and has not yet missed three in a row.
  - **silent:** three status requests in a row went unanswered, and either it used to answer them, or a client's sign-in timed out in the last 10 minutes and none was answered since.
  - **unverified:** it has never answered a status request, and no sign-in says anything either way. A server that takes no status requests is here whenever no client is signing in.
  - **unknown:** not asked three times yet.
- **It goes in the state report as `radius`,** one entry per network: the server and port, the verdict, the last round trip, how long since the last answer, and the counts.
- **A silent server is a critical alert,** `radius-silent`, on each AP and network it is silent for. The AP's own page says so with its uplink and DHCP troubles.
- **The AP reports at once when a server goes silent or comes back,** as it does for a tunnel (0059), not with its next report up to 5 minutes later. In the lab's first outage test the APs knew in 100 seconds and the manager 90 seconds after that, when the reports came round.
- **The Networks tab shows it as "RADIUS on each AP",** beside the transports and DHCP of each AP: server, status, last answer, and sign-ins accepted, refused and timed out.
- **The prober logs when a server goes silent and when it is back,** not each change between up and unverified.

## Consequences

- **A wrong secret reads as silent,** since a server drops what it can't verify. The message names all three causes.
- **A server that takes no status requests is only caught when a client tries.** Its alert comes with the first sign-in that times out, and ends 10 minutes after the last one, or sooner when a sign-in is answered.
- **Counts start again whenever the network does,** as on a config change. They show what happened lately, not a history.
- **Each AP speaks for itself.** A server one AP can't reach and another can is silent on the one and up on the other, which is what tells a routing or firewall fault from a dead server.
- **The accounting server and the Disconnect client (0111) are not asked.** Only the server that signs clients in is.
- **Not done here:** a second, fallback server, which the schema does not have yet.
