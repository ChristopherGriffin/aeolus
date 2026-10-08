# 0080. The agent as an OpenWrt package

- Status: Proposed
- Date: 2026-10-07
- Proposed by: Griff: Aeolus on an AP should simply be an install package, so an AP running plain OpenWrt (an Arista C-360 on its own OpenWrt image, say) stays plain until someone adds Aeolus; later updates stay as 0079 has them; each manager serves its own package. Written up by Claude
- Refines: 0040, 0069, 0079

## Context

- **The agent is installed by copying files.** `agent/install.sh`, run over SSH, copies the agent onto the AP and configures it (0040, which says "a package in a feed comes later"). Nothing on the AP knows it is installed, and nothing removes it.
- **APs exist that run OpenWrt and nothing else.** The Arista C-360 now has an OpenWrt image of its own, meant for anyone who has one, with or without Aeolus. Aeolus must be something added to it by OpenWrt's own means, `apk add`, never something its image carries.
- **After the first install, the agent updates itself** from the manager, file by file, with a trial and a rollback (0079). That stays.
- **Not every image has every module the agent needs.** The C-360's image has no `ucode-mod-uclient`, the agent's HTTPS client. A sysupgrade keeps the agent's files (`keep.d`) but not the packages installed after the image, and an agent that imports a missing module cannot start to put it back.

## Decision

### One package, for every AP

- **`aeolus-agent`**, built from `agent/openwrt/aeolus-agent/Makefile`. It holds exactly the files under `agent/files`, the ones a release's bundle carries (0079), so a package and a bundle of one release are the same agent.
- **The agent is pure ucode, so the package is architecture-independent** (`PKGARCH:=all`). One file serves every target.
- **It depends on the modules the agent cannot start without:** `ucode` and its `fs`, `uci`, `ubus`, `uloop`, `uclient`, `math`, `log` and `digest` modules. apk installs those that are missing. The optional ones stay with `aeolus-packages` (0069): a missing one turns off only what needs it.
- **Its version is the release it was built from**, as apk spells it: v0.49.2 is `0.49.2`, three commits past it `0.49.2_p3`.

### Installed, it is inert

- **`apk add` puts the files in place and changes nothing else.** The service is installed, but starts nothing until the AP has a manager URL (`aeolus.agent.url`).
- **`aeolus-setup <manager URL> <uplink port> <manager certificate>`** joins the AP to a manager: what `install.sh` did after copying. It writes the agent's settings, installs the manager's certificate and the optional packages (0069), checks the agent reaches the manager, and starts it. The AP enrolls into Landing Zone, as before.
- **`install.sh` stays, for an AP without the package.** It copies the files and hands over to `aeolus-setup`.
- **`apk del aeolus-agent` stops the service and removes the files.** The certificate, token and settings in `/etc/aeolus` and `/etc/config/aeolus` stay, and so does the AP's network: taking Aeolus away leaves the AP running as it was (0040).

### Updates

- **The package is the first install. From then on the agent updates itself** from the manager (0079). apk still lists the version it installed; the agent's state report says which bundle it runs.

### Where it comes from

- **Each manager serves the package of its own release,** so an AP gets it from the manager it will join, over the same pinned TLS, without the internet (0069). The route, and how a release builds its package, are the next step (Open).
- **It is not signed** (0032, 0079): it is installed with `apk add --allow-untrusted`, from the manager the operator chose.

### After a sysupgrade

- **The init script, plain shell, checks the agent's modules before starting it.** If one is missing, it starts `aeolus-packages restore`, which now puts back the required modules as well as the optional ones, from the manager's cache of OpenWrt's feeds. procd retries the agent until they are back.

## Consequences

- **An AP's image never carries Aeolus.** A plain OpenWrt AP is one `apk add` and one `aeolus-setup` from being managed, and one `apk del` from being plain again.
- **On the C-360, apk resolves the dependencies from OpenWrt's own 25.12 feed,** directly or through the manager. Checked on a C-360 (2026-10-07): only `ucode-mod-uclient` was missing, and apk would install exactly it, built from the same uclient as the image's `libuclient`.
- **After 0079 updates the files, `apk audit` reports them as changed.** That is expected: the agent, not apk, owns its files from then on.

## Open

- The manager's route for the package, and how a release builds it: with apk-tools (`apk mkpkg`) on the manager's host at build time, or written by the manager itself from its bundle.
- Whether `aeolus-packages restore` should also re-register the package with apk after a sysupgrade, from the manager.
