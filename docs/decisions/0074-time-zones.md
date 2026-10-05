# 0074. Time zones from a list

- Status: Accepted
- Date: 2026-10-05
- Proposed by: Griff: a time zone can be set per folder, picked from a dropdown as on Linux. Written up by Claude, and accepted with the merge
- Refines: 0040, 0052

## Context

- **`system.tz` can already be set on any Locations folder or AP,** and inherits like any other field. The System tab's editor offers it as free text, so any name is taken.
- **An AP needs two things for a zone (0040):**
  - its name, UCI's `zonename`;
  - its rule as a POSIX `TZ` string, UCI's `timezone`, such as `CST6CDT,M3.2.0,M11.1.0`. That's what sets the AP's clock: OpenWrt ships no zoneinfo files.
- **The agent looks the rule up in LuCI's table,** `/usr/share/ucode/luci/zoneinfo.uc`.
  - An AP without LuCI gets the name but no rule. It keeps running on UTC, and nothing says so.
  - A name that isn't in the table, a typo, has the same effect.
  - LuCI's table changes with its version. On 2026-10-05 the lab APs' copies differed for America/Vancouver: `PST8PDT,M3.2.0,M11.1.0` on luci-base 26.117, and `MST7` on 26.180, after British Columbia's move to permanent daylight time.
- **Why it matters now:** RRM's window for planned moves (0073) is in the AP's local time. The lab APs have no zone set, so 02:00–05:00 is GMT.

## Decision

- **Aeolus carries its own table of zones:** each IANA name, with its POSIX rule.
  - It's OpenWrt's own table, LuCI's `zoneinfo.uc` from luci-base 26.180, generated from tzdata, plus `UTC`.
  - It's one file in two places: the manager embeds `internal/schema/zones.uc`, and the agent ships `aeolus/zones.uc`. A test holds them byte for byte alike.
- **The manager takes only a name from the table, when one is set.** The schema marks `system.tz` with `x-aeolus-enum: zones`. A change setting a name outside the table is refused, and the schema's description gives the UI the list.
  - A name set before the list was, such as an alias like `US/Eastern`, stays as it was: a whole config is not checked against the list, so no AP's config is held for it. It renders as before: the name, with LuCI's rule if LuCI has one. Setting it again means picking from the list.
- **The System editor offers it as a dropdown,** grouped by region as Linux lists zones: UTC first, then Africa, America and the rest, each zone by its city. It's set on any folder or AP, and inherits as before.
- **The agent looks the rule up in its own table,** and in LuCI's only if its own is missing. So every AP renders the same rule for a name, LuCI or not.
- **The render check holds the AP to both:** `zonename` is the name, and `timezone` the table's rule, for a name in the table.

## Consequences

- A zone set anywhere sets the clocks below it, on any AP, with no LuCI needed.
- A rule change in tzdata reaches the APs with an Aeolus release, the table copied again from a newer luci-base.
- The schema gets one marker, not 446 names: the list lives in the table.
- No zone is set in the lab today, so none needs moving to the list.
