# 0138. Steering, a network at a time

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff. A mix of ways to steer clients, each to be had on its own; no lowering of power to do it; Aeolus to say whether steering is working or failing; and the putting off of a client used for sticky clients too. A feature of an SSID, not of a whole AP: "the surgical way I want most features implemented." Aeolus is not to become a controller, and an AP is not to depend on it: steering done by a daemon on the AP was accepted "if it can fail open". Written up by Claude
- Refines: 0040, 0050, 0051, 0039

## Context

- **Band steering was usteer's (0050).** A network asked for it, and Aeolus turned usteer's band steering on for that SSID. What usteer steers by is one set of options for the whole AP; its own switches for refusing a client are off as OpenWrt ships it; and what it did was reported as two counts a band (0051).
- **There is more than one way to steer, and they suit different clients.** A client may be asked to move (802.11v), which only some take and any may refuse. It may be kept off a band for a moment as it joins, which works on every client and is felt by one that has nowhere else to go. One that holds on to an AP it hears badly may be told to go, and put off.
- **hostapd offers all three, a network at a time,** over ubus: it says each probe, authentication and association it hears and each client it lets on; it sends a BSS transition request to a client named; it disconnects a client, and keeps one off for a time by a ban it lifts by its own timer.
- **A listener's answer can refuse a client, where hostapd is told to wait for its listeners.** usteer tells it so on every network. hostapd then waits for each listener, up to 100 ms a frame, and takes an answer other than "well" as a refusal: a listener written in ucode that returns nothing answers "no data", and one that throws answers "error". Found on the C-360, 2026-10-11, from hostapd's source and ucode's.
- **Tried on the C-360, 2026-10-11, with its scan radio as a client (wpa_supplicant):**
  - Asked up from 2.4 GHz, it was on 6 GHz within the second, and said yes.
  - Kept off 2.4 GHz as it came to join, it was refused with status 17 (the AP cannot take a new station): with WPA3, at the second message of its authentication; with WPA2, at its association, 40 ms after its authentication. Free to choose, it was on 6 GHz 0.8 s later. With nowhere else to go, it was on 2.4 GHz four seconds after its first try.
  - Told to go from 5 GHz, it went within the second. Told to go with an hour named, and staying, it was put off twice: by hostapd when the hour came, and by the daemon after.
  - Refused four times running by the only AP it could join, it gave the network up for ten seconds. That was a ban of five seconds; at two and a half its third try got in.

## Decision

- **A network's clients are steered in three ways, each a switch of the network (`network.*.steering`), and each on its own:**
  - **Suggest a higher band** (`suggest`). A client that can use a higher band the AP has this network on, and is near enough for it, is asked to move there, by a BSS transition request that names it.
    - Near enough is its signal where it is, at two looks running: -65 dBm on 2.4 GHz for 5 GHz, -55 for 6 GHz; -60 on 5 GHz for 6 GHz.
    - It is asked once it has been on ten seconds; again two minutes later; and a third time ten minutes after that. Then no more, while it stays joined.
    - A client that takes no such request (802.11v) is not asked.
  - **Hold back 2.4 GHz** (`hold_back`). A client known to use a higher band the AP has this network on is kept off the network's 2.4 GHz for two and a half seconds, from the moment it is heard to come there, by a probe or an authentication. Then 2.4 GHz is open to it again.
    - Only one that is near enough for the higher band, is not on this AP already, and was not held in the last thirty seconds: its next try gets in.
  - **Move weak clients** (`sticky`). A client whose signal stays under `sticky_signal` (-75 dBm unless set) for three looks, while the network has a better place for it, is told to go, with those places named; and put off, for two and a half seconds, if it is still there and still weak ten seconds later. One whose signal is not known then is not put off.
    - A better place is a lower band of the network on this AP, or the network on another AP, as hostapd's neighbour list has it.
    - One that takes no request is put off at once.
    - Once in five minutes a client at most. One that comes straight back is left alone for half an hour: it has nowhere better. Each time it comes back again it is left alone twice as long, up to a day, so that a device heard weakly at the far end of a house is put off a few times in all, and not every half hour for ever.
    - Told to go, it is given no hour: hostapd puts a client off itself when one it was given comes.
- **`band_steering` is the older name of `suggest` (0050),** and is read as it where `suggest` is not set.
- **A client's bands are known by what it said and where it has been:** the operating classes in its association request, and each band it has been let on at this AP, for thirty days, or until the AP starts again.
- **An AP's own test client is not steered.** A client test (0121) is an AP's scan radio joining a network as a client would, at its own AP or another. Steered, it was asked up and said no, as it joins the one network it was sent to, and was counted among the clients that say no; held off 2.4 GHz, it would have joined late, and the test would have said the network is slow to join.
  - **The test says what it is, in its association request:** a vendor element of the OUI Aeolus's beacons use (0073), with a type of its own. Tried on the C-360, 2026-10-11: wpa_supplicant puts it there (`VENDOR_ELEM_ADD`), and hostapd shows it of the client.
  - **An AP's steering that reads it leaves the client alone,** then and whenever it comes again, as long as the client is remembered. Nothing is handed out by the manager, and it holds whichever AP's test it is.
