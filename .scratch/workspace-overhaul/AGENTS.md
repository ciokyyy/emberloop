# AGENTS.md — .scratch/workspace-overhaul

## Purpose

The spec and ticket area for the workspace overhaul agreed in the 2026-10-13 grilling Session: `spec.md` (Quota, Speed, Providers, Project scope, the draft Project pick, the Settings redesign and a stall-proof Run test suite), `issues/` (one file per ticket with scope, blockers, status and acceptance checkboxes) and `ticket-map.md` (blocker table and waves; kept outside `issues/` because the Run driver reads every file there as a ticket).

## Ownership

Owns `spec.md`, `ticket-map.md` and `issues/` here. The spec copies, and does not replace, `.scratch/quota-tab/spec.md` and `.scratch/projects-providers-settings/spec.md`; those stay unedited apart from the quota-tab ticket move. Code changes land in `emberloop-web/`, owned by its docs.

## Local Contracts

- This spec decides conflicts with the two source specs, and between a ticket and itself.
- Part 7 goes first: every ticket blocks on 03 directly or through its chain. Quota-tab tickets 03 to 16 moved here as Part 1; quota-tab keeps only its done tickets 01 and 02.
- Ticket files carry status and checkboxes; only the driver or a merge into the integration branch moves them.

## Work Guidance

- Read the spec before the ticket.

## Verification

No check.
