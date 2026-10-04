Label: wayfinder:map
Status: open

## Destination

A harness that automates Matt Pocock's implementation route. `/implement <ticket-path>` in a PiG (`pig`) session hands a spec to herdr; herdr schedules N worker agents — `pig` or `claude` — across the spec's dependency DAG, each in its own git worktree; the run reports status to a human off-box. Runs on a VPS, reached over Tailscale.

## Notes

- Domain: agent-delegation harness. `pig` (PiG, pi-in-go.dev) is the trigger surface and one worker harness; `herdr` owns panes, worktrees and agent-state reporting; `claude` is the second worker harness.
- Required skills for every session: `wayfinder`, `domain-modeling`, `grilling`, `research`, `prototype`.
- Standing preferences:
  - Simplest thing that works. No new UI: herdr already ships sidebar, web and push plugins. The trigger is a pig slash command.
  - Manual up to the spec. Grilling and wayfinding stay HITL. Only the implementation step is automated.
  - Human keeps the veto. Workers open PRs and stop. No auto-merge.
  - Bidirectional human Q&A is an exploration, not a requirement. The transport already works (ticket 02); question *detection* does not transfer to pig. Do not build a UI for it until a real blocking question appears.
- Established 2026-10-05, verified on this machine — context, not decisions:
  - `pig` 0.4.0+1.0.0 installed at `~/go/bin/pig` (via `go install`); `fish_add_path` added to `~/.config/fish/config.fish`.
  - PiG loads `.ts` extensions from `~/.pig/agent/extensions/`. herdr's `herdr-agent-state.ts` loads there unmodified.
  - PiG ctx carries `mode` (returns `"print"` in `-p` mode) and `isIdle()`. `session_start`, `agent_start`, `agent_settled` all fire.
  - **herdr has no `pig` agent kind** — a live pig TUI in a herdr pane does not appear in `herdr agent list`. Ticket 03 resolved this: self-report bypasses the compiled-in kind list entirely.
  - herdr already offers `workspace create/send-text/send-keys/read`, `worktree create/open/remove`, `agent prompt/wait/read`, `plugin link/action invoke/pane open`, and an `integration` slot per supported harness.
  - No `pig` API key configured yet; first run needs `/login`.

## Decisions so far

<!-- the index: one line per closed ticket, enough to judge relevance, then zoom the link for the detail the ticket holds -->

- [What is Matt Pocock's implementation route, mechanically?](issues/01-the-route.md): The automation target already exists. `implement-spec` is a spec+task-graph fan-out harness, `to-tickets` already writes a machine-readable `Blocked by:` DAG, `triage/AGENT-BRIEF.md` already is the worker brief, and `implement` already carries its gate. The destination is a runtime for these, not a new orchestration layer.
- [What does herdr expose for a harness that is not a built-in agent kind?](issues/03-herdr-extension-surface.md): No blocker. Agent kinds are compiled into herdr's binary and plugins cannot add one, but any agent self-reports under its own name via `herdr pane report-agent --agent pig`, and screen-detection yields to self-report. Only `agent start --kind pig` is lost, replaced by `pane run` plus `agent wait --until`.
- [What is the notification and reply channel from a VPS over Tailscale?](issues/02-notification-channel.md): The `herdr-remote` relay already runs here on `127.0.0.1:8375` behind a Cloudflare tunnel, token-authenticated. Outbound is a `POST /?d=`. Inbound is `herdr pane send-text` — unconditional, so bidirectional works. What does not transfer is `detect_question`, which matches the OMP TUI's "Other (type your own)" and will not fire on pig.

## Not yet specified

<!-- see "Fog of war": in-scope fog you can't ticket yet; graduates as the frontier advances -->

- **Question detection for pig.** A worker needs a human and the relay cannot tell. `detect_question` recognises only the OMP/pi TUI's option blocks. Decide whether a pig-side question surface renders into a recognisable shape, or whether questions stay plain text a human reads and answers by hand.
- **Where the relay lives.** It runs on this laptop. Either a second relay on the VPS or a Tailscale bind — undecided, and it decides how the harness reaches it.
- **The reply client.** The relay's own WebSocket client, or web push (installed, zero subscriptions, blank VAPID key). Which device the human answers from.
- **Worker failure policy.** Retry count, repair turns, and what "stuck" means for a worker — depends on the worker contract (04) and the executor loop (05).
- **Multi-worker integration.** How N worker PRs from one spec get rebased, ordered and merged. Depends on the executor loop (05).
- **Spec-to-spec DAGs.** Running several specs where one blocks another. The destination is one feature at a time.
- **Other pig plugins.** Beyond the implement harness — parked deliberately, revisit after one run works end to end.
- **Observability surface.** Anything richer than a status notification: live output streaming, diff viewing, cost per run.
- **Provider and budget policy.** Per-worker cost caps, and whether the VPS needs a different provider setup from the laptop.

## Out of scope

<!-- see "Out of scope": work ruled beyond the destination; closed, never graduates -->

- **Upstreaming a `pig` agent kind into herdr.** Real work in someone else's binary. The harness works around pane detection instead. Returns only if the workaround proves untenable.
- **Building new UI.** herdr-sidebar, herdr-web and herdr.push are already installed. The trigger is a pig slash command.
- **Auto-merge.** Workers open PRs and stop. The human merges.
- **Automating grilling, wayfinding or spec-writing.** The destination begins at a finished spec.
- **General-purpose task runner.** This automates one route, not arbitrary delegation.
- **Replacing or patching pig and herdr.** Both are consumed as they ship.