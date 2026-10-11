# 0127. How the clients' own joins go

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Claude, as the part 0118 left for later: "an alert when a network's joins keep failing, which these records would feed". Griff asked for a Service Assurance tab to gather what proves the service works from a client's side (0123)
- Refines: 0118, 0123

## Context

- **Assurance (0123) tries a network as a client would,** from an AP's scan radio. Only an AP with a scan radio can, and it tries one thing at a time.
- **Every AP records each attempt of every client to come online** (0118): which stage it reached, whether it got online, and why not. A client's own attempts can be read one by one.
- **Nothing adds them up.** Ten clients that cannot get an address on one network show as ten clients, each with a failed attempt, to someone who opens each.
- **One client failing says little about the network.** A wrong passphrase, a device at the edge of its range, a device that gives up: those are its own. Several failing in the same place at the same time is the network's.
- **Some failures are the client's however many have them.** A device kept with an old passphrase tries for ever, and a house can have several. A network that lets a client on and then gives it no address is another matter: what the client lacks then is the network's to give.

## Decision

- **A client that tried a network in a time is one of three things there:**
  - **it got on,** where one of its attempts came online, or was let on and seen no further (0118);
  - **it did not,** where an attempt of its failed and none got on;
  - **neither,** where it only ever left of its own accord, as for another AP.
- **A client that failed and then got on has got on.** Its failed attempts are still counted as attempts that failed, and where they stopped.
- **The Assurance tab of a folder shows it by network,** over the time the tab shows, a day unless asked: how many clients tried, got on and did not; how long a client typically took to come online, the middle one of the clients' averages; where the failed attempts stopped and why, the most clients first; and each AP's part.
  - **A network is an Aeolus network by its ID,** whatever its SSID was. A network of the AP's own that Aeolus does not manage is counted by its SSID, and said to be not Aeolus's.
  - **Reasons that differ only in a figure are one reason:** "no answer to its 3 DHCP requests" and "to its 4".
  - **Only the APs the caller may view are counted.**
- **An AP's own scan radio, making a client test (0121), is left out.** It is no one's device, and a test that fails is the tests' to say.
- **An alert, `joins-failing`, a warning on the AP:** over the last hour, three clients or more were let on a network at this AP and got no further, and they are half or more of the clients let on there, those that got on and those. It says how many of how many, and where most of them stopped and why, as the AP put it.
  - **Got no further is a failure at DHCP, the gateway or DNS,** with no attempt of the client's getting on.
  - **A client never let on is not counted,** on either side: a wrong passphrase, a refused sign-in, an association that did not complete. The tab shows those; they raise no alert.
  - **It ends when it is true no longer:** more clients get on, those that failed do, or the hour passes.
  - **It is an alert a network at an AP,** so a second network failing there is a second alert.
- **The Overview of a folder has a tile for it,** Clients getting on: of the clients that got on a network or did not over the last day, how many did.
- **`GET /v1/assurance` carries it as `joins`,** and the Aeolus MCP's `get_assurance` with it.

## Consequences

- **A site with no scan radio has assurance too:** what its clients themselves found.
- **A DHCP server that stops answering, or a gateway gone, shows within the hour** as clients that cannot get on, with the stage. The DHCP watch (0065) and the tests on a schedule (0123) may say so sooner; this says it from the clients' side, and for a network no test reaches.
- **A changed passphrase shows in the tab,** as clients stopping at the keys, and raises no alert: it is what was asked for.
- **A RADIUS server that refuses everyone raises none either.** One that does not answer is its own alert (0114).
- **Three clients is few for a busy AP and many for a quiet one.** Half of those let on keeps a busy AP's few failures from being an alert. A site of two devices raises none, whatever happens to them.
- **Clients that never try are not counted.** A network no client can even find shows nothing here; the client tests are for that.
- **An AP on an agent older than v0.69.0 records no attempts,** and counts for nothing here.
- **The manager's store is at schema 16,** for an index on when attempts began. A manager older than this one will not open it.

## Tried

- **In the manager's tests:** two clients let on that got no further are no alert, nor are four more that were never let on; a third makes three of the five let on, and is; two more clients getting on end it; and so do two of the failed ones getting on.
- **On the lab's manager, 2026-10-11,** the day's 59 attempts read back the same way by hand: no client let on and then left without an address, a gateway or DNS, so no alert.

## Not done here

- **How long each stage takes, across clients:** the time to an address, to the first DNS answer.
- **Joins by band, by kind of device or by operating system.**
- **A threshold a site can set.**
- **Roams:** a client that moves between APs is an attempt on each (0118).
