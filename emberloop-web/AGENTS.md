# AGENTS.md — emberloop-web

## Purpose

Enderbloom Web: local Daemon (Go) plus embedded browser UI driving
claude and pig Sessions behind one `Backend` interface. See
`.scratch/enderbloom-web/spec.md`.

## Ownership

Owns `emberloop-web/`. May import emberloop core packages; core never
imports it. No herdr, no ticket state.

## Local Contracts

- `spike/` holds live-only probe programs, never imported by product code.
- Terms follow root `GLOSSARY.md`.
- Real Backends prove out in live probes, never the main check.
- API is the test seam: `api_test.go` drives the Daemon's HTTP and SSE API
  with the fake Backend (create, prompt, replay, guard, restart, UI).
- Event log is `enderbloom/sessions/{id}/` under the state directory:
  `meta.json` plus append-only `events.jsonl`; the Daemon reloads both on start.
- The UI is a static build: `web/src/` is the Svelte source, `web/dist/`
  is the embedded build served at `/`.

## Verification

In `emberloop-web/`: `go vet ./... && go test ./...`. Wired into the repo check.

## Child DOX Index

- `emberloop-web/spike/README.md` — the Backend spike probes
- `emberloop-web/web/README.md` — the Svelte source and embedded build
- `emberloop-web/cmd/emberloop-web/main.go` — the loopback-only Daemon binary
