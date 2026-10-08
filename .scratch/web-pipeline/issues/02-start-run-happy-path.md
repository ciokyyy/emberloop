# 02: Start a Run and merge Tickets with Worker Sessions

**What to build:** From the preview, the user clicks Start. The Daemon runs the core deterministic driver with a web-backed world. Each Ticket starts in number order once its blockers merged. Each gets a Worker Session (a Subagent of the Run) in its own worktree and branch, and merges by rebase plus fast-forward-only when the Worker submits done. The Runs view shows a live Ticket table (state, Worker Session link) updated over server-sent events, and the Worker link opens the normal Session transcript. Workers use the harness, model and effort from the settings roles, falling back to the Session default. Only one active Run per spec; Runs for different specs may run side by side. Reviewers and the gate come in ticket 03, so here a submitted done merges after the existing core merge checks that do not need Reviewers. Where core assumes a herdr pane, core accepts it being absent.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] Start creates a Run and returns it; a second Start on the same spec while active is refused
- [ ] Tickets start in number order, only after blockers merged, up to the live-Session cap
- [ ] Each Ticket has a Worker Session in its own worktree and branch, recorded as a Subagent of the Run
- [ ] A Worker that submits done is merged by rebase and fast-forward-only; a dirty worktree or rebase conflict keeps the Worker alive and shows the refusal
- [ ] Checkboxes in the ticket file are ticked from the Worker's completed list, written only by the driver
- [ ] The Runs view shows the live Ticket table and links each Worker to its Session view
- [ ] Roles from settings choose harness, model and effort; a missing role falls back to the Session default
- [ ] API tests run a whole Run with a fake Backend and fake git; a view test covers the table
- [ ] No herdr import in web; core exports only what the world needs
