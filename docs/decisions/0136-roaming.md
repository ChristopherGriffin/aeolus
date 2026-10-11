# 0136. Roaming

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff. The Overview needs a roaming dashboard: the average time clients took to roam, and the roaming failures, with what there is to know of each; each roam by its kind, a hard roam, PMK caching or OKC, or 802.11r, the time it took, and whether it was good or bad. Written up by Claude
- Refines: 0102, 0118, 0133

## Context

- **A roam was an attempt like any other (0118).** The AP a client came to recorded its authentication, association and keys; nothing said it had been on the network a moment before, or how much of coming online it had to do again.
- **hostapd says how a client came.** On the C-360, 2026-10-11, its scan radio joined a network on the office AP and was sent to the C-360 and back (`ROAM`, wpa_supplicant), on three networks:
  - **A passphrase, no fast transition:** `authentication OK (open system)`, `associated`, `MLME-REASSOCIATE.indication`, the four key messages, `AP-STA-CONNECTED … auth_alg=open`. 30 to 280 ms from its first frame to connected; the keys were in the radio 65 to 250 ms after the fourth message.
  - **A fast transition (802.11r):** `authentication OK (FT)`, `MLME-AUTHENTICATE.indication(…, FT)`, `associated`, `AP-STA-CONNECTED … auth_alg=ft`, `MLME-REASSOCIATE.indication`, `FT authentication already completed - do not start 4-way handshake`. 17 ms.
  - **WPA3, back at an AP it had been on:** open authentication, `PMKID found from PMKSA cache`, `MLME-REASSOCIATE.indication`, the four messages. The AP still held the client as on, and with protected management frames asked the one it remembered to prove itself first: `association SA Query attempt 1` to `5`, `SA Query timed out`, a second in all. 1.2 s.
  - `MLME-REASSOCIATE.indication` is said of a client that asked as one already on the network, and `MLME-ASSOCIATE.indication` of one that did not. Both at debug.
  - `auth_alg=` is in the line hostapd says at any level.
- **Of a client it still holds as on, hostapd does not say it is connected.** After the four messages it says they completed; after a fast transition, nothing at all past `associated`. Such returns were recorded as failed (fixed for the four messages in v0.83.1; for the fast transition here).
- **hostapd says `authenticated`, at any level, of open authentication alone.** Of a fast transition and of WPA3 it says the authentication at debug. Where it does not log at debug, an attempt by either begins, in its log, at `associated`.
- **Its words for a key made from a sign-in at another AP are two lines:** `OKC match for PMKID`, and then `PMKID found from PMKSA cache`, as of a key of its own. From its source; not seen.
- **On a network that signs clients in by 802.1X hostapd is not asked for its debug lines (0133).** Of a roam there it says `associated`, the sign-in where there is one, and `auth_alg=`.
- **On a network that signs clients in, each roam is a sign-in again, and nothing says it is a roam.** The same radio, with a user made for the trial, between the same two APs: `authenticated`, `associated`, `CTRL-EVENT-EAP-STARTED`, `CTRL-EVENT-EAP-SUCCESS2`, the key handshake completed, `AP-STA-CONNECTED … auth_alg=open`, `authenticated - EAP type: 25 (PEAP)`. 91 ms at the AP, the sign-in resumed from the one before. The client offered a key made from its first sign-in (OKC), and the AP signed it in again all the same: OpenWrt has hostapd keep no keys for such a network unless told to. hostapd's words for a key from before on such a network (`PMKID found from OKC`) are from its source, and were not seen.
- **What the AP sees is the end of a roam.** The client has left one AP, changed channel, and looked for the next before its first frame arrives: that part of the gap no AP sees.

## Decision

- **An attempt's record says how the client came,** `how`, where anything is known of it:
  - `re`: it asked as one already on the network, a reassociation;
  - `alg`: its authentication, as hostapd names it: `ft`, `sae`, `open`;
  - `pmk`: a key from before, `pmksa` (this AP held it) or `okc` (made from its sign-in at another AP);
  - `saq`: how many times the AP asked a client it still held to prove itself;
  - `on_ms`: from the first line its AP has of it to its being let on;
  - `began`: `assoc`, where that first line was its association and not its authentication, so that the time is short of what it took.
