# 0070. Per-user keys, as built on OpenWrt

- Status: Accepted
- Date: 2026-10-05
- Proposed by: Griff: write up 0014. Written up by Claude, and accepted with the merge
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
- **Adding or removing one of those VLANs restarts the radios** that carry the network (found in the lab, below). Like any wireless change, it drops their clients for seconds, or for a minute or more on a DFS channel. So a site adds the VLANs it will need at setup, and the preview says so.

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

- **The key agent writes the keys as `wifi-station` sections,** named `aeolus_key_<n>`, through ucode's UCI library in its own process. The `uci` command is far too slow: 500 keys took more than 30 seconds. The library took 0.8 seconds. Each section holds:
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

## As built

- **`internal/keys`** is the store: keys by ID, each with:
  - its Services folder and network;
  - a name;
  - a sealed passphrase, under the path `keys.<id>.passphrase`, so it opens only as that key's;
  - an optional VLAN, MACs and expiry.

  `PSK` works the 64-hex PSK out with Go's `crypto/pbkdf2`.
- **`internal/change`** adds `add-key`, `set-key` and `remove-key`, applied to the state's key store.
  - Every refused key is `ErrBadKey`, answered 400. "No key" is 404, and an ID in use or too many keys is 409.
  - After any change to the trees, `checkKeys` refuses one that would strand a key, as an `InUseError` naming the keys.
  - Whether an expiry is still ahead is checked by the API, not on replay.
  - An operator on the folder may change its keys.
- **The API:**
  - It makes a missing ID (`k` and 10 hex digits).
  - It checks a passphrase given in plain text, opens the network's and the other keys' to refuse a repeat, and seals it. A sealed value handed in is refused.
  - `GET /v1/keys` lists a network's keys.
  - `GET /v1/ap/keys` works out the AP's set from its resolved networks: WPA2-PSK and on, from the key's folder or one below. Its version is a hash of the keys' sealed passphrases, VLANs and MACs and the SSIDs, so rotating a key, or one expiring, changes it.
  - A held request looks again every second, up to 50 seconds. When the set changes while it is held, the answer waits until it has been still for 2 seconds, up to 10, so a paste of many keys reaches an AP as one reload, not one a key.
  - PSKs are kept in a cache, by sealed passphrase and SSID.
  - The AP's config view says how many keys it should have and their version, and its state report says which it has.
