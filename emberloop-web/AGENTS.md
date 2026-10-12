# AGENTS.md — el-web

## Purpose

Enderbloom Web: a local Daemon and embedded browser UI driving claude, pig and
grok Sessions behind one `Backend` interface. See
`.scratch/archive/enderbloom-web/spec.md`.

## Ownership

Owns `emberloop-web/`. It may import el core packages through the
`replace example.com/emberloop => ../emberloop` in `go.mod`; the module builds
only beside a sibling `emberloop/` checkout, and core never imports it. No
herdr. Ticket files are preview input; only the Run driver writes them, ticking
boxes and marking Tickets done.

## Local Contracts

- Follow the UI design rules in `web/README.md`.
- `/api/auth/signup|login|logout`, `/api/auth/me` and `/api/auth/state` do not
  require a cookie. Other `/api` routes, including SSE, answer 401 without an
  `el_session` cookie or valid machine Bearer token. The machine token is not a
  User; Session/Run creation with it answers 403. `auth_test.go` pins the gate;
  `GLOSSARY.md` defines the token.
- Sessions and Runs retain their creating User as owner; children inherit it and
  ownerless data stays hidden. Prompt senders are a signed-in User (`user`) or
  the Daemon/agents (`system`). `GLOSSARY.md` defines Owner and Sender;
  `owner_test.go` pins inheritance, visibility and sender identity.
- Harness starts use per-User Claude and Pig dirs, carried in `SessionMeta.Dirs`;
  `ownerDirsFor` supplies them. Claude dirs expose only the host login; Pig dirs
  expose approved shared state, not host instruction files, skills or prompts.
  Child Sessions use their parent's dirs. `harnessdirs_test.go` and
  `handle_test.go` pin isolation and handle identity.

## Test seams

- Daemon HTTP/SSE API and lifecycle: `api_test.go` and `lifecycle_test.go` use
  the fake Backend.
- Session tools: `steer_test.go`, `steer_child_test.go`, `spawn_test.go` and
  `submitgate_test.go`.
- Users and harness identity: `auth_test.go`, `owner_test.go`,
  `users_list_test.go`, `harnessdirs_test.go` and `handle_test.go`.
- Web-v1 design and injection invariants at the Daemon seam:
  `injection_test.go`, `invariants_test.go` and `prompts_wiring_test.go`.
- Run driver: `runs_driver_test.go` uses a fake Backend and fake git;
  `runs_reviewers_test.go`, `runs_persist_test.go`, `runs_chain_test.go`,
  `runs_pause_switch_test.go`, `runs_specbranch_test.go`,
  `runs_worktree_test.go`, `runs_final_test.go`,
  `runs_markdone_basered_test.go` and `runs_overlap_test.go` cover Run seams.
- Backend and Handoff seams: `claude_*_test.go`, `pig_test.go`,
  `handoff_test.go`, `compress_retry_test.go` and `grok_handoff_test.go`.
- Memory package tests are mapped in `memory/AGENTS.md`; browser tests are
  mapped in `web/README.md`.

## Verification

- In `emberloop-web/`: `go vet ./... && go test ./...` (included in
  `.emberloop/check`).
- In `emberloop-web/web/`: `npm test` and `npm run build`; these are not part of
  the repo check.
- In `emberloop-web/web/`: `npm run test:e2e` runs the Chromium suite after
  building the UI; it is not part of the repo check. `web/e2e/serve.sh` starts
  a fake Daemon with fixtures in `/tmp/el-e2e`; `web/e2e/tests/auth.setup.e2e.ts`
  creates and saves the admin session.
- Live probes are env-gated and never part of the check: `EL_LIVE_PIG=1` with
  `PIG_LIVE_MODEL` runs `TestLivePigBackend`; `EL_LIVE_PIG_ACP=1` with
  `PIG_ACP_SPAWN_BIN` set to a built Daemon runs `TestLivePigACP`; and
  `EL_LIVE_JEV=1` with `JEV_API_KEY` (or `JEV_BASE_URL` for a local server;
  `JEV_MODEL` overrides the default) runs `TestLiveJevDecider`.

## Child DOX Index

- `emberloop-web/spike/README.md` — the Backend spike probes
- `emberloop-web/web/README.md` — the Svelte source and embedded build
- `emberloop-web/cmd/emberloop-web/main.go` — the loopback-only Daemon binary
- `emberloop-web/memory/AGENTS.md` — the memory engine package and its contracts
