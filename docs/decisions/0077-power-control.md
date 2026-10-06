# 0077. Automatic power control (APC)

- Status: Proposed
- Date: 2026-10-05
- Proposed by: Griff: each AP looks for its three nearest neighbours (a number that can be set) and raises its power until they all hear each other at −70 dBm or better. It's APC, beside RRM's channels. Written up by Claude
- Refines: 0073

## Context

- **0073 left power to a later decision:** transmit power that brings each AP's three neighbours to −70 dBm or better, the signal seamless roaming wants.
- **The pieces are there:**
  - RRM (0073) knows each neighbour on each band, and how strongly each hears this AP (`their_signal`, from its hellos).
  - Since v0.47.0 each AP reports each radio's power.
- **The lab check, 2026-10-05:**
  - **A running radio's power changes at once, with no restart,** by `iw phy <phy> set txpower fixed <mBm>`, and goes back to its configured power with `auto`. The OpenWrt One (mt76) takes only the per-radio form, `iw phy`; the R7800 (ath10k) takes both.
  - **No client noticed:** the pumphouse's four 2.4 GHz clients and the office's eight stayed on, and hostapd logged nothing.
  - **What a radio reports is a ceiling, not what it sends.** The pumphouse's 2.4 GHz radio reports 30 dBm, the US limit, but the office heard it at:

    | Pumphouse's setting | The office heard it at |
    | --- | --- |
    | 30 dBm (auto) | −74 dBm |
    | 24 dBm | −78 dBm |
    | 12 dBm | −90 dBm |

    So below about 24 dBm the setting moves the signal decibel for decibel. Above that the radio tops out, near 26 dBm: about 400 mW, not 1 W.
  - **netifd sets each radio's power when it starts it:** `fixed` where `txpower` is set, else `auto`. So a power held at run time is lost whenever the radio restarts.

## Decision

- **APC is its own setting, beside RRM, in Locations:**
  - `apc.enabled`, off unless set;
  - `apc.neighbours`, how many neighbours each AP looks for, 1 to 6, 3 unless set;
  - `apc.target`, how strongly the weakest of them must hear it, −85 to −50 dBm, −70 unless set.

  APC needs RRM's neighbours. A config with APC on and RRM off is held.
- **The rule,** for each band, on each radio whose power Aeolus doesn't set:
  - **The neighbours that count** are the ones on the band whose hellos say how strongly they hear this AP. The count strongest of them are what this AP aims for.
  - **Raise the power one step** when fewer than the count hear it at all, or when the weakest of them hears it below the target. So an AP looks for its neighbours, and makes sure they hear it.
  - **Lower it one step** when all of them hear it at least 6 dB above the target. So a crowded floor doesn't creep up to full power.
  - **Otherwise hold.**
- **Steps:**
  - A step is 3 dB, between a floor of 8 dBm and the radio's ceiling, the power it reports when not held down.
  - A radio takes at most one step every 10 minutes, so its neighbours' readings of it, smoothed over several visits, can settle between steps.
  - The loop works from what the neighbours hear, never from the setting, so steps above what the radio can really send just pass.
- **How:** `iw phy <phy> set txpower fixed`, in RRM's daemon, a minute after it starts.
  - **The ceiling** is read by letting the radio go back to `auto` and reading what it reports, when APC takes the radio over.
  - **The daemon holds the power again whenever the radio's network is made anew,** as netifd has put it back. It keeps the power across its own restarts, as it does its moves.
  - **Off, the radio goes back to `auto`, its configured power.** With a power set by Aeolus, it is left to netifd, which already applied it.
  - **A radio that shares its wiphy with another is left alone:** the power is set per wiphy.
- **What the AP reports:** for each radio APC holds, its power and ceiling, how many of the neighbours it looks for hear it, the weakest of them, the last step, and why. The reasons are `new`, `looking`, `below`, `ceiling`, `target`, `above`, `floor` and `failed`.
- **Where it shows:**
  - The Neighbours view has APC's switch, its count and target, and each AP's radios under it.
  - The band cards show the power APC holds.
- **Reported power says "up to":** a radio's reported power is its ceiling, so the cards show "up to 30 dBm" where APC doesn't hold it.

## The trial, 2026-10-05

On the office AP, with the step interval cut to a minute and 2.4 GHz left out (it carries Sweet_Spot_IoT), APC was turned on by hand: one neighbour, −70 dBm. The 5 GHz radio was seeded with a kept power of 16 dBm.

- **It took the radio over at 16 dBm,** having read its ceiling, 25 dBm.
- **It climbed a step a minute:** 19, 22, then 25 dBm. Each step was "below": the pumphouse heard it at −88 dBm.
- **It then held, "ceiling".**
- **Turned off, it let go:** a power forced to 19 dBm by hand went back to 25.
- **2.4 GHz stayed at 25 dBm, with its eight clients.**

## Consequences

- With APC on, each AP's three nearest neighbours hear it at −70 dBm or better where they can, and a dense floor runs below full power.
- **Where an AP has fewer neighbours in range than the count,** it raises its power to its ceiling, still looking. The lab has two APs, so with the count at 3 both would run at their ceilings. The count can be set lower.
- Clients don't notice a step: there's no restart.
- **Stopped for good, the daemon leaves a radio at the power it held** until the radio next starts and netifd puts back its configured power.
- **APC and RRM's channel moves both change how neighbours hear each other.** A step moves a channel's rating little, and moves need ten minutes of steady reason, so they shouldn't chase each other.

## Open

- The step, the floor, the 6 dB margin and the 10 minutes.
- Whether a client's own signal, such as the weakest client heard, should keep an AP's power up.
