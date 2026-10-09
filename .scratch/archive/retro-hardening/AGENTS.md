# AGENTS.md — .scratch/archive/retro-hardening

## Purpose

The spec and ticket area for the retro-hardening run: `spec.md` (the
decisions every ticket implements) and `issues/` (one file per ticket with
scope, acceptance checkboxes, and status). Workers implement from these
files; the merge gate judges against them.

## Ownership

Owns `spec.md` and `issues/` in this directory. Code changes from these
tickets land in `el/` and the extension directories, owned by their
docs.

## Local Contracts

- The spec's Hard limits bind every worker in this run; the spec decides
  conflicts between a ticket and the limits.
- Ticket files carry status and checkboxes; only a merge into the
  integration branch moves them.

## Work Guidance

- Read the spec before the ticket; the ticket states what, the spec states
  why and what is out of scope.

## Verification

No check.

## Child DOX Index

No child docs.
