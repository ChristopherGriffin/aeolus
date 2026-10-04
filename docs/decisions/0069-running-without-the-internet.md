# 0069. Running without the internet

- Status: Proposed
- Date: 2026-10-04
- Proposed by: Griff: Aeolus should work independent of the cloud. Pulling packages once at setup is fine, but nothing should need the internet continuously. And a "swarm": packages come to the manager once, then to the APs, rather than every AP fetching them from outside. Written up by Claude
- Refines: 0033, 0040

## Context

What reaches the internet today, checked in the code and on the lab on 2026-10-04:

- **Already local:**
  - **The AP agent** talks only to its manager, by the address it was installed with.
  - **The manager** makes no outbound call when running.
  - **The UI** loads nothing from outside.
  - **The maker table** (0067) is built into the manager.
  - **The DHCP listeners** (0068) only hear local traffic.
- **Packages for an AP** (`install.sh`): usteer, snmpd-ssl, vxlan, kmod-nft-bridge, ucode-mod-socket and kmod-veth come from OpenWrt's feeds at `downloads.openwrt.org`.
  - Every AP fetches them itself.
  - They're lost at each sysupgrade, so every AP fetches them again after one.
  - The feeds are in `/etc/apk/repositories.d/distfeeds.list`, all under `https://downloads.openwrt.org/releases/<version>/`. For OpenWrtnight that's seven indexes for 25.12.5, `ipq806x/generic`, `arm_cortex-a15_neon-vfpv4`, and its kernel's kmods.
  - **`apk` checks OpenWrt's signature on each index, and each package's hash against its index,** so the files don't need to come from OpenWrt's own server to be trusted.
- **Time:**
  - Aeolus renders `system.ntp` when it's set. When it isn't, OpenWrt's default pool servers stay, and those are on the internet.
  - **On OpenWrtnight, the name Symtus sets, `rutm50.symtus.com`, doesn't resolve.** The AP's dnsmasq has rebind protection on, and drops a public name's answer of a private address (10.0.1.253). ntpd keeps time only because DHCP also handed it 10.0.1.253.
- **Local names in general:** the same rebind protection is why the agent reaches the manager by IP, not by name (an open item from 0033 and 0040). Offline, every local name answers with a private address, so none would resolve.
- **Manager updates** (`aeolus-update`) fetch the release from GitHub, and Go modules from Go's proxy. A person runs it, now and then.
- **The manager's certificate** is self-signed, and good until **2028-09-26**. APs pin it.
  - When it lapses, they're expected to stop reaching the manager. ustream-ssl checks certificate dates; that isn't confirmed in the lab.
  - That isn't the internet, but it's the one thing that would stop an offline site that's left alone.
- **Not Aeolus's:** an OpenWiFi AP needs the internet to knock on the option 224 listener (0068). Claude's access through MCP is over the internet too, but nothing needs it.

## Decision

### The rule

**Once a site is set up, nothing Aeolus runs needs the internet.** That covers the manager, the agents, the UI, and the APs' Wi-Fi, time and names.

Only two things go out, each when a person starts it:
- the manager fetching packages it doesn't yet have, for APs;
- a manager update.

### Packages come through the manager

- **The manager keeps a cache of OpenWrt's feeds,** served read-only at `/feeds/` on its HTTPS port, 8443.
  - A path there is the same path under `https://downloads.openwrt.org/releases/`, and nothing else is fetched. So the cache can't be used as a proxy to anywhere else.
  - **A file it doesn't have, it fetches once,** keeps, and serves to every AP after.
  - **Package files never change,** so they're kept until the cache is full. Then the least recently used go first, within 2 GB by default.
  - **An index is fetched again when it is more than a day old,** if the internet answers. If it doesn't, the copy kept is served. Offline, the cache serves what it has.
- **APs fetch through the manager:**
  - `install.sh` points the AP's feeds at the manager's cache before installing anything. The agent does the same when it starts, since a sysupgrade puts OpenWrt's back. The first AP of a kind brings the packages into the cache; the rest get them from the manager.
  - **After a sysupgrade, the agent installs again what Aeolus needs,** from the manager, when it finds a package missing. A sysupgrade then needs no internet, once the cache has the new release.
- **The cache holds nothing secret,** so it needs no token: `apk` can't send one. It only answers paths in OpenWrt's release tree.
- **`apk` has to trust the manager's certificate.** `install.sh` puts it beside the AP's CA bundle. If `apk` won't take it there, the cache is served on a plain HTTP port instead: OpenWrt's signatures protect the files either way.
- **Firmware images** live under the same tree, so a sysupgrade image can be fetched through the cache too.

### Time from the site

- **Aeolus never leaves OpenWrt's internet pool in place.** When `system.ntp` is set, it's the only time source rendered. When it isn't, none is, and the AP takes the time servers its DHCP gives it (option 42), as it does now.
- **The AP reports whether its clock is synchronized,** and from what, from ntpd's hotplug events. The manager warns about an AP that isn't.

### Local names resolve on the AP

- **The renderer exempts from rebind protection the names Aeolus itself uses on the AP:** the manager's host, the NTP servers and the syslog host. It does this by name, in dnsmasq's `rebind_domain`, so their private answers are allowed and nothing else changes.
- With that, an AP can be installed with the manager's name, as 0033 wanted, as long as the site's DNS answers it.

### Keeping it so

- **A check in CI:** the UI's files and the agent's files name no internet address. The SVG namespace and the cache's upstream, on the manager, are the only exceptions.

