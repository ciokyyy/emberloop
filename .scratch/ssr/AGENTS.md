# AGENTS.md — .scratch/ssr

## Purpose

The spec and ticket area for server-side rendering in `emberloop-web`: the Go route table that replaces the `isSPARoute` allowlist, the embedded JS engine that renders the Svelte shell, per-route prefetch with a Boot payload, and the dependency updates that land first. `spec.md` holds the decisions every ticket implements; `issues/` holds one file per ticket with scope, blockers, status and acceptance checkboxes.

## Ownership

Owns `spec.md` and `issues/` in this directory. Code changes from these tickets land in `emberloop-web/` and `emberloop-web/web/`, owned by their docs.

## Local Contracts

- The spec decides conflicts between a ticket and itself.
- Ticket files carry status and checkboxes; only the driver or a merge into the integration branch moves them.
- Ticket order follows the `Blocked by` lines.
- Ticket 03 is a gate: a failed spike switches tickets 04 to 06 to the fallback in the spec.

## Work Guidance

- Read the spec before the ticket.

## Verification

No check.