- **The schema** adds `network.*.keys.vlans`, up to 512 VLANs. The render check covers it: for each VLAN, a bridge-vlan, an interface `aeolus_<net>_k<v>` on `<bridge>.<v>`, and a wifi-vlan `aeolus_<net>_kv<v>` with name `k<v>`, its VID, that interface, and the network's wifi-ifaces. A stray `aeolus_` wifi-vlan is refused.
- **The renderer** makes those, and keeps every `wifi-station` section, `aeolus_` or not. `without_keys` leaves them out of the UCI the render check is sent, and out of the test goldens.
- **The key agent:**
  - It's `aeolus-agent keys`, a procd instance of its own.
  - It asks with `wait=40` and a 55-second timeout. `http()` takes a timeout now.
  - It writes `aeolus_key_<id>` sections (the ID's `-` as `_`) on the network's wifi-ifaces present on the AP, with the wildcard MAC if the key is bound to none. Then `ubus call network reload`.
  - It keeps the version in `/etc/aeolus/keys.version`, which a sysupgrade keeps.
  - If its sections change behind it, such as by a revert of the wireless config, it asks for the whole set again.
- **MCP:** `list_keys`, and the key kinds in `make_change`.
- **The UI:**
  - **A Services folder's page** has a "Per-user keys" panel for each WPA2-PSK network the folder offers: its keys, with Rotate and Revoke; a form to add one, with a passphrase generator; and "Add many", for those who may edit.
    - **Add many:** paste one key a line, its name, passphrase (blank or `*` makes one) and VLAN, by commas or tabs. The keys it made are shown once with their passphrases, to hand out, and can be saved as a CSV file. The lines it couldn't add stay in the box, with the reasons.
    - **Generated passphrases** are four groups of four letters and digits, without ones that look alike.
  - **A network's card on a location's Networks tab** has a "Per-user keys" line: how many keys, and the VLANs offered. It opens to the keys and links to the folder's page.
  - The network's edit form has a "Per-user keys" section for `keys.vlans`.
  - The AP's status line says how many keys it should have, and warns when its key agent hasn't reported them yet.
- **Checked:**
  - **Tests:** the change kinds, the strand guard and who may change keys; the API's checks, its list, delivery to an adopted AP, a held request answering when a key is rotated, removal, and that no passphrase reaches the change log.
  - **On OpenWrtnight:** the render case `keys` (with the existing goldens unchanged), the agent compiling, and probe.out.
  - **In the UI harness:** the panel listing, adding (with a made ID), and refusing a repeated passphrase; and the AP's status line.
- **The prober reads key VLANs too** (v0.37.1). A key's client is on its VLAN's own Wi-Fi interface, `<bss>-k<vlan>`, not the BSS's, so the prober finds those interfaces by name.
  - It reads their stations, with the features from the BSS's hostapd.
  - It reads what their clients say in DHCP, for their address and identity. The DHCP watch's findings (0065) stay with the network's own segment.
  - A client's report says its `vlan`, and the Clients tab shows it under the network.
- **Live, on 2026-10-05,** with v0.37.0 on both lab APs:
  - **A key without a VLAN** (change 47, `lab-key-a` on `tedt`): it reached the pumphouse AP's key agent within the second it was made. hostapd updated both `tedt` BSSes' files with no drop, and the PSK was the one for its SSID.
  - **Griff's phone on that key,** on OfficeOpenWrt: `tedt` at 5 GHz, Wi-Fi 6, 192.168.20.84 on VLAN 20.
  - **VLAN 50 offered to `tedt`'s keys** (change 48): this restarted both APs' radios, as the lab check said. hostapd made `phy0-ap4-k50` and `phy1-ap4-k50` on the office AP, and netifd put them in `br-lan` as VLAN 50.
  - **A key with VLAN 50** (change 49, `lab-key-b`): hostapd logged `AP-STA-CONNECTED 56:9e:48:9c:e8:20 vlanid=50`. The phone's MAC was on `phy1-ap4-k50` in VLAN 50, and the site's VLAN 50 DHCP server leased it 192.168.50.16.
  - **The gap this showed:** the phone was missing from the Clients tab, since the prober read only the BSS interfaces. Fixed in v0.37.1, as above.

## Lab checks

On OpenWrtnight on 2026-10-05, on `tedt` (WPA2-PSK, `phy0-ap2` and `phy1-ap3`), with Griff told first. Every key was given as a 64-hex PSK, worked out for the SSID as the manager will.

- **One key, without a VLAN** (`wifi-station`, wildcard MAC):
  - after `ubus call network reload`, hostapd logged "Update config data files" and "Reloaded settings";
  - the key was in both BSSes' PSK files;
  - the four IoT clients' connected times kept counting. No one dropped.
- **A key with VLAN 50, and a `wifi-vlan` putting VLAN 50 in aeolus-50's bridge:**
  - **This restarted both radios.** The 2.4 GHz BSSes went down and came back in about 12 seconds, and the IoT clients rejoined within 15 to 27.
  - On 5 GHz, ACS chose DFS channel 60, so it came back after its 60-second radar check. No one was on 5 GHz.
  - hostapd made `phy0-ap2-v50` and `phy1-ap3-v50`, and netifd put both in `br-n351c496e`, beside aeolus-50's own interfaces and tunnel.
  - The PSK file had the line `vlanid=50 00:00:00:00:00:00 <psk>`.
- **500 more keys:**
  - The `uci` command, one `uci batch`, didn't finish in 30 seconds.
  - ucode's UCI library staged them in 0.71 s and committed in 0.11 s.
  - The reload took about a second: "Update config data files" on each BSS. The PSK files grew to 556 lines and no one dropped.
  - Taking the 500 out again was the same.
- **To do, with Griff's phone on `tedt`:**
  - the lab key without a VLAN should land on VLAN 20, `tedt`'s own segment;
  - the lab key with VLAN 50 on aeolus-50's segment;
  - and whether hostapd's `get_clients` says each client's VLAN.
- **Then, the lab keys and the VLAN come out.** Taking the VLAN out restarts the radios again.

## Not now

- WPA3 per-user keys, with SAE password identifiers or keys bound to MACs.
- A key landing its client in another Aeolus network, VXLAN included, rather than a VLAN.
- A tenant seeing or rotating their own key.
- Showing which key each client used: OpenWrt's `wifi-station` has no `keyid`.
- A network with only per-user keys and no passphrase of its own.
