# 0010. One codebase, two tiers

- Status: Accepted
- Date: 2026-09-25
- Decided by: Griff

## Context

Networks with little management hardware should be able to run the manager on an elected AP. This is wanted but not strictly required, so the design must keep it possible without depending on it.

## Decision

- The manager runs either on a server (Aeolus) or on an elected AP.
- APs cannot tell which kind of manager they are talking to: the API and the agent contract (0007, 0008) are identical on both tiers.
- The manager is written in **Go**: one self-contained binary, cross-compiled for x86 and for AP ARM/MIPS.
- Both tiers are tested from day one.
- Only APs with enough memory and storage are eligible to be elected (for example, the OpenWrt One: 1 GB RAM and an NVMe slot).
- Election is designed now and built later. Standby APs copy change-log entries past their last sequence number, and one takes over when the manager fails. Exactly one manager at a time is guaranteed by a proven consensus method (Raft), not a homemade one.

## Consequences

- The numbered, append-only change log (0009) is what makes replication and takeover possible.
- Features that only fit on a server must sit behind interfaces with an AP-tier alternative, as conditions storage does (0009).
