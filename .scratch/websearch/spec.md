# Spec: execute tool, tool settings, overflow store, read-only toolkit, codebase graph, websearch, URL read, SQLite stats, scout cleanup

## Problem Statement

Agents running under emberloop (pig and claude sessions, through the MCP server) cannot search the web or read a link. `read` handles only local paths, `emberloop://` addresses, directories and globs, and there is no search tool at all. Separately, stats live in append-only JSONL files that the TUI polls, there is no place for a search cache or provider quota, and every scout an agent spawns leaves an idle herdr pane open forever, so tab "2" fills with dead scouts. `tell` and `finish` also only know an agent by id, although agents often hold only a pane id.

Tool use is also expensive and rigid. Every tool result goes straight into context (bash truncates at 30KB and drops the rest). Workers spend turns on read-then-grep loops that one scripted call would answer. There is no MCP client, so no outside MCP server can be used, and no setting turns a tool off, renames it or restricts it by role. Reviewers and scouts can only `read` and `find`: `find` is semantic and cannot do exact-string or regex lookups (a reviewer asking for references to a removed symbol got four off-topic hits at 0.36-0.52), each `find` costs 10-20 backend generations, reviewers cannot run a check or look at git history, and when `read` refuses a big file it tells them to "use bash", which they do not have. Reviewer sessions grow to about 188K cached tokens. They repeat the worker's claims ("234 passed") without verifying them.

## Solution

Add a provider-neutral `websearch` tool (Brave, then Exa as fallback) and make `read` accept URLs through a tiered reader (native fetch, then Jina, then a stealth browser added later). Move usage stats into one SQLite database that also holds a search cache, per-provider quota for the TUI to show, and an FTS5 overflow store. Close a scout's pane as soon as its result is delivered, and let `tell` and `finish` address an agent by pane id as well as agent id, with `spawn` returning both.

Add an `execute` tool: a Python script (system `python3`, stdlib only) that calls emberloop's native tools and lazily-loaded MCP servers as functions, so one call replaces many and only what the script prints enters context. Add emberloop settings, global and per project, that enable, disable, rename and place every tool (in the main tool list or behind `execute`) globally and per role, and that register MCP servers. When a tool result is too big, store it in the database and return a short summary plus a handle, searchable with BM25 through `ctx_search`. Add read-only native tools reachable through `execute`: `grep` (ripgrep), a non-destructive `git`, and `check` (the repo's named checks, re-runnable). Optionally integrate codebase-memory-mcp as a code graph that emberloop indexes automatically per worktree and removes on `finish`.

Make the whole toolkit installable and sandboxed. `grep`, `git` and `check` live behind `execute` for every role, so reviewers and scouts get `execute` too, and for them the Python runs inside a bubblewrap sandbox (read-only filesystem, no network) while emberloop executes the tools it calls outside it. A `emberloop deps` installer, also run by `emberloop install`, installs and verifies every external dependency (`python3`, `rg`, `git`, `bwrap`, and the optional code graph binary), so a missing dependency is something the installer fixes and not something agents meet.

## User Stories

