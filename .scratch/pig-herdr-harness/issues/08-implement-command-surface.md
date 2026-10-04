# What does `/implement` actually look like in a pig session?

Type: grilling
Status: open
Blocked by: 04, 05

## Question

The trigger is `/implement <path-to-ticket>`, typed in a pig session. Decide the surface around it.

1. **What the path points at.** `to-tickets` writes `.scratch/<feature>/issues/<NN>-<slug>.md` with `Blocked by:` edges. So `/implement` could take a single ticket, or a feature directory and run every ready ticket, or `spec.md` and generate tickets first. Ticket 01 confirms ticket-granularity approval is a human gate, so tickets must already exist — which rules out the last option. Decide between one ticket and a whole ready frontier.
2. **Where it is implemented.** A pig extension registering a slash command, a skill invoked by name, or a prompt template. An extension has real tools and lives in the TUI; a skill is prose an agent follows. The route research points at the extension, because this needs to drive herdr and hold state, not advise.
3. **What the user sees.** The run is long and mostly unattended. Decide what the command prints as it starts, where progress is visible afterwards, and how to stop or resume a run.
4. **Blocking confirmation.** `/new`, `/resume`, `/fork` and `/clone` all rebuild extensions from scratch — a fresh runner, fresh extension processes, a new `session_start`. Decide whether a run in flight survives those, and what happens to its worktrees if it does not.
5. **Where run state lives.** A run spans a VPS restart. PiG offers `~/.pig/state/<extension-id>/` for extension-owned namespaced state — decide whether that is the store, or whether herdr's own view of panes and worktrees is sufficient to reconstruct a run in progress.
6. **Naming.** PiG extension identity is established by runtime registration, not a manifest file, and `/reload` re-resolves the same inputs in first-seen order. The extension must survive reload with its run state intact, or declare that it does not.