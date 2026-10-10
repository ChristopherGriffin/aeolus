# 0104. Asking an AP to act once: Locate, Restart Wi-Fi, Reboot

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Claude, as one of the features Wi-Fi managers commonly have that Aeolus lacked (Griff, "create features common in these systems")
- Refines: 0007, 0030, 0040

## Context

- **Aeolus tells an AP only what it should run.** Its config is versioned, checked and applied with a revert (0008, 0040).
- **A person also needs an AP to do something once:**
  - blink its LEDs, to find it on a shelf or ceiling;
  - restart its Wi-Fi;
  - reboot it.
- **None of these is config.** Each is asked, done once, and finished.

## Decision

- **An action is `locate`, `restart-wifi` or `reboot`,** asked of one AP by someone with operator on it, as changing it needs (0030). The manager keeps each in its conditions database: who asked, when, and what came of it.
  - It is pending until the AP says it did it or could not.
  - One the AP has not taken up within 10 minutes expires, so an AP that comes back later does not reboot because someone asked an hour ago.
  - A second ask of the same kind while one waits is the same action.
- **The AP hears of it on its next poll.** Every config poll's answer, a 304 included, says in `Aeolus-Actions` how many wait. The AP then fetches them (`GET /v1/ap/actions`), does each, and says what came of it (`POST /v1/ap/actions/{id}`).
  - **locate:** blinks every LED for a minute, then puts each back as it was, and has OpenWrt set its own LEDs again. It was run on PumphouseAP: all 11 LEDs blinked, and every trigger came back.
  - **restart-wifi:** runs OpenWrt's `wifi`.
  - **reboot:** waits five seconds, so the manager hears first. It never reboots during an apply, whose revert would then not run.
- **`POST /v1/aps/{ap}/actions {kind}` asks one; `GET /v1/aps/{ap}/actions` lists the latest.** The MCP adapter offers `ap_action`.
- **On the AP's Overview, an Actions panel** has the three buttons, Restart Wi-Fi and Reboot each confirmed first, and the latest actions, each with who asked and what came of it.

## Consequences

- An action takes up to a poll, about a minute, to happen.
- An AP whose agent is older than this one ignores the header, and its actions expire.
- More actions fit here as they are needed, such as a speed test from the AP, or reading its log.
