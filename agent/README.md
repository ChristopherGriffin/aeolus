# The Aeolus agent

The program that runs on each AP (0040). It's written in ucode and needs nothing beyond OpenWrt's default image. It was built against OpenWrt 25.12.5 on PumphouseAP.

| Path | What it is |
|---|---|
| `files/usr/sbin/aeolus-agent` | The agent. It enrolls, polls, renders, has the manager check the result, applies it with an automatic revert, and reports. |
| `files/usr/share/ucode/aeolus/render.uc` | The renderer: intent plus the current UCI in, new UCI out. It's a pure function, so it can be tested anywhere. |
| `files/usr/share/ucode/aeolus/uciexport.uc` | Writes packages as `uci export` text. The same text is checked and then applied. |
| `files/etc/init.d/aeolus` | procd service. |
| `files/lib/upgrade/keep.d/aeolus` | Keeps the token, the certificate and the settings across a sysupgrade. |
| `install.sh` | Installs everything on an AP. |
| `test/` | Test cases for the renderer: an intent, the config it starts from, and the UCI it must produce (`cases/*.uci`). |

## Install

Copy this directory and the manager's certificate (`/etc/aeolus/tls.crt` on the manager) to the AP. Then run:

```sh
sh install.sh https://192.168.20.60:8443 wan /tmp/manager.crt
```

The arguments are the manager's URL, the uplink port (the one carrying the VLANs), and the certificate the agent will trust. The script checks the agent can reach the manager, then starts it. The AP enrolls into Landing Zone and waits for a person to adopt it. `logread -e aeolus` shows what it is doing.

Use the manager's IP address, not its name. OpenWrt's dnsmasq has rebind protection on by default, which drops DNS answers that point to private addresses, so an AP usually can't resolve a LAN name for the manager.

## Tests

`internal/rendercheck` in the manager holds both halves of the contract:
- Each `cases/*.uci` must pass the render check and leave every section Aeolus doesn't own unchanged.
- Where ucode is installed, the renderer must produce exactly that output. CI builds ucode to run this.

To run one case by hand:

```sh
ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/render.uc agent/test/cases/sandbox.json
```