1. As an agent, I want to call `websearch` with a query, so that I can find current information the model does not have.
2. As an agent, I want results as a compact numbered list of title, URL, snippet and publish date, so that I can choose what to read without spending context on page text.
3. As an agent, I want to pass `provider` (`brave`, `exa` or `auto`) on a single call, so that I can pick the engine that suits the question.
4. As an agent, I want an explicitly named provider to return its real error instead of silently falling back, so that I can decide what to do next.
5. As an agent, I want `auto` to fall back from Brave to Exa on error, timeout, quota exhaustion or zero results, so that a single outage does not stop my research.
6. As an agent, I want a query that the provider rejects as malformed to fail without falling back, so that I fix the query instead of retrying elsewhere.
7. As an agent, I want `count`, `freshness` and `domains` arguments, so that I can narrow a search without learning each provider's parameters.
8. As an agent, I want the answer footer to say which provider answered and what fallbacks happened, so that I can judge the result.
9. As an agent, I want `fresh: true` to bypass the cache, so that I can force a live search when I suspect staleness.
10. As an agent, I want a missing key to produce a clear "<NAME> not set" error, so that I do not waste turns guessing.
11. As a user, I want to set the default provider with `emberloop settings`, so that every session uses the engine I prefer.
12. As a user, I want to set `search.mode` to `parallel` in settings, so that I can later trade quota for coverage without changing the tool.
13. As a user, I want parallel results merged with reciprocal rank fusion and tagged with the providers that returned each URL, so that agreement between engines is visible.
14. As a user, I want repeated identical searches to hit a 15 minute cache, so that agents do not burn my quota repeating themselves.
15. As a user, I want each search's quota or cost recorded (Brave rate-limit headers, Exa reported cost), so that the TUI can show what I have left.
16. As a user, I want the cache and quota to live in the same database as my usage stats, so that there is one source of truth to back up and read.
17. As an agent, I want `read` to accept `http://`, `https://` and bare `www.` links, so that I read a page the way I read a file.
18. As an agent, I want a collapsed `https:/host` scheme repaired, so that path normalisation does not break links.
19. As an agent, I want HTML converted to Markdown with navigation and boilerplate removed, so that a page costs few tokens.
20. As an agent, I want JSON pretty-printed and plain text or Markdown returned as is, so that APIs and raw files stay readable.
21. As an agent, I want line ranges on a URL (`:50-100`) and a `:raw` selector, so that I can page through a long document or see the unshaped body.
22. As an agent, I want output longer than 300 lines truncated with the rest saved to a file I can read, so that a huge page cannot flood my context.
23. As an agent, I want the output header to name the tier that answered, so that I know how much to trust the extraction.
24. As an agent, I want a blocked, challenged, thin or timed-out page to escalate to the next tier automatically, so that I rarely see a dead end.
25. As an agent, I want PDFs and office documents converted through the Jina tier, so that I can read papers and reports.
26. As a user, I want private, loopback and link-local addresses blocked on every redirect hop, so that a prompt-injected page cannot reach my machine or cloud metadata.
27. As a user, I want fetched content wrapped in an untrusted-content header, so that the model treats page text as data, not instructions.
28. As a user, I want URLs carrying credentials or token-like query parameters never sent to Jina, so that secrets do not reach a third party.
29. As a user, I want the response size capped and a timeout enforced, so that one slow or huge page cannot hang a session.
30. As an agent, I want a clear "not installed" error from the browser tier until it exists, so that I know the page is out of reach and why.
31. As a user, I want the stealth-browser tier chosen by research on RAM, startup time, detection resistance, integration and licence, so that I do not pick on guesswork.
32. As a user, I want a scout's pane to close once its result is delivered, so that idle panes stop piling up in herdr.
33. As an agent, I want `spawn` to return both the agent id and the pane id for each entry, so that I can refer to a scout either way.
34. As an agent, I want `tell` and `finish` to accept either `agent` or `pane`, so that I can act with whichever identifier I have.
35. As an agent, I want an unknown or ambiguous pane to give a precise error, so that I never act on the wrong agent.
36. As a user, I want my existing usage history imported into the database once, so that the TUI loses no data.
37. As a user, I want `prune` to delete database rows as well as files, so that pruning a run still removes its usage.
38. As a TUI developer, I want to open the database read-only while other processes write, so that the TUI never blocks them.
39. As a user running a reviewer on an Antigravity model, I want a model turn that ends in a malformed function call to be retried instead of killing the run, so that one bad tool call does not cost me a whole review.
40. As a user, I want the error, when retries are exhausted, to say the model emitted a malformed tool call and show what the provider reported, so that I can tell it apart from a genuinely empty answer.
41. As an agent that spawned reviewers, I want a reviewer that died without submitting to be reported to me as lost, so that I am never left waiting on a ghost.
42. As a worker, I want my stop gate to nudge me to submit even when a child I spawned has died, so that I never end a turn text-only without my parent hearing from me.
43. As an orchestrator, I want a child that ended its turn in a provider error to tell me it stopped with an error, so that I can retry or replace it.
44. As an agent, I want to call `execute` with a Python script that calls `tools.read`, `tools.grep`, `tools.find`, `tools.bash` and other tools as functions, so that one call replaces a chain of calls and only the printed result enters my context.
45. As an agent, I want the `execute` description to list each lazily-loaded MCP server in one line, and `tools.search("query")` and `tools.help("name")` inside Python to return full signatures, so that I can discover tools without paying for every schema up front.
46. As a user, I want to register MCP servers in emberloop settings (global and per project), started only on first use, so that I can add capabilities without bloating every session.
47. As a user, I want to enable, disable, rename and place (main list or behind `execute`) any tool, globally and per role, with include and exclude patterns for an MCP server's tools, so that each agent type sees the tools I choose and no others.
48. As a user, I want a disabled tool to be unavailable everywhere, including inside `execute`, so that disabling is a real restriction.
49. As an agent, I want a Python exception in `execute` to come back as a traceback tail, and a failing tool call to raise a catchable `ToolError`, so that I can see and handle failures.
50. As an agent, I want output over the size threshold from `execute`, `bash` and URL `read` to be stored and replaced by a short summary and a handle, so that a large result cannot flood my context.
51. As an agent, I want `ctx_search` to take several queries and an optional handle and return BM25-ranked snippets around the matches, so that I can pull the relevant part of a stored output.
52. As an agent, I want `execute` to accept an optional `intent` so that over-threshold output comes back already filtered to the sections matching it, saving a round trip.
53. As a reviewer or scout, I want `tools.grep(...)` inside `execute`, with regex or fixed-string search, globs, context lines, a result cap and `.gitignore` respected, so that exact lookups cost one cheap call instead of a semantic `find`.
54. As a worker, I want `tools.grep` too, so that a capped, structured search is available without writing a bash pipeline.
55. As a reviewer, scout or worker, I want a read-only `tools.git(...)` (diff, log, show, blame, status, ls-files, grep, rev-parse, merge-base, cat-file, branch and tag listing), so that I can check history and the exact change without being able to alter the repo.
56. As a reviewer or worker, I want `tools.check(...)` to re-run a named repo check (`test`, `vet`, ...) with optional arguments such as a package, so that I verify claims and my own work.
57. As a user, I want the `.emberloop/check` file to support a map of named checks while a plain single command keeps working, so that existing repos need no change.
58. As a reviewer or scout without bash, I want a refused oversized `read` to point me at `tools.grep` in `execute` or `read` with offset and limit, so that I am not told to use a tool I do not have.
59. As a worker, I want the role prompt to tell me to use `execute` for searching and processing content, and reviewers and scouts to use `tools.grep` and `tools.git` there, so that the cheaper path is the default.
60. As a user, I want emberloop to index each worktree into a code graph automatically when it is created, refresh it before each reviewer or scout spawn, and delete it on `finish`, so that structural questions ("who calls X") cost one call and no stale graphs accumulate.
61. As a scout or reviewer, I want graph tools (`search_graph`, `trace_path`, `get_code_snippet`, `get_file_outline`, `get_architecture`) in my main tool list, so that structural lookups need no Python.
62. As a user, I want the code graph to be optional once installed: if it is disabled in settings, `find` and `grep` keep working and no graph tool is exposed.
63. As a user, I want a reviewer's or scout's `execute` Python to run in a sandbox with a read-only filesystem and no network, so that read-only roles stay read-only even though they can run scripts.
64. As a user, I want `emberloop deps` to detect what is missing (`python3`, `rg`, `git`, `bwrap`, the graph binary), install it with my system package manager or a checksum-verified download, and prove each works, so that agents never meet a missing dependency.
65. As a user, I want `emberloop install` to run the same dependency step, so that one command sets up a machine.

