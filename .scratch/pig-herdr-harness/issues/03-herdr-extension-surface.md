# What does herdr expose for a harness that is not a built-in agent kind?

Type: research
Status: resolved
Blocked by:

## Question

`pig` is not one of herdr's 24 compiled-in agent kinds and cannot become one without an upstream binary change. What does herdr expose anyway — how does a non-native agent become visible, scriptable, and waitable, and what exactly is lost?

## Answer

**No blocker. `pig` self-reports under its own name; everything downstream works.**

### Agent kinds are compiled in, and plugins cannot add one

`herdr agent start --kind` accepts exactly 24 values: `pi claude codex gemini cursor devin agy cline omp mastracode opencode copilot kimi kiro droid amp grok hermes kilo qodercli qwen letta maki muse`. The literals are baked into the binary (`strings $(which herdr)`). Plugin v1 manifest sections are `[[build]] [[startup]] [[actions]] [[events]] [[panes]] [[link_handlers]] [[keys.command]]` — there is no agent-kind entrypoint, and the docs state runtime action registration and non-terminal plugin UI are out of scope for v1.

### Detection: self-report beats screen-scraping

Two paths. Screen-manifest detection reads the bottom of the pane. Self-report lets the agent push state itself, and **when an integration reports state, herdr uses those reports instead of reading the screen**. The live `screen_detection_skipped:true` on this machine's omp panes is that switch — their idle/working states came from the extension, not the screen.

The documented path for a new agent (`docs/add-herdr-support.mdx`):

```bash
"$HERDR_BIN_PATH" pane report-agent "$HERDR_PANE_ID" \
  --source pig --agent pig --state working --seq 1
```

guarded by `HERDR_ENV=1`. `--agent` is the user-facing name and must be the agent's own, not a name herdr already supports; `--source` must not start with `herdr:`. Release on exit via `pane release-agent`. After that, `herdr agent list`, `agent wait`, `agent prompt` and `agent read` all work on `pig`.

### What `pig` loses, and the workaround

Lost: `herdr agent start --kind pig` (managed launch + readiness wait), the screen-manifest fallback, and `herdr integration install pig`.

Workaround: `herdr pane run <pane> pig …` instead of `agent start`. This costs little — `agent start` is documented as never creating, splitting or moving layout anyway, so the real loss is the readiness gate, which `agent wait --until working` replaces.

### Two transports, one API

The installed `pi` extension posts JSON over `$HERDR_SOCKET_PATH`; the documented path shells out to `$HERDR_BIN_PATH`. Both land in the same `pane.report_agent` API. Shelling out is the portable one (it survives Windows named pipes), and it is one exec per state change rather than a socket client.

**Resume is the one caveat:** docs say resume commands need herdr ≥ 0.10.0; this host runs 0.9.1 (docs index says stable is 0.9.3). State reporting and release work today. [UNVERIFIED] — resume was never exercised here. If run resumption matters, verify before relying on it.

### The whole orchestration loop is scriptable

```bash
created=$(herdr workspace create --cwd ~/project --label api --no-focus)
pane_id=$(jq -r .result.root_pane.pane_id <<<"$created")
herdr pane split
herdr agent start reviewer --kind claude --pane "$pane_id" -- -m claude-opus-4-5
herdr agent prompt reviewer "…" --wait --timeout 120000
herdr agent read reviewer --source recent-unwrapped --lines 120
```

`herdr worktree create --workspace ID --cwd PATH --branch NAME --base REF --path PATH` requires the workspace to sit inside a git work tree. CLI errors emit JSON on stderr with exit 1 (2 for bad syntax). Waits have no default timeout — always pass `--timeout`. Moving a pane re-IDs it; continue from `.result.move_result.pane.pane_id`.

### Plugin manifest, if one is ever needed

`id name version min_herdr_version` plus the sections above. Actions get `HERDR_PLUGIN_ACTION_ID`, `HERDR_PLUGIN_CONTEXT_JSON` (workspace/tab/focused-pane/worktree/agent/selected-text/clicked-URL fields), `HERDR_SOCKET_PATH`, `HERDR_BIN_PATH`, `HERDR_ENV=1`, `HERDR_PLUGIN_ID/ROOT/CONFIG_DIR/STATE_DIR`. argv only, no shell; cwd is the plugin dir. There is no stdin contract — context arrives via env, callbacks go out through the `$HERDR_BIN_PATH` CLI. v1 has no herdr-managed plugin storage; state and config are plain plugin-owned files.

**Not needed at the destination.** The self-report path needs no plugin at all. Recorded so the option is known, not because it is owed.