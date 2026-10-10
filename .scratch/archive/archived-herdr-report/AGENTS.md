# AGENTS.md — herdr-report

## Purpose

A pig extension that teaches herdr a running pig pane is an agent: it
reports pane state so the pane appears in `herdr agent list` under its own
name and stays addressable by `herdr agent prompt`. It also carries the stop
gate (`gate.go`): a spawned agent cannot settle without calling submit, with
the shared decision living in `el pig-gate` so every harness follows
one set of rules. `voice.go` adds the register's per-turn reminder to every
turn (text from `el voice-reminder`).

## Ownership

Owns this directory as a separate Go module (`extension.go`, `gate.go` and
their tests). Agent identity and naming here; review and merge policy in
`emberloop/`.

## Local Contracts

- The extension reports state and gates settling only; no agent tools, no
  schema cost.

## Work Guidance

- Verify against the herdr version named in the package doc; the lifecycle
  probe is `herdr-report-lifecycle.sh` beside this doc (live agents only).

## Verification

 Archived; no check and not covered by the repo check.

## Child DOX Index

No child docs.