- **A client that will not stay moved is left alone.** One moved up that is back down within five minutes is said to have come back, and is neither asked nor held for half an hour, and twice as long each time after. One sent down for a weak signal is not asked or held back up for half an hour, and twice as long each time, up to a day: a client near enough on 2.4 GHz to be asked up, and weak on 5 GHz once there, would otherwise go up and down every half hour.
- **The AP steers by itself, by a daemon of the agent's** (`aeolus-steer`), which procd runs beside the agent. The manager is in none of it: it says which networks are steered and how, in the AP's config, and hears what came of it.
- **It fails open:**
  - **hostapd does not wait for it.** On every network it steers, at every look, five seconds apart, it tells hostapd to wait for no listener. usteer's own refusals, which are off as OpenWrt ships it, are then not made there. Between usteer's telling hostapd otherwise, which it does as the Wi-Fi starts, and the next look, hostapd waits for both, 100 ms a frame at most, and the daemon answers at once: it waits for nothing itself, hostapd or netifd.
  - **What keeps a client off is hostapd's own ban,** two and a half seconds long, which hostapd lifts by its own timer, the daemon there or not.
  - **It answers hostapd that all is well, whatever happens in it,** and asks hostapd for nothing from inside one of hostapd's notifications, nor waits for anything it asks.
  - **It acts only on what it knows to be so now.** What hostapd said while the daemon was held up is let go, and nothing is acted on, at its start, after such a stop, or on a network it has just begun to listen to, before it has looked at who is on: a client that was coming to join then may be on by now, and to keep it off would put it off. For the same reason a hold decided on a client's authentication is not asked of hostapd once the client is let on, however little later.
  - **A band counts only while hostapd says the network is up there,** asked at every look: one that netifd lists and hostapd has not up, as on a radio that listens for radar before it sends, is no place to ask a client to, and no reason to hold one off 2.4 GHz.
  - **Tried on the C-360, 2026-10-11:** with the daemon stopped where it stood, a client joined in 0.38 s, as on a network that is not steered; with hostapd told to wait for its listeners as well, in 0.70 s; started again after 17 s, the daemon let go what it had heard and asked nothing; killed outright with a client kept off, the client was on four seconds after its first try, and hostapd's list of bans was empty.
- **Steering that asks a client turns on 802.11k and 802.11v for the network,** as before (0050): a request is 802.11v's, and the place it names is the network's neighbour report, which is 802.11k's. Hold back alone turns on neither, and the daemon asks hostapd for no neighbour report of such a network: asked, hostapd begins to say in its beacons that it gives them.
- **usteer steers nothing.** Aeolus keeps its band steering off and its list of SSIDs empty (0050's two options), on every AP that has it: two that ask a client at once would ask it for different things. It still trades neighbour reports between APs, and still hears clients' probes (0118). An AP without it is none the worse: a network that asks for steering is no longer refused there.
- **The daemon is told by one section of the agent's own package** (`aeolus.aeolus_steer`), which lists the networks each way is on for, and sticky's signal for each. With nothing on there is no section, and the daemon is idle. The render check holds the section, usteer's two options, and 802.11k and 802.11v, to the intent (0039).
- **Each asking is followed to its end, and that is what Aeolus shows of whether steering works.**
  - One record an asking: the client, the network, the way, the band it was on, its signal, what was asked (a request, a hold, a putting off), what it answered, what came of it, and after how long.
  - What came of it is where the client then was: moved; said no; stayed; left the AP; joined 2.4 GHz all the same; came straight back; heard well again, and left where it was.
  - A hold made on a probe alone, after which the client joined nowhere on the AP, is no record: it was looking around.
  - The AP reports, for each steered network, the count of what each way asked and of each thing that came of it, since the daemon started, and the last fifty records (`steer` in its state report).
  - Where the config has an AP steer and its daemon has said nothing for a minute, the AP says its steering is not running.
- **In the UI, on the Networks tab:** each network has the three switches, each previewed before it is recorded, with what it does; and under the networks, for each AP and each steered network, how many of those asked by each way then moved. The records are one click down.

## Consequences

- **What a client does when refused is its own.** A ban of two and a half seconds is two or three refusals to a client that keeps trying one AP. wpa_supplicant bears three. A client that gives a network up sooner will show as having joined 2.4 GHz late, or as having come back; that is in the counts, and is the reason to have each way on its own switch.
- **The first frame is answered before the ban is laid,** since hostapd does not wait: a client's first probe is answered, and its WPA2 authentication accepted; it is its association that is refused. One that has associated before the ban is laid is put off by it, as hostapd, asked to keep a client off, first puts it off. One the daemon has heard to be let on by then is left on 2.4 GHz: the ban is not asked, and that is no hold and no record; it is then asked up, if Suggest is on. One let on in the instant between the asking and hostapd's acting on it is still put off, which nothing here can prevent.
- **A client is known by its MAC.** One that scans from another MAC than it joins from is held only from its authentication on.
- **Counts are since the daemon started,** which an update of the agent does. The manager keeps the last report, not a history: a history, and a place for steering beside roaming (0136), is a later step.
- **Not done here:**
  - A client is not asked what it hears (802.11k beacon reports), so a weak client is told of the places hostapd lists, not the one it hears best.
  - hostapd's neighbour lists are usteer's, between APs that reach one another on the LAN usteer uses. An AP outside that, and an AP with no 6 GHz radio naming another AP's 6 GHz network to its clients, wait for the manager to hand each AP its neighbours.
  - Steering between APs for load.
- **An AP with an agent older than this renders usteer steering for a network that asks for it, and no section for the daemon:** the check refuses that config until the agent is the manager's own, which is a minute; an AP kept on an older agent stays refused while such a network reaches it.
