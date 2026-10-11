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
- **hostapd says each message, at debug.** On the C-360, 2026-10-11, with `logger_syslog_level` at 1, a join from its scan radio:
  - Right passphrase: `sending 1/4 msg of 4-Way Handshake`, `received EAPOL-Key frame (2/4 Pairwise)` 14 ms later, `sending 3/4` at once, `received EAPOL-Key frame (4/4 Pairwise)` 4 ms after that, and the keys in place 63 ms later.
  - Wrong passphrase: M1, M2 15 ms later, `invalid MIC in msg 2/4`, `AP-STA-POSSIBLE-PSK-MISMATCH`, `EAPOL-Key timeout` a second after M1, and M1 again; four times, then `PTKSTART: Retry limit 4 reached`.
  - At its usual level, info, it says none of the messages.
- **WPA3 with a wrong passphrase left no trace at all.** SAE proves the passphrase in the authentication, before the association. hostapd says the client's commit and confirm at debug only, and nothing after a confirm it does not take: the client was refused (status 15), and the AP's log at info had not one line of it. So no attempt was recorded.
- **The level can be changed while hostapd runs.** `SET logger_syslog_level 1` on a network's control socket is taken at once: no radio restarts, no client drops. hostapd goes back to its config's level when the config is loaded again.
- **The messages cannot be seen any other way.** They go to hostapd over nl80211, not through the network interface, so a packet socket sees none. The test client's own wpa_supplicant has its words for them compiled out.

## Decision

- **`aeolus-journey` asks hostapd for its debug lines,** on each network's control socket, at every look (10 s), as hostapd forgets it when its config is loaded again. A radio whose `log_level` the AP's own config has at debug or below is left as it is. As the daemon stops, each is put back to its radio's level.
- **A line hostapd says only at debug is read for what is known of it, and is otherwise nothing.** There are about three times as many lines of a join at debug. Which are debug is told by the line's priority.
- **A step from the log is timed by when the log took its line in,** which `logread -t` says to the millisecond, and no longer by when the line reached the daemon: the lines come a few at a time, and the messages are milliseconds apart. Tried without, the four came out at 14, 30, 30 and 30 ms where the log had 14, 29, 29 and 33. A time that cannot be right, as with the clock just set, is passed over for when the line came.
- **A step that is of one message names it:** `k`, `m1` to `m4`, beside its stage, `key`.
  - `message 1 of 4 sent to it`, and `, try 2` where it is sent again; `message 2 of 4 came from it`; and so the third and the fourth.
  - hostapd's word for a wrong passphrase is of the second.
  - Its giving up is a step of the keys, and why is kept for the reason: `it gave no answer the AP could use to message 1 of the key handshake, sent 4 times`.
- **WPA3's authentication is two steps of `auth`:** the client's commit, and its confirm. An attempt that has a confirm and is never authenticated failed at authentication: `its WPA3 confirm did not check out: a wrong passphrase`.
- **Whether an attempt failed or the client left is as before.** One sent off after the AP gave up waiting is still one that left, as it may have gone: its reason now says what the AP waited for.
- **The manager takes the keys apart as it reads an attempt (0130):** the point keeps its state and its time, and has `parts`, the four messages.
  - A message went well where the handshake went on from it.
  - The first that did not is where it stopped: `wrong passphrase` where the answer did not check out; `no answer` where a message waited for did not come. Bad where the attempt failed at the keys; only begun where the client is known just to have gone.
  - Each has how long it took after the one before it: the AP's first after the association, the client's answer after the AP's first as last sent.
  - With no message said, the keys have no parts, and are drawn as before.
- **What many attempts come to has the same parts,** over the attempts that said them: at each message how many passed it, how long it typically took, and how many stopped there.
- **The keys are drawn with the four under them,** in a client's attempt and in a network's way online: `M1 2 ms`, `M2 14 ms`, `M3 0 ms`, `M4 4 ms`; the one it stopped at in red, `M2 wrong passphrase`, or amber, `M2 no answer`.
- **A step's `k` the manager does not know is left out, and the step kept.** A record from an agent of this kind is refused by a manager from before it, which does not know the field: the manager is updated first, as it always is.

## Consequences

- **A wrong passphrase and a client that does not answer are told apart,** on WPA2 at M2, and on WPA3 at authentication.
- **WPA3 clients with a wrong passphrase are counted** among those that did not get on (0127), where they were not seen at all. A network that has such clients shows more failures than it did.
- **hostapd logs more:** about 20 lines of a join where it logged 7, in the AP's own log, and to a log server where the AP sends its log to one. On a busy AP the log's memory (128 KiB unless set) goes back less far.
  - There is no setting to turn it off. If one is wanted, it is the daemon's asking that it turns off: the rest reads what is there.
- **For up to 10 s after hostapd loads its config again, a handshake is recorded as one step,** as before.
- **A fast roam (802.11r) has no key handshake:** its keys come with the roam, and it has no parts.
- **The times are the log's, to the millisecond.** Tried on the C-360, 2026-10-11, from `/tmp` beside its own daemon stopped: hostapd's log had M1, M2, M3 and M4 at 12, 25, 26 and 36 ms after the authentication, and the record 13, 27, 27 and 38.
- **Not in this:** the same of a client test's own way (0121), which would take it from the record of the AP it joined; the group key's handshake; and why a WPA3 commit was refused.
