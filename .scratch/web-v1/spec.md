# Web v1: Handoff compress, Users, Memory

This spec merges three earlier specs (Handoff compress, Multi-user, Memory and Profile). It also carries the review decisions of 2026-10-10 that replaced the mnemopi sidecar with an in-process Go engine. ADR `docs/adr/0003-in-process-memory-engine-with-grounded-rules.md` records why.

## Problem Statement

1. A long Session fills its harness context window. Only a pig Session on the SDK Backend can compact, and it compacts in place with the harness's own summarizer. Claude Sessions and pig-over-ACP Sessions cannot compress. The user cannot choose the summarizer model or control what the summary keeps.
2. The Daemon has no notion of who is using it. Two people on one tailnet share one host, one set of provider keys and one Session list, but cannot tell whose Sessions are whose or who sent a prompt.
3. Sessions forget everything when they end. A User cannot tell an agent once how they want it to behave, and keeps repeating the same corrections.

## Solution

**Handoff compress.** Compress a Session by Handoff. A user-chosen pig model writes one Handoff document from the Event log. The Daemon starts a fresh harness handle for the same Session and sends the Handoff as its first prompt. A fallback chain always yields a Handoff. Only an unusable summarizer account (credits, auth) stops the Session.

**Users.** Username and password accounts with a cookie login (the tailnet is the perimeter). Every Session and Run records its creating User as `owner`. Every prompt records its sender. The sidebar groups Sessions by User. Provider keys, quota and `max_workers` stay host-global.

**Memory.** Two artifacts, one engine:

- **Profile**: a small per-User file of standing instructions, always injected. Changes are line-ID'd patches that a human approves. Every version is kept.
- **Rules**: short approved notes mined from what Users said in past Sessions (preferences, corrections, decisions with rationale, "we tried X and it failed"). Rules live in a per-User SQLite bank owned by the Daemon, in-process, Go only. A candidate becomes a Rule only after a human approves it. Project knowledge belongs in `AGENTS.md` (DOX) or in reviewed Project notes files inside the repo, never in a shared database.

The main goal of Memory is less repeated correction (UX), not task success. It is measured that way.

## Design rules

These rules bind every ticket below. Each has a test in ticket 31.

1. **Never delete.** No row is deleted outside a human purge. A schema trigger aborts every `DELETE`. Edits are new versions.
2. **Grounding.** Every candidate carries an Event ID and a verbatim quote. Go checks that the Event has `sender=user` and that the quote is a substring of its text. Anything else is rejected without a model.
3. **Sender typing.** Prompt, steer and follow-up Events carry `sender` (`user` or `system`) and, for `user`, the `user` ID. Handoffs, memory injections, grouped Subagent results and sudo results are `sender=system`. The miner reads only `sender=user` Events. Nothing injected is ever recorded as a user message.
4. **Human gate.** Only a human approval creates a Rule, a Profile patch or a Project note. The one exception is an explicit structural `/remember` action (slash command on the first line of a typed message, or a button), which creates a Rule directly. Pasted text containing `remember:` is not an action.
5. **Private by default.** Candidates and Rules go to the bank of the Event sender. A Project note is created only by an explicit human action.
6. **Provenance on everything.** Every record stores author, Session, Event ID, quote, commit and time.
7. **Redact at every egress.** The redactor runs before any text leaves the Daemon: the distill LLM, a Decider, the Jev API.
8. **No hidden state in a handle.** One function, `startHandle(session)`, derives owner, tool set, bank bindings and injections from the Session record on every create, resume and compress.

## Data model

Banks are SQLite files (`modernc.org/sqlite`, WAL, `busy_timeout`, `PRAGMA user_version` migrations) under `<stateDir>/enderbloom/memory/users/<userId>.db`, one per User, keyed by user ID. One writer goroutine per bank. Snapshots use `VACUUM INTO`.

