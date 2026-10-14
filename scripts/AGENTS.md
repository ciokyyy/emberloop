# AGENTS.md — scripts

## Purpose

Live probes that only mean something against running agents: the read-precedence comparison, the
claude-effort probe (two claude Sessions, effort low vs max, answer the same reasoning prompt through the web Daemon;
prints thinking totals for a human verdict on the SDK effort option).
Live probes against the web Daemon: the runs-actions probe (a 2-ticket Run
through `/api/runs` against real agents, exercising the per-ticket steer,
park, retry and discard routes plus the ticket table the needs-you banner
reads), the compress probes (one script each for pig SDK, pig-over-ACP and
claude: each plants a fact, compresses by Handoff restart, then asks for
the fact from the new handle), and the Jev probe (the real decision
endpoint through `TestLiveJevDecider`, behind `EL_LIVE_JEV=1`).
Run-upgrade live probes run by hand against real Backends, never in the check:
`run-upgrade-reviewer-submit-probe.sh` observes a real Pig Reviewer's typed
submit Event and fresh verdict; `run-upgrade-chain-probe.sh` exercises a real
Claude auth failure and Pig Chain fallback; `run-upgrade-switch-probe.sh`
switches and resumes a paused Ticket with real Backend/model settings;
`run-upgrade-two-specs-probe.sh` starts two simultaneous Runs on separate spec
branches. They share `run-upgrade-probe-lib.sh` for an isolated Daemon,
temporary repo and cleanup.
Quota live probes exercise Claude and ChatGPT Plus separately through an
isolated local Daemon: `claude-quota-live-probe.sh` reads the host Claude
OAuth login and `chatgpt-quota-live-probe.sh` reads pig's `openai-codex`
login. Each requires its provider's explicit `EL_LIVE_*_QUOTA=1` gate,
prints only the normalized plan/windows/percent/reset values, and keeps the
Daemon state and quota cache in a temporary directory. The upstream calls
are read-only usage GETs; neither probe refreshes or writes a login.

Plus one offline measurement: `explore-replay.sh` replays web-v1 exploration
patterns and prints baseline vs guided call counts (no agents, no network).
`web-dev.sh` runs the dev Daemon in the foreground. Each script names its
prerequisites in its header comment.

## Ownership

Owns the shell scripts in this directory. Anything a script verifies in
another package is owned there.

## Local Contracts

- Scripts run only by hand; live probes use temporary state. Agent probes
  need live sessions, while quota probes need the real provider login; no
  script is part of an automated check and none runs inside ticket work.

## Work Guidance

- Read the header before running; confirm the named binaries exist first.

## Verification

No check.

## Child DOX Index

No child docs.
