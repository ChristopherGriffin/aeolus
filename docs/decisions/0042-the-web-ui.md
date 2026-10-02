# 0042. The web UI

- Status: Proposed
- Date: 2026-09-27
- Proposed by: Claude, to start M6 from the mockup
- Refines: 0003, 0024, 0026, 0029

## Decision

- **Aeolus serves the UI itself**, from its own address, and the UI uses the same API everything else does (0003). There is no separate web server and no second way into the data.
  - A browser opening `https://<manager>:8443/` gets the UI.
  - API clients asking for JSON at `/` still get the pointers they get today.
- **No build step and no framework.** The UI is plain HTML, CSS and JavaScript modules, embedded in the `aeolus` binary.
  - There is nothing to install, bundle or keep patched.
  - It loads nothing from the internet, not even fonts, so it works on a network with no way out.
- **It looks like the mockup.**
  - A dark header with the Org's name.
  - Locations, Services, APs, Library and Changes.
  - A tree on the left.
  - Each value shown with where it comes from (set here, from a folder above, locked, branch baseline), and the Overrides menu.
- **Signing in is with the person's own API token** until 0024's password and one-time code arrive.
  - The token is kept for the browser tab by default, or on that browser if the person asks.
  - Every request carries it, exactly like any other client.
- **Safe by construction.**
  - Everything shown comes from the API and is written as text, never as HTML, so an SSID or a reason cannot inject anything.
  - The page allows scripts, styles and connections only from Aeolus itself (Content-Security-Policy), and cannot be framed.
- **Read-only first (M6 part 1):**
  - both trees, each node with its values, origins, overrides, locks and problems;
  - each AP with its sync state, last seen, running version, latest check, apply and state report, its history (including the UCI it ran, secrets blanked, 0041) and its enrollment facts;
  - Landing Zone with the APs waiting there;
  - all APs at a glance;
  - the library;
  - the change log, newest first.
- **Editing next (M6 part 2).**
  - Setting values, locks, breaks, moving and adopting APs, assigning services and editing the library, each previewed first and made with a reason.
  - The page guard (0029): a person cannot leave a page they are editing while it breaks a rule.
- **A new read for the fleet view.** `GET /v1/aps` lists every AP the caller can view: its place, version, whether its config passes, and its condition (last seen, running version, in sync). This avoids one request per AP.

## Consequences

- Updating Aeolus updates the UI, and they always match.
- A person sees exactly what their role allows, because the UI can only see what the API shows them (0025).
- Pasting a token is a stopgap. Sign-in with a password and one-time code (0024) replaces it without changing anything else.
