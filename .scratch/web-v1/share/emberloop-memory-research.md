# Emberloop memory: research findings and proposed design (2026-10-10)

Purpose of this document: give a larger model full context so it can critique the design, find gaps, and answer open questions. Everything here is self-contained. Claims marked "vendor" are self-reported and unverified.

## 1. Context

Emberloop is a harness for running parallel coding-agent implement/review loops. The web module (`emberloop-web`, Go) runs a Daemon that owns Sessions. Each Session has an append-only `events.jsonl` (user prompts, steers, follow-ups, assistant events) under `<stateDir>/enderbloom/sessions/<id>/`. Backends are fake, pig and claude.

Three specs are being merged and finished together:

1. **Handoff compress**: replace in-place compaction with a "Handoff restart". A user-chosen model writes one Handoff document from the Event log, a fresh harness handle starts for the same Session ID, and the Handoff is sent as the first prompt.
2. **Multi-user**: Users (tailnet perimeter, cookie login), `owner` on Sessions and Runs, `user` sender on prompts, sidebar grouped by User.
3. **Memory and Profile**: a per-User Profile file (always injected, 2 KB cap) plus Memory (User-private and Project-shared notes) behind a replaceable Go `Memory` interface, with `remember`/`recall` tools, auto-recall at Session create (top-5, 1.5 KB, fenced as untrusted), a Memory page, and nightly maintenance. Hard rule: never auto-delete.

Constraints for memory: Go daemon; LLM and embedder may be remote APIs (RAM for models is not an issue); the engine itself must be self-hosted and small; project knowledge already lives in AGENTS.md ("DOX") files, so memory should only cover what never reached AGENTS.md: things the user said in past sessions (preferences, corrections, decisions and rationale, "we tried X and it failed").

Earlier plan (ADR-0002): adopt the `@oh-my-pi/pi-mnemopi` package (Bun/TypeScript, SQLite) as a sidecar behind the Go `Memory` interface.

## 2. What the research found

### 2.1 Benchmarks do not transfer to coding agents

- Vendor benchmarks (LoCoMo, LongMemEval, BEAM) measure conversational recall, not coding task success. Headline numbers (Mem0 92.5/94.4, Zep 94.7, MemPalace 96.6, OMEGA 95.4) are self-reported, use different judges and prompts, and several are recall@k rather than answer accuracy. Neutral reruns of Mem0 landed at 50-74%.
- Best-supported engine on breadth: Hindsight (LongMemEval-S about 91-94, BEAM-10M 64.1, public harness artifacts), but the harness is run by the vendor.
- A long-context baseline beats every memory engine on raw accuracy by roughly 33 points; memory buys cost and scale, not accuracy.
- Coding-specific evidence: procedural/trajectory memory helps (SWE-Exp +7.2% relative on SWE-bench Verified, Agent KB 41.3 to 53.3, Agent Workflow Memory +24-51% on web tasks with 30-50% fewer steps; GitHub Copilot agentic memory, a production A/B, raised PR merge rate 83% to 90% with a 28-day expiry). A controlled test (n=9) showed 22-32% cost and 28-40% turn savings on complex repos with flat quality, and the reverse on a toy repo.
- Fact/preference memory is unproven for coding repair. Flat auto-generated context files hurt (ETH study: human-written +4%, LLM-generated -2 to -3%, about +20% cost).
- Failure modes: stale notes, memory poisoning (MINJA injection above 95% in studies), provenance needed.

Implication: pick the engine on operations, per-note CRUD, provenance and safety, not on benchmark scores.

### 2.2 Candidate engines surveyed

LLM-extraction engines (mem0, Graphiti/Zep, Cognee, Letta, MemOS, Memobase, MemU, Hindsight, Mastra Observational Memory) are heavier than needed for a few hundred short notes per User and add LLM cost per write. Notable facts:

- mem0 OSS self-host: three containers, about 1-1.5 GB; open bug where LLM and embedder share one base URL.
- Graphiti: 3-6+ LLM calls per message; bitemporal edges.
- Hindsight: one container with embedded Postgres; no clear per-note edit/delete; license contradiction (LICENSE file says MIT, API docs say Apache-2.0).
- Cognee: documented 8B local-LLM path and a no-LLM GLiNER mode; document-pipeline shaped.
- Letta: self-host deprioritised; its sleep-time agent design is the useful part.

