# 0043. Getting back in when a token is lost

- Status: Accepted
- Date: 2026-10-02
- Proposed by: Claude, when Griff needed a token for the UI; accepted by Griff
- Refines: 0024, 0030

## Decision

- **Root on the manager host can issue a new token for an account** with `aeolus token -account <id>`. The trust is the same as for `aeolus init`: whoever controls the host controls the manager.
  - The token is printed once in that terminal and never passes through the API, the UI or Claude.
  - The change is logged under the account itself, which may always issue and revoke its own tokens (0030), with the reason "issued on the manager host".
  - `-revoke-others` also revokes the account's other tokens, for a token that is lost rather than just forgotten.
- **One process at a time.** The change log keeps its state in memory, so a second process writing beside the service would leave the service unaware of the change. The log now takes an exclusive lock on its database, and the command asks for the service to be stopped first. APs keep running their config meanwhile.
- **As the service's user.** `/usr/local/bin/aeolus` runs the manager as the `aeolus` user when called by root, so no file it writes is out of the service's reach.

## Consequences

- A lost admin token costs a minute of downtime, not the Org.
- Two managers can never run on one database by accident.