- **An authentication is a new try:** what was known of the one before it in the same attempt, as that it was a fast transition, is not said of this one.
- **The manager takes of `how` what it can:** a word it does not know is passed over, a value it cannot take costs `how` and not the attempt.
- **The lines that say so are steps of the attempt,** in words: that it asked as one already on the network; that the AP still held it and asked for proof, and that none came; that its key from before was taken; that its keys came with the fast transition.
- **Associated after a fast transition, a client is let on,** said or not.
- **Associated, and nothing said after for twenty seconds, a client is let on.** A client that is to have keys has a handshake, which hostapd says completed, or gives up on and sends the client off, and of one that signs in it says the sign-in began: each at any level. Nothing at all is a client whose keys came with it, to an AP that still held it, where hostapd does not log at debug. Such an attempt was a failure, "it was never let on".
- **What hostapd says beside an attempt begins none:** a reassociation, the asking for proof, a key from before.
- **A roam is an attempt by a client that asked as one already on the network, or came by a fast transition,** to an AP or a radio other than the one it was last let on at. One that asks again on the radio it is on is no roam.
  - Where it came from is where it was last let on, on any AP, however long before: read from its own attempts, an agent's from before this among them, which say only how each ended. A client with no attempt before has no such place, and its roam is one all the same.
- **Its kind is how much it did again,** the least first:
  - `ft`, 802.11r: its keys came with it;
  - `okc` and `pmksa`: a key from before, and the four key messages;
  - `cached`: one of those two, on a network that signs clients in, where hostapd does not say which: a reassociation with no sign-in;
  - `keys`: a network with a passphrase, and the four messages, which is all there is to do;
  - `full`: the sign-in again, or WPA3's exchange.
- **It is timed at the AP the client comes to,** from its authentication there, as hostapd's log has it, to its being let on.
  - **One whose AP has no line of its authentication is let on and not timed** (`began: assoc`): a fast transition on a network that signs clients in, where hostapd is not asked for its debug lines (0133), or on a radio whose `log_level` is someone's own. The millisecond from its association to its being connected is not what it took, and is in no typical time and never slow.
- **It went well, slowly, or not at all:** let on within 250 ms; let on after more; not let on, which is a failure; or given up by the client before anything was refused, which is counted apart.
  - 250 ms is Claude's, for a call in progress, and Griff's to change.
- **`GET /v1/roaming`** is the roams of a time on the APs the caller may view: how many, failed and slow; the middle of their times; the same a kind at a time, with the least and the most; the latest fifty; and those that went badly, each with what its AP said: the reason it failed, or that the AP made it wait for proof.
  - The attempts read are those of the APs asked about, 5000 at the most, the latest; where a client was before is read from its own attempts on every AP. What is said is of the APs the caller may view, and a place it may not view is not named.
  - The tests' own clients are left out (0121).
- **On the Overview, a tile:** how long a roam typically took, with how many there were and how many failed or were slow; amber where any did, red where three or more failed and they are a tenth of all. It opens **a panel, folded until asked for:** the kinds, those that went badly, and the latest. A client's name opens its page.
- **On a client's page, an attempt that was a roam says so,** with its kind and time: a roam as the Overview counts one, and not every reassociation.
- **The MCP adapter has `get_roaming`.**

## Not done

- **Where a client came from is not read from its own frame.** A reassociation names the AP it leaves; with captures (0135) the frame is at hand, and the place could be exact, and known for the first roam too.
- **A roam on a network that signs clients in is known as one only by its fast transition,** or where hostapd logs at debug. A reassociation there with a key from before, or with a sign-in again, is an attempt like any other. The frame would say.
- **A fast transition back to an AP that still held the client, where hostapd does not log at debug, is let on and is no roam:** nothing says how it came.
- **A manager taken back to a release before this one refuses each record that says how the client came,** as it does any with a field it does not know: for the minute or two until its agents are taken back with it.
- **The gap a client hears is longer than what is timed:** leaving, changing channel and looking are the client's own.
- **No alert on roams that fail.**
- **What is slow is one figure for every kind.** A fast transition past 50 ms is slow for what it is.
