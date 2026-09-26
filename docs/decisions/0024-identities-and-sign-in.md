# 0024. Identities and sign-in

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Decision

- Every actor has its own account: each person, each automation, and Claude. Nobody acts under someone else's name (0009).
- Accounts authenticate to the API with API tokens. The manager stores only a hash of each token.
- Sign-in for the UI with a password and a one-time code comes later, and single sign-on after that.
- Account changes (creating an account, issuing or revoking a token, granting a role) are changes like any other and go through the change log.

## Consequences

- "Who gave whom access, and when" has the same exact answer as "who changed what".
