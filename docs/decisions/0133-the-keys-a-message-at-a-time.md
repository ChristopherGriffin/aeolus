# 0133. The keys, a message at a time

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff. The keys should be the four messages, each on its own, so it is known where one stalls: at the second, a bad PSK. Written up by Claude
- Refines: 0118, 0127, 0130

## Context

- **The keys were one point on the way online (0130).** A client that did not get its keys stopped "at the keys", and no more was known but hostapd's one word for a wrong passphrase.
- **The key handshake is four messages.** The AP's first (M1); the client's answer (M2), made with the passphrase, which the AP checks; the AP's third (M3), sent once that checked out; and the client's last (M4).
  - A wrong passphrase is an M2 that comes and does not check out.
  - A client that does not hear the AP, or has gone, is an M2 that never comes.
  - Those are different faults, and looked the same.
- **hostapd says each message, at debug.** On the C-360, 2026-10-11, with `logger_syslog_level` at 1, joins from its scan radio:
  - Right passphrase: `sending 1/4 msg of 4-Way Handshake`, `received EAPOL-Key frame (2/4 Pairwise)` 15 ms later, `sending 3/4` at once, `received EAPOL-Key frame (4/4 Pairwise)` 4 ms after that, and the keys in place 35 ms later.
  - Wrong passphrase: M1, M2 15 ms later, `invalid MIC in msg 2/4`, `AP-STA-POSSIBLE-PSK-MISMATCH`, `EAPOL-Key timeout` a second after M1, and M1 again; four times, then `PTKSTART: Retry limit 4 reached`, and five seconds on `deauthenticated due to local deauth request`.
  - At its usual level, info, it says none of the messages.
- **Of WPA3 it says less still.** SAE proves the passphrase in the authentication, before the association.
  - With the right one hostapd says the client's commit and confirm, and `MLME-AUTHENTICATE.indication`, all at debug. It never says `authenticated` of a WPA3 client, as it does of one that came by open authentication: at info a WPA3 join begins at `associated`.
  - With a wrong one it says the commit and the confirm at debug, and nothing more. The client is refused (status 15), and the AP's log at info has not one line of it: no attempt was recorded.
- **A client that goes of its own accord is said at debug too:** `IEEE 802.11: deauthenticated`, with no reason. At info, one that had not been let on yet left no line.
- **The level can be changed while hostapd runs.** `SET logger_syslog_level 1` on a network's control socket is taken at once: no radio restarts, no client drops. hostapd goes back to its config's level when the config is loaded again. It answers to a socket under `/var/run/aeolus`, though it runs in a jail as its own user.
- **The messages cannot be seen any other way.** They go to hostapd over nl80211, not through the network interface, so a packet socket sees none. The test client's own wpa_supplicant has its words for them compiled out.
- **At debug hostapd says much else:** about three times as many lines of a join; and on a network that signs clients in by 802.1X, each client's identity, and every word with the RADIUS server.

## Decision

- **`aeolus-journey` asks hostapd for its debug lines,** on each network's control socket, at every look (10 s), as hostapd forgets it when its config is loaded again. As the daemon stops, those it asked are put back to info.
  - **Not on a network that signs clients in by 802.1X:** there the lines would put each client's identity into the AP's log, and into a log server where the AP sends its log to one. Such a network's keys stay one point.
  - **Not on a radio whose `log_level` the AP's own config sets:** that is someone's choice.
- **A line hostapd says only at debug is read where it is known, and is otherwise nothing.** Which are debug is told by the line's priority.
  - Known: the four messages; the AP giving the handshake up; WPA3's commit and confirm; that a client is authenticated; a client's own leaving; and an answer of the AP's the client did not acknowledge.
- **A step that is of one message names it:** `k`, `m1` to `m4`, beside its stage, `key`.
  - `message 1 of 4 sent to it`, and `, try 2` where it is sent again; `message 2 of 4 came from it`; and so the third and the fourth.
  - hostapd's word for a wrong passphrase is of the second.