## Implementation Decisions


**Websearch tool.**
- A new tool in the tool layer, served the same way as the existing file tools (pig through the extension adapter, claude through the MCP server) and included in every role that can read.
- Arguments: `query` (required), `count` (default 10), `freshness` (`day`, `week`, `month`, `year`), `domains` (include list), `provider` (`brave`, `exa`, `auto`; default from settings, else `auto`), `fresh` (cache bypass). There is no `mode` argument.
- Provider-neutral only: Exa's neural features and categories are out of v1.
- Each provider sits behind one adapter interface (search, parse results, parse quota) so Tavily and others are one adapter each later.
- Keys come from the environment: `BRAVE_API_KEY`, `EXA_API_KEY`. A provider with no key is skipped under `auto` with a note, and is an error when named explicitly.
- Fallback chain under `auto` is Brave then Exa. It advances on network error, timeout, HTTP 429 or quota exhaustion, 5xx, and zero results. A 4xx caused by the query does not advance.
- A named provider never falls back.
- Parallel mode is implemented behind the setting `search.mode` (`fallback` default, `parallel`): all configured providers run concurrently with a per-provider timeout, results are deduplicated by normalised URL and fused by reciprocal rank fusion, and every result carries a `sources` list. It is off by default and not exposed on the tool.
- Output is plain text: a numbered list (title, URL, snippet, published date when known, sources when more than one) and a footer naming the answering provider, any fallbacks, whether the result came from cache, and the latest known quota.
- Settings: `search.provider` (`auto|brave|exa`) and `search.mode`, edited through the existing `emberloop settings` command, stored in the existing settings file, with environment overrides in the style of the existing ones.

**Search cache.** Results are cached in the database for 15 minutes, keyed by query, resolved provider and arguments. Only searches are cached; fetched pages are not. `fresh: true` bypasses the cache and refreshes the entry.

**Quota.** After each live call the adapter extracts what the provider reports: Brave's rate-limit remaining and reset headers, Exa's reported cost per request. One row per call is written with provider, used or cost, remaining, reset time and timestamp. The TUI shows the latest row per provider. Nothing is enforced; nothing is estimated when a provider reports nothing.

