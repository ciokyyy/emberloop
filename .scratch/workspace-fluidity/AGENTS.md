# AGENTS.md — .scratch/workspace-fluidity

## Purpose

The spec and ticket area for workspace fluidity in `emberloop-web`: the Ladder (peak- and credit-aware virtual model routing, DeepSeek Flash through Relace, credit quota cards), a compact Active/All sidebar, streaming and history performance, fluid navigation with Project-scoped Settings and Memory. Part E (offline cache with TanStack DB) is listed in the spec with no Tickets. `spec.md` holds the decisions; `issues/` holds one file per Ticket (`code` or `design`) with blockers, status and acceptance boxes.

## Ownership

Owns `spec.md` and `issues/`. Code lands in `emberloop-web/` (Daemon seams: Ladder provider, credit flag, quota, settings) and `emberloop-web/web/`, owned by their docs.

## Local Contracts

- The spec decides conflicts between a Ticket and itself.
- Ticket files carry status and boxes; only the driver or a merge moves them.
- Order follows `Blocked by`.
- Design Tickets run on the Designer Chain and are judged at 390 px and 1280 px.
- The Relace key lives only in the pig agent directory; no file here or in the repo holds it.

## Verification

No check.

## Child DOX Index

- `spec.md` — workspace fluidity decisions and contract
- `issues/` — Tickets 01 to 16
