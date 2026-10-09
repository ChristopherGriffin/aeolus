# The Aeolus agent

The program that runs on each AP (0040). It's written in ucode. It ships as the OpenWrt package `aeolus-agent` (0080), which depends on the ucode modules it needs; on most images they are there already. Setting it up also adds usteer, for band steering (0050), snmpd, for SNMP (0052), vxlan and kmod-nft-bridge, for tunnels (0054), and ucode-mod-socket, for the prober (0059). It was built against OpenWrt 25.12.5 on PumphouseAP; it needs OpenWrt 24.10 or later, the first release whose feeds have all its modules, and its scripts speak 24.10's opkg as well as apk (0083).

| Path | What it is |
|---|---|
| `files/usr/sbin/aeolus-agent` | The agent. It enrolls, polls, renders, has the manager check the result, applies it with an automatic revert, and reports. |
| `files/usr/sbin/aeolus-setup` | Joins the AP to a manager (0080): the agent's settings, the manager's certificate, the modules the agent needs and the optional packages, then it starts the agent. |
| `files/usr/libexec/aeolus-enroll` | Joins the AP to a manager a person names (0083): it fetches the manager's certificate, checks its fingerprint, makes a manager's name resolvable past dnsmasq's rebind protection, and runs `aeolus-setup`. LuCI's Aeolus page runs it. |
| `files/usr/share/ucode/aeolus/render.uc` | The renderer: intent plus the current UCI in, new UCI out. It's a pure function, so it can be tested anywhere. It leaves alone a radio another service owns, such as airscan's scan radio (0081). |
| `files/usr/share/ucode/aeolus/uciexport.uc` | Writes packages as `uci export` text. The same text is checked and then applied. |
| `files/usr/sbin/aeolus-prober` | The prober (0059): it probes each tunnel's segment and its concentrator, which keeps the tunnel open too, and guards tunnel ports against loops. Its plan is the agent's UCI package, as the renderer made it. |
| `files/usr/share/ucode/aeolus/probe.uc` | The prober's frames, kernel filters and verdicts: pure, so they are tested anywhere. |
| `files/usr/sbin/aeolus-rrm` | Radio resource management (0073): it marks the AP's beacons as an Aeolus AP's, listens on its channels for the others, and exchanges signed hellos with its radio neighbours over the wire. It stays idle until Aeolus turns it on. |
| `files/usr/share/ucode/aeolus/rrm.uc` | Its advert, HMAC-SHA256, hellos and choice of neighbours: pure, so they are tested anywhere. |
| `files/etc/init.d/aeolus` | procd service: the agent, its key agent, the prober where ucode-mod-socket is installed, and radio resource management where ucode's nl80211, digest and socket modules are. It starts nothing until `aeolus-setup` has run, and after a sysupgrade it puts back a missing module first (0080). |
| `files/lib/upgrade/keep.d/aeolus` | Keeps the agent, the token, the certificate and the settings across a sysupgrade. |
| `openwrt/aeolus-agent/Makefile` | The package (0080): exactly the files under `files/`, for every architecture. |
| `luci/` | LuCI's Aeolus page, under Services (0083): the manager's name or address, its certificate's fingerprint, the uplink, and a button that joins. |
| `openwrt/luci-app-aeolus/Makefile` | The page as a package: exactly the files under `luci/`. |
| `web-install.sh` | The installer the manager serves at `/install` (0083), with the manager's URL filled in. |
| `install.sh` | Installs the agent from a copy of this directory: copies the files, then runs `aeolus-setup`. |
| `test/` | Test cases for the renderer: an intent, the config it starts from, and the UCI it must produce (`cases/*.uci`); `probe.uc`, which prints the prober's frames and verdicts, with `probe.out`, what it must print; and `rrm.uc` and `rrm.out`, the same for radio resource management. |

## Install

On the AP, from the manager it will join (0083):

```sh
wget -qO- --no-check-certificate https://192.168.20.60:8443/install | sh
```

It prints the manager's certificate's fingerprint, installs the agent, and LuCI's Aeolus page where LuCI is, with every file checked over TLS pinned to that certificate, then joins the AP to the manager with the uplink the default route leaves by. Options go after `sh -s --`: `-u <port>` names the uplink, `-f <fingerprint>` refuses a manager whose certificate is not that one, and `-n` installs without joining. On the manager, `aeolus fingerprint` prints the fingerprint to compare. The script itself comes over unchecked TLS, so the first fetch trusts the network between the AP and the manager, as copying a certificate by hand trusts the copy.

To join later, or to a manager the AP did not find by itself: LuCI's Services, Aeolus page takes the manager's DNS name or IP address, shows its certificate's fingerprint for you to check, and joins. Without LuCI, the same from a shell:

```sh
aeolus-enroll cert aeolus.example.com
aeolus-enroll enroll aeolus.example.com <fingerprint> [uplink]
```

With the package, copy it and the manager's certificate (`/etc/aeolus/tls.crt` on the manager) to the AP, then:

```sh
apk add --allow-untrusted /tmp/aeolus-agent.apk
aeolus-setup https://192.168.20.60:8443 wan /tmp/manager.crt
```

The package is not signed (0080); it comes from the manager you chose. Installed, the agent does nothing until `aeolus-setup` runs, and `apk del aeolus-agent` takes it away again, leaving the AP's network as it is.

Without the package, copy this directory and the certificate to the AP, and run `sh install.sh` with the same arguments: it copies the files and runs `aeolus-setup`.

The arguments are the manager's URL, the uplink port (the one carrying the VLANs), and the certificate the agent will trust. Setup installs usteer for band steering and turns its steering off, so it steers nothing until Aeolus asks (0050). It installs snmpd too, turned off and without OpenWrt's default communities, until Aeolus turns SNMP on (0052). This needs the AP to reach OpenWrt's package feeds, through the manager (0069); without them, everything else works. Then setup checks the agent can reach the manager, and starts it. The AP enrolls into Landing Zone and waits for a person to adopt it. `logread -e aeolus` shows what it is doing.

OpenWrt's dnsmasq has rebind protection on by default, which drops DNS answers that point to private addresses, so an AP can't resolve a LAN name for the manager by itself. `aeolus-enroll` adds the manager's name to dnsmasq's `rebind_domain`, as the agent does for the names it uses (0069); with `aeolus-setup` alone, use the manager's IP address.

## Building the package

In an OpenWrt buildroot or SDK, add this directory's `openwrt` as a feed and build it:

```sh
echo "src-link aeolus /path/to/aeolus/agent/openwrt" >> feeds.conf
./scripts/feeds update aeolus && ./scripts/feeds install aeolus-agent luci-app-aeolus
make package/aeolus-agent/compile package/luci-app-aeolus/compile
```

## Tests

`internal/rendercheck` in the manager holds both halves of the contract:
- Each `cases/*.uci` must pass the render check and leave every section Aeolus doesn't own unchanged. A radio another service owns must come out exactly as it went in (0081).
- Where ucode is installed, the renderer must produce exactly that output, and `test/probe.uc` must print `test/probe.out`. CI builds ucode to run this.

To run one case by hand:

```sh
ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/render.uc agent/test/cases/sandbox.json
```
