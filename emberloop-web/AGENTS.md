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
  with the fake Backend (create, prompt, replay, guard, restart, UI);
  `lifecycle_test.go` covers the cap, idle release, restart resume and
  read-only paths the same way; `pig_test.go` does the same with the pig
  Backend on the SDK test-faux provider, and `pig_live_test.go`
  (env-gated) with a real pig.
- `pig.go` is the pig Backend: embedded SDK pinned in `go.mod`, stripped to
  ExtraTools only, models from the SDK catalog, one SDK stack per live
  Session released on idle or DELETE and reloaded through `Runtime.Resume`.
  Backends register per harness; the fake stays default.
- `claude.go` holds the claude ACP Backend (pinned adapter, shared
  new/load stripping set, allow-once auto-approve) with stub-transport
  tests in `claude_*_test.go` and a `-tags live` probe for the real adapter.
- `GET /api/models?harness=` offers models per harness (claude: opus,
  sonnet, haiku). Creating a claude Session with another model is 400;
  creating one without the claude Backend wired, or when the adapter
  fails to start, is 500 with no Session left behind.
- Lifecycle (`lifecycle.go`, `daemon.go`): the Daemon caps live Sessions at
  the global `max_workers` setting (env, settings file, default 3; over-cap
  creates and resumes are 429), releases an idle Session's agent process,
  and resumes it on the next prompt: claude through ACP `session/load`,
  pig through its saved session file, fake from the log. Sessions that
  cannot resume stay readable with a why (`resumable`/`resumeNote` on
  session GET, read-only banner in the UI); a new prompt there is 409.
- Event log is `enderbloom/sessions/{id}/` under the state directory:
  `meta.json` (now carrying the resume token) plus append-only
  `events.jsonl`; the Daemon reloads both on start.
- The UI is a static build: `web/src/` is the Svelte source, `web/dist/`
  is the embedded build served at `/` with its bundle assets and correct
  content types. Styling is Tailwind with shadcn-svelte-idiom components
  in `web/src/lib/ui/`; component logic lives in `web/src/lib/*.js`.

## Verification

In `emberloop-web/`: `go vet ./... && go test ./...`. Wired into the repo check.
In `emberloop-web/web/`: `npm test` (vitest) and `npm run build`. The JS
suite is not in the repo check; the check covers Go only.
The live probe is env-gated, never in the check: `EMBERLOOP_LIVE_PIG=1` with
`PIG_LIVE_MODEL` set to a healthy provider/model runs `TestLivePigBackend`.

## Child DOX Index

- `emberloop-web/spike/README.md` — the Backend spike probes
- `emberloop-web/web/README.md` — the Svelte source and embedded build
- `emberloop-web/cmd/emberloop-web/main.go` — the loopback-only Daemon binary
