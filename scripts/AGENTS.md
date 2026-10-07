# AGENTS.md — scripts

## Purpose

Live probes that only mean something against running agents: the herdr
report lifecycle and idle checks, the read-precedence comparison, the
tell-worker proofs (live and mid-turn), the header-strip spike (claude
and pig usable under the wrapper), the input-queue probe (row, steer and
queue delivery, stale clear, raw mode, OSC colors and themes for claude
and pig, plus the startup-dialog swallow case), the borderless-look probe (the
tinted-block strip with the wickb themes, plus the restored rule and
frame for comparison), and the strip-look probe (each symbol preset and
separator under wrap, with the Nerd Font codepoint check). Each script
names its prerequisites
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
