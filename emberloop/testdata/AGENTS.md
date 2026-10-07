# AGENTS.md — emberloop/testdata

## Purpose

Captured fixtures tests parse instead of calling the network: `agy/`
(transcripts, stream captures, hook payloads, `models.txt` from the agy
probe), `usage/` (pig and claude usage logs plus the `context-*.jsonl`
context-core transcripts with tool calls and results), and `theme/` (the
generated claude and pig themes, Mocha and Latte, byte-compared by the
theme golden tests). `agy/README.md` records what each capture answers.

## Ownership

Owns the fixture files in this directory. The tests that consume them live
with the code they cover.

## Local Contracts

- Paths inside payloads point at the capture machine; tests parse structure,
  never absolute paths.

## Work Guidance

- Re-capture rather than hand-edit when a fixture goes stale, and note the
  tool version and date as `agy/README.md` does.

## Verification

No check; fixtures are validated by the tests that read them.

## Child DOX Index

No child docs.
