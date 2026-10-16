# AGENTS.md — .scratch/designer-agents

## Purpose

The spec and ticket area for Designer agents in `emberloop-web`: Ticket types (`code`, `design`), Designer Workers and design Reviewers on their own Chains, the `agent-browser` CLI as every agent's way to inspect local UI, the `to-tickets` override that splits design from code, and removal of the grok Backend. `spec.md` holds the decisions every ticket implements; `issues/` holds one file per ticket with scope, blockers, status and acceptance checkboxes; `research-browser-tooling.md` holds the browser-tooling research (its Playwright MCP recommendation is superseded by `docs/adr/0005-designer-tickets-agent-browser-cli.md`).

## Ownership

Owns `spec.md`, `issues/` and `research-browser-tooling.md` in this directory. Code changes from these tickets land in `emberloop-web/`, `scripts/` and a new skill library, owned by their docs.

## Local Contracts

- The spec decides conflicts between a ticket and itself; the ADR decides tool choice.
- Ticket files carry status and checkboxes; only the driver or a merge into the integration branch moves them.
- Ticket order follows the `Blocked by` lines; ticket 01 (remove grok) goes first so later tickets carry no grok branch.
- Live probes (tickets 05 and 15) land in `scripts/` and never join the repo check.

## Work Guidance

- Read the spec and the ADR before a ticket; read the research only for browser-tool facts.
- Re-read the named code before editing: it moves faster than these tickets.

## Verification

No check.
## Child DOX Index

- `spec.md` — Designer agent decisions and implementation contract
- `issues/` — ticket sequence and acceptance criteria
- `research-browser-tooling.md` — browser tooling research; its Playwright MCP recommendation is superseded by ADR 0005
