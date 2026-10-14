# AGENTS.md — emberloop-web/el/tools

## Purpose

The agent tools both hosts serve: pig through the extension adapter, claude
through the MCP server.
Files: `tool.go` (the `Tool` interface: name,
schema, execute over a workspace root), `all.go` (the `All()` registry),
`files.go` (read, edit, write; `read` also pages stored output via `ctx:`
handles), `readurl.go` (the tiered URL reader `read`
serves: native fetch, then Jina, then the tier-3 browser
(`chrome-headless-shell` first, full Chrome/Chromium fallback, per-request
`--dump-dom` via `EL_CHROME_BIN` or the shared resolver; rendered HTML
re-enters the tier-1 shaping; oversize
bodies go to the overflow store, not a file, while an explicitly narrowed
view that fits still answers inline),
`bash.go` (oversize output goes to the overflow store, not truncation),
`overflow.go` (the shared store-or-passthrough helper over
`overflow.threshold_bytes`, head/tail summary, searchable terms, `intent`
excerpts, `ctx:` handle parsing), `ctxsearch.go` (the `ctx_search` tool and
the `ctx:` handle reader), `background.go`, `find.go` (semantic search with
three tagged sources: the decisions-model cascade as `jev`, the lexical
fallback as `grep`, and the `codebase-memory` index as `graph` when one
exists for the project, via `findgraph.go`; exact symbol-name graph hits
rank first, an absent or failing index degrades to the two cascade sources,
plus the `cascade.go` ranker), `grep.go` (exact and regex search: ripgrep
via the shared bin-dir-then-PATH resolver with a Go fallback), `execute.go`
(the Python bridge: a fresh `python3 -I` per call, a socket plus a generated
`tools` module in a private temp dir, `ToolError` on refusal, `search`/`help`
  discovery, stdout-then-stderr through the overflow store; sandboxed for
  read-only roles; MCP tools serve under their configured names and as
  `tools.<server>.<name>`, with one description line per server),
  `webproxy.go` (the Daemon-hosted tools the single el server serves under the
  web Daemon: spawn, steer, check, cancel, profile, remember and recall as thin
  HTTP proxies bound by `EL_WEB_PARENT`/`EL_WEB_DAEMON_URL`, registered only
  for the web roles that allow them, marked so `execute` never serves them;
  profile reads memory://profile and proposes patches that wait for human
  approval, only on an explicit User request; remember proposes Rule candidates
  that wait for human approval, only on top-level Sessions; recall searches the
  caller's bank and Project notes on every role; Run-worker submit carries one
  Disposition per finding (blocker/major required, minor optional); submit stays
  defined but unoffered while `WebSubmitEnabled` in `toolresolve.go` is false),
  `websearch.go`
(the provider-neutral search tool: Brave then Exa under auto, a 15-minute
answer cache, quota rows; adapters and response parsing live in
`websearch_providers.go`), `git.go` (read-only inspection: diff, log, show,
blame, status, ls-files, grep, rev-parse, merge-base, cat-file, branch
and tag listing only), `check.go` (the repo's named checks from `.emberloop/check`:
plain command or JSON name map, one `{args}` slot, metacharacters refused),
`hashline.go` (tag-anchored edits),
`listing.go`, `guard.go`,
`credentials.go`, `settings.go` (global and project files, the tools,
mcp_servers (url/headers/oauth beside command/args/env for HTTP servers),
search and graph maps, the macros list, the per-server summary,
the search defaults, the overflow and context thresholds, the strip rows,
frame, rule, padding, symbols and separator, the auto-compress switch and
cooldown, the model window overrides, the per-harness compress models
(covered by `context_test.go`), and their merge), `macros.go` (the row macro
registry: name/kind/template entries, `{{args}}` expansion, load-time
validation where emberloop's built-ins win), `rename.go` (serve a tool under a new name, forwarding its MCP markers and images),
`image.go` (`Multimodal`: a tool returns images next to text; `read` attaches a local PNG/JPEG/GIF/WebP up to 5 MB; MCP hosts get an image content block, pig an `ai.ImageContent`, a text-only caller a note), `sudogate.go` (`SudoGate` hook: `bash` hands sudo commands to a host for user approval; unset means unchanged),
`usage.go`, `sandbox.go` (the bubblewrap
runner read-only roles use: read-only root, no network, bridge bind,
self-test, `el deps` error), `mcpclient.go` (the stdio JSON-RPC client
and the per-process server registry: one entry per server over either
transport, behind the same resolution; `Snapshot` lists the servers in parallel, so el-mcp's startup costs the slowest server, not the sum), `mcphttp.go` (the Streamable-HTTP
client: session ids, JSON or SSE replies, one OAuth retry on 401),
`mcpoauth.go` (the OAuth flow: discovery, dynamic registration, PKCE browser
approval, refresh, the per-user token store, reauth-needed), `graph.go` (the
graph server synthesis: path-slug projects, binary resolution, the exposed
tool include over the hidden management set, per-role placement, and the
first-query index-status gate), `systemone/` (the decisions-model client used
by the find cascade and draft Project picker; `NewOpenRouterWithEndpoint`
keeps HTTP-seam tests local while production uses OpenRouter), and `mcpfake/`
(the fakes the client, execute and graph tests spawn: stdio plus
Streamable-HTTP with its own authorization server).

## Ownership

Owns this directory, `systemone/`, and `mcpfake/`. The interface (`tool.go`) is owned
here; the transports that serve it (`mcp/`, `cmd/`) live with their docs.

## Local Contracts

- A tool imports nothing from the harness root (`example.com/emberloop-web/el`
  would cycle); the one shared dependency is the `statedb` leaf, which owns
  the state database the tools and the harness both write, so a plain `go
  build` of `cmd/el-mcp` keeps working.
- `edit` answers with the changed region plus context and flags a patch
  whose deletions greatly exceed its additions; tag semantics do not change.
- `read` refuses loopback, private and link-local URL targets on every
  redirect hop, caps the body near 10 MB at 20 s, and never sends
  credentialed or token-like URLs to Jina. Tier 3 re-resolves redirects
  through the same checked client before launch and confines the browser's
  own resolver with host rules over the IPv4 local ranges; page-script
  subresource fetches inside the browser see only those rules.
- `websearch` answers from Brave then Exa under `auto` (a named provider
  never falls back, a bad-query 4xx neither); keys come from
  `BRAVE_API_KEY` and `EXA_API_KEY`, never settings or the database.
  Answers cache 15 minutes keyed by query, resolved provider and arguments
  (`fresh` bypasses); each live call writes one quota row with only what
  the provider reported, and the footer reads the latest row that reports
  something. `EL_BRAVE_ENDPOINT` and `EL_EXA_ENDPOINT` redirect the
  adapters at test fakes.
- Tool output above `overflow.threshold_bytes` (about 8KB by default) is
  stored with a `ctx:` handle instead of entering context; `ctx_search`
  is read-only and rides with `read` in every role that can read.
- `execute` serves exactly the role's resolved toolset minus itself and the
  orchestration tools, native and MCP alike: a disabled tool is uncallable,
  renames show under their new names, MCP tools answer as
  `tools.<server>.<name>` too, Daemon proxies never reach a script (by mark,
  so the same-named repo checks stay callable), and `Guard` wrappers stay on
  the instances it calls.
- The sandbox never runs a command unsandboxed: missing bwrap or a failed
  self-test is a typed error naming `el deps`.
- Binaries resolve from the emberloop bin directory first, then PATH, through the
  shared resolver the installer, grep, the sandbox runner, execute and the
  graph use.
  never serve.
- An `mcp_servers` entry sets command (stdio child) or url (Streamable-HTTP),
  never both. Static HTTP credentials ride `headers`; servers that challenge
  with 401 use the `oauth` browser flow, whose tokens live per server under
  the user's home (`EL_MCP_TOKEN_DIR` in tests), never in settings. A dead
  credential is a reauth-needed error naming the token file, never fatal.
  Only a call or `el settings mcp-server <name> login` opens the browser
  approval; a listing never does, so a server without a token fails fast with
  that hint instead of stalling every session start. A host with no browser
  prints the URL and keeps waiting for the callback.
- `check` shares its timeout (3 minutes), 60-line tail and status strings
  between the tool and the harness spawn-time run; the map's `default`, or
  its first name when no default exists, is what the spawn run executes.

## Work Guidance

- New tools implement `Tool` and register beside the existing ones; wire
  both hosts (extension adapter and MCP path) or say why one is excluded.

## Verification

`.emberloop/check`.

## Child DOX Index

No child docs.
