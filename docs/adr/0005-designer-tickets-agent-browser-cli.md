---
status: proposed
---

# Design Tickets run on a Designer that sees UI through the agent-browser CLI

## Decision

A Ticket has a type, `code` or `design`, set by a `**Type:**` header line (default `code`). A `design` Ticket is run by a Designer on its own Chain role (`worker-design`) and is reviewed by the spec, standards and design Reviewers (`reviewer-design`, its own Chain). Work that needs logic and look is split into a `code` Ticket and a `design` Ticket blocked by it. Designers and design Reviewers see rendered pages through the `agent-browser` CLI (Vercel, pinned release binary installed by `el deps`), driven from `bash` or `execute`. No MCP server and no Daemon-owned screenshot tool is added. The Daemon sets per-Session environment (session name, localhost-only allowed domains, screenshot directory, idle timeout) and the el bin dir on PATH, closes the Session's browser when the Session ends, and injects a short snapshot-first skill into Designer and design Reviewer prompts; other roles get a one-line mention.

## Context

Cheaper models implement backend logic well but cannot judge or direct UI, and reviewers on the same models call any UI good. Routing visual work to a stronger model with the ability to look at the result addresses both gaps. The grok Backend has no image path, so it cannot run these Chains in v1.

## Rejected alternatives

- A global shared browser MCP (`@playwright/mcp`, `chrome-devtools-mcp`): the tool schema alone is large (agent-browser's MCP lists 29 tools, about 78 KB), and its surface lacks device and viewport emulation, console, network, a11y and computed styles, which the CLI has. Parallel Workers also need one isolated browser each, which is per-Session configuration either way.
- A Daemon-owned `screenshot` tool or a Go-native browser driver: Emberloop would own browser plumbing, orphan-process cleanup and tests for no gain over the CLI.
- Type inference from changed paths: guessy, and mixed Tickets defeat it.

## Consequences

- agent-browser is a vercel-labs project with a fast release cadence; the pin must be bumped deliberately. Playwright MCP is the fallback if its churn becomes a cost.
- Reviewer set becomes per Ticket type instead of a fixed pair.
- `to-tickets` must split design from code. The upstream skill library is overwritten by `el skills install`, so Emberloop ships an override in a second library.
- The Run preview type flip is stored in the Run file, because only the Run driver writes Ticket files.
- Research and measurements: `.scratch/designer-agents/research-browser-tooling.md`.
