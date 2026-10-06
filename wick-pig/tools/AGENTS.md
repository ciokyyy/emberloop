# AGENTS.md — wick-pig/tools

## Purpose

The agent tools both hosts serve: pig through the extension adapter, claude
through the MCP server.
Files: `tool.go` (the `Tool` interface: name,
schema, execute over a workspace root), `all.go` (the `All()` registry),
`files.go` (read, edit, write; `read` also pages stored output via `ctx:`
handles), `readurl.go` (the tiered URL reader `read`
serves: native fetch, then Jina, then the tier-3 browser
(`chrome-headless-shell` first, full Chrome/Chromium fallback, per-request
`--dump-dom` via `WICK_CHROME_BIN` or the shared resolver; rendered HTML
re-enters the tier-1 shaping; oversize
bodies go to the overflow store, not a file, while an explicitly narrowed
view that fits still answers inline),
`bash.go` (oversize output goes to the overflow store, not truncation),
`overflow.go` (the shared store-or-passthrough helper over
`overflow.threshold_bytes`, head/tail summary, searchable terms, `intent`
excerpts, `ctx:` handle parsing), `ctxsearch.go` (the `ctx_search` tool and
the `ctx:` handle reader), `background.go`, `find.go`
(plus the `cascade.go` ranker), `grep.go` (exact and regex search: ripgrep
via the shared bin-dir-then-PATH resolver with a Go fallback), `execute.go`
(the Python bridge: a fresh `python3 -I` per call, a socket plus a generated
`tools` module in a private temp dir, `ToolError` on refusal, `search`/`help`
discovery, stdout-then-stderr through the overflow store; sandboxed for
read-only roles; MCP tools serve under their configured names and as
`tools.<server>.<name>`, with one description line per server), `websearch.go`
(the provider-neutral search tool: Brave then Exa under auto, a 15-minute
answer cache, quota rows; adapters and response parsing live in
`websearch_providers.go`), `git.go` (read-only inspection: diff, log, show,
blame, status, ls-files, grep, rev-parse, merge-base, cat-file, branch
and tag listing only), `check.go` (the repo's named checks from `.wick/check`:
plain command or JSON name map, one `{args}` slot, metacharacters refused),
`hashline.go` (tag-anchored edits),
`listing.go`, `guard.go`,
`credentials.go`, `settings.go` (global and project files, the tools,
mcp_servers, search and graph maps, the per-server summary, the search
defaults, the overflow threshold, and their merge), `rename.go` (serve a tool
under a new name, forwarding its MCP markers),
`usage.go`, `sandbox.go` (the bubblewrap
runner read-only roles use: read-only root, no network, bridge bind,
self-test, `wick-pig deps` error), `mcpclient.go` (the stdio JSON-RPC client
and per-process server registry behind the same resolution), `systemone/`
(the decisions-model client the find cascade asks), and `mcpfake/` (the fake
MCP server the client, execute and graph tests spawn).

## Ownership

Owns this directory, `systemone/`, and `mcpfake/`. The interface (`tool.go`) is owned
here; the transports that serve it (`mcp/`, `cmd/`) live with their docs.

## Local Contracts

- A tool imports nothing from the harness root (`example.com/wick` would
  cycle); the one shared dependency is the `statedb` leaf, which owns the
  state database the tools and the harness both write, so a plain `go
  build` of `cmd/wick-mcp` keeps working.
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
  something. `WICK_BRAVE_ENDPOINT` and `WICK_EXA_ENDPOINT` redirect the
  adapters at test fakes.
- Tool output above `overflow.threshold_bytes` (about 8KB by default) is
  stored with a `ctx:` handle instead of entering context; `ctx_search`
  is read-only and rides with `read` in every role that can read.
- `execute` serves exactly the role's resolved toolset minus itself and the
  orchestration tools, native and MCP alike: a disabled tool is uncallable,
  renames show under their new names, MCP tools answer as
  `tools.<server>.<name>` too, and `Guard` wrappers stay on the instances it
  calls.
- The sandbox never runs a command unsandboxed: missing bwrap or a failed
  self-test is a typed error naming `wick-pig deps`.
- Binaries resolve from the wick bin directory first, then PATH, through the
  shared resolver the installer, grep, the sandbox runner, execute and the
  graph use.
- `check` shares its timeout (3 minutes), 60-line tail and status strings
  between the tool and the harness spawn-time run; the map's `default`, or
  its first name when no default exists, is what the spawn run executes.

## Work Guidance

- New tools implement `Tool` and register beside the existing ones; wire
  both hosts (extension adapter and MCP path) or say why one is excluded.

## Verification

`.wick/check`.

## Child DOX Index

No child docs.