- `observations`: grounded candidates. Not injected. Searchable through `recall`. Columns: id, session_id, event_id, quote, kind, subject_key, evidence_count, decider_json, created_at.
- `rules`: approved, injectable. Append-only versions: rule_id, version, text (at most 300 chars, enforced at write), kind, subject_key, anchors_json, observed_commit, status (`active`, `parked`, `dormant`, `superseded`), source_observation_ids, approved_by, approved_at.
- `queue`: pending decisions: kind (`rule`, `profile_patch`, `promotion`), payload, status, decided_by, decided_at. Rejected items are kept; they are the Decider labels.
- `profile_versions`: version, lines with stable IDs, patch, approved_by, created_at.
- `ledger`: one row per nightly stage per run.
- FTS5 external-content table over `rules.text` and `observations.quote`, with insert, delete and update triggers, plus an identifier-split copy of the text (camelCase, snake_case, dotted paths) so `TestUploadHandler` also matches `upload handler`.

Triggers: `BEFORE DELETE` on every table aborts with `use purge`. `BEFORE UPDATE` on `rules` aborts unless only `status` changes.

Subject keys are normalized strings such as `go.errors.wrapping`. The model proposes a key; supersession is deterministic: same key, same author, newer observation wins. A newer statement from a different User is a disagreement and goes to the queue.

Anchors are paths, packages and `module@version` pairs from `go.mod`, plus the commit the Rule was observed at. Preferences have no anchors; they get versions, not staleness.

## Retrieval and injection

