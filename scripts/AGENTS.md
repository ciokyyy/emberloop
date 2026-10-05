# AGENTS.md — scripts

## Purpose

Live probes that only mean something against running agents: the herdr
report lifecycle and idle checks, the read-precedence comparison, and the
tell-worker proofs (live and mid-turn). Each script names its prerequisites
in its header comment.

## Ownership

Owns the shell scripts in this directory. Anything a script verifies in
another package is owned there.

## Local Contracts

- Scripts run only by hand against live sessions, with temp directories for
  state; they are never part of an automated check and never run inside
  ticket work.

## Work Guidance

- Read the header before running; confirm the named binaries and panes
  exist first.

## Verification

No check.

## Child DOX Index

No child docs.
