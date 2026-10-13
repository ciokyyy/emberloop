# AGENTS.md — .scratch/workspace-overhaul

## Purpose

One combined spec (`spec.md`) for everything agreed in the 2026-10-13 grilling Session: Quota, Speed, Providers, Project scope, the draft Project pick, the Settings redesign, and a stall-proof Run test suite. Tickets are not cut yet; they go in `issues/` (keep any wave map outside `issues/`, as the Run driver reads every file there as a ticket).

## Ownership

Owns `spec.md` here. It copies, and does not replace, `.scratch/quota-tab/spec.md` and `.scratch/projects-providers-settings/spec.md`; those keep their own tickets and stay unedited. Code changes land in `emberloop/` and `emberloop-web/`, owned by their docs.

## Local Contracts

- This spec decides conflicts with the two source specs.
- Run parts one at a time: Parts 1, 3, 4 and 6 all edit the Rail, `areas.js` and `App.svelte`. Part 7 goes first.

## Work Guidance

- Read the spec before cutting tickets.

## Verification

No check.
