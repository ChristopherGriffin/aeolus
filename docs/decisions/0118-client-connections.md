# 0118. Every client, and each time it came online

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff: "I also want a historic database of every client. I want to be able to click on a client and see his connection history, and opening each one (pass or fail) shows me the whole process of the client getting online: probes, association, authentication, DORA, DNS exchange and hitting the gateway." Written up by Claude
- Refines: 0065, 0066, 0103
- Is the next part 0103 named

## Context

- **A client's journey (0103) is made from the APs' state reports,** every few minutes. It shows where a client was and how well. It can't show the steps between: the association, the keys, DHCP's four messages. 0103 said so, and that the AP would have to record them as they happen.
- **A client that never gets online is in no state report at all.** A wrong passphrase, a refused sign-in or a DHCP server that doesn't answer leaves nothing to read back. Those are the ones worth finding.
- **The manager keeps state reports for 30 days.** A client not seen for a month is gone.
- **What an AP can see of a client coming online,** read on the lab's APs, 2026-10-10:
  - **hostapd's own words, in the system log:** authenticated, associated, each step of an 802.1X sign-in, the key handshake, a wrong passphrase, being let on, and why it left.
  - **The client's first frames,** on the network's Wi-Fi interface: DHCP both ways, the ARP for its gateway and the answer, DNS lookups and answers, and the frames that open and answer a TCP connection.
  - **Where it was heard asking for networks,** from band steering's usteer where that runs, which shares it between the APs.
  - **Not its probe requests themselves.** hostapd offers them to a subscriber on ubus, but with band steering on it then waits for every subscriber's answer to each one, up to 100 ms. A second, slower subscriber would hold up every client's joining.

## Decision

### The AP records each attempt

- **A daemon, `aeolus-journey`, follows each client from its authentication to its first traffic,** and writes one record of the attempt.
  - It reads hostapd's lines from the log: `logread`, which it starts, sends the lines that name hostapd to it over UDP on the loopback, as they are logged.
  - It reads the frames from a packet socket on each Wi-Fi interface, with a kernel filter. Until a client is being watched the filter keeps only ARP and DHCP. For a minute after one starts, it keeps DNS and the frames that open TCP connections too.
  - It sends nothing, and holds no client up.
- **The stages, in order:** authentication, association, sign-in (802.1X, where the network has it), keys, DHCP, gateway, DNS, first connection. Each step is kept with how long after the start it came.
- **An attempt ends, and is one of four things:**
  - **online:** it has an address, and something beyond itself answered it: its gateway's ARP, a DNS server, or a connection;
  - **failed:** with the stage it stopped at, and why: a wrong passphrase, a refused sign-in, no answer to DHCP, a gateway that never answered;
  - **left:** it went of its own accord before it was seen online, wherever it had got to. Leaving is a failure only where it was refused an address, or had waited for an answer that never came: 8 seconds for DHCP, and 4 for its gateway from when it first asked for it. A client that leaves while associating, as one that picks another AP, has failed nothing;
  - **connected:** let on, and nothing more seen in 30 seconds, as a device that keeps its address and says little.
- **A line of hostapd's that names the client and is not known is kept as it reads.** It may be the one that explains a failure.
- **Bounds:** 48 steps, 4 lookups and 2 connections an attempt; 256 attempts followed at once; 128 records waiting for the agent, the oldest let go past that and counted.
- **The agent sends the records with its poll,** 32 at a time, and notes how far it got, so the daemon lets them go.

### The manager keeps them

- **`POST /v1/ap/connections`** takes them. Each is read and checked by itself: one not well formed, or with a field this manager does not know, is passed over and counted, and does not hold the others back.
- **One sent again is not kept twice.** An attempt is told from any other by its AP, its client and the start its AP gave, which is the same however often it is sent. When it began, as shown, is that start where it is near the manager's time, else when it came in, each of those sent together a millisecond after the one before.
- **Every client seen is kept for good:** its MAC, when it was first and last seen and where, what it calls itself, who it signed in as, its address, how many times it tried to come online and how many failed. A state report's clients (0066) count as seen too.
- **Its attempts are kept 90 days,** unless `--keep-connection-days` or `AEOLUS_KEEP_CONNECTION_DAYS` says otherwise.
- **`GET /v1/clients`** lists the clients, the latest first, with `?q=` to find one and `?under=` a Locations node.
- **`GET /v1/clients/{mac}/connections`** is one client's attempts, the latest first, each with its record.
- **Both show only what the caller may view:** a client by the AP it was last seen on, an attempt by the AP it was made on.

### The Clients tab shows them

- **"On now" or "Everyone seen".** Everyone seen is the clients' list, the latest first, with a search.
- **A client opens its journey, with its connections first:** each attempt a line with when, where, and how it went. Its arrow opens the stages in a row, done, failed or not seen, and then every step with its time.

## Consequences

- **A failed join can be read back:** who, where, when, at which stage, and hostapd's own word for why.
- **The names a device looks up in its first seconds online are stored,** at most four an attempt. They are what tells a captive portal's check from a real failure. With a user name (0113) they are personal data, kept 90 days, and shown to whoever may view the AP.
- **DNS over TLS or HTTPS is not seen,** nor a lookup over TCP. A client that uses only those shows no DNS stage, and is online by its gateway or its first connection.
- **A time is when the AP read the line or the frame,** not when the radio sent it: right to a few milliseconds, and steps read together share one.
- **Probes are where the client was heard, not each request.** Where band steering does not run, the record says they were not recorded.
- **An AP's clock, if unset, would misdate its attempts,** so one not near the manager's time is given the time it came in.
- **A client that was already on when recording began has no attempt** until it joins again.
- **Cost on the AP,** measured on the office AP and the C-360: about 2 MB of memory and no measurable CPU with no client joining.

## Tried

- **On the C-360, 2026-10-10,** beside the running agent: its spare radio joined Aeolus-Enterprise by EAP-TLS. The record had authentication, association at 11 ms, the sign-in started and accepted at 155 ms, the keys at 165 ms, DHCP's discover and offer at 658 ms and request and ack at 766 ms, with the gateway and DNS server it was given, and the client leaving at 896 ms.

## Not done here

- **The same history for an AP itself,** and its memory and processor over time: asked for with this, and next.
- **Following a client across a roam as one journey.** Each AP's attempt is its own record.
- **An alert when a network's joins keep failing,** which these records would feed.
- **Turning off the names looked up,** for a site that would rather not keep them.