Tiny SQLite engines (zero or optional LLM): mnemopi / Mnemosyne (same family: upstream `mnemosyne-oss/mnemosyne` is Python and solo-maintained; mnemopi is the Bun/TS port inside `can1357/oh-my-pi`), Engram (Gentleman-Programming, Go, 68k non-test LOC), cortexdb (Go, 161k LOC), ggwhite/memory-mcp (Go, no LICENSE file), okooo5km/memory-mcp-server-go (Go, no LICENSE file), edg-l/engram-mcp (license conflict), local-memory-mcp (Rust, MIT, cleanest patterns), conch, mcpmem, ai-memory (Rust), and others.

License traps: cortexgraph and memcurve are AGPL (ideas only); basic-memory is AGPL; YourMemory is non-commercial; EngramMemory is BSL.

### 2.3 Code-level study (read-only clones, nothing executed)

Key conclusions:

- mnemopi's no-LLM core is small and portable. Estimated 3k LOC of Go (4.5-6k for a faithful port) on `modernc.org/sqlite` with FTS5, no cgo.
- No existing Go engine is adoptable: too big, unlicensed, or license-conflicted.
- Therefore: write a thin in-process Go engine behind the `Memory` interface, borrowing schema and constants from MIT/Apache sources, and reimplementing ideas from AGPL sources without copying text.

Concrete reusable pieces found (with source):

