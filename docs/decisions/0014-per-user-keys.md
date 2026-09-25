# 0014. Per-user keys have their own channel

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Context

Per-user keys give each person or unit its own passphrase on a shared SSID, and each key decides which segment the client lands on. They change constantly (move-ins, move-outs, revocations). If they were part of the config, every key change would bump every affected AP's version and run the full pull, render, check and apply cycle of 0007 and 0008, and applying Wi-Fi config can briefly drop clients.

## Decision

- Keys are hosted on the manager, in their own store with their own version number, separate from config.
- A dedicated key agent on each AP checks the manager continuously and updates only the AP's key data. It never downloads the full config.
- Updating keys must not drop connected clients (hostapd can reload its key file without a Wi-Fi restart).
- Key changes go through the same API and are recorded in the same change log as everything else (0003, 0009).
- APs keep their last key set. If the manager is down, existing keys keep working; new keys and revocations wait until it returns.

## Open

- Delivery details: long-poll (the AP holds a request open until something changes) versus short polling, and incremental changes versus the full key list per network.
- Whether key updates skip the render/check step of 0008, since keys are validated when they are entered.
