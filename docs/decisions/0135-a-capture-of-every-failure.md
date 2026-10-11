# 0135. A capture of every failure

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff. Every failure should have a pcap, from the radio's ring buffer, linked right beside the client's failure and downloadable for Wireshark; the last 10 kept, so that a client with a bad PSK cannot fill the system. Written up by Claude
- Refines: 0118, 0133

## Context

- **A failed attempt was its steps (0118, 0133),** read from hostapd's log: what the AP says happened. What went over the air, and what the client sent, was not kept.
- **A radio that serves can be listened to without stopping it.** A monitor interface with no flags, beside a radio's networks (`iw phy <phy> interface add <name> type monitor flags none`), hears what the radio heard anyway and what it sent. Tried on the three kinds of AP in the lab, 2026-10-11, each with a join of the C-360's test client: probes, authentication, association, the key messages both ways and the deauthentication, each with its radiotap header. No client dropped.
  - ath11k (the C-360): a harmless `No VIF found for vdev` in the kernel's log as it is made.
  - ath10k-ct (the R7800): its firmware then hands up every frame on the channel, 17,000 in 45 s on a quiet one. With a filter in the kernel, soft interrupts on an idle AP went from 1.4 % of its two processors to 2.7 %.
  - mt76 (the OpenWrt One): its own frames and those sent to it.
- **The key messages cannot be had from the network's own interface:** they go to hostapd over nl80211 (0133).
- **What a radio hands up is not quite a frame, and not quite on time.**
  - ath11k leaves a management frame's check sequence, and four bytes more, behind it, and says neither in the radiotap header: Wireshark calls such a frame malformed.
  - ath11k handed the client's key messages up a second after it heard them, the AP's next try already sent.
  - mt76 says it sent a key message 1 to 12 ms after the client's answer to it has come in.
  - Frames heard carry the radio's own time of them (TSFT); frames sent do not.
- **A monitor interface has the radio's own address unless told another, and Wi-Fi set up afresh takes the addresses of a radio's interfaces as used.** With one there, a radio brought down and up gave each of its networks the next address: every BSSID of the C-360's 5 GHz radio moved on by one, and the test client no longer found its network.
- **A radio's channel can be moved, and the radio set up afresh, with the interface there:** a channel switch (CSA) and `wifi down`, `wifi up` of the 5 GHz radio on each of the three, the interface made with an address of its own. The networks kept their addresses, and captures went on.
- **ucode reads the kernel's time of a frame wrongly on a 32-bit processor** (the R7800): asked for in microseconds, it gives the second and no part of it. Asked for in nanoseconds, it hands the kernel's bytes as they are, on both.
- **A capture holds a client's key handshake,** from which a passphrase can be guessed at off the network; and, on a network that signs clients in by 802.1X, the identity a client gives in the clear.
- **What it costs a radio under load was not measured.** The lab has no client fast enough: the C-360's test client moves 20 Mbit/s.

## Decision

- **`aeolus-journey` has a monitor interface on each radio that serves,** `aemon<n>`, with no flags, and reads it through a filter in the kernel: management frames but beacons, and the messages of a key handshake or a sign-in (EAPOL). Everything else is dropped there.
  - **Its address is its own:** `06:ae:0c` and the last three bytes of the radio's. It sends nothing, so the address is never on the air; and the radio's networks keep theirs when the radio is set up afresh.
  - A radio with a scan radio's work (0081) has none: it serves nothing.
  - One whose interface went, as when its Wi-Fi is set up afresh, has it made again at the next look. As the daemon stops, they go.
- **A radio's frames are kept in a ring:** three minutes of them, 1 MB at the most, in memory.
- **Of an attempt that fails, the client's frames are taken from the ring:** those it sent and those sent to it, from ten seconds before the attempt began, for the probes that led to it, to two after it ended; with the frames of it that crossed the wire side, which the daemon reads already (DHCP, ARP, DNS, a first connection). 600 frames from the radio at the most, and 512 kB, the latest.
- **They are a pcapng file,** of two interfaces: the radio, 802.11 with radiotap headers, and the wire side, Ethernet. Wireshark opens it.
  - A management frame's check sequence, and what a radio left after it, go, where they are the check sequence of what is before them.
  - **A frame the radio heard is put at the radio's own time of it,** set against the calendar's clock by the frame of those minutes that came soonest after it was heard.
  - **A message of the AP's that the client answered is put just before the answer,** where the radio said it had sent it up to 250 ms after the answer came; and what the AP sent before it stays before it. Which answer is which message's is told by the key message's replay counter, or the EAP request's identifier. Such a frame has a comment in the file that says by how much.
  - A frame's time is the kernel's of it, in nanoseconds, read from the kernel's own bytes; where there is none that can be right, when the daemon read it.
- **One capture a client a minute, at the most, on an AP;** and ten of them wait for the manager, the latest, in memory (`/var/run/aeolus/captures`). An AP that restarts loses those not yet sent.
- **The attempt's record says there is one:** `capture: {id, frames}`. The agent sends the capture after the records, two a poll: `POST /v1/ap/captures`.
- **The manager keeps a capture only of an attempt it has as failed,** and holds its file to what an agent writes: one section, its interfaces, and as many frames as it says; 1 MB at the most.
  - **Ten an AP are kept, the latest; and three of one client at the most,** so that a client that fails over and over, as with a wrong passphrase, leaves the other seven for others. Griff asked for the last ten; the three are Claude's, and his to change.
  - They go with the attempts, by age.
- **It is read where the failure is:** on a client's page, beside the attempt's outcome, `Capture`, which gives the file, named for the AP, the client and the time (UTC).
  - `GET /v1/connections/{id}/capture`, by the attempt's ID.
  - **It needs operator on the attempt's AP.** One who may only view it is told there is one. Each reading is in the manager's log, with who.
- **It can be set off:** `system.captures`, on unless set, at any level of Locations. Off, an AP's config has `aeolus.aeolus_journey.captures '0'`, its radios have no monitor interface, and nothing is kept.

## Not done

- **What a monitor interface costs a radio that is busy is not measured.** On the R7800 its firmware hands every frame on the channel to the kernel, to be dropped there. A phone's speed test with captures on and off, on each kind of AP, would say; until then the switch is there.
- **The Assurance tab's list of clients that did not come online** names each client and leads to its page; the capture is one step on, beside the attempt, and not on the tab itself.
- **A frame the AP sent that no answer places** has the time the radio said it was sent, which on mt76 is some milliseconds late.
- **Data frames are not kept,** but the key messages: a capture says how a client failed to come online, not what it did once on. After the keys are in place, management frames are encrypted where the network protects them (802.11w), and are kept as they are.
- **Captures are in the manager's database with everything else:** ten an AP of at most 1 MB each, though one of a few seconds' handshake is 5 kB. A site of hundreds of APs wants a count of them first.
- **A manager taken back to a release before this one will not open its database,** as after any change to what it keeps.
