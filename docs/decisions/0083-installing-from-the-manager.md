# 0083. Installing from the manager, and joining one by hand

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff: the Aeolus package should install with a simple wget on any OpenWrt, and LuCI should have a blank for the manager's FQDN or IP, for when finding the manager by DNS or DHCP doesn't work out. Written up by Claude
- Refines: 0033, 0040, 0069, 0080

## Context

- **Installing the agent means copying files by hand.** The package (0080) or `install.sh` goes onto the AP over SSH, with the manager's certificate, and `aeolus-setup` takes the manager's URL. 0080 left open how the manager would hand out the agent itself.
- **APs do not find the manager by themselves yet.** 0033 leaves discovery open: a DNS name first, a DHCP option as an override (never 43, 60, 138 or 224). Wherever that fails, a person has to name the manager, and not everyone who runs an OpenWrt AP will use SSH.
- **A manager's name rarely resolves on an AP.** dnsmasq's rebind protection drops answers pointing to private addresses (0040), so `aeolus-setup` has needed the manager's IP address. The agent adds the names it uses to `rebind_domain` (0069), but only once it runs.
- **The agent pins the manager's certificate** (0033). Whoever gives an AP that certificate decides which manager it trusts.

## Decision

### One command, from the manager

- **On the AP:** `wget -qO- --no-check-certificate https://<manager>:8443/install | sh`. `wget` is OpenWrt's `uclient-fetch`, which every image has with TLS.
- **The manager answers four routes without a token,** as enrollment needs none (0033): `/install`, the script; `/install/manager.crt`, its certificate; `/install/manifest`, its own release's agent bundle (0079) and LuCI's page; and `/install/files/<sha256>`, one of their files. None of it is secret. A file is handed out only by a SHA-256 the manifest names.
- **The script names the manager it came from.** The manager fills in `https://` and the request's Host. A Host that is not a DNS name or an address, with an optional port, is refused, because it goes into a shell script.
- **The script fetches the certificate first and prints its fingerprint.** Everything after is fetched over TLS pinned to that certificate. Each file is checked against the manifest's SHA-256, and against the places Aeolus's files have. Nothing is put in place until every file has been fetched and checked.
- **It then joins the AP to that manager** through `aeolus-enroll`, with the uplink the default route leaves by. Options go after `sh -s --`:
  - `-u <port>` names the uplink;
  - `-f <fingerprint>` refuses a certificate that is not that one;
  - `-n` installs without joining.
  Where it can't tell the uplink, it installs and says how to finish.
- **Files a package owns are left alone.** With `aeolus-agent` installed, the agent updates itself from the manager (0079); with `luci-app-aeolus` installed, the page is the package's.
- **Any OpenWrt with ucode,** apk or opkg. `aeolus-packages` speaks both, and `aeolus-setup` now first installs the modules the agent can't start without. An AP given only the files lacks the package's dependencies.

### Joining a manager by hand

- **`aeolus-enroll`, a new agent file, joins the AP to a manager a person names.** It takes the manager's DNS name or IP address (port 8443 unless one is given), or a URL. It fetches the manager's certificate and shows its SHA-256. It joins only once given the fingerprint, which must match, and then runs `aeolus-setup`. `aeolus-enroll start` does the same in the background, and `status` reports how it goes, for LuCI. A join outlasts a LuCI call, and the network restart for vxlan cuts one off (0057).
- **LuCI's Aeolus page, under Services,** shows:
  - this AP's manager, uplink, pinned certificate and agent;
  - a blank for the manager's name or address;
  - a list of the AP's wired ports and bonds for the uplink, with the current or guessed one chosen;
  - **Fetch its certificate**, which shows the fingerprint;
  - a blank to paste the expected fingerprint, or a box to tick that it is right;
  - **Join this manager**, which starts the join and follows its log.
- **Its ACL lets LuCI run `aeolus-enroll` and nothing else:** `status` and `ports` to read, `cert` and `start` to write.
- **The page comes with the web installer where LuCI is installed, or as `luci-app-aeolus`,** which depends on `aeolus-agent` and `luci-base`. A fleet update never carries it, because a bundle holds only agent files (0079). Its own `keep.d` list keeps it over a sysupgrade, as the agent's keeps the agent.
- **The manager says its fingerprint, for the person to compare:** `aeolus fingerprint` on the manager's host, and its log when it starts. A browser shows the same SHA-256 for the manager's page.

### A manager by name

- **`aeolus-enroll` adds a manager's name to dnsmasq's `rebind_domain`.** It records the name in `aeolus.agent.rebind` too, the list the renderer keeps of the names it added (0069). Once the agent runs, the name is the agent's own, and it is taken out again if the manager changes.
- **A name the AP cannot look up is reported as that,** not as a manager that didn't answer.

## Consequences

- **A plain OpenWrt AP is one command from being managed:** that one `wget`, or the LuCI page after installing the packages. Neither needs SSH access to the manager or a certificate copied by hand.
- **The first fetch trusts the network once.** The script comes over TLS that nothing checks, so whoever can change it in transit can change what it does; `-f` can't guard against that. That is trust on first use, as a certificate copied by hand trusts the copy. Where that matters, install the packages and join from LuCI or `aeolus-enroll`: there the fingerprint is checked by code already on the AP.
- **A wrong manager still gets nothing.** An AP that joins, whichever manager it joins, waits in Landing Zone until a person adopts it (0033).
- **Checked in the lab (2026-10-09), each part on its own:**
  - On PumphouseAP, the certificate fingerprint `aeolus-enroll` computes (ucode's `b64dec`, then `sha256sum`) matched `openssl x509 -fingerprint -sha256` on the manager.
  - `uclient-fetch --ca-certificate=` pinned to the manager's certificate fetched by IP address, and refused under another CA.
  - LuCI's own package manager grants exec with arguments as `"<command> <sub> *"`, the form this page's ACL uses.
- **Not yet run end to end:** the manager at 192.168.20.60 runs a release without `/install`.
- **opkg is untested on hardware:** every lab AP runs 25.12, with apk.
- **0080's open route for the package is answered with files, not an `.apk`.** The web installer installs the manager's bundle, and apk doesn't list it. An AP that wants apk to know installs the packages instead.

## Open

- Discovery (0033): with a DNS name or a DHCP option, the web installer and this page become the fallback, not the way in.
- A manager with a CA-signed certificate. `/install/manager.crt` serves the manager's own certificate, which the AP pins. A CA's certificate would need serving instead, as 0033 has it.
- Limiting requests to `/install`, with the same limits as enrollment (0033).
- **Older OpenWrt** (Griff, 2026-10-09). In time, an AP whose firmware can't do the newer features should still get the basics: its SSIDs and the control plane (enroll, poll, check, apply with revert, report). What it can't do would be left out, and the manager would say so, instead of the whole config being held as it is now (0057). The installer already speaks opkg. The agent needs ucode and its modules, though, and which releases' feeds have them is still to be checked. An AP older than ucode would need another agent.
