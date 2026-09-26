# 0033. AP enrollment and identity without a CA

- Status: Accepted
- Date: 2026-09-26
- Proposed by: Claude, after 0032 set aside the CA; accepted by Griff
- Refines: 0007

## Decision

- **Enrollment.**
  - An AP with the agent installed calls the manager's enrollment endpoint with its hardware facts: MAC addresses, model, OpenWrt version, and radio capabilities (0008).
  - The endpoint needs no token.
  - The manager records the AP in Landing Zone (0032) and answers with an **AP token**, which is shown once, just like an account token.
  - The manager keeps only a hash of the token (0024).
- **Identity.**
  - The AP sends its token on every poll and report (0007).
  - A token identifies exactly one AP, and every report and change is recorded under that AP's name.
  - Removing an AP revokes its token.
- **Trusting the manager.**
  - The agent installer puts the manager's certificate on the AP, and the agent pins it.
  - Because it is pinned, an AP never talks to a manager it was not set up for.
  - When a CA arrives later, the pinned certificate is replaced by the CA's.
- **Why enrollment is safe without a CA.** Anyone who can reach the endpoint can enroll a device, but an enrolled device only lands in Landing Zone, which gives it nothing. Configs, and the secrets in them, reach an AP only after a person adopts it. The manager shows each enrollment's source address, MACs and time, so the person can tell a real AP from an impostor.

## Open

- How APs find the manager: a DNS name first, with a DHCP option as an override for sites that need one.
- **Constraint (verified 2026-09-26):** Aeolus discovery must never use DHCP options 43, 60, 138 or 224. OpenWiFi (uCentral) APs request those by default and find their gateway through 224, so reusing any of them could send an OpenWiFi AP to Aeolus or confuse its discovery. A DNS name cannot affect them.
- ~~Detecting unconfigured OpenWiFi APs.~~ Resolved by 0034.
- Limiting enrollment requests, and clearing Landing Zone of devices that never get adopted.
