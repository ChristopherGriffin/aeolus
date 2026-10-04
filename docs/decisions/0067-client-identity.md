# 0067. Who each client is, and how well it connects

- Status: Accepted
- Date: 2026-10-04
- Proposed by: Griff: for each client, its host name, its maker by OUI, its device type, the 802.11 features it supports, such as r, k and v, and its retry rates. Written up by Claude, and accepted with the merge
- Refines: 0065, 0066

## Context

- **The Clients tab (0066) shows a client's host name and address only on Aeolus's networks,** and only once the client has asked for DHCP while watched.
- **A MAC's first three bytes name its maker** (the OUI), in the IEEE's registry. It is published as `oui.csv`, about 38,000 MA-L entries in 3.9 MB, refreshed daily. A phone's private MAC sets the locally administered bit, and names no maker.
- **What hostapd says of each client,** checked on OpenWrtnight:
  - its 802.11n, ac and ax support (`ht`, `vht`, `he`);
  - WMM;
  - protected management frames, 802.11w (`mfp`);
  - MBO;
  - its 802.11k abilities (the RRM bits);
  - its extended capabilities, whose bit 19 is 802.11v's BSS transition.
- **802.11r isn't visible.** A client shows it only by the key management it chose, an FT one. hostapd's ubus doesn't give that, and this OpenWrt build of hostapd has no per-station commands on its control socket: `STA-FIRST` is "UNKNOWN COMMAND". Checked on OpenWrtnight.
- **A client's DHCP request says more than its host name:**
  - its vendor class (option 60), such as `android-dhcp-14`, `MSFT 5.0` or `udhcp 1.36`;
  - the options it asks for, and in what order (option 55). That order is a well-known fingerprint of its operating system.
- **nl80211 already gives each client's frames sent, retried and failed** (0066).

## Decision

### What the AP adds

- **The prober reads DHCP on every Wi-Fi interface,** the AP's own included, for each client's name and fingerprint:
  - host name (option 12, else 81);
  - vendor class (60);
  - parameter request list (55).

  The DHCP findings of 0065 stay with Aeolus's networks.
  - A client already joined gives these when it next asks: at its renewal, after half its lease.
- **For each client, from hostapd:**
  - its 802.11 generation: n, ac or ax;
  - 802.11k, from any RRM bit;
  - 802.11v, from the BSS transition bit;
  - 802.11w;
  - MBO;
  - WMM.
- **802.11r is reported as not known,** until hostapd says which key management a client chose.

### What the manager adds

- **The maker, from an IEEE OUI table** built into the manager: the registry's MA-L list, taken at build time, compressed.
  - A private MAC reads as "private MAC". These are mostly phones, tablets and computers.
  - The table is refreshed with each release.
- **A device type and operating system, guessed from each client's evidence in this order:**
  1. its host name, such as `iPhone`, `Galaxy-S22`, `DESKTOP-…` or `ESP_…`;
  2. its DHCP vendor class;
  3. its DHCP fingerprint, a short table of well-known ones;
  4. its maker, such as Espressif, Tuya or Shelly for IoT, Sonos for speakers, or Roku for TVs;
  5. whether its MAC is private.

  Each guess says what it went on, such as "from its host name". It stays "unknown" rather than guess wildly.
- These are added when a report arrives, and kept with it. The AP sends only what it saw.

### What the Clients tab shows

- **Client:** its host name or MAC; beneath it, its maker and device type, such as "Apple · phone, iOS" or "private MAC · phone, Android".
- **Wi-Fi:** its generation and features as small chips, such as `ax k v w`, with r as "?" while unknown.
- **Retries:** the share of frames sent to it that were retried, and that failed, since it joined. Over 10 % is amber, over 25 % red.
- Filters and search take the maker and type too.

## Consequences

- **The manager's binary grows** by the compressed OUI table, about 400 kB.
- **Device types are guesses.** A tab that says what each guess went on is honest about it.
- **Names and fingerprints from the AP's own networks reach the manager too,** as clients' MACs already do (0066).
- **The manager goes first.** It refuses a report with fields it doesn't know, so an AP with this prober can't report to an older manager. Roll the manager out, then the APs.

## Not now

- **802.11r per client:** needs hostapd to tell the key management, by ubus or a fuller build.
- Clients' history, and acting on clients (0066).

## As built

- **The OUI table:** `internal/oui/oui.txt.gz`, made by `go run ./internal/oui/gen` from the IEEE's `oui.csv`. It holds 40,296 MA-L prefixes, 386 kB compressed, read into memory on first use. The generator refuses a list of fewer than 10,000.
- **The guess:** `internal/identify`, in tables of host names, vendor classes, four fingerprints (Apple, two Android, Windows), and makers.
  - Apple's maker gives the OS "Apple", and a private MAC gives "phone, tablet or computer". Each only fills what's still blank.
  - `basis` names only the evidence that filled something: "host name", "DHCP vendor class", "DHCP fingerprint", "maker" or "private MAC".
- **The prober:**
  - It opens the DHCP watch's socket on every Wi-Fi interface, keeping by MAC what each client says of itself. Requests going out are skipped, as in 0065.
  - It takes ARP only as it comes in from a client. The bridge floods the segment's ARP out to every Wi-Fi interface, and taken as it was, every wired host would have been kept.
  - What it keeps of a client is forgotten an hour after the client leaves.
  - Features come from `hostapd.<ifname> get_clients` at each scan:
    - `gen` from `eht`, `he`, `vht`, `ht`;
    - k from any non-zero RRM byte;
    - v from bit 3 of the third extended-capabilities byte;
    - w from `mfp`.
- **The report** grows to at most 512 kB, room for 256 clients. The manager checks the new fields: `vendor_class` printable and at most 64 bytes, `params` at most 64 numbers, and `gen` one of the four or none.
- **The tab:**
  - The Client cell's second line is the maker, without "Inc." and the like, or "private MAC", then the guess. Its tooltip says what the guess went on.
  - **Wi-Fi** shows the generation, then chips k, v, w and "r?". Each chip's tooltip names the feature.
  - **Retries** is a chip, "N % retried", coloured by the worse of retried and failed, with "N % failed" beneath when there are any.
  - The search takes maker, kind and OS.

### Checked

- **On OpenWrtnight,** with the new prober for a few minutes:
  - Two Espressif clients showed `gen` n with WMM; a third showed none, a/b/g, which hostapd confirmed (`ht` and `wmm` false).
  - None showed k, v or w.
  - Two had their addresses from ARP within a minute, on the AP's own `Sweet_Spot_IoT`.
  - Host names and fingerprints wait for each client's next DHCP renewal.
- **The probe tests,** on OpenWrtnight, read a vendor class and parameter list from a discover.
- **Harness:** the tab with a private-MAC iPhone ("private MAC · phone, iOS", Wi-Fi 6, k v w), a legacy Espressif ("Espressif · iot, Linux", a/b/g, 2 % failed) and a Wi-Fi 4 one.
- **The v0.33.0 manager** refused the new prober's report: "unknown field vendor_class". Hence the manager goes first.