**URL read.**
- `read` detects `http(s)://` and `www.` paths, repairs a collapsed scheme, and peels trailing selectors (`:N`, `:N-M`, `:N+M`, comma lists, `:raw`).
- Tier 1: plain HTTP fetch with a browser User-Agent, redirects followed, one retry on 429 honouring a bounded `Retry-After`, and in-process HTML to Markdown with boilerplate removal in pure Go (no external binaries). JSON is pretty-printed; plain text and Markdown pass through; `:raw` returns the body unshaped and never escalates.
- Tier 2: the Jina reader, authenticated with `JINA_API_KEY` (kept in the user's shell environment, never in the repo, settings or database). Without a key the tier still works at Jina's lower anonymous limit and the header says which was used. PDFs and office documents go straight to this tier.
- Tier 3: a stealth browser. In this spec it is an interface that returns a "not installed" error; the real tier follows the research ticket.
- Escalation from a tier happens on HTTP 403, 429 or 503, a recognised bot-challenge page, fewer than 100 non-whitespace characters after conversion, or a timeout.
- Output starts with a header (URL, content type, tier that answered, notes) and the body. Output longer than 300 lines is truncated and the remainder saved to a file whose path is in the output; line selectors apply to the full content.
- Safety: block loopback, private and link-local addresses, re-checked on every redirect hop. Cap the body at about 10 MB and the request at 20 seconds. Wrap the body in an untrusted-content header. Skip the Jina tier for URLs that fail the address check or carry credentials or token-like query parameters, and say so in the header. An allow flag for localhost is added only when needed.
- Left out of the reference design on purpose: site-specific scrapers, SQLite and archive and notebook and image handling, and external reader binaries.

**SQLite foundation.**
- One database in the emberloop state directory, redirected by the existing state-directory override, owned and migrated by emberloop with a schema version table.
- Pure-Go driver (no cgo). WAL mode and a busy timeout. The MCP server, the pig extension and the usage tailer write; the TUI opens it read-only.
- The TUI read model in the `emberloop-tui` module is updated in the same ticket to read usage from the database (read-only, incremental by row id) instead of tailing JSONL files; its existing tests are adapted, not deleted.
- Usage records (request, find, cost, plus the new search kind) move into the database. A one-time import reads the existing JSONL logs on first run; afterwards usage is written only to the database. The old files stay until pruned. `prune` deletes rows as well as files. The usage tailer inserts instead of appending lines and keeps its idempotence (no double counting on restart).
- The schema leaves room for workers, runs and the archive index, which migrate later.

**Scout and pane hotfix.**
- A scout's pane closes as soon as its result is delivered to the parent. There is no keep option and no idle reaper. A follow-up means spawning a new scout.
- `spawn` returns the agent id and the pane id for every entry.
- `tell` and `finish` take either `agent` or `pane` (the herdr pane id); exactly one is required. A pane id resolves to an agent through the agent record, which already stores its pane. An unknown pane, or one that matches no live agent, is an error that says so.
- No automatic sweep of existing panes or old agent records in this spec.

**Antigravity malformed function call.**
- The provider extension currently treats any terminal candidate with no content as "returned an empty response". When the model's turn ends with the reason `MALFORMED_FUNCTION_CALL`, the candidate has no parts, so the run fails with that unhelpful message. The extension also has no retry of any kind.
- A terminal candidate that ends in `MALFORMED_FUNCTION_CALL` (and the equivalent invalid-tool-call finish reasons the provider reports) with no output so far is retried: the same request is re-sent up to 2 more times with a short backoff, before anything is shown to the agent. A retry is never attempted once any content or tool call has been emitted for the turn.
- If every attempt fails, the error names the finish reason and includes the provider's finish message when one is present, instead of "empty response". A genuinely empty `STOP` stays the existing "empty response" error.
- The finish message is read from the candidate so it is available for the error text.
- Retried attempts must not duplicate session state: the request is rebuilt from the same transcript, and the session is marked complete only on the attempt that succeeds.

**Dead children and the stop gate.**
- A child counts as pending (still owing a result) only while it is alive: its pane still exists, and its last turn did not end in a provider error. A child that has no submit but is dead is lost, not pending.
- A lost child sends its parent the existing lost notice once, and then stops counting as pending.
- A turn that ends in a provider error also sends the parent a "stopped with error" notice naming the agent and the error, instead of an allow with no message. This applies to reviewers and scouts as well as workers.
- Before the stop gate decides to allow because children are pending, it runs the dead-child sweep, so a stranded group is finalised when the parent stops and not only when the parent next calls `spawn`, `tell` or `finish`.
- The block counter, the pin and the three-block give-up are unchanged.
- Origin: a worker's reviewer died on an Antigravity error, stayed pending forever, and the worker then stopped text-only with no nudge and no notice to its parent.

**Delivery order for the second half of this spec.** Tickets 01-08 keep their order above, except that 04 is not blocked and keeps saving truncated URL output to a file until the overflow store ticket retrofits it. New tickets: 09 overflow store and `ctx_search` (blocked by 03 and 04, since it retrofits URL `read`), 10 tool settings (none), 11 `grep` (none), 12 read-only `git` (none), 13 `check` (none), 14 bubblewrap sandbox runner (none), 15 MCP client (blocked by 10), 16 `execute` core with native tools and role gating (blocked by 09, 10, 11, 12, 13, 14), 17 MCP tools inside `execute` (blocked by 15, 16), 18 role toolsets, `read` oversize message and prompt directions (blocked by 16), 19 `emberloop deps` installer (none), 20 code graph integration, optional (blocked by 10, 15, 17, 19). 06 and 07 are unchanged. Independent tickets run in parallel; 20 is last, so its failure blocks nothing.

**Tool settings (ticket 10).** The existing settings file keeps unknown keys, so new keys are additive. Global (`~/.emberloop/settings.json`) and project (`.emberloop/settings.json`) files are both read; project wins field by field, maps merge by key. Shape:

```json
"tools": {
  "bash":      {"enabled": true, "placement": "main", "rename": "sh",
                "roles": {"scout": {"enabled": false}}},
  "websearch": {"enabled": false}
},
"mcp_servers": {
  "github": {"command": "npx", "args": ["..."], "env": {},
             "placement": "execute", "include": ["search_*"], "exclude": ["delete_*"],
             "rename": {"search_repositories": "gh_search"},
             "roles": {"review": {"placement": "main"}}}
}
```

`enabled` defaults to true. `placement` is `main` (a top-level tool) or `execute` (callable only inside `execute`); native tools default to `main` except those documented here. A per-role block (`roles.<role>`) overrides `enabled` and `placement` for that role. `include` and `exclude` take glob patterns over an MCP server's tool names; `rename` maps original name to new name and applies in both places a tool can appear. `submit` cannot be disabled or moved behind `execute`. Resolution happens in one function that both the MCP server tool list and the `execute` stub generator call, so the two cannot disagree. A disabled tool is absent from the tool list and from `execute`. Unknown tool names in settings are reported once in the log and ignored. The same resolution feeds the role allowlists in `roles.go`: settings can only narrow a role's allowlist, never widen it past what the role is defined to have.

**MCP client (ticket 15).** A hand-written stdio JSON-RPC client in Go, matching the existing hand-written server (no third-party MCP library). It starts a configured server on first use, performs `initialize`, lists tools, caches the list for the session, calls tools, and kills the child when emberloop-mcp exits. A server that fails to start or answers with a protocol error yields a clear tool error naming the server, never a hang (initialize and call timeouts). Servers are per emberloop-mcp process; nothing is shared across sessions.

**Execute tool (tickets 16, 17).**
- Arguments: `code` (required, Python source), `timeout` (seconds, default 120, max 600, as bash), `intent` (optional string, see overflow store).
- Offered to roles `work`, `plan`, `full`, `review` and `scout` (every role with any tools beyond orchestration), and only when not disabled in settings. It is on by default. The `implement` role (an orchestrator with no bash) does not get it.
- Runtime: system `python3` found on `PATH`, run as `python3 -I`, standard library only. A missing interpreter gives a clear error naming what to install. Each call is a fresh process with no state kept between calls; a script that wants to keep data writes a file.
- Bridge: emberloop creates a private temp directory with a unix socket and a generated `tools` module, and puts it on the script's import path. `tools.<name>(**kwargs)` sends a JSON line over the socket, emberloop runs the real tool through the same resolved toolset and `Guard` wrappers the role already has, and returns the result string (or raises `ToolError`). MCP tools appear under their configured (renamed) names and under `tools.<server>.<name>`. The socket and directory are removed after the call; the whole process group is killed on timeout.
- Allowed tools inside the script are exactly the role's resolved toolset (settings applied, `execute` itself excluded, orchestration tools excluded). For `work`, `plan` and `full` the script is not otherwise sandboxed: Python can still use `open` and `subprocess`, which is acceptable because those roles already have bash.
- **Sandbox for `review` and `scout`.** The Python process runs under bubblewrap: the root filesystem bind-mounted read-only, the worktree read-only, no network (`--unshare-net`), PID namespace, `--die-with-parent`, a private tmpfs for `/tmp` holding the bridge socket and the generated `tools` module, and a minimal environment. Emberloop itself, outside the sandbox, receives each tool call over the socket and runs the tool, so `check` can write its own build caches while the script itself can write nothing. Before first use emberloop runs a one-time self-test of the sandbox (a script that tries to write a file and open a socket must fail). If `bwrap` is missing or the self-test fails, `execute` for these roles returns an error that names `emberloop deps`; it never runs these roles' Python unsandboxed, and never silently drops the tools.
- Discovery: the tool description lists native tools placed behind `execute` by name, and one line per MCP server (name, summary, tool count). `tools.search(query)` and `tools.help(name)` inside Python return matching names with signatures and descriptions. Full schemas are never in the tool list.
- Output: whatever the script prints (stdout, plus stderr after it), through the same sanitising as bash. A non-zero exit appends the traceback tail and `[exit status N]`.
- The generated stub and the MCP integration are split: ticket 16 does native tools only; ticket 17 adds MCP tools to the stub and to the description.

**Overflow store and `ctx_search` (ticket 09).**
- Same database as ticket 03, FTS5 table with the porter tokenizer, plus a `ctx_blobs` table (id, session, source label, created at, content type, bytes). Content is chunked by markdown heading and by blank-line blocks, keeping code blocks intact; titles and headings weigh more in ranking. Chunks carry line ranges.
- Threshold: output above about 8KB is stored. The tool returns a short head and tail, the total size and line count, a handle (`ctx:<id>`), and a list of searchable terms. Execute with `intent` returns the matching chunks inline in addition. The threshold is a setting (`overflow.threshold_bytes`).
- Applies to `execute`, `bash` (replacing its 30KB truncation) and URL `read` (replacing the truncate-and-save-a-file behaviour of ticket 04). Output under the threshold is unchanged.
- `ctx_search` arguments: `queries` (one or more strings), optional `source` (a handle, to scope the search), `limit` per query (default 3). It returns BM25-ranked snippets focused on the match, each with its handle and line range, and says plainly when nothing matched. `read` also accepts a handle with a line range (`ctx:12:50-100`) to page through a stored output.
- Lifetime: scoped to the session; rows older than 24 hours are purged on database open and by `prune`. Offered to every role that can read.

**Native read-only toolkit (tickets 11-13, 18).** All three tools default to `placement: execute` for every role: they are functions inside `execute` and absent from the main tool list (settings can move any of them to `main` for any role).
- `grep`: ripgrep (`rg`) when installed, otherwise a Go fallback. Arguments: `pattern`, `fixed` (bool), `path`, `glob`, `context` (lines), `max_results` (default 100), `ignore_case`. Respects `.gitignore`, skips binary files, output is `path:line:text` with a total and a "more results" note. Output goes through the overflow store above the threshold. Available to every role that has `execute`.
- `git`: one tool taking `args` (list of strings) run as `git -C <worktree> --no-pager`. Allowed subcommands: `diff`, `log`, `show`, `blame`, `status`, `ls-files`, `grep`, `rev-parse`, `merge-base`, `cat-file`, `branch` and `tag` only with a list flag. Anything else is refused with the allowed list. Refused flags: `-c`, `--output`, `--ext-diff`, `--textconv`, `--exec`, `--upload-pack`. `--no-ext-diff --no-textconv` are always added and the git config environment is cleared (`GIT_CONFIG_GLOBAL`, `GIT_CONFIG_SYSTEM` pointed at the null device, `GIT_EXTERNAL_DIFF` unset). Available to every role that has `execute`.
- `check`: runs a named repo check. `.emberloop/check` may be a plain command (name `default`) or a JSON map of names to commands, where a command may contain one `{args}` slot. Arguments: `name` (default `default`), `args` (a string substituted into the slot only; shell metacharacters in it are refused). Same 3-minute timeout and 60-line tail as the existing spawn-time run, which stays and is shared by co-launched reviewers. Available to `review`, `work`, `plan` and `full`, not `scout`.
- Role toolsets after this spec: `review` is read, find, execute, submit; `scout` is read, find, execute, submit; `work`, `plan` and `full` gain `execute` and keep their existing tools. The oversize message in `read` names `tools.grep` in `execute` or `read` with offset and limit when the role has no bash, and `bash` when it does. The reviewer and scout agent types list `execute`.

**Role prompt directions (ticket 18).** Each worker-class role prompt gets one positive line: search and process content with `execute`. The `execute` description carries the how-to, including a two-line example of `tools.grep`. Reviewer and scout agent types get one line each: use `tools.grep` for exact lookups, `find` for concepts, `tools.git` for history and the diff, `tools.check` (reviewers) to verify a worker's claim. No prohibitions.

**Dependency installer (ticket 19).** `emberloop deps` (and the same step at the end of `emberloop install`):
- Checks `python3` (3.9 or newer), `rg`, `git`, `bwrap`, and, unless disabled in settings, the code graph binary.
- A missing system package is installed with the detected package manager (`pacman`, `apt`, `dnf`, `zypper`, `brew`), printing the exact command and running it through `sudo` only when attached to a terminal; otherwise it prints the command and exits non-zero. Package names per manager live in one table.
- The code graph binary is downloaded from its GitHub release for the platform, verified against the release's published SHA-256, and placed in `~/.emberloop/bin` (`EMBERLOOP_BIN_DIR` overrides). Its own `install` command is never run and no agent client config is touched.
- Emberloop resolves these binaries from `~/.emberloop/bin` first, then `PATH`.
- After installing, each dependency is proved, not assumed: `python3 -I -c` runs, `rg --version`, `git --version`, the bubblewrap self-test from the execute section, the graph binary answers an MCP `initialize`. The command prints one line per dependency with its version and status, and exits non-zero if any required one fails. Optional ones (the graph binary) warn.
- Idempotent: a second run changes nothing and says so.

**Code graph integration (ticket 20, optional).**
- Uses codebase-memory-mcp (MIT, a single native binary, stdio MCP server), installed by the dependency installer (ticket 19). Emberloop finds it in `~/.emberloop/bin`, on `PATH`, or at `graph.binary` in settings. It never runs the binary's `install` command and never writes to agent client configs. The graph is optional: `graph.enabled` in settings turns it off, and if the binary is absent nothing graph-related is exposed and nothing errors.
- Registered through the ticket 10 and 15 machinery as an `mcp_servers` entry named `graph`. Exposed tools are `search_graph`, `trace_path`, `get_code_snippet`, `get_file_outline` and `get_architecture`; `detect_changes` (fails inside git worktrees in the spike) and the index management tools (`index_repository`, `list_projects`, `delete_project`, `index_status`, `check_index_coverage`) are excluded from every agent. Placement is `main` for `review` and `scout` and `execute` for the other roles, using per-role placement.
- Lifecycle, driven by emberloop and invisible to agents: an asynchronous index of the worktree when emberloop creates it; an incremental re-index just before each reviewer or scout spawn so the graph matches the tree under review (the file watcher is not relied on); `delete_project` for the worktree's project in `finish`, and on any path that drops a worktree. Indexing failure is logged and never blocks spawn or finish. Emberloop waits on the index status for a short bounded time before the first graph query returns rather than answering from a missing graph.
- Project names are path slugs, so worktrees never collide with each other or the main checkout, and the main checkout's index already excludes `.emberloop-worktrees`. Cache directory: the binary's default, unless `graph.cache_dir` is set in settings (then passed as `CBM_CACHE_DIR`).
- Not in this spec: replacing the candidate stage of `find` with the graph. Decide after the tool has been used on real reviews.

## Testing Decisions

- A good test drives the tool through its public contract (the tool's name, arguments and returned text, as the MCP server calls it) and asserts on observable results, not on internals.
- **Websearch:** the highest seam is one tool call. Provider adapters are exercised against fake local HTTP servers returning recorded Brave and Exa responses, including quota headers, 429, 5xx, empty results and bad-query 4xx. Cover chain order, named-provider no-fallback, missing key, cache hit, `fresh`, and parallel merge order. No live calls in the standard check; a live probe goes with the other live probes.
- **URL read:** one seam, the `read` tool with a URL path, against a local test server serving HTML, JSON, text, a PDF stand-in, a challenge page, a thin page, slow and oversized responses, and redirects to private addresses. A fake Jina endpoint covers tier 2. Assert escalation triggers, the header's tier name, selectors, truncation with the overflow file, and that blocked addresses are refused on every hop.
- **SQLite:** import from fixture JSONL, restart idempotence, concurrent writers with a reader, `prune`, and schema version upgrade. The existing usage tests stay the prior art; a snapshot-style read of the database mirrors how the TUI reads.
- **Scout hotfix:** the existing fake-herdr tests for spawn, tell and finish are the prior art. Assert that the pane close call is issued after a scout's result is delivered, that `spawn` output contains both ids, that `tell` and `finish` resolve a pane id, and that the unknown and both-given cases fail clearly.
- **Antigravity retry:** stream-shaping tests with fixtures, as the extension's existing tests do (no live calls): a first response ending in a malformed function call followed by a good one succeeds; three malformed responses give the specific error with the finish message; an empty `STOP` keeps the old error; no retry once partial content was emitted.
- **Dead children:** the existing fake-herdr and agent-state fixtures are the prior art. Assert that a child with no submit and no pane is not pending and produces one lost notice, that a live child still lets the parent stop, that an errored turn produces a stopped-with-error notice, and that the gate runs the sweep before allowing.
- The repo check (`vet` and the non-live tests) must pass for every ticket.
- **Tool settings:** table tests on the resolution function: global and project merge, per-role override, rename, include and exclude globs, disabled tools absent from both the main list and `execute`, `submit` undisableable, settings only narrow a role's allowlist. Compare the MCP server's `tools/list` output with the generated `execute` stub for the same settings.
- **MCP client:** a fake MCP server (a small Go test helper over stdio) covers initialize, list, call, a server that never answers, a server that exits, and child cleanup on exit.
- **Execute:** run real `python3` in tests (skipped with a clear message if absent). Cover native tool calls through the socket, `ToolError` on a refused call, a disabled tool unavailable in the script, `Guard` still enforced for a guarded role, timeout kills the process group, traceback tail on exception, overflow with and without `intent`, and temp directory removal. MCP tools in the stub use the fake server.
- **Overflow store and `ctx_search`:** chunking keeps code blocks whole, BM25 ranks an exact-term chunk first, handle scoping, porter stemming matches `caching`/`cached`, purge after 24 hours, and the store used by `bash` and URL `read` above the threshold. The pure-Go driver must provide FTS5; a test creates the virtual table so a driver without it fails loudly.
- **`grep`, `git`, `check`:** `grep` against a fixture tree (regex, fixed string, glob, context, cap, `.gitignore`, binary skip, no `rg` fallback). `git` against a temporary repository: each allowed subcommand works, every refused flag and subcommand is refused, and the repository is byte-identical afterwards. `check` with a plain command, a named map, args substitution, a refused metacharacter, timeout and failure tail.
- **Code graph:** a fake MCP server stands in for the binary. Assert the exposed tool set per role, that management tools are never exposed, index on worktree creation, re-index before a reviewer spawn, `delete_project` on `finish`, failure never blocking spawn or finish, and a missing binary exposing nothing. A live probe against the real binary sits with the other live probes, outside the standard check.
- **Sandbox:** bubblewrap tests (skipped with a clear message if `bwrap` is absent): a sandboxed script cannot write inside or outside the worktree, cannot open a network socket, can still call tools through the socket, and emberloop-side `check` can still write its caches. A broken sandbox (forced self-test failure) makes `execute` for `review` and `scout` refuse, naming `emberloop deps`.
- **Installer:** a fake package manager and a fake release server cover detection, the install command chosen per manager, non-terminal behaviour (prints the command, exits non-zero), checksum verification (a wrong checksum is refused), placement in the bin directory, the per-dependency proof lines, and idempotence on a second run. No real package installs or downloads in the standard check.

## Out of Scope

- Tavily, Serper, Perplexity and other providers (the adapter interface leaves room).
- Exa-specific features (neural similar search, categories, page text in the search call).
- Enforcing quota or budgets, and any cost estimation.
- A page cache for fetched URLs.
- Site-specific scrapers, notebook, archive, SQLite and image reading in `read`.
- Choosing or building the stealth browser (research and the browser tier are follow-up tickets).
- Migrating workers, runs, the archive index and memory logs to the database.
- A keep or follow-up option for scouts, an idle reaper, and a command to prune existing scout panes or agent records.
- Storing any API key in the database or repo.
- Languages other than Python in `execute`, and a persistent Python session between `execute` calls.
- Replacing `find`'s candidate stage with the code graph.
- Mutating git commands, and arbitrary commands in `check`.

## Further Notes

- All three keys live in the user's fish configuration: `BRAVE_API_KEY`, `EXA_API_KEY`, `JINA_API_KEY`. `TAVILY_API_KEY` is intentionally unset. Running sessions need a restart to pick up `JINA_API_KEY`.
- The Jina key was pasted into a chat transcript, so the user should rotate it.
- The reference for the URL read design is omp's URL read path in its fetch tool; this spec deliberately copies a small part of it.
- The TUI (`emberloop-tui`) is now in this repository and reads usage logs incrementally from the per-agent JSONL files (see `.scratch/emberloop-tui/spec.md`, read model). Moving usage into SQLite therefore also moves the TUI's usage reading: ticket 03 updates the TUI read model to open the database read-only and keeps its tests passing.
- The reviewer slots in settings now use the Antigravity Gemini 3.8 Flash model through pig. That was a settings change made during the interview and has not been tested with a live review.
- The tool shapes follow context-mode (github.com/mksglu/context-mode): sandboxed execute that returns only stdout, SQLite FTS5 with BM25 and porter stemming, `intent`-driven filtering. The code graph is DeusData/codebase-memory-mcp, spiked on this machine: path-slug project names, `delete_project` removes the project's database, about 3.4s CPU and 16MB for this repository, and a one-shot CLI call costs 2-3s, so emberloop keeps one MCP session open.
- The retro behind the read-only toolkit: reviewer transcripts under `~/.pig/state/emberloop/readonly/sessions` show only `read`, `find` and `submit` calls, `find` fanning out to 10-20 backend generations per call, and sessions reaching about 188K cached tokens.
- `execute` is on by default for every role that has any tools beyond orchestration; the way to turn it off, or any other tool, globally or per role, is the settings above. Reviewers and scouts run it sandboxed.