- **Schema and FTS** (mnemopi, MIT): working and episodic tables with veracity, trust tier, scope, bank, `valid_until`, `superseded_by`, tier; external-content FTS5 with insert/delete/update triggers (note: mnemopi's `fts_facts` lacks an update trigger, do not copy that bug).
- **Live-row search query** (mnemopi): `MATCH` combined with an `EXISTS` live filter; the source comment cites 9.19 ms down to 0.042 ms on 40k rows.
- **Query safety**: quote every token (local-memory-mcp, kcp-memory), cap length, and render synonym groups as `OR` (mnemopi's expansion emits invalid `(a|b)` syntax, a known bug).
- **Scoring** (mnemopi): hybrid weights, relevance gates, tier multipliers (1.0/0.85/0.7), veracity weights, Weibull per-kind decay (21 types), MMR with lambda 0.7, RRF with k=60.
- **Injection**: clip each note to 500 chars, three notes, about 1.5 KB; sandwich ordering.
- **Redaction** (deja-vu, MIT): multilingual and CLI-flag-complete secret regex set.
- **Bank safety**: bank names restricted to `^[A-Za-z0-9_-]{1,64}$`; re-validate server side.
- **Hygiene** (mnemosyne Python): noise scorer with archive-not-delete and an audit log.

### 2.4 Mining user statements from past sessions

Several MIT tools mine past transcripts for corrections and preferences (claude-reflect, deja-vu, claude-memory-compiler, episodic-memory, kcp-memory, claude-self-reflect). Findings:

- claude-reflect has the best heuristic tables: explicit `remember:`, guardrail patterns ("don't add X unless asked"), strong corrections (`^no`, `^don't`, `^stop|^never`, "that's wrong", "I meant"), weak corrections, positive patterns, a deny-list ("no problem", "never mind"), a forward-pivot reject ("Perfect! Now let's..."), a 25-char floor for bare praise, CJK correction patterns, a fixed evaluation order, and a cheap-LLM second pass returning JSON (`is_learning`, `type`, `confidence`).
- Byte-offset cursors (kcp-memory) beat mtime cursors; scan only new bytes of `events.jsonl`.
- Two-stage nightly distill (claude-memory-compiler): flush each session into a brief with a `FLUSH_OK` sentinel for trivia, then compile into a small knowledge set; hash-gated.
- Profile compile prompt (memsearch): JSON-only `{action: none|replace, reason, content}`, stable-evidence-only, preserve rather than rewrite, firewall between user-level preferences and project-level decisions.
- Approval gate: queue items, human approve/reject/edit, contradiction check, auto-accept only explicit `remember:` at high confidence after redaction and dedup.
- Evidence for lexical-first retrieval: deja-vu reports 88% hit@1 on LongMemEval-S with no vectors; claude-self-reflect's preprint reports that embedding similarity can drown intent.

### 2.5 Nightly maintenance that never deletes

Synthesised from realMemory, memoripy, open-second-brain, hippo-memory, zenbrain, MemoryOS, agent-sleep, MemOS:

1. Snapshot the DB first (keep 10, throttle to one per 6 hours).
2. Classify as a pure read: retention = base x exp(-days/tau) (tau about 30 days episodic, 365 semantic); activation blend (decay, utility with failures weighted x2, retrieval count, salience); heat (visits, interactions, recency). Rank is a weighted sum; judgement kinds (preference, decision, antipattern, policy) never decay.
3. Reinforce on recall (base x1.25, cap 1.0); wrong-penalty 0.5^min(netWrong,3).
4. Promotion candidates go to a reviewable queue (keep/improve/park/promote with reason strings); the job never flips kinds itself.
5. Compress stale clusters into summary rows that point to kept originals (cluster of 5+, similarity 0.75); originals only re-tiered to `cold`.
6. Dormancy instead of deletion when rank is below a threshold past a grace period; pinned and policy rows exempt.
7. Per-stage ledger (SUCCESS/NO_DATA/FAILED with counts) shown on the Memory page.

Anti-patterns to avoid: any `DELETE` (agent-sleep, hippo default, MemoryOS LFU, realMemory GC), heat reset, Anki-tuned FSRS weights, mis-tuned Graphiti recipes (RRF k=1, MMR lambda=1).

### 2.6 Prompts worth adapting (Apache/MIT sources)

- Convert relative dates to absolute dates inside the note text; keep observation date separate from today's date (Hindsight, mem0).
- Retain gate: "would this be useful in 6 months?"; one good sentence beats three; when in doubt, extract; topic-coverage checklist.
- Consolidation contract with create/update/delete operations, reason audit, worked examples, at most one update per observation (Hindsight).
- Contradiction conservatism: mark contradicted only if both facts cannot be true; never infer from recency alone; dates decide, not the LLM verdict (Graphiti).
- Anti-hallucination: map real UUIDs to sequential ids before sending to the model (mem0); reject line-number leakage in edit tools (Letta).
- Sleep pass phases: investigate, extract, update, review, commit; memory-versus-skill gate (Letta Code).

## 3. Jev: new decision-model option

Jev (TypeSafe AI) is a "System One" decision model: non-autoregressive, it never generates text, and answers typed questions with calibrated probabilities in one forward pass. Question types: Noul (binary yes/no probability), Choice (categorical), Score. Jev-Mem (arXiv 2609.23986, 2026-09-21, repo `libingzheren/Jev-Mem`, Python) uses it as the memory controller: typing a memory at write time, choosing relations, query routing, retrieval budget, candidate scoring and adaptive stopping. The answering LLM is invoked only for final synthesis. Paper claims on LoCoMo: LLM-judge 0.777 (11% over the strongest baseline in its comparison), memory construction 158 s (6.6x faster), query latency 0.93 s.

Caveats:
- Baselines were A-MEM, Nemori, MemoryOS and MAGMA, not Hindsight or mem0. LoCoMo only.
- Paper numbers use the hosted Jev API. The open local backend, Laya (`convaiinnovations/laya`, about 33 ms per call, 100+ languages, `pip install laya`, ONNX and MCP extras), was not benchmarked in the paper. Laya zero-shot scores 0.362 on its typed-decisions benchmark; fine-tuned scores 0.766.
- Jev-Mem itself is a Python research system; not a sidecar candidate.

## 4. Proposed design for version one (including Jev)

Decision: in-process Go engine behind the existing `Memory` interface (ADR-0002 superseded). One SQLite file per bank (`user:<name>`, `project:<main-repo-id>`), FTS5 only (no embeddings), `modernc.org/sqlite`.

Components:

1. **Store and recall**: mnemopi-derived schema, quoted-token FTS5 queries, `EXISTS` live filter, hybrid rank without vectors, veracity and trust tiers, Weibull kind decay, MMR, optional later embeddings behind the interface.
2. **Inject**: Profile first, then top-5 merged User and Project recall, 3 x 500 chars (about 1.5 KB), fenced as untrusted; resume injects Profile only. Provenance on every note (author, session, commit).
3. **Miner**: heuristic first pass (claude-reflect tables), byte-offset cursor per `events.jsonl`, redaction before queueing, dedup by normalised hash plus Jaccard.
4. **Jev decision layer (v1)**: a pluggable `Decider` interface answering typed questions about a candidate statement. Two implementations: regex heuristics (default, no network) and Jev-style (hosted Jev API or local Laya). Questions, all with probabilities:
   - Noul: "Is this a durable user preference or correction reusable across sessions?"
   - Noul: "Does this contradict stored note X?" (candidate pairs from FTS)
   - Choice: route a candidate to `profile | decision-note | project-note | discard`.
   - Choice: classify memory kind (preference, decision, antipattern, fact, procedure) for decay tier.
   - Score or Noul at recall time: "Is this note relevant to the current task?" to decide whether to inject, which also implements the "do not auto-inject everything" lesson.
   Output is validated by ordinary Go code (as Jev-Mem does); thresholds are configurable; the final decision plus probability is stored for audit. Combine rule: max(regex confidence, Jev probability), with human approval still required except for explicit `remember:` at 0.9 or higher.
5. **Nightly distill** (cheap remote LLM): per-session brief with `FLUSH_OK` sentinel, then Profile compile (2 KB cap, JSON `none|replace`), decision notes with session backlinks, contradiction check, approval queue.
6. **Maintenance**: algorithm in section 2.5.
7. **Promotion to AGENTS.md**: an action on a note that proposes a DOX edit for the nearest AGENTS.md; prompts enforce that Workers update AGENTS.md for project knowledge instead of calling `remember`.
8. **Roles**: Workers, Reviewers and scouts are recall-only; only the Daemon-owning Session writes.
9. **Measurement**: post-ship paired on/off eval (tokens and turns to green), not recall accuracy.

Merged ticket order (from the spec read): MU-01 and the memory port in parallel; MU-02 (owner) next; H-02; then MU-03, H-03/04/05 and M-02 in parallel; then M-03/04/07 and MU-04; then M-05/06; then H-08; H-09 last.

Cross-spec conflicts to resolve in the merge:
- Handoff swap must preserve `owner` across resume-token rotation.
- Stacking order on a post-compress handle: Handoff first prompt versus Profile and recall.
- Memory tool gating must be re-applied to the new handle after a compress.
- Compress and nightly maintenance need one shared idle definition.
- Settings panel gains `signup_enabled`, `compressModel`, sidecar/nightly status.
- Auth gate: all `/api` and SSE tests must run authenticated.

## 5. Open questions for the reviewer

1. Is FTS5-only retrieval sufficient for a few thousand short notes per User, or should a small embedder (remote API) be in v1 for paraphrase recall? What evidence would settle it cheaply?
2. Is a Jev/Laya decision layer worth it in v1 given Laya's weak zero-shot score? Would a cheap remote LLM with a strict JSON schema match it on cost and latency, and be simpler? What labelled data would we need, and where do we get it from existing `events.jsonl` logs?
3. Is the proposed nightly algorithm over-engineered for this scale? Which stages could be dropped without losing the never-delete guarantee?
4. Risks in mining user turns: privacy across Users, poisoning through assistant or tool output, treating a steer as a durable preference when it was situational. How should the miner distinguish situational from durable?
5. Is the ADR-0002 reversal (in-process Go port instead of a mnemopi sidecar) sound given maintenance burden of a port versus Bun sidecar supervision?
6. Is the per-User Profile plus Project notes split right, or should Project notes live only in AGENTS.md with memory used only for User-level statements?
7. Which of the cross-spec conflicts in section 4 are most likely to bite, and what is the safest resolution for each?
8. What is missing from this research (engines, papers, failure cases) that a skeptical reviewer would raise?