## Consequences

- **With 30 APs of one model, OpenWrt's servers see one download of each package,** not 30. A site can then lose its internet without anything noticing.
- **The manager uses disk for the cache,** up to 2 GB by default.
- **`install.sh` now needs the manager reachable,** which it already did, to enroll.
- **Rebind protection is relaxed for a few named hosts only,** the ones the site's own config names.

## Lab plan

- **On OpenWrtnight:** point one feed at the manager's cache.
  - Run `apk update`, then `apk fetch` a package and verify its signature.
  - Check that a second fetch comes from the cache, and that the cache serves it with the manager's internet cut, using a temporary firewall rule on the manager host.
  - Check whether `apk` takes the manager's certificate from beside the CA bundle.
- **Time:** render `system.ntp` with its rebind exemption. Check that `rutm50.symtus.com` resolves and ntpd uses it, and that no pool server is left.

## As built

- **`internal/feeds`**, mounted at `/feeds/` on the manager's port. It needs no token.
  - **What it answers:** only `GET` and `HEAD`, for paths of at most 16 parts, each letters, digits and `._+~-`, starting with a letter or digit.
  - **Indexes and files:** an index is a name ending in `.adb`, `.json`, `.sig` or `.asc`, or starting with `Packages` or `sha256sums`. Anything else is a file that never changes.
  - **Fetching:** one fetch a path, however many ask; it finishes for the cache even when the request that started it goes away. A file is at most 256 MB, and its length must match what the server said.
  - **The cache:**
    - It lives in `/var/lib/aeolus/feeds`, within 2 GB by default.
    - It's read back when the manager starts.
    - The least recently served goes first, never the file just fetched.
- **On the AP:**
  - **`/usr/libexec/aeolus-packages`** is shared by `install.sh` and the agent:
    - `feeds` points the feeds at the manager. It moves a feed from OpenWrt's tree, or from a manager the AP knew before, and puts the manager's certificate in `/etc/ssl/certs`, where uclient-fetch, which `apk` fetches with, finds it.
    - `install` installs and configures the packages as `install.sh` did. It records what it installed in `/etc/aeolus/packages`.
    - `restore` installs again what that list names and is missing. It runs only while `/usr/lib/aeolus/packages`, which a sysupgrade wipes, is gone.
    - An AP installed before this records what it has, and installs nothing.
  - **The agent** runs `feeds` when it starts. While a restore is due, it starts one in the background every hour. A restore restarts the network if it installed vxlan, and the agent if it installed ucode-mod-socket.
  - **The sysupgrade keep list** now holds the agent's programs, its init script and start link, the package script, the ntp hook and the manager's certificate, as well as `/etc/aeolus` and its config. Before, only those two were kept, so a sysupgrade lost the agent.
  - **`/etc/hotplug.d/ntp/50-aeolus`** writes ntpd's last word to `/var/run/aeolus/time.json`. The agent reports it as `time`, with the servers from ntpd's command line.
  - **The renderer:**
    - When `system.ntp` is unset, it drops the servers ending in `.openwrt.pool.ntp.org`, and keeps any other.
    - It adds `dhcp` to the packages it renders. Into dnsmasq's `rebind_domain` it puts the manager's host from the agent's URL, and each NTP and syslog host that is a name. It records them in `aeolus.agent.rebind`, so it can take them out when they are no longer used.
- **The manager's render check** refuses OpenWrt's pool left in place, and a time or syslog host name missing from `rebind_domain` where the AP runs dnsmasq.
- **The UI:** a clock warning on the AP page, when ntpd says the clock isn't synchronized, or the AP has no time server.
- **The CI check** is `TestNoInternetAddresses` in `internal/ui`. Three addresses are allowed:
  - the SVG namespace;
  - OpenWrt's release tree, which the package script rewrites;
  - the manager in `install.sh`'s example.

### Checked

- **Tests:**
  - **The cache:** fetched once and then served; twenty at once making one fetch; an index fetched again after a day, and kept when offline; 502 for what was never fetched; only OpenWrt's tree answered; least-recently-used eviction; read back on reopening; and `serve` mounting it without a token.
  - **The API:** a report's clock, accepted and refused.
- **On OpenWrtnight:**
  - **`apk` and the manager's certificate:** with the certificate in `/etc/ssl/certs`, `apk` fetched from the manager over HTTPS. Without it, uclient-fetch exited 5, a certificate error. So the cache stays on HTTPS.
  - **The renderer**, run with ucode for the render cases:
    - without `system.ntp`, the pool servers are gone and only the manager's name is in `rebind_domain`;
    - the clamp files are unchanged.
  - **The package script,** on copies of its files: the feeds were moved and a second run changed nothing; a feed for another manager was moved; an older AP's packages were recorded; and a package that couldn't be installed left the restore due.
  - **The agent:** its read-only `state` showed `time` from the hook, with ntpd's two servers, `rutm50.symtus.com` and 10.0.1.253.
- **The lab plan's live checks follow the release.**

## Not now

- **Renewing the manager's certificate locally,** before 2028-09-26: its own decision, well before then.
- **Updating the manager from a file,** for a site with no internet at all. `aeolus-update` stays a person-run fetch from GitHub.
- **Fetching ahead:** the cache filling itself for a new AP's release and model before that AP asks.
- **Feeds other than OpenWrt's,** and packages Aeolus doesn't need.
