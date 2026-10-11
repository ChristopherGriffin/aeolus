# 0128. A client test over the wire

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Claude, for the Service Assurance Griff asked for (0123): two of the lab's three APs have no scan radio, and could test nothing
- Refines: 0060, 0064, 0121, 0123

## Context

- **A client test (0121) needs a scan radio,** which most APs have not. On a site of such APs, assurance has only what its clients found (0127): nothing until clients come, and nothing on a network no client is on.
- **Most of what a client needs once it is on is not the air's:** an address from DHCP, a gateway that answers, a DNS server that answers. Those come over the AP's uplink, on the network's VLAN or through its tunnel, and can be asked for there.
- **A site is often built before its clients arrive.** Whether each VLAN reaches each AP, and has a DHCP server behind it, is what an installer wants to know before the doors open.
- **The prober already sends frames of its own on a segment** (0060, 0064): DHCP and ARP from a MAC made for the segment, on the uplink's port with the VLAN's tag, or on a tunnel's device, read back by a packet socket with a filter. It works on every AP Aeolus runs on.
- **An address on the AP for the client's network would be a way in:** the AP's own services, reachable from it. The scan radio's test takes one, on an interface nothing else uses (0121). Over the wire there is no need.

## Decision

- **A client test can be asked for over the wire:** the action `client-test` with `band` `wire`. It is no band; it is where a band would be said.
- **Any AP can make one,** a scan radio or none, whose agent says it can (`wire_tests` in its report).
- **It is made from the AP itself,** of one of the networks it offers, however that is secured: it joins nothing.
- **Where:** on the wire the network's clients' frames take from this AP.
  - **A network on a VLAN of the uplink's:** on the uplink's port the AP's traffic takes now (0096), with the VLAN's tag or without, as the port carries it (0064).
  - **A network on a tunnel (0054):** on the tunnel's own device.
  - **A network with a fallback (0061):** on whichever of its transports is in its bridge now.
  - **Anything else is not tested,** and the result says why.
- **From a MAC of the test's own:** `0e`, the last three bytes of the AP's MAC, and the VLAN's or the tunnel's number in four digits. Not one the prober uses (`02`, `06`, `0a`), so a test never gives back the prober's lease.
- **What it does, each step timed:**
  - **DHCP,** as a client: discover, offer, request, acknowledgement. It asks for its mask, router, DNS servers and lease, and to be answered by broadcast, having no address yet.
  - **The gateway:** an ARP request for it, from the address given. Its answer, and how long it took, is the gateway being there. No ping: the test has no IP of its own to be pinged back at but its frames.
  - **A name looked up,** at the first DNS server the lease named: by ARP where that is on the same subnet, through the gateway where not. The datagram has its UDP checksum.
  - **The address is given back** (DHCP release).
- **It answers ARP for the address while it holds it,** so that what answers it can find it.
- **The AP takes no address and makes no interface.** The test is frames sent on a packet socket, and a filter that keeps only frames to the test's MAC, and of the broadcasts ARP and DHCP to a client. The clients' traffic on the same port never reaches it.
- **Its result is a client test's** (0121), with `band` `wire`; `wire`, where it was made; no link, no BSSID; and the gateway's `arp_ms`. It passes and fails by the same rules: no address, a lease with no gateway or no DNS server, a gateway or a DNS server that does not answer.
- **On a schedule (0123), where a folder says so:** `assurance.tests.wire`, off unless set. Then every AP below is a tester, and tests each network it offers over the wire, in its turn with what it tests through the air.
  - **A network that fails twice running is an alert on that AP,** as for the air, and says it was the wire.
- **By hand, from the AP's page:** Client tests offers "Over the wire" beside the bands, and is there on an AP with no scan radio too.

## Consequences

- **An AP with no scan radio can say whether a network works behind it.** A site can be checked VLAN by VLAN before a client arrives.
- **It says nothing of the air:** not whether the network can be heard, joined, or signed in to. Passing over the wire and failing through the air is the radio's side; failing both is the network's.
- **It does not go through the AP's own bridge.** A network whose Wi-Fi is not bridged to its VLAN on this AP passes over the wire all the same. The prober's own checks are for that (0064).
- **Its address is new each time,** as on a device's first visit, since it gives it back. A DHCP server that looks whether an address is free before it offers it shows that wait here, every time, where a returning client would not see it.
- **A DHCP pool is not filled by tests:** one address for a few seconds, then free again. An AP that is stopped mid-test leaves a lease to run out.
- **A test takes seconds, and no radio is off its work.** Two to three in the lab, two of them the DHCP server's.
- **No speed is measured over the wire:** there is no TCP in it.
- **The switch learns one more MAC on the AP's port for each VLAN tested,** as it does for every client.
- **A network on the uplink with no VLAN at all is not tested yet:** the AP's own address is there.

## Tried

- **On the C-360, 2026-10-11, by hand from `/tmp`:**
  - **Aeolus Lab, VLAN 20 on eth1:** an address in 2.0 s, the gateway's ARP answer in 1.6 ms, and a name from the DNS server on the same subnet in 35 ms.
  - **Aeolus-50, through its tunnel:** an address in 2.0 s, the gateway in 1.7 ms. Its DNS servers are on the internet, and did not answer until the datagram had its UDP checksum: something on the way drops one without.
  - **The 2.0 s is the DHCP server's.** With no second discover sent, the offer still came at 2.0 s: it looks whether the address is free first. Through the air the same server answers the scan radio in 0.3 s, as it keeps that lease.

- **On the office AP (an OpenWrt One) and the pumphouse AP (an R7800), by hand, the same day:** both networks passed on each, on VLAN 20 of `eth0` and of `wan`, a switch's port, and through the tunnel. Each showed the same 2.0 s for an address, and a gateway answering ARP in 2 to 5 ms.

## Not done here

- **A network on the uplink with no VLAN.**
- **Reaching something past DNS:** a connection to a server on the premises or the internet, which needs TCP.
- **IPv6:** a router advertisement, and an address from it.
- **Testing through the AP's own bridge,** from a port of it, which would say the Wi-Fi's frames reach the wire.