- **The AP giving the handshake up is a failure at the keys:** `no answer the AP could use to the first message of the key handshake, sent 4 times`. Before, the client was said to have left, as hostapd then sends it off: an AP whose own messages did not go out would have shown no failure at all. A client that says it is going has left, as before, and failed nothing.
- **WPA3's authentication is steps of `auth`:** the client's commit, its confirm, and that it is authenticated. An attempt that has a confirm and is not authenticated failed there: `its WPA3 confirm did not check out: a wrong passphrase`.
- **Associated, a client is authenticated, said or not;** and let on, what went wrong before in the same attempt was got over, as a wrong passphrase and then the right one: such an attempt did not fail.
- **Its keys in place, a client is let on, said or not** (added 2026-10-11, v0.83.1). Of a client it still holds as on, as one that went to another AP without a word and is back within the minutes hostapd remembers it, hostapd says that the key handshake completed and not that the client is connected, which to it the client never ceased to be. Such a return was recorded as failed at the keys, "it was never let on". Seen on the C-360 with a network that protects management frames, where the AP first asks the client it remembers to prove itself (SA Query), for a second.
- **An attempt begins at a client's authentication or its association, and at nothing else** (added 2026-10-11, v0.83.1). A client long on has its keys made again, or signs in again, by 802.1X every hour unless set otherwise: hostapd says the key messages, or the sign-in, with no authentication and no association before them. Each was taken as a client coming online, and recorded as one that failed, "it was never let on". Three lines more begin one, each all hostapd says of what it is of: the two it has for a device refused at its authentication for the RADIUS server's answer (a VLAN not offered, a passphrase not well formed), the one for a WPA3 client refused for a password identifier the AP has not, and its word for a client connected, which it says only as a client goes from not on to on. And one that is signing in is waited for 40 s with nothing said, not 20: hostapd goes over to a second RADIUS server only after 21 s without the first's answer, and such a client got on with a failed attempt for its record.
- **A step from the log is timed by when the log took its line in,** which `logread -t` says to the millisecond, and no longer by when the line reached the daemon: the lines come a few at a time, and the messages are milliseconds apart. A line that says no time, or one that cannot be right, as with the clock just set, has the time it came at.
- **The manager takes the keys apart as it reads an attempt (0130):** the point has `parts`, the four messages.
  - A message went well where the handshake went on from it.
  - The first that did not is where it stopped: `wrong passphrase` where the answer did not check out; `no answer` where a message waited for did not come. Bad where the attempt failed at the keys; only begun where the client is known just to have gone.
  - Each has how long it took after the one before it: the AP's first after the association, the client's answer after the AP's first as last sent.
  - With no message said, the keys have no parts, and are drawn as before.
  - The keys' own time is then from the AP's first message to the keys in place; without the messages it is from the association, as before. The two differ by the millisecond or two the AP takes to begin.
- **What many attempts come to has the same parts,** over the attempts that said them: at each message how many passed it, how long it typically took, and how many stopped there.
- **The keys are drawn with the four under them,** in a client's attempt and in a network's way online: `M1 2 ms`, `M2 14 ms`, `M3 0 ms`, `M4 4 ms`; the one it stopped at in red, `M2 wrong passphrase`, or amber, `M2 no answer`.
- **A step's `k` the manager does not know is left out, and the step kept.** A record from an agent of this kind is refused by a manager from before it, which does not know the field: the manager is updated first, as it always is.

## Consequences

- **A wrong passphrase and a client that does not answer are told apart,** on WPA2 at M2, and on WPA3 at authentication.
- **More attempts are failures than were:**
  - WPA3 clients with a wrong passphrase are counted among those that did not get on (0127), where they were not seen at all.
  - A client the AP gave up on failed at the keys, where it was said to have left.
- **A good WPA3 join has its authentication,** where it had none: its way began at the association.
- **hostapd logs more:** about 20 lines of a join where it logged 7, in the AP's own log, and to a log server where the AP sends its log to one. On a busy AP the log's memory (128 KiB unless set) goes back less far.
  - There is no setting to turn it off but the radio's own `log_level`: set, the daemon leaves that radio alone.
- **A network that signs clients in by 802.1X has no messages,** and its WPA3 authentication is not said either: what is wrong there is the sign-in, which hostapd says at info.
- **For up to 10 s after hostapd loads its config again, a handshake is recorded as one step,** as before.
- **A fast roam (802.11r) has no key handshake:** its keys come with the roam, and it has no parts.
- **The times are the log's, to the millisecond.** Tried on the C-360, 2026-10-11, from `/tmp` with its own daemon stopped:
  - WPA2: hostapd's log had the association, M1, M2, M3, M4 and the keys in place at 16, 16, 31, 31, 35 and 70 ms after the client was authenticated; the record has 17, 17, 31, 31, 35 and 71.
  - WPA3: the log had the confirm, authenticated, the association, M1, M2, M3, M4 and let on at 31, 33, 46, 46, 62, 62, 66 and 67 ms after the commit; the record has the same.
  - A first cut of this divided whole numbers, and timed nothing closer than a second: the messages then came out together, as the lines reached the daemon.
- **Daemon stopped, hostapd is at info again:** a join after it logged no debug line.
- **Not in this:** the same of a client test's own way (0121), which would take it from the record of the AP it joined; the group key's handshake; why a WPA3 commit was refused; and a client's attempts begun by a rekey or a reauthentication, which are recorded as before.
