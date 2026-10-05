# Spec: wick-owned skills library

## Problem

A wick session finds skills by absolute path under `~/.agents/skills` (`skillsDir`, `extension.go`). That path is machine-specific, the claude harness cannot read outside its cwd, and a bare `wick-pig --claude` session has no way to discover skills such as grill, research or wayfinder.

## Decision

wick owns a skills library at `~/.wick-pig/skills/<namespace>/<skill>/SKILL.md`, filled from `mattpocock/skills` into the namespace `matt-pocock`. Sessions reach it through a virtual `wick://` address in the `read` tool; reading `wick://matt-pocock` returns an index of the skills. The system prompt names that one address, so an open session can load any skill when asked ("grill this plan").

## Out of scope

Other namespaces (a `wick://local/` for the user's own skills), per-skill slash commands, auto-update, embedding skills in the binary.

## Tickets

22 install command → 23 wick:// read and index → 24 system prompt and skillPath.
