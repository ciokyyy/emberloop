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
The Designer browser live probe, `browser-probe.sh`, proves Session env,
snapshot and screenshot `read` on Claude, Pig ACP and Pig SDK. The Designer
Run live probe, `designer-run-probe.sh`, proves a real code-then-design Run
through merge, browser snapshots/screenshots and cleanup. Both use temporary
Daemons/worktrees and are never part of checks; gates and models are below.
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
Plus one offline fake: `fakellm/` is a stdlib-only Go module serving scripted
OpenAI/Anthropic/Gemini endpoints for benchmarking harnesses (own README;
never calls real model APIs), with COOPERATE fuzzy tool-calling, FAULT:
failure injection (`--faults`/`--fault-rate`), and a mutable catalog
(`PUT /__catalog`, fetch counts in `/__stats`).
 Plus the `acpbench/` standalone Go module (stdlib only, own README): a
 black-box ACP client probe running thirteen scenarios against any agent
 command with one JSON report (schemaVersion + per-scenario durations +
 summary, plus a `compare` subcommand for markdown tables), plus an in-repo
 fake ACP agent its own `go test` covers (it simulates auth-gated sessions,
 real model discovery, namespaced tools, stub-side MCP invocation, and the
 FAULT provider-fault contract). Hand runs against real agents need a fake
 LLM endpoint.
`web-dev.sh` runs the dev Daemon in the foreground. Each script names its
prerequisites in its header comment.

## Ownership

Owns the shell scripts and the `fakellm/` and `acpbench/` modules.
Anything a script verifies in another package is owned there.

## Local Contracts

- Scripts run only by hand; live probes use temporary state. Agent probes
  need live sessions, while quota probes need the real provider login;
  `explore-replay.sh` needs only bash (no agents, no network) and `fakellm/`
  needs only its own module (no agents, no real model APIs). `acpbench/`
  self-tests against its in-repo fake agent; hand runs against real agents
  need a fake LLM endpoint. `browser-probe.sh` needs `EL_LIVE_BROWSER=1` and
  `EL_LIVE_CLAUDE_MODEL`, `EL_LIVE_PIG_ACP_MODEL` and `EL_LIVE_PIG_SDK_MODEL`;
  `designer-run-probe.sh` needs `EL_LIVE_DESIGNER_RUN=1`, a Claude or Pig
  harness, `EL_LIVE_DESIGNER_MODEL` and optionally
  `EL_LIVE_DESIGNER_CODE_MODEL`; it uses real Sessions and never joins the check.

## Work Guidance

- Read the header before running; confirm the named binaries exist first.

## Verification

No repo-level check. `fakellm/` and `acpbench/` verify themselves with
`go test ./...` inside their own modules.

## Child DOX Index

- `scripts/fakellm/README.md` — the fake LLM benchmark server
- `scripts/acpbench/README.md` — the ACP probe/benchmark module and scenarios
