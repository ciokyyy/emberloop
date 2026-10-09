# AGENTS.md — .scratch/archive/enderbloom-web

## Purpose

The spec and ticket area for Enderbloom Web: `spec.md` (the decisions every ticket implements) and `issues/` (one file per ticket with scope, acceptance checkboxes, blockers, and status).

## Ownership

Owns `spec.md`, `spike-findings.md` and `issues/` in this directory. Code from these tickets lands in the `el-web/` module, owned by `el-web/AGENTS.md`. Terms live in the root `GLOSSARY.md`; the backend decision lives in `docs/adr/0001-claude-via-acp-pig-via-sdk.md`.

## Local Contracts

- Ticket 01 (spike) blocks every other ticket. Its answers live in `spike-findings.md` and may change tickets 03 to 05.
- Tickets use the root `GLOSSARY.md` vocabulary.
- Ticket files carry status and checkboxes; only a merge into the integration branch moves them.

## Work Guidance

- Read the spec before the ticket; the ticket states what, the spec states why and what is out of scope.

## Verification

No check.

## Child DOX Index

No child docs.
