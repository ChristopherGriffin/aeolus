# 0070. Per-user keys, as built on OpenWrt

- Status: Proposed
- Date: 2026-10-05
- Proposed by: Griff: write up 0014. Written up by Claude
- Refines: 0014, 0027, 0030, 0040

## Context

- **0014** gives each person or unit its own passphrase on a shared SSID, with each key choosing the segment its client lands on. The keys live on the manager, in their own store with their own version, apart from config. A key agent on each AP keeps its keys, without the full config cycle and without dropping clients. 0014 left open:
  - how keys are delivered: a long poll or a short one, and whole lists or changes;
  - whether key updates skip the render check (0008).
- **What OpenWrt 25.12 does,** read on OpenWrtnight (`wpad-basic-mbedtls`):
  - **The sections:** UCI has `wifi-station` sections (`iface`, `mac`, `key`, `vid`) and `wifi-vlan` sections (`iface`, `name`, `vid`, `network`).
    - From the stations, OpenWrt writes hostapd's `wpa_psk_file`, one line a key: `vlanid=<vid> <mac> <key>`. A MAC of `00:00:00:00:00:00` matches any device.
    - From the VLANs, it writes its `vlan_file`, and puts each VLAN's Wi-Fi interface in that VLAN's network.
  - **A change to those files alone doesn't restart the BSS.** The hostapd script finds the rest of the config unchanged, updates the files and sends hostapd `RELOAD_WPA_PSK`.
  - **The binary has it:** this basic hostapd has `RELOAD_WPA_PSK`, `wpa_psk_file`, `vlan_file` and `dynamic_vlan`, so no fuller wpad is needed.
  - **The PSK is hostapd's work.** For each passphrase, hostapd works out the PSK for the SSID: 4,096 rounds of PBKDF2. A key given as the 64-hex PSK skips that.
- **WPA3:** SAE can't try several passwords for one client the way WPA2-PSK tries several PSKs, unless the client names its password, which few do. Per-user keys are a WPA2-PSK feature.

## Decision

### What a key is

- **A key belongs to one Aeolus network** (0013), which lives in a Services folder. Its fields:
  - a **name**, such as "Unit 101" or a person's name;
  - a **passphrase**, 8 to 63 printable characters, sealed like every secret (0027) and never read back;
  - optionally, the **VLAN** its client lands in;
  - optionally, the **MACs** it's bound to;
  - optionally, an **expiry**.
- **The network's own passphrase stays,** and lands its clients on the network's own segment. A key's passphrase must differ from it and from every other key on the network, or hostapd couldn't tell which VLAN was meant.
- **Per-user keys need `wpa2-psk`.** A network with keys and other security is a problem the config check holds (0039).
- **The VLANs a key may name are the network's config:** `network.*.keys.vlans`, a list of VLAN IDs. The renderer gives each one:
  - a `wifi-vlan` on the network's Wi-Fi interfaces;
  - a bridge of its own;
  - the VLAN tagged on the uplink.

  Adding a unit's VLAN is a config change, made once at setup. Adding or revoking a key isn't.

### Keys are changes, but not config

- **A key is added, changed or removed by a change in the change log** (0003, 0009): `add-key`, `set-key`, `remove-key`, with who and when, and the passphrase sealed.
- **A key change re-versions no AP.** Each AP's keys have their own version: a hash of the key set it is sent. That changes with a key, with the network's SSID, and when a key expires.
- **Who may change keys:** an operator on the network's Services folder, the leasing office of 0030's example. A viewer there sees the keys' names, VLANs and expiries, never their passphrases.

### Delivery: a long poll, of the whole set

- **The key agent is a second procd instance of the agent** (`aeolus-agent keys`), with the AP's own token.
- **It asks `GET /v1/ap/keys` with the version it has.** The manager answers at once when the AP's set differs. If it doesn't, the manager holds the request up to 50 seconds, and answers sooner if a key changes. A change reaches the APs in a second or two, with one request a minute while nothing happens.
- **The answer is the AP's whole key set,** for each of its networks with keys. A set of a thousand keys is about 100 kB, and a whole set can't drift the way a stream of changes can.
- **Each key is sent as the 64-hex PSK** for its network's SSID, worked out on the manager. That saves the AP 4,096 rounds of PBKDF2 a key at each reload. The passphrase never leaves the manager, and a PSK opens only that SSID.
- **Expired keys aren't sent.** The manager wakes the long polls when a key expires.

### On the AP

- **The key agent writes the keys as `wifi-station` sections,** named `aeolus_key_<n>`. Each section holds:
  - the network's wifi-ifaces;
  - the key's MACs, or the wildcard;
  - its PSK;
  - its VLAN.

  The agent then has the wireless config reloaded. OpenWrt updates hostapd's files and sends `RELOAD_WPA_PSK`, and no client drops.
- **Keys skip the render check:**
  - The manager checked them when they were entered.
  - The agent checks each one's form before writing it: a 64-hex PSK, a VLAN in the network's list, and MACs.
  - The renderer leaves `wifi-station` sections alone.
  - The UCI sent for the render check leaves them out, so per-user keys never reach a stored check (0041).
- **The two instances take turns on UCI,** through a lock, so neither loses the other's change.
- **The keys are kept on flash,** in the wireless config. A rebooted AP, or one whose manager is away, keeps its last set (0014).
- **The AP reports its key version** in its state report, so the manager shows which APs have the latest keys.

### Where it shows

- **API:** `GET /v1/networks/<id>/keys` lists a network's keys, without passphrases. Changes go through `POST /v1/changes`, and MCP gets `list_keys`.
- **UI:** a network's page under Services gets a Keys panel, to add, rotate, revoke and see each key. The AP's page says whether it has the latest keys.

## Consequences

- **Moving in, moving out or losing a phone** is one logged change, which reaches every AP of the network in seconds and drops no one.
- **Per-user keys are WPA2-PSK only.**
- **A unit's VLAN is set up once,** as config. The number of VLANs one AP can carry this way is measured in the lab.

## Lab plan

On OpenWrtnight, with Griff told first, because it touches the wireless config:

- **Two `wifi-station` sections by hand on `tedt`,** one with a VLAN:
  - Check the reload: hostapd logs `RELOAD_WPA_PSK`, and the IoT clients' connected times don't reset.
  - With Griff's phone: the network's passphrase lands on its own segment, a key without a VLAN the same, and a key with one in its VLAN.
- **Reload with 500 keys:** how long it takes, and that no client drops.
- **Which reload command** touches only what changed.
- **Whether hostapd reports each client's VLAN,** so the Clients tab can show which key's VLAN a client is in.

## Not now

- WPA3 per-user keys, with SAE password identifiers or keys bound to MACs.
- A key landing its client in another Aeolus network, VXLAN included, rather than a VLAN.
- A tenant seeing or rotating their own key.
- Showing which key each client used: OpenWrt's `wifi-station` has no `keyid`.
- A network with only per-user keys and no passphrase of its own.
