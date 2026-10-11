# 0134. A speed server near, and one far

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff. The speed server needs to be a local and a remote one; and a pulldown for Cloudflare's public speed tests, and anyone else who hosts them. Written up by Claude
- Refines: 0121, 0123, 0130

## Context

- **A client test measures speed against one server (0121):** `system.speed_server`, an OpenSpeedTest server on the site's own network. That shows what the air and the wire carry, and nothing of the way out.
- **A slow site is slow somewhere.** With one figure to a server next door, and one to a server out on the internet, it can be said which side of the gateway.
- **The test speaks plain HTTP,** from sockets bound to its own interface, with no TLS: a GET that gives bytes, a POST that takes them, a few connections at once, for five seconds each way after one to warm up.
- **Public servers that answer that way, both ways,** tried from the lab on 2026-10-11 from the C-360's own uplink:

  | Server | Down, Mbit/s | Up, Mbit/s |
  |---|---|---|
  | Cloudflare (`speed.cloudflare.com`, its nearest site) | 480 | 39 |
  | LibreSpeed, Atlanta (Clouvider) | 275 | 45 |
  | LibreSpeed, Los Angeles (Clouvider) | 231 | not run |
  | Tele2 (Europe) | 18 | 45 |

  - Five more of the LibreSpeed project's public servers answer plain HTTP both ways, all in Europe: London, Amsterdam and Frankfurt (Clouvider), Prague (CESNET, and Turris) and Belgrade (SOX). Each gave a megabyte and took a POST; none was measured.
  - Its other thirteen, Chicago, Denver and New York among them, send plain HTTP to HTTPS, which the test cannot speak.
  - Ookla's servers are left out: their terms do not allow this use without a licence.
- **What the public servers differ in is paths, and how much a POST may carry.** Cloudflare gives bytes at `/__down?bytes=N`, up to 99,999,999 a request, and takes them at `/__up`. LibreSpeed gives them at `garbage.php?ckSize=N` megabytes and takes them at `empty.php`, and refuses a POST of 32 MB (413), which is what the test sent its own server. Tele2 gives a file and takes a POST.
- **A measurement is not small.** At the lab's link one to Cloudflare moved 226 MB down and 20 MB up. Every test on a schedule doing that, on a metered or a shared link, is a cost someone should choose.

## Decision

- **Two settings, both the APs' own:** `system.speed_server`, the local one, as before; and `system.speed_server_remote`, the remote one.
  - The remote one is a public server by its name, `cloudflare`, `librespeed-atlanta` and so on, or an OpenSpeedTest server of one's own by its URL, as the local one is.
  - **The page offers the public ones in a pulldown,** by where they are, with one's own as its last choice, which shows a field for the URL.
  - The names are the manager's list. The schema has the same, with how each is said; a test holds the two together, and holds each server to what an agent will take.
- **The manager hands the AP the remote server whole:** `speed_remote` in what a test is asked, `{name, host, port, down, up, body}`: the host and port, the path a GET of which gives bytes, the path a POST to which takes them, and how many bytes a POST carries. An agent knows no list, and needs no update for a server added to it.
  - The agent takes only a plain host and plain paths, as it writes them into its requests.
  - An agent from before this does not read it, and measures the local speed alone.
- **The test measures the local speed, and then the remote.** The remote server's name is looked up of the network's own DNS server, as a client's would be, and its bytes go by the network's own gateway.
  - A server may close a connection after so many requests: another is opened in its place.
  - A server has five seconds to take a connection: one that answers nothing does not hold the test, and the scan radio, for the minutes a connection otherwise waits.
  - Its steps are of `speed`, and name their point, `remote`.
- **What it found is `speed_remote`, beside `speed`:** the server's name, Mbit/s down and up, or why not.
- **That the remote speed could not be measured fails no test.** The server is not the site's own: one that is away, or that refuses, would otherwise be an alert at every site that uses it. It is said, in red, at its point.
- **Its way has a point more (0130):** `speed`, drawn Local, and `remote`, drawn Remote, each with its figures.
- **On a schedule, each speed has its own switch:** `assurance.tests.speed`, the local one, as before; and `assurance.tests.speed_remote`, off unless set. A round of every test asked by hand (0129) follows the same two, as before. One test asked by hand of an AP measures both where they are set.
- **A thing's tests in brief have the remote speed too,** `remote_down` and `remote_up`, and each AP that tests says which speeds its tests measure.

## Consequences

- **Through the air the remote speed is no more than the scan radio's:** on a band where it links at a legacy rate, both figures are its own limit, and say only that neither side is slower than that. Tried on the C-360, 2026-10-11, from its scan radio on 5 GHz: 19.9 down and 11.9 up to the local server, 19.5 and 12.3 to Cloudflare, where its uplink alone had 480 and 39. The page says so of a legacy rate already (0121).
- **A test with both takes some twelve seconds longer,** within the scan radio's loan: 29 s in all on the C-360.
- **A public server is someone else's.** It sees the site's address; it may change its paths, refuse a site that asks too much, or go: the list is the manager's, and changes with it.
- **The list is short where the lab is not:** two LibreSpeed servers in the United States answer plain HTTP. Cloudflare's is near everywhere.
- **Not in this:** a server over HTTPS; the remote speed over the wire, where it would be the uplink's own and not the scan radio's (the test over the wire has no TCP of its own, 0128); a measurement that stops at so many bytes; and trends of either speed.
