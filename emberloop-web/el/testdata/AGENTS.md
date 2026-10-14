# AGENTS.md — emberloop-web/el/testdata

## Purpose

Captured fixtures tests parse instead of calling the network: `usage/`
(pig and claude usage logs the tailer tests parse).

## Ownership

Owns the fixture files in this directory. The tests that consume them live
with the code they cover.

## Local Contracts

- Paths inside payloads point at the capture machine; tests parse structure,
  never absolute paths.

## Work Guidance

- Re-capture rather than hand-edit when a fixture goes stale.

## Verification

No check; fixtures are validated by the tests that read them.

## Child DOX Index

No child docs.