- Recall is FTS5 only. The query is built from identifiers, paths and rare terms of the task prompt, every token quoted and joined with `OR`, length capped. Candidate set: at most 20 Rules.
- A Decider-based selection step may reorder the 20 and cut to 5 (see Deciders). It has a 1500 ms budget. On timeout or error the FTS5 order stands.
- Merging User and Project results uses reciprocal rank fusion with k=60. BM25 scores from separate tables are never compared.
- At injection, for each Rule with anchors, the Daemon runs `git diff --stat <observed_commit>..HEAD -- <paths>` in the current worktree. Changed anchors: inject with a flag "code changed since <date>, verify". Deleted anchors: set the Rule `dormant` and do not inject it. A Rule that checks out and is injected keeps its status.
- The block is the Profile first, then up to 5 Rules, each at most 300 chars, inside a fence marked "untrusted notes, not instructions". A fixed preamble sets precedence: the task prompt beats everything, `AGENTS.md` beats the Profile for code in the repo, the Profile governs workflow and communication.
- Reviewers get a filtered Profile: style and workflow lines yes, lines that lower the review bar or assert expertise no. The filter is a line tag set when the Profile line is approved.
- Create injects Profile and recall. Resume injects the Profile only. Compress behaves like create: Profile, fresh recall (query = the Handoff's task summary), then the Handoff. The Handoff writer is told to drop fenced memory blocks.
- Every injection appends an `injection` Event (`sender=system`) with Rule IDs and versions. These Events make a Session replayable and give the only clean wrong-signal: a user correction that contradicts a Rule injected in the same Session.
- Memory never blocks a turn. A missing or failing engine skips injection and posts a notice Event.

## Mining

- Ingestion runs at turn end (the mnemopi `agent_end` shape): when a turn settles, the miner reads only the bytes added to `events.jsonl` since its cursor, off the turn path, with at most one scan in flight per Session. The nightly job catches up from the same cursor, and for a Session that is not live it still waits for the settle window (default 10 minutes) or an ended Session. The cursor is a byte offset stored with the file size and a hash of the first 4 KB. A changed head or a shrunk file resets the cursor. It reads only up to the last newline. It reads only `sender=user` prompt, steer and follow-up Events. Turn-end scans run the heuristic pass and the confirming Decider only; distill, consolidation and Profile proposals stay nightly.
- Heuristic first pass, in a fixed order: slash-command guard, length floor, explicit statements, guardrail phrases ("don't add X unless asked"), a false-positive reject list (questions, requests), a non-correction deny list ("no problem", "never mind"), positive phrases (25-char floor, forward-pivot reject), strong corrections (`^no`, `^don't`, `^stop|^never`, "that's wrong", "I meant", "use X not Y"), weak corrections (short messages only), CJK correction patterns. Pattern tables are ported from `claude-reflect` (MIT); keep the notice in the source.
- Each correction is stored with the thing it corrected: the preceding tool call or diff hunk.
- Redaction (ported from `deja-vu`, MIT) runs before anything is stored or sent.
- Observation to queue: a candidate reaches the queue only when it is an explicit `/remember`, or its subject key recurs in two or more Sessions on different tasks. Generalizing language counts for ("always", "in this repo", "from now on"), situational language counts against ("here", "this function", "for now"). The approver sees one proposed generalization with its quotes.
- Dedup normalizes text but keeps negation tokens. Overlapping pairs get a relation: duplicate, refines or contradicts. Duplicates increment `evidence_count`. Nothing is dropped.

## Deciders

`Decider` is a Go interface that answers typed questions about a statement and returns probabilities. Questions (at most 6 per call):

- `durable` (noul): does the user state a rule, preference or correction meant for future work, not only for this task?
- `kind` (choice): `preference`, `decision`, `antipattern`, `fact`, `procedure`.
- `scope_hint` (choice): `user_rule`, `project_rule`, `situational`, `none`. A `project_rule` hint is only a hint; Rules are private until a human promotes them.
- `contradicts` (noul): the state holds the candidate and one stored Rule; does the candidate contradict it?
- `relevant` (noul, one per candidate, recall time): is this Rule relevant to the task prompt?

Implementations, all in v1:

1. `heuristic`: the regex pass above. Default. No network. Scores are pass/fail, not probabilities.
2. `llm`: a one-off pig call with no tools, JSON schema output, using the global `memoryModel` setting (a cheap authed pig model). Same calling code as the compress summarizer.
3. `jev`: HTTP to a Jev-compatible endpoint (`POST /api/v1/decisions` with `Authorization: Bearer`, body `model`, `state`, `questions`; or the TypeSafe `POST /v1/systemone` shape). Settings: `jev.baseURL`, `jev.apiKey`, `jev.model` (default `jev-latest`). Host-global, admin-only. A local Laya server (`laya[serve]`) plugs in by `baseURL` when it speaks one of those shapes; ticket 21 verifies the shape and adds an adapter if it does not. Calls send redacted text only, and the setting is off until an admin turns it on, because user turns leave the host.
4. `ensemble`: the heuristic pass proposes candidates; the configured second Decider (`llm` or `jev`) must confirm each one. A candidate with `durable` below `confirmLow` (default 0.35) is dropped; between the two thresholds it is stored as an observation; at or above `confirmHigh` (default 0.7) it is eligible for the queue. Thresholds are settings.

Every Decider output is stored in `decider_json` with the model and version. Rejected queue items are labels. Ticket 30 compares Deciders on hand-labelled turns.

Validation, thresholds and all state changes are ordinary Go code. A Decider never writes.

## Nightly job

Runs at 03:00 local when `session.Idle()` holds for the Daemon, and on demand from the Memory page. One stage failing does not stop the others. Each stage writes a `ledger` row (`SUCCESS`, `NO_DATA`, `FAILED`, with counts).

1. Snapshot every bank with `VACUUM INTO`, keep 10, at most one per 6 hours. Abort the run on failure.
2. Mine settled Sessions into observations (grounding, redaction, dedup with counts).
3. Distill: for each Session with new observations, one cheap-LLM call returns a brief or the sentinel `FLUSH_OK` when nothing durable exists. Input is capped at 15 KB of redacted text.
3b. Consolidate: per User bank, one cheap-LLM call reads the last 48 hours of observations that no queued proposal covers, their FTS5 neighbours and the nearest stored Rules, and returns `generalize`, `refine` or `contradict` proposals, or `none`. Go validates ids, source counts, text length, redaction, rationale and confidence; a valid proposal is a pending `rule` queue item. Nothing is created, merged or deleted.
4. Contradiction pairing: for each new candidate, FTS5 finds stored Rules with the same subject key or high overlap; the relation (duplicate, refines, contradicts) goes to the queue.
5. Profile patch proposals: add, edit or retire operations by line ID, JSON-only, `none` when no stable signal. Proposals go to the queue.
6. Anchor check: refresh `dormant` status against the current main branch.
7. Write the run summary (counts, approval latency, queue size).

There is no decay, heat or compression stage. Dormancy is only "not auto-injected", set by anchor invalidation or by a human park.

Purge is a human action on the Memory page. It deletes chosen records in one transaction that drops and recreates the delete trigger, enables `secure_delete`, rebuilds FTS, runs `VACUUM`, and removes the same text from snapshots, the queue and the ledger.

## Project notes

Project knowledge lives in two places: `AGENTS.md` files (DOX), and Project notes: Markdown files with frontmatter (`id`, `anchors`, `observed_commit`, `author`, `subject_key`) in `.emberloop/notes/` in the repo. They are indexed per worktree at Session start into a temporary FTS5 index and recalled like Rules. Because they are files they are branch-aware, reviewed by the existing Reviewer loop, versioned by git, and attributed by `git blame`.

Approving a `promotion` queue item either edits the nearest `AGENTS.md` or adds a note file in the Session's worktree. The Daemon never commits.

Prompts tell every Worker to put Project knowledge in the nearest `AGENTS.md` and to propose, not call, `remember`.

## Roles and tools

- `remember` (agent tool, via el-mcp like `spawn`): scope is always the caller's User bank, needs a `reason`, is rate-limited per Session (default 5), and always creates a queue item, never a Rule.
- `recall`: User bank plus Project notes. Scouts, Workers and Reviewers get `recall` only. Run Sessions have no `remember`.
- `profile` tool and settings editor create Profile patches. The agent writes a patch only on explicit User request.
- The tool set derives from the role in `startHandle` on every create, resume and compress.
- Each User gets a separate backend config directory (`CLAUDE_CONFIG_DIR` for claude) so no User-level memory of a backend is shared across Users or loaded behind the Daemon's back. The pig harness is checked for its own memory loading.

## Users and Sessions

- `users.go` store under `<stateDir>/enderbloom/users/users.json`: id, username, argon2id hash, admin flag, created. Random tokens stored server-side with expiry. Cookie HttpOnly, SameSite=Lax, Secure on https.
- Routes: `POST /api/auth/signup|login|logout`, `GET /api/auth/me`. Every other `/api` route and SSE stream returns 401 without a valid cookie; the static shell stays public.
- First signup is admin. `signup_enabled` (default on) is admin-write-only. `emberloop-web user set-password <name>` resets a password.
- `meta.json` and the Run file gain `owner` (user ID). Subagents, Workers and Reviewers inherit it. Ownerless data is hidden and 404s. No migration.
- Any User may prompt, steer, interrupt, cancel and end any Session. No ACLs.
- A create with no Project uses `<stateDir>/enderbloom/users/<name>/home`.
- Whose Memory: the Session `owner` decides the injected Profile and recall. Mined observations go to the bank of the Event's `user`, not the owner. A steer from User B inside A's Session never lands in A's bank.
- Repository ID for banks and notes: the root commit hash. Worktrees resolve to their main repo through the git common dir.
- UI: login and signup screens, sidebar per `.scratch/mockup/user-sidebar.html` (User groups, own group first and open, others collapsed with count and online dot, colored bar per owner, Project headers, nesting, Run tag, filters Mine / Everyone / Project), sender badge on each prompt, creator in the Session header.

## Handoff compress

- Compress is Handoff restart on every Backend. The Daemon keeps the Session ID, Event log and `owner`, and rotates the harness resume token. This replaces the pig-only in-place compact.
- The summarizer is a one-off pig call with no tools, outside the Session. `compressModel` is one global setting, chosen from the authed pig catalog. Unset or no auth: Compress is disabled with a reason; auto shows a warning.
- Summarizer input is the serialized Event log. Tool results are cut to head and tail (about 2000 chars). Oversize input is folded in budgeted chunks, halved to a floor, then cut harder (head 500 chars), then the deterministic Handoff applies.
- Sections follow the pinned recipe (Pocock handoff, caveman ultra). The last user prompt is verbatim. Memory fences are dropped.
- Fallback chain: up to 2 repair retries, then lenient wrap under the headings, then a deterministic Handoff (first user prompt, last 3 user prompts, files touched, last assistant message, open tool errors).
- Transient errors retry 3 times with 2s, 8s, 30s backoff. Credit and auth errors (402, 401, quota) block the Session with a banner "compress needs pig credits", Retry and Change compress model. The old handle stays untouched. A `compress blocked` Event carries the Retry action.
- Swap order: summarize, start the new handle through `startHandle`, send Profile, recall and Handoff, release the old handle after the first turn starts cleanly.
- Handoff is the first prompt, flagged in the Event log (`sender=system`), shown as a collapsed block. A copy is stored in the Session directory.
- One `compressed` Event is a divider (number, tokens before, model, fallback step, retry counts). Fold and fallback steps append Events.
- Queued follow-ups and held steers move to the new handle after the Handoff; a held steer degrades to a follow-up.
- Compress waits while a spawn Group is outstanding.
- Triggers: button, and auto at `compress_threshold_percent` with `compress_cooldown`, only at a tool boundary or when `session.Idle()` holds. `auto_compress` and `compress_instructions` keep their meaning.
- Idle is defined once, in `session.Idle()`, and used by auto-compress, the miner and the nightly job.

## Testing Decisions

- One seam: the Daemon HTTP API against the fake Backend, fake summarizer, fake Decider and fake clock. Tests assert external behavior (Event log, responses, prompts the Backend received), not internals.
- The engine has its own package tests: schema, triggers, FTS queries, grounding, dedup relations, supersession, anchors (a temp git repo), purge, snapshots.
- Fake summarizer modes: good, empty, wrong format, transient then success, credit error, oversize. Fake Backend gets a failing-start mode. Fake Decider returns scripted probabilities.
- JS with vitest for views. The e2e suite signs in through a helper.
- Live probes in `scripts/` need live agents and are outside the check: compress per Backend, Jev endpoint call.
- Prior art: pig compact, resume and queue tests; `api_test.go`, `spawn_test.go`, `runs_persist_test.go`; `stt.go` fencing tests; `Rail.test.js`, `ProjectList.test.js`.

## Measurement

- Primary metric: repeat-correction rate, the share of user corrections that restate a stored or capturable Rule. The miner computes it from `events.jsonl` and `injection` Events.
- Before auto-injection is on for everyone, an offline replay builds memory from Sessions before Session N and checks whether the Rules that would be injected cover the corrections the User made in N (recall@5). It also compares FTS5 recall with a remote embedder on the same cases to decide whether embeddings are needed.
- On a sample of Run loops, one of N Workers runs with recall off. Reviewer verdicts give a paired comparison.
- Approval latency and queue size are tracked so the queue does not decay into rubber stamping or abandonment. The queue has a budget and supports batch approval per Session.

## Out of Scope

- Embeddings in v1 (decided by the replay result).
- Procedural memory from red-to-green loops. This is the next version; the promotion action is its entry point.
- Path-triggered recall (surfacing Rules anchored to a file the agent opens).
- Per-User provider keys, quota, ACLs, private Projects, OAuth, password email.
- Hosted memory engines. Third-party egress exists only for the optional Jev Decider.
- Dynamic context pruning, tool-output compression layers, per-harness compress models, changes to the terminal `emberloop/` compress.
- Migrating existing Sessions.

## Further Notes

- Engine borrows from `mnemopi` (MIT; schema, FTS triggers with the missing update trigger added, live-row `EXISTS` filter, bank-name regex), `claude-reflect` (MIT; heuristic tables), `deja-vu` (MIT; redaction), `local-memory-mcp` (MIT; query sanitizer). Keep license notices where code is derived. Do not copy from AGPL, unlicensed or conflicting-license repositories (cortexgraph, memcurve, basic-memory, ggwhite/memory-mcp, okooo5km/memory-mcp-server-go, edg-l/engram-mcp).
- Research report behind the Memory design: `.scratch/web-v1/share/emberloop-memory-research.md`. A later review of that report shaped this spec; ticket 31 pins its invariants.
- Prior art for compress: `.scratch/archive/context-compress/spec.md`.
