# AGENTS.md — .scratch/run-upgrade

## Purpose

The spec and ticket area for the Run upgrade in `emberloop-web` and `emberloop`: Reviewer `submit` tool, Dispositions, prompt injection, Chains and Switch, spec branches, hygiene guards, and `find` graph hits. `spec.md` holds the decisions every ticket implements; `issues/` holds one file per ticket with scope, blockers, status and acceptance checkboxes.

## Ownership

Owns `spec.md` and `issues/` in this directory. Code changes from these tickets land in `emberloop/` and `emberloop-web/`, owned by their docs. Live probes land in `scripts/`.

## Local Contracts

- The spec decides conflicts between a ticket and itself.
- Ticket files carry status and checkboxes; only the driver or a merge into the integration branch moves them.
- Ticket order follows the `Blocked by` lines.
- The terminal Run loop in `emberloop/` is out of scope; do not change it.

## Work Guidance

- Read the spec before the ticket.

## Verification

No check.
