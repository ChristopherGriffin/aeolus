# 0121. A client test from the scan radio

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff, on seeing the C-360's spare radio join a test network as a client: "Are you able to use the C-360 spare radio as a client to do this testing? That would be so cool." And before, 2026-10-08: the spare radio as a client, with a speed test to the AP, to a server on the premises and to the internet, "a full view of the chokepoints in the system". Written up by Claude
- Refines: 0081, 0104, 0116, 0118

## Context

- **An AP with a scan radio has a radio that serves no one** (0081). The C-360's is airscan's: it scans, sweeps the spectrum and looks at clients, so the serving radios never have to.
- **That radio can be a client.** With airscan stopped and a client interface made on it, wpa_supplicant joins a network as a phone would. It was done by hand on 2026-10-10, to try the RADIUS work (0111 to 0115).
- **Nothing in Aeolus says whether a network works from where a client stands.** The APs report what they serve, and each client's own way online once one comes (0118). A network no one has joined yet, or one that is failing everyone, is known only when someone complains.
- **What the lab's C-360 can do,** read and tried on it, 2026-10-11:
  - **Its kernel answers ARP only for the address of the interface asked** (`arp_ignore` 1, OpenWrt's own setting). So a second address of the AP's, on the client interface, is not answered for on its wired side.
  - **A socket bound to an interface sends and hears only there** (`SO_BINDTODEVICE`), which ucode's socket module can ask for.
  - **It has no network namespace tools,** no `ip netns`, and no iperf or curl. It has wpa_supplicant, udhcpc, ping and ucode.
  - **It moves bytes fast enough to measure with:** in ucode, over its wire, 1.9 Gbit/s down and 1.5 Gbit/s up from the lab's speed server.
  - **Its scan radio is a slow client.** One spatial stream. On 2.4 GHz it does 802.11ax at 20 MHz (143 Mbit/s). On 5 GHz the driver gives it no HT or VHT, so it joins as 802.11a: 54 Mbit/s, about 21 Mbit/s of TCP.
- **The lab has an OpenSpeedTest server** (Zephyrus): a file of 30 MB to fetch and a place to send as much, over plain HTTP.

## Decision

### What a test is

- **The scan radio joins one of the AP's own networks as a client would,** and each step of its way online is timed:
  - **finding** the network on the AP asked for;
  - **authentication, association and the keys,** as wpa_supplicant says them;
  - **DHCP:** the address given, by whom, and the gateway and DNS servers with it. A lease that names no gateway, or no DNS server, fails the test at that stage: a client has no way online without them;
  - **the gateway:** three pings, and whether it answered ARP. One that answers ARP is there, though it answer no ping, as many are set not to: only one that answers neither fails the test;
  - **DNS:** one name looked up at the first server DHCP gave;
  - **speed,** where a speed server is set: bytes fetched for five seconds and sent for five, over three connections at once, after a second to warm up.
- **It joins with the network's own passphrase,** which the AP has, as it serves the network. So a test is of a network the AP itself offers.
  - It is read from Aeolus's own section for that network, found by the network's ID: another interface with the same SSID, made by hand or for another network, is not it.
- **A network that signs clients in by 802.1X it joins as the network's test user,** where it has one (0124). It can't yet join one where each device has its own passphrase from RADIUS (0115): it has no passphrase of its own.
- **Where it joins:** an AP of the tester's choosing, the testing AP itself among them, or else the strongest other AP the scan radio hears. A band may be asked for.
  - The manager gives the AP the BSSIDs of the AP to join, from that AP's own report.
  - The client keeps to those, and looks for them where the scan radio last heard them, or on every channel of the band where it did not: a look can miss a beacon, most of all on a DFS channel, where it may only listen.

### The radio is borrowed, and the AP's own traffic left alone

- **The radio is airscan's.** The test stops airscan, makes its client interface, and when it is done takes the interface away and starts airscan again, whatever happened.
- **An airscan that lends its radio is asked for it instead,** and runs on (its `lend` and `reclaim`, which it says it has among its `features`). It takes its own interfaces off the radio, runs no job on it while it is lent, and takes it back when told, or after a minute and a half should the test die.
  - It says no while it scans or sweeps, and the test asks again.
  - It keeps its place in its own round, so the test costs it no scan or sweep.
  - **Such an airscan sees to a deaf radio itself** (`heal`): it binds the driver again where a look hears nothing. So the test waits for no sweep. It asks airscan for a look, and where that hears nothing, waits for airscan to bring the radio back, and binds the driver itself only if airscan does not.
  - **A test that had the radio on a DFS channel binds its driver again before it gives it back,** inside the loan, while airscan has nothing on the radio. That radio goes deaf within half a minute of such a test, every time (see Tried), so airscan is not handed one that is about to. The result says so. A test that only looked on a DFS channel, for a network it then joined elsewhere, does the same.
- **It takes the radio only while airscan is between its jobs,** looking at clients, and waits up to two minutes for that. Never while airscan scans or sweeps the spectrum, nor within three seconds of when it next will.
- **It holds the radio for under a minute.**
- **The client's traffic is its own:**
  - its address is the only one on its interface, with no route of the subnet's beside the AP's own;
  - its routes have a metric nothing else would pick, so only a socket bound to the interface takes them;
  - every packet of the test is sent from such a socket;
  - IPv6 is off on the interface, so a router's advertisement gives the AP itself no route.
- **The test is not done until the scan radio is seen to hear again.** A scan radio that has been a client can go deaf, at once or at airscan's next sweep of the spectrum (see Tried). So, the radio given back, the test waits for airscan's first look, then for the sweep it makes next, and asks for a look after it.
  - **A scan radio that then hears nothing, not even the AP's own networks, is deaf,** and its driver is bound again, which is what brings it back. The result says so, and how many networks it hears after.
  - **One not seen to hear is treated the same:** where airscan makes no look, no sweep in a minute and a half, or no look when asked twice. What it heard before the sweep says nothing of after it.
  - **Only a radio on PCI is bound again,** by its own address there, as airscan names it. No other radio is unbound with it.
  - **One already deaf when a test is asked** is bound again first, and the result says that too.

### How it is asked for, and what comes back

- **It is an action** (0104): `POST /v1/aps/{ap}/actions` with `{kind: "client-test", network, on?, band?}`, by someone with operator on the AP.
  - The manager refuses one for an AP that has not said it has a scan radio, for a network the AP does not offer or the test can't join, and for an AP to join that has not said its BSSIDs.
  - One test at a time for an AP: asked again while one is open, it is the same one.
- **An action now carries what it is asked, and brings back what it found:** `params` and `detail`, both JSON, kept with it.
- **The AP takes it up on its next poll** and runs it beside the agent, as it takes two or three minutes. While one runs the agent polls every five seconds, and says what came of it as soon as it is done.
- **A test has seven and a half minutes for all its waits together.** Each at its longest, they come to more: for airscan to be free, to look, to sweep, for a driver to come up. So the last of them give way, and the result says what was not seen, where the agent would otherwise have taken a test still waiting for a dead one.
- **A test that dies is said to have,** after eight minutes at most, and what it may have left is put right: its interface taken down and airscan started.
- **The result is checked by the manager** as any report is, and kept as what the manager knows of it.
- **`system.speed_server`** is the OpenSpeedTest server a test measures against, as a plain HTTP address, its port one a socket can be opened to. Unset, a test measures no speed. It is given to the AP with the test, not rendered into its config.

### What is shown

- **An AP with a scan radio has Client tests on its Overview:** a form (the network, the AP to join it on, a band) and each test asked, latest first.
- **A test opens to its stages in a row,** done, failed or not reached; why it failed; what it measured, a line each; and every step with its time, as a client's own connection opens (0118).
- **The link's rates are shown with the speed.** Where the link ran at a legacy rate, the page says the speed is the test radio's own limit.
- **The join as the AP that served it saw it** (0118) opens from there: the test client is a client like any other to that AP.
- **System has the speed server,** under Client tests.
- **Claude and other clients ask through `ap_action`,** and read results where an AP's actions are listed.

## Consequences

- **A network can be tried before anyone is on it,** and a complaint checked from where an AP stands: the passphrase, the VLAN, DHCP, the gateway and DNS, each seen to work or not, with its time.
- **Only an AP with a scan radio can test,** and only networks it offers itself.
- **While a test runs the AP has no scan radio:** airscan is stopped for a minute or so, and radio resource management, which listens through it (0116), hears nothing new for that time.
- **Starting airscan again starts its work from the beginning:** a scan and a spectrum sweep, about a minute, before it looks at clients again.
- **A test takes two to three minutes in all** where airscan has to be stopped, of which the test itself is under one. The rest is airscan's scan and sweep, waited for so the result can say the scan radio still hears; and half a minute more where its driver had to be bound again.
- **Where airscan lends its radio a test takes about half a minute:** the test, and one look after it; and twenty seconds more after a DFS channel, for the driver. A radio that goes deaf later, for another cause, is airscan's to bring back, and the test's result does not say so.
- **Binding the driver again gives the scan radio a new name** (`phy8` for `phy7`). airscan finds it by where it is, not by that name, and so does the test.
- **The speed is the test radio's as much as the network's.** The C-360's scan radio on 5 GHz shows about 21 Mbit/s whatever the network can do. On 2.4 GHz it showed 72 down and 69 up over a 143 Mbit/s link.
- **A test's client is seen by everything that sees clients:** the serving AP's client list and its record of the join, DHCP, which gives it a lease, and airscan on other APs. Its MAC is the scan radio's own, and it calls itself `aeolus-test`.
- **The passphrase is written to a file for wpa_supplicant,** in memory (`/var/run`), readable by root alone, and removed when the test ends.
- **No traffic of the AP's own goes over the client interface,** and none of the test's over the wire: checked by what each interface counted.

## Tried

- **On the C-360, 2026-10-11,** first as a script by hand, then as the agent's own:
  - **The office AP, 5 GHz, on DFS channel 132:** found in 0.4 s, let on in 0.6 s, an address in 0.3 s, the gateway in 4 ms, a name in 42 ms, 21 Mbit/s each way over a 54 Mbit/s link.
  - **The office AP, 2.4 GHz:** let on in 0.5 to 2.6 s, 72 down and 69 up over a 143 Mbit/s link.
  - **The C-360's own 5 GHz network, on channel 44:** 12 Mbit/s each way, the test radio and the serving radio sharing the air a metre apart.
  - **A BSSID the scan radio had not heard in its last look:** found by the client's own look across the band.
- **The scan radio went deaf,** hearing no network in any look, its own AP's among them, and was brought back each time by binding its driver again. What was seen of when:
  - **After a test that had it on DFS channel 132, every time, six of six,** and within half a minute of the test's end. Looks at one such radio heard 42 networks 14 s after, 13 at 37 s and none at 99 s; at another, 15 at 13 s and none at 34 s. One of the six only looked on the DFS channel, and joined on 2.4 GHz.
  - **It is not airscan's sweep that does it,** as first thought, when each such test was followed by a restart of airscan and so a sweep: with the radio lent and no sweep made, it went deaf as soon.
  - **After a test that kept off DFS, never, five times of five:** four on 2.4 GHz, with and without the speed measured, and one on 5 GHz channel 44.
  - **After airscan was only stopped and started, with no test, never, five times of five.**
  - **A look on other channels before the client interface was taken down did not help:** deaf 34 s after.
  - **Binding the driver again before airscan had the radio back did:** it then heard 40, 38 and 42 networks at 40, 90 and 150 s, and airscan had nothing to put right.
- **The test bringing it back itself,** with airscan stopped: a test on the DFS channel passed, the look after airscan's sweep heard nothing, the test bound the driver again, and 34 seconds later airscan heard 31 networks. Two minutes and a half in all.
- **With an airscan that lends its radio** (0.2, run by hand on the C-360):
  - **2.4 GHz:** 29 seconds from start to result, 56 down and 58 up.
  - **DFS channel 132, the driver bound again inside the loan:** 37 seconds, 26 down and 24 up, the radio hearing after.
  - **airscan's own cure,** the driver left alone by the test: 23 s after the test's end a whole round of its client surveys heard no beacon, its look heard nothing, it bound the driver again, and 20 s later heard 31 networks.
- **On a schedule (0123), its first test,** 2026-10-11 03:15: the C-360's own `aeolus_50`, 21 seconds from being taken up to its result, which was "failed at gateway". The gateway, a switch's VLAN interface, answered ARP and passed the DNS lookup on, and answered no ping. That was the test's fault, and why a gateway that answers ARP now passes.
- **Through the manager, from the AP's page,** 2026-10-11 02:09: Aeolus Lab on the office AP, 2.4 GHz: let on in 0.5 s, an address in 0.3 s, 70 down and 68 up; asked, taken up 38 s later, and its result there 101 s after that, with airscan stopped and started.

## Not known yet

- **Why being a client on a DFS channel deafens the C-360's scan radio.** It is the driver's or the firmware's (ath11k; the card is a QCN9072, driven as a QCN9074), not Aeolus's. Until it is known, the test binds the driver again after such a test, and airscan after any deafness it finds.
- **Whether other radios do the same.** The C-360's is the only scan radio tried, and the only one airscan runs on.

## Not done here

- **Saying in Aeolus that airscan brought a deaf radio back,** at a sweep after a test or at any other time. airscan logs it and counts it in its status; no report carries it yet.
- **A speed test to the AP itself and to the internet,** beside the one to a server on the premises.
- **HT and VHT for the C-360's scan radio on 5 GHz,** which is the port's (board data or driver), not Aeolus's.
