# AGENTS.md — .scratch/ui-overhaul

## Purpose

The spec and ticket area for the UI overhaul in `emberloop-web/web`: one design language across desktop and phone, scroll stability, message actions, per-message queue control and Subagent reports as dismissible notices. `spec.md` holds the decisions; `issues/` holds one file per Ticket (`code` or `design`) with blockers, status and acceptance boxes.

## Ownership

Owns `spec.md` and `issues/`. Code lands in `emberloop-web/` (one Daemon seam: queue control) and `emberloop-web/web/`, owned by their docs.

## Local Contracts

- The spec decides conflicts between a Ticket and itself.
- Ticket files carry status and boxes; only the driver or a merge moves them.
- Order follows `Blocked by`; ticket 01 precedes every other design Ticket.
- Design Tickets run on the Designer Chain and are judged at 390 px and 1280 px.

## Verification

No check.

## Child DOX Index

- `spec.md` — UI overhaul decisions and contract
- `issues/` — Tickets 01 to 13
