# 0078. Per-user keys at scale, with RADIUS as an option

- Status: Proposed
- Date: 2026-10-06
- Proposed by: Griff: stay with local keys for now, a revocable key per user is enough; but with a thousand residents in a building, build RADIUS as an option for larger deployments. Written up by Claude
- Refines: 0014, 0070

## Context

Per-user keys (0070) live on the APs: the manager sends each AP the whole key set for the networks it broadcasts, as `wifi-station` sections, and hostapd loads them. A key is at most one of 4,096 on a network. This is simple and has no runtime dependency: an AP keeps working through a manager outage, and a key change drops no one.

For an MDU or hospitality site — a building of residents or guests, each with their own revocable key — the question is how far that goes.

### The load check, 2026-10-06

On the office AP (OpenWrt One, the stronger of the two), keys were added to `tedt` as the agent does (the UCI library, then `ubus call network reload`), measured, and removed. The pumphouse R7800 was left alone: it is the weaker CPU and carries the well and heater IoT.

| Keys | Stage | Commit | Memory used | Radios | Clients |
| --- | --- | --- | --- | --- | --- |
| 2 (baseline) | — | — | 75 MB | — | — |
| 500 | 0.4 s | 0.1 s | 87 MB | no restart | no drop |
| 2,000 | 2.5 s | 1.4 s | 112 MB | no restart | held |
| 4,096 | 11 s | 9.4 s | 122 MB | no restart | brief blip, recovered |

- **Capacity is not the ceiling.** At 4,096 keys, 841 MB was still free, about 11 KB a key. The radios never restarted; hostapd reloaded its settings in place. This OpenWrt passes hostapd its config inline, so the keys load into memory, not a growing PSK file.
- **Apply time is the ceiling, and it grows super-linearly.** The agent rewrites the whole key set on every change (0070's `write_keys`), and the UCI library slows as the config grows: 4,096 keys took about 20 seconds of agent work. Adding or revoking one key on a large network pays that whole cost.
- **Very large sets aren't as seamless as small ones.** At 500 the reload was clean; at 4,096 a large config change briefly dropped clients, which reconnected within seconds.
- **Not yet measured:** join time with thousands of keys. Without a key bound to a device's MAC, hostapd tries each key in turn as a client joins, so join time grows with the key count. This is the number that matters most for a dense building, and it needs a test key and a client.

### What hostapd offers

hostapd can ask a RADIUS server for a station's key *during* the four-way handshake, so a client never sees a failure: `wpa_psk_radius=3`, "ask RADIUS server during 4-way handshake if there is no locally configured PSK/passphrase for the STA" (hostapd.conf). It sends the station's handshake message and its own nonce as FreeRADIUS vendor attributes, and the server answers with the matching key as `Tunnel-Password`, optionally a VLAN and a session timeout.

Both lab APs run `wpad-basic-mbedtls`, whose build has `wpa_psk_radius`. OpenWrt passes the options through `auth_server_*` and `hostapd_bss_options`.

**The server that matches the key is the part to buy, not build.** FreeRADIUS has a `dpsk` module made for exactly this hostapd mode; it verifies which key a station used. Aeolus will not hand-roll the handshake cryptography.

## Decision

- **Local keys stay the default** (0070), unchanged. A revocable key per user, up to 4,096 a network, no runtime dependency. This is right for a home, a small building, or any site that wants to keep working through a manager outage.
- **A network may instead be marked to look keys up at join time,** for large deployments. This is a new, optional mode, off unless chosen:
  - The AP runs hostapd with `wpa_psk_radius=3` and an `auth_server` pointing at Aeolus.
  - When a client joins with a key the AP doesn't hold, hostapd asks Aeolus during the handshake. Aeolus (through FreeRADIUS's `dpsk` module, or an equivalent) finds the key and answers. The client joins on the first try.
  - The AP then caches that key locally, bound to the device's MAC, so the device's next joins are local, with no lookup. The cache is bounded (the 4,096 local limit becomes a working set, oldest out first), and can be pushed to the building's other APs so moving around stays instant.
- **Aeolus stays the one source of truth for keys.** Whichever mode a network is in, keys are added, rotated and revoked the same way (0070): a change in the log, sealed passphrase, reaching the APs or the lookup server in seconds. The RADIUS mode is how keys are *delivered to clients*, not a second place they live.
- **The trade RADIUS makes:** a device's *first* join at an AP needs the lookup server reachable. Already-cached devices keep working through an outage; brand-new joins don't, until it is back. So RADIUS buys scale at the cost of the local mode's full offline independence (0069), and a site chooses per network which it wants.

## Consequences

- **A thousand residents fit either way.** 1,000 local keys apply in a few seconds and use little memory; RADIUS is not needed for capacity. It earns its place where the per-change apply cost or the join time of a large local set becomes the problem, which the join-time test will show.
- **The per-change apply cost should be cut regardless of RADIUS.** Rewriting the whole key set for one change is O(n²) in the UCI library. An incremental write — add or remove just the one section — would make local keys scale further on their own, and is the cheaper first step. A separate decision.
- **RADIUS is a real build:** a RADIUS responder beside the manager (FreeRADIUS with `dpsk`, or a small service that speaks the same protocol), a way for Aeolus to feed it the key set, the AP-side config and the local cache, and a health signal for when the lookup path is down. It is scoped here, not built.
- **WPA2 only,** as per-user keys already are (0070). WPA3 per-user keys are still out (0070's "Not now").

## Open

- The join-time measurement with a large local key set, which decides where RADIUS actually starts to pay.
- Incremental key writes on the AP, to push the local ceiling up first.
- Whether the lookup server is FreeRADIUS with `dpsk`, or a small responder Aeolus runs itself speaking RADIUS to hostapd.
- Per-building versus central key VLANs (0070's roaming note): a cached key carries its VLAN, and buildings with different egresses keep their own.
