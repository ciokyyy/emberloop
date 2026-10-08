# AGENTS.md — emberloop-web

## Purpose

Enderbloom Web: local Daemon (Go) plus embedded browser UI driving
claude and pig Sessions behind one `Agent` interface. See
`.scratch/enderbloom-web/spec.md`.

## Ownership

Owns `emberloop-web/`. May import emberloop core packages; core never
imports it. No herdr, no ticket state.

## Local Contracts

- `spike/` holds live-only probe programs, never imported by product code.
- Terms follow root `GLOSSARY.md`.
- Real Backends prove out in live probes, never the main check.

## Verification

Repo check once product code exists.

## Child DOX Index

- `emberloop-web/spike/README.md` — the Backend spike probes
