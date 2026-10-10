# AGENTS.md — .scratch/handoff-compress

## Purpose

The spec and ticket area for Handoff compress in `emberloop-web`: `spec.md` (the decisions every ticket implements) and `issues/` (one file per ticket with scope, blockers, status and acceptance checkboxes).

## Ownership

Owns `spec.md` and `issues/` in this directory. Code changes from these tickets land in `emberloop-web/`, owned by its doc. Live probes land in `scripts/`.

## Local Contracts

- The spec decides conflicts between a ticket and itself.
- Ticket files carry status and checkboxes; only the driver or a merge into the integration branch moves them.
- The terminal compress in `emberloop/` is out of scope; do not change it.

## Work Guidance

- Read the spec before the ticket.
- Prior art: `.scratch/archive/context-compress/spec.md`.

## Verification

No check.
