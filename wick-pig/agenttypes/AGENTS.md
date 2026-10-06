# AGENTS.md — wick-pig/agenttypes

## Purpose

The built-in agent types wick spawns: `worker.md` (implements one ticket in
its worktree), `reviewer-spec.md` and `reviewer-standards.md` (persistent
reviewers), `scout.md` (facts with citations). Repos add or override types
under `.wick/agents` (see `agenttype.go`); the loader merges both sources.

## Ownership

Owns the four type files in this directory. Spawn policy (which roles may
spawn which types) lives in `spawn.go` with the main package doc.

## Local Contracts

- Type files carry name, description, tools, checkout, blocking, and status
  shape; read-only types (reviewers, scout) stay on `read`, `find`,
  `websearch` and `ctx_search` and judge from the review packet.

## Work Guidance

- Keep prompts operational and non-overlapping: ticket fit is the spec
  reviewer's, code standards the standards reviewer's. The DOX check (flag a
  contract or structure change with no owning-doc update, or a stale index)
  is the one shared duty both reviewers carry; review is where doc upkeep is
  enforced.

## Verification

No check; types are exercised through the spawn and review tests owned by
the main package.

## Child DOX Index

No child docs.
