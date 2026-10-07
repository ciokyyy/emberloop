# Spec: emberloop TUI with Runs and Settings tabs

Status: ready-for-agent

## Problem Statement

While specs are being implemented I can't see, in one place, which specs are running, which tickets are done or stuck, which agents are attached, what they cost, or how much memory they use. I have to hunt through herdr tabs and JSON files. Changing default harnesses and models means editing JSON or remembering CLI subcommands.

## Solution

`emberloop tui` opens a mouse-clickable terminal UI in the current pane, in its own Go module built on bubbletea, lipgloss, bubbles and glamour. It has two tabs (Stats is added by the stats spec). **Runs** lists specs from every repo on the left with status, ticket progress, live cost and memory; the selected spec's tickets on the right; a ticket detail view with the ticket's Markdown and its agents, each with harness, model, status, tokens, reported cost and RSS. Enter or a double-click on an agent jumps to its herdr tab and pane while the TUI stays where it is. **Settings** edits `~/.emberloop/settings.json` with model pickers per harness. It refreshes by polling every second.

## User Stories

1. As a user, I want to run `emberloop tui` in any herdr pane, so that I get the dashboard where I am.
2. As a user, I want specs from every repo grouped by repo, current repo first, so that I see all my work.
3. As a user, I want specs with no run yet listed too, so that I can see what's waiting.
4. As a user, I want each spec to show running/ended status, `merged/total` ticket progress, live token total, reported cost and memory, so that I can scan everything at a glance.
5. As a user, I want the selected spec's tickets with a status glyph each: not started, working, retrying, idle, submitted done or blocked, in review with code and spec verdicts, merged, discarded, failed, so that I see where each ticket is.
6. As a user, I want to open a ticket and read its title and Markdown body rendered and scrollable, so that I know what it's for.
7. As a user, I want the ticket detail to list every agent attached (worker, code reviewer, spec reviewer, scouts) with harness, model and effort (`opus · high`), status, tokens, reported cost and current RSS, plus ticket totals, so that I see who works on it and what it costs.
8. As a user, I want the orchestrator shown on its spec, so that its spend is visible.
9. As a user, I want to press Enter or double-click an agent and land in its herdr pane, so that I can watch or talk to it.
10. As a user, I want the TUI to keep running in its pane when I jump, so that I can come back to it.
11. As a user, I want single click to select and double click to open, so that the mouse works like I expect.
12. As a user, I want vim keys and arrows, tab to switch tabs and `q` to quit, so that I never need the mouse.
13. As a user, I want numbers updating every second, so that costs and memory feel live.
14. As a user, I want `find`/jev usage shown per agent next to the harness tokens, so that I see both kinds of spend.
15. As a user, I want a model with no reported cost to show tokens and a "no cost reported" mark, so that nothing is guessed.
16. As a user, I want a Settings tab listing every role slot with its harness, model and effort, so that I can change defaults visually.
17. As a user, I want the model picker filled from the chosen harness's `Models()` list, with free text allowed, so that I pick valid ids but am never blocked.
17a. As a user, I want the effort shown as a row of the levels the selected model supports with the chosen one highlighted (`low  medium [high]  xhigh  max`), hidden when the model has none, so that I can see and change it at a glance.
17b. As a user, I want `r` in Settings to refresh the harness model lists, so that new models show up.
17c. As a user, I want switching a slot's harness to keep model and effort when still valid, and otherwise reset them with a short notice, so that I never save an impossible combination.
18. As a user, I want to see where each value comes from (file or built-in default), so that I know what's set.
19. As a user, I want the archive size shown in Settings, so that I know when to prune.
20. As a user, I want machine-wide memory used by all running agents shown, so that I see total load.

## Implementation Decisions

 - Own module beside the core and herdr-report, so the core module stays dependency-free. Base deps: bubbletea, lipgloss, bubbles, glamour. `emberloop tui` execs the TUI binary (or the TUI binary is installed beside it; decided in the first ticket).
 - UI toolkit (fits spec directly): bubbles/list + bubbles/viewport for runs/tickets list and scrollable ticket Markdown; bubbles/table for agent rows (harness, model, tokens, cost, RSS); bubbles/textinput for free-text model id in Settings; bubbles/progress for merged/total bar; charmbracelet/huh for Settings forms/select pickers (harness → model → effort, embeds in bubbletea); lipgloss/tree (lipgloss ≥0.12) for spec → ticket → agent tree rendering.
 - Mouse via lrstanley/bubblezone named click regions (single-click select, double-click open; double-click detection stays ours — track last click time + zone). Polling via tea.Tick every second, no extra dep.
 - Version lock: pick bubbletea v1 vs v2 up front (charm.land/... paths, changed mouse/key message types in v2); bubblezone and some bubbles components lag one major behind — run `go list -m -versions` before locking. Explicitly skipped: ntcharts (Stats later), evertras/bubble-table (only if bubbles/table falls short), tview/gocui/ratatui ports (different paradigm, don't mix).
- A read model is the deep module: `Snapshot(stateDir, herdr) → []Repo{Specs{Run, Tickets{Status, Agents{harness, model, status, tokens, cost, rss}}}}`. It reads run records, worker entries, ticket history, reviewer records, submits, usage logs and the latest memory samples from the run-records spec, and spec directories on disk. Ticket status derivation from those signals lives here, in one function.
- Specs with no run are discovered from known repos' spec directories (repos taken from run records plus the current repo).
- Usage logs are read incrementally with per-file offsets so polling every second stays cheap.
- Jumping: `herdr tab focus` then pane focus for the agent's recorded pane, through an `Exec` seam. The TUI never closes or moves panes.
- Settings tab edits through the global settings spec's load/save functions and gets choices from each harness's cached `Models()`; no separate format and no hard-coded model lists in the TUI.
- Status glyphs and colours defined once in the view layer.
- Display only: no starting, killing, telling, merging in v1.

## Testing Decisions

- Most logic lives in `Snapshot`; it is tested against fixture state directories (run records, workers, history, reviewers, submits, usage logs, memory samples) asserting the derived tree, ticket statuses and totals.
- Ticket status derivation gets a table test over every status in the list.
- The bubbletea model gets a few tests that feed key and mouse messages and assert: selection moves, Enter on a ticket opens detail, Enter on an agent calls herdr focus with the right tab and pane through a fake `Exec`, settings edits save the file.
- Prior art: the core's fake-`Exec` pattern and temporary state directories.

## Out of Scope

- Stats tab (stats spec).
- Actions on runs (start implement, kill, tell, merge): later; start implement first.
- Subscription quota bars.
- Emberloop-calculated prices.
- Auto-opening the TUI from `implement`.
- Web search tool: needs its own grill session.

## Further Notes

Spec 4 of 5. Blocked by the run-records spec (data) and the global settings spec (Settings tab).
