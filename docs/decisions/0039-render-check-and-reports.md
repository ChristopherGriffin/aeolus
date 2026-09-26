# 0039. The render check, AP reports and the conditions store

- Status: Accepted
- Date: 2026-09-26
- Proposed by: Claude, while building M4 part 3; the check verifies intent, retention is adjustable, and last-seen is updated on every request, as Griff decided; accepted by Griff with the part 3b merge
- Refines: 0008, 0009, 0020, 0022, 0027, 0029

## Decision

- **The render check (0008).** Before applying a config, the AP sends the UCI it rendered (`uci export` text) and the version it rendered, to `POST /v1/ap/render`. The manager answers:
  - `stale` when that is no longer the AP's version: poll again;
  - `refused`, with the problems, when the UCI is malformed or does not carry the intent, or when the config itself is held (0029);
  - `ok`: apply it.
- **What "carries the intent" means in v1.** Aeolus names what it renders, so the check knows what to look at and leaves everything else alone.
  - **Radios.** Each `wifi-device` is matched to a band by its `band` option.
    - `enabled: false` becomes `disabled '1'`.
    - `channel` becomes `channel`.
    - `width` becomes the number in `htmode` (`HE80` for 80).
    - `power` becomes `txpower`; `auto` means no `txpower`.
    - `country` goes on every radio.
    - A band the AP has no radio for is skipped.
  - **Networks.**
    - Each enabled network is a `wifi-iface` named `aeolus_<network>_<radio>` (dashes become underscores), on every radio whose band the network uses, or on every radio when `bands` is not set. Names use the radio, not the band, because some APs have two radios on one band.
    - Each has `device` (that radio), `mode 'ap'` and `ssid`.
    - `encryption`: `none` for open, `owe`, `psk2` for wpa2-psk, `sae` for wpa3-sae, `sae-mixed` for wpa2-wpa3.
    - `key` is the passphrase where there is one.
    - `hidden`, `isolate`, `ieee80211r`, `ieee80211k` and `bss_transition` carry the flags.
    - `network` names interfaces that exist.
    - An `aeolus_` interface that no network calls for is stale, and is refused.
  - **Transports.**
    - Every transport a network keeps (after 0037's filtering) has its path in the network config.
    - A VLAN is an 802.1Q device (`vid`) or bridge VLAN (`vlan`) with that ID.
    - A VXLAN is a `vxlan` interface with the concentrator's `peeraddr`, `port` and `mtu` and the VNI as `vid`.
    - Both transports are rendered, because the AP switches between them itself (0020, 0022).
  - **System.** `tz` becomes `zonename`, `ntp` becomes the `ntp` server list, and `syslog` becomes `log_ip` and `log_port`.
  - **Checked with the agent in M5.** These depend on the device's port layout or on the agent's own settings, which the agent will define:
    - ports and the management interface;
    - SSH keys, the poll interval and rate limits;
    - HA timing (`ha`, `failback`, `holddown`).
  - **Every schema field is listed as checked or deferred** in `internal/rendercheck`. A test fails when a field is in neither list, so no field goes unchecked without anyone deciding it.
- **Rendered UCI is not kept** (superseded by 0041: it is kept with secrets blanked). It carries passphrases in plain text (0027). The manager records its SHA-256, the version, the result and the problems.
- **Reports.**
  - `POST /v1/ap/applied`: after each apply attempt, the version and UCI hash applied, whether it worked, and the error if not.
    - An apply whose hash no `ok` check covers is recorded as unchecked.
  - `POST /v1/ap/state`: periodically. It carries:
    - the version the AP runs;
    - uptime and OpenWrt version;
    - its radios;
    - the VLANs it sees on its uplink (0020);
    - each network's active transport and the health of both (0020).
- **Last seen.** Every authenticated AP request updates the AP's last-seen time and address.
- **Landing Zone APs** can only poll. Their checks and reports are refused and not recorded: they have no config to check, and nobody has vouched for them yet (0033).
- **The conditions store** (0009): `conditions.db`, a second SQLite database beside the change log. It is not the system of record.
  - Render checks and apply results are kept for good, append-only like the change log: what each AP was told and what it did.
  - State reports are trimmed after a retention period: 30 days unless `AEOLUS_KEEP_STATE_DAYS` in `/etc/aeolus/serve.env` (or `serve -keep-state-days`) says otherwise.
- **Drift.** `GET /v1/aps/{id}/config` shows:
  - when the AP was last seen and from where;
  - the version it last reported running, and whether that is its current version;
  - its latest check, apply and state report.

  `GET /v1/aps/{id}/history` lists its recent checks, applies and state reports.

## Consequences

- The check is the rendering rules the agent must meet. The ucode renderer in M5 is written against it, and a mismatch shows up as a refusal, not as a broken AP.
- Until M5 adds the deferred fields, a config can pass the check with ports, management, SSH keys, poll interval, rate limits or HA timing rendered wrong.
- Retention is a host setting, not a logged change. If it needs to be set from the UI, it can become an Org setting in the change log later.

## Open

- Whether the agent removes UCI it did not create, or leaves non-Aeolus sections alone: decided with the agent in M5.
- Rate limits on state reports, if an adopted AP ever floods them.
