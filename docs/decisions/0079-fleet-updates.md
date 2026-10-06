# 0079. Fleet updates: APs update their own agent

- Status: Proposed
- Date: 2026-10-06
- Proposed by: Griff: a release reaches every AP by itself, a folder can pin the version it runs, the AP checks and rolls back on its own, and the APs tab and tree show which agent each runs. Written up by Claude
- Refines: 0040, 0070

## Context

- **Every agent release is installed by hand.** For each AP, someone fetches the agent's files from the manager, checks their hashes, compile-checks them, backs up the old copies, installs, and restarts the service. That is fine for two lab APs and the first thing to break at dozens of buildings.
- **The manager already holds the agent.** Each release builds the manager from a checkout that has the agent's files beside it, and the APs already poll the manager every minute over HTTPS, with its certificate pinned.
- **The AP has what it needs:**
  - ucode's `digest` module hashes files (`sha256_file`).
  - `uclient` gives a response's headers.
  - BusyBox has `sh -n`, `setsid` and `jsonfilter`.
  - `/etc/aeolus` survives a sysupgrade (`keep.d`), with 184 MB free on the office AP's overlay.
- **`keep.d` misses `aeolus-rrm`,** so a sysupgrade today loses the radio daemon. Fixed here.

## Decision

### The manager carries a bundle per release

- **The agent's files are built into the manager** (`go:embed`). A manager build is one agent bundle, so a release is the manager and the agent together.
- **A bundle is a manifest:** each file's path, mode and SHA-256, and the release it came with. Its identity is the SHA-256 of the manifest's lines, so two releases with the same agent files are the same bundle.
- **At start the manager files its bundle away** in `/var/lib/aeolus/agent`: the manifest by release, and the files by their hash. It keeps the last ten releases, so a folder pinned to an older one keeps getting it after the manager moves on.

### A folder says which agent its APs run

- **`system.agent`** is a Locations setting:
  - `current`, the default, means the manager's own release.
  - Otherwise it names a release the manager holds.

  It inherits like any setting.
- **A staged rollout is two settings:**
  1. Before upgrading the manager, pin the Org to the release it runs now.
  2. After upgrading, set a test building to `current`.
  3. When that looks right, set the Org back to `current`.
- **A pin to a release the manager doesn't hold is refused when it is set.** The check is the API's, not the change log's, so replaying old changes never fails over a release since pruned. A pin left pointing at a pruned release keeps its APs where they are, and the APs tab says why.
- The setting is in the config, so changing it re-versions the folder's APs. That is harmless: they apply the same config.

### The AP updates itself, and rolls itself back

- **Every poll answer names the bundle the AP should run,** in an `Aeolus-Agent` header, on a 304 as on a 200. So an unchanged config still carries it, and a held config or an AP in Landing Zone gets agent updates too.
- **Where that differs from the bundle the AP last confirmed:**
  1. It fetches the manifest (`GET /v1/ap/agent`).
  2. It hashes its own copies, and fetches only the files that differ (`GET /v1/ap/agent/files/{sha256}`), into `/tmp`.
  3. It checks each file's hash.
  4. It checks the shell scripts with `sh -n`, compiles the ucode scripts against the staged modules, and imports each staged module.
  5. It backs up its current copies of those files to `/etc/aeolus/agent.prev`.
  6. It copies its own rollback script, the old one, to `/tmp`.
  7. It installs the new files with their modes, and writes a trial marker.
  8. It starts the old rollback script, detached, and restarts the service.
- **The new agent confirms itself** once it has been up 30 seconds, has reached the manager, and procd shows every Aeolus instance running. It records the bundle as confirmed, and clears the marker.
- **The rollback script waits 180 seconds.** If the trial marker for that bundle is still there, it puts the backed-up files back, records the rollback, and restarts the service. The rollback script is the old one, run from `/tmp`, so a broken bundle can't break its own way back.
- **A bundle that failed or was rolled back isn't tried again for an hour,** unless the manager names another one.
- **Files a newer bundle no longer has are left in place.** That is harmless, and safer than deleting.

### The manager and the UI show it

- **The AP's state report says which bundle it runs,** with its release, and the last failed or rolled-back update with why.
- **The fleet view gives each AP's agent beside the one it should run.**
- **The APs tab has an Agent column:** the release, and *current*, *updating*, *behind*, *rolled back* or *failed*.
- **In the tree, an AP whose agent is behind** is amber, like an AP out of sync.
- **The System tab's Agent version** is a dropdown: `current` and the releases the manager holds.

### Getting there

- **The first agent that can update itself is installed by hand,** as every one has been so far. From then on, a release reaches the APs by itself.
- **Checked so far (2026-10-06):**
  - On the office AP, the new agent compiles against its modules (`ucode -c`), its state report carries `agent`, ucode reads `0755` as octal, and the rollback script parses.
  - The rollback script's logic is a unit test (`TestRollback`), run in its own directory rather than on an AP: an unconfirmed bundle gets its replaced files back, the rollback recorded and the service restarted; a confirmed update, or another bundle's trial, is left alone.
- **Still to check, before the APs rely on it:** the same on a live AP, with procd — a deliberately broken agent put in place under a trial, and the rollback watched putting the old one back. That breaks the agent on a production AP for the length of the trial, so it waits for Griff's go-ahead.

## Consequences

- **A manager upgrade updates every AP following `current` within a minute or so.** A site that wants to go slowly pins first.
- **The manager can now change the code on an AP, not only its config.** It already could, in effect: a config runs as root through netifd and hostapd. The same pinned TLS and AP token carry it. Signing stays set aside (0032).
- **An AP that reboots during its three-minute trial loses the rollback script.** If the new agent is broken, it stays broken until someone steps in. The window is short and noted.
- **An update restarts the service, not the Wi-Fi.** Clients don't notice; a config the new renderer renders differently applies as any config does.

## Open

- A cap on how many APs update at once, for very large fleets.
- Removing files a bundle no longer has.
- Whether `current` should wait for an hour of a release on the manager before APs take it.
