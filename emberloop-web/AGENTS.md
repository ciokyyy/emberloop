# AGENTS.md — emberloop-web

## Purpose

Enderbloom Web: local Daemon (Go) plus embedded browser UI driving
claude and pig Sessions behind one `Backend` interface. See
`.scratch/enderbloom-web/spec.md`.

## Ownership

Owns `emberloop-web/`. May import emberloop core packages; core never
imports it. No herdr, no ticket state.

## Local Contracts

- UI design rules (shadcn-svelte primitives everywhere, ChatGPT + Apple look,
  smooth motion everywhere) are in `web/README.md` "Design rules": follow them
  for every UI change.

- `spike/` holds live-only probe programs, never imported by product code.
- Terms follow root `GLOSSARY.md`.
- Real Backends prove out in live probes, never the main check.
- API is the test seam: `api_test.go` drives the Daemon's HTTP and SSE API
  with the fake Backend (create, prompt, replay, guard, restart, UI);
  `lifecycle_test.go` covers the cap, idle release, restart resume and
  read-only paths the same way; `steer_test.go` covers steer, follow-up,
  queue list/clear and the claude Steer-to-Follow-up degradation at the
  same seam; `spawn_test.go` covers spawn, nesting, grouped delivery and
  scout read-only at the same seam; `pig_test.go` does the same with the
  pig Backend on the SDK test-faux provider, and `pig_live_test.go`
  (env-gated) with a real pig.
- `pig.go` is the pig Backend: embedded SDK pinned in `go.mod`, stripped to
  ExtraTools plus the per-Session spawn tool (scouts: ExtraTools without
  spawn), models from the SDK catalog, one SDK stack per live Session
  released on idle or DELETE and reloaded through `Runtime.Resume`.
  Steer and Follow-up use the SDK native queues (`QueuedMessages`/
  `ClearQueued` expose them); the Daemon merges them with its own pending
  list. Backends register per harness; the fake stays default.
- `claude.go` holds the claude ACP Backend (pinned adapter, shared
  new/load stripping set, allow-once auto-approve) with stub-transport
  tests in `claude_*_test.go` and a `-tags live` probe for the real adapter.
- `GET /api/models?harness=` offers models per harness (claude: opus,
  sonnet, haiku). Creating a claude Session with another model is 400;
  creating one without the claude Backend wired, or when the adapter
  fails to start, is 500 with no Session left behind.
- Lifecycle (`lifecycle.go`, `daemon.go`): the Daemon caps live Sessions at
  the global `max_workers` setting (env, settings file, default 3; over-cap
  creates and resumes are 429), releases a settled (idle or failed)
  Session's agent process, and resumes it on the next send: claude
  through ACP `session/load`,
  pig through its saved session file, fake from the log. One resume runs at
  a time and sends fence an in-flight release: a send in either window is
  409 and retryable, so a stale release never kills a resumed process.
  Sessions that cannot resume stay readable with a why
  (`resumable`/`resumeNote` on session GET, read-only banner in the UI);
  a new send there is 409.
- Steering is `POST /api/sessions/{id}/steer|followup` with `{"text"}`.
  `POST .../prompt` accepts an optional `mode` (prompt, steer, followup).
  Live idle Sessions start a turn at once; mid-turn messages queue: pig
  steers and follow-ups in the SDK native queues, everything else in the
  Daemon pending list. A mid-turn claude Steer degrades to a Follow-up and
  the transcript says so. Prompt Events carry the delivery in `mode`
  (empty for plain prompts, else steer or followup).
- `GET /api/sessions/{id}/queue` lists not-yet-delivered messages as
  `{"queue":[{"mode","text"}]}` (pig native queues first); `DELETE` on the
  same path clears them before delivery and answers `{"cleared":N}`.
  Steer, follow-up and prompt-on-working answers are 202 with
  `{"seq","queued"}` (queued Daemon messages add `"position"`); unknown
  Sessions are 404, empty text is 400, ended Sessions are 410 on send.
- Event log is `enderbloom/sessions/{id}/` under the state directory:
  `meta.json` (now carrying the resume token plus `parentId`, `groupId`,
  `scout`) plus append-only `events.jsonl`; Groups persist as
  `enderbloom/groups/{id}.json`; the Daemon reloads all three on start.
- Subagents are `POST /api/sessions/{id}/spawn` with
  `{"agents":[{"prompt","type"}]}` (type worker or scout, default worker).
  Each child inherits the parent's harness, model and working directory,
  records its parent and Group, and runs its prompt at once; over-cap the
  whole call is 429 with no half Group. The session list nests children
  under `parentId` and each opens its own transcript. The last member to
  settle delivers one grouped result to the parent as a Steer (queued when
  the parent works); the parent transcript also keeps one
  `subagent_spawned` Event per child and one `subagent_finished` Event per
  Group. Delivery is exactly-once per Group: a parent that cannot resume
  (cap, ended) still gets its `subagent_finished` marker, but the grouped
  Steer is dropped, and a Group mid-flight across a restart is never
  redelivered; member transcripts stay readable either way. Scouts are
  read-only: sends and nested spawns are 403, the UI hides the composer
  with a banner, pig scout loadouts carry no spawn tool, and claude scout
  Sessions run the base server under `--role scout` with no spawn server.
- `spawn` is Daemon-hosted (`spawn.go`, `spawnmcp.go`): pig takes it as a
  per-Session ExtraTool bound to its own id, claude through a per-Session
  `emberloop-spawn` MCP server (the Daemon binary in `spawn-server` mode,
  parent and Daemon URL in its env) alongside the base `emberloop-mcp`
  server. The existing MCP server never calls back; all Group state stays
  in the Daemon.
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
