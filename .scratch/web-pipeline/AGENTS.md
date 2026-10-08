# AGENTS.md — .scratch/web-pipeline

## Purpose

The spec and ticket area for the web pipeline: running the deterministic implementation Run from the browser UI. `spec.md` holds the decisions; `issues/` holds one file per ticket with scope, acceptance checkboxes, blockers and status.

## Ownership

Owns `spec.md` and `issues/` in this directory. Code from these tickets lands in `emberloop-web/`, owned by `emberloop-web/AGENTS.md`, with minimal exports in `emberloop/`, owned by `emberloop/AGENTS.md`. Terms live in the root `GLOSSARY.md`.

## Local Contracts

- Tickets use the root `GLOSSARY.md` vocabulary.
- Ticket files carry status and checkboxes; only a merge into the integration branch moves them.
- Core never imports web; web may import core; neither imports herdr into web.

## Work Guidance

- Read the spec before the ticket; the ticket states what, the spec states why and what is out of scope.

## Verification

No check.

## Child DOX Index

No child docs.
