# Emberloop

A browser UI and local daemon that run coding-agent sessions and deterministic implement/review Runs, and show them live.

## Language

**Daemon**:
The local Go process that owns every Session, talks to Backends, and serves the UI over HTTP and server-sent events.
_Avoid_: Server, backend service

**User**:
A signed-in account on the Daemon, with a username, a password and an admin flag. The first signup is the admin.
_Avoid_: Account, member, login

**Machine token**:
The Daemon's bearer token for local machine clients: agent callbacks and the `el` CLI. It is not a User, so it cannot write admin-only settings or create Sessions and Runs.
_Avoid_: API key, service token

**Session**:
One conversation between a user and one agent, with a harness, a model and a working directory. Persisted as an event log.
_Avoid_: Chat, thread, run

**Project**:
A registered directory that Sessions and Runs belong to. A Session started in a Project uses the Project's path as its working directory. The UI scopes its sidebar, dashboard and URLs by Project.
_Avoid_: Workspace, folder, repo

**Dashboard**:
The global `/` view of active Sessions and Runs that are working or need you, grouped by Project. Idle history stays in each Project view.
_Avoid_: Home, activity feed

**Provider**:
A model service or authentication account listed in the Providers area; some
also expose quota. It is distinct from a Backend, which drives a Session.
_Avoid_: Backend, harness

**Quota percent**:
The integer 0–100 percentage of a provider limit already used. It is the common value for quota bars and Live status rows; the UI does not derive it from money or tokens.
_Avoid_: Remaining percent, balance

**Backend**:
The adapter that drives one harness behind the Daemon's `Agent` interface: claude and grok through ACP, pig through ACP or its Go SDK.
_Avoid_: Driver, adapter

**Handle**:
The harness-side agent process and resume token behind one Session. A Compress swaps the handle and the Session ID stays. Every handle start derives its owner, role and injections from the Session record (`startHandle`).
_Avoid_: Connection, process, harness session

**Subagent**:
A Session spawned by another Session through the `spawn` MCP tool. It records its parent Session.
_Avoid_: Child agent, worker, sub-session

**Group**:
The Subagents created by one `spawn` call. The parent receives one grouped result when the last member finishes.
_Avoid_: Batch, swarm

**Steer**:
A prompt delivered to a running Session mid-turn: at its next tool boundary for pig, injected into the turn through the adapter for claude. A parent steers its own Subagent with the `steer` tool; an idle child starts as a follow-up instead.
_Avoid_: Interrupt, nudge

**Follow-up**:
A prompt delivered to a Session after its current turn ends.
_Avoid_: Queue message, next prompt

**Switch**:
A User's change of Backend and model for a paused Ticket or Run. It replaces the active Chain entry; only a manual Resume starts a new Session with a Handoff on the same branch.
_Avoid_: Restart, fallback

**Check**:
A read-only peek by a parent at its own Subagents: one status line each, or one child plus its recent events. Never a poll: the grouped result still arrives on its own.
_Avoid_: Poll, status query

**Chain**:
One Run role's ordered Backend/model entries. A quota, credit, auth or session-limit failure advances to the next entry; the active position is shown on each Ticket.
_Avoid_: Fallback list

**Cancel**:
Stopping a parent's own Subagent mid-turn without touching the parent turn; kill also ends the child Session. A stop, not a message, so scouts can be stopped.
_Avoid_: Kill, abort

**Event**:
One normalized record of something a Session did or received: message chunk, tool call, prompt, status change.
_Avoid_: Update, message

**Event log**:
The append-only file of a Session's Events. The UI renders a Session by replaying it.
_Avoid_: Transcript file, history

**Owner**:
The User who created a Session or Run. Subagents, Workers and Reviewers inherit it. Data with no owner is hidden.
_Avoid_: Creator, author, tenant

**Sender**:
Who typed a prompt Event: a signed-in User (`user`, with the User's ID) or the Daemon and agents (`system`).
_Avoid_: Author, from

**Run**:
One execution of the deterministic pipeline over one spec: it drives every Ticket of that spec to merged, parked or discarded.
_Avoid_: Job, pipeline instance

**Ticket**:
One unit of work from a spec, kept as a file with blockers, status and checkboxes. Each Ticket gets one Worker.
_Avoid_: Task, issue

**Worker**:
The Subagent that implements one Ticket in its own worktree and branch.
_Avoid_: Implementer

**Reviewer**:
A persistent Subagent that judges one Ticket on one axis, spec fit or repository standards, and keeps its context across Passes.
_Avoid_: Judge, checker

**Pass**:
A Reviewer verdict recorded at a specific tip of a Ticket's branch. A Pass at an older tip is stale and does not count.
_Avoid_: Approval


**Disposition**:
A Worker's recorded response to a Reviewer finding: fixed, declined with a reason, or deferred to a Ticket. Blocker and major findings require one; minor findings are optional and do not block.
_Avoid_: Resolution, answer

**Needs-you**:
The state of a Ticket that the Run cannot advance without the user. The UI shows it as a banner with actions.
_Avoid_: Alert, escalation

**Compress**:
Shrinking a Session's harness context by Handoff: a pig model writes a Handoff, the Daemon starts a fresh harness handle for the same Session and sends it. The Session ID and Event log stay.
_Avoid_: Compact, summarize, reset

**Handoff**:
The document a pig model writes from a Session's Event log, sent as the first prompt of the new harness handle after a Compress. It is shown as a collapsed block, not as user input.
_Avoid_: Summary, checkpoint

**Fold**:
Part of a Compress for input over the summarizer window: the Event log is summarized in parts, the part summaries are summarized again, and the Handoff call reads what fits. Oversize tool results are cut first.
_Avoid_: Truncate, trim

**Blocked**:
A Session whose Compress stopped on a credit or auth failure. Its new messages queue and do not run until a Retry succeeds or the block is dismissed.
_Avoid_: Stuck, paused, frozen

**Idle**:
A Session is idle when no turn runs or starts, no input is queued, no Group it leads is outstanding (a member is working, or a fresh Group's results are unsent), and its last Event is older than the quiet time. The Daemon is idle when every Session is idle, none is starting and no Run runs.
_Avoid_: Ready, free

**Bank**:
One User's memory file: a SQLite database holding that User's observations, Rules, queue, Profile versions and ledger. The Daemon owns every bank.
_Avoid_: Store, memory file, database

**Observation**:
A grounded candidate: a verbatim quote from a User's prompt, with its Event ID. Searchable through recall, never injected.
_Avoid_: Hint, log entry

**Rule**:
An approved note of at most 300 characters that recall finds and that can be injected into a Session. An edit adds a version; the old version becomes superseded.
_Avoid_: Note, memory item, lesson

**Purge**:
The one human action that deletes memory rows. It removes the chosen records and clears their text from the queue, the ledger and the snapshots.
_Avoid_: Delete, forget, wipe

**Boot payload**:
The route's prefetched data that the Daemon embeds in the page it serves. The browser reads it first and fetches only what it lacks.
_Avoid_: Preload, hydration data, initial state

**Prefetch table**:
The Daemon's list of routes, each naming the data its page needs on first load.
_Avoid_: Route loader, route map
The route-to-API mapping the Daemon uses to load page data in-process before serving a page shell.
_Avoid_: Route fetch list
The user-specific JSON results of route prefetch, embedded as `window.__EL_BOOT__` for the browser to consume.
_Avoid_: Bootstrap data
