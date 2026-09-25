# 0011. Code and deployment

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- Code lives in the private GitHub repo `ChristopherGriffin/aeolus`.
- Changes are made in the repo, on a branch, with tests, then deployed. Nobody edits code by hand on the manager host.
- The manager host pulls tagged releases with a read-only deploy key.
- Only state lives on the host, in `/var/lib/aeolus`, and it is backed up. The host can be rebuilt from the provisioning script, which belongs in this repo.

Correction (2026-09-25): the first version named `texaswifiguy/aeolus`. No GitHub account or organization by that name exists, so the repo was published under Griff's account, and Griff kept it there.

## Prototype host

- Aeolus: LXC 118 on powermox, Debian 13, VLAN 20, `aeolus.symtus.com` (192.168.20.60, DHCP reservation).
- Code `/opt/aeolus`, config `/etc/aeolus`, state `/var/lib/aeolus`.
