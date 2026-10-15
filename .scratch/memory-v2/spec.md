# Memory v2: automatic retention

This spec replaces the ingestion, recall and approval parts of `.scratch/web-v1/spec.md` (sections Mining, Deciders, Nightly job, Retrieval and injection, Design rule 4). Everything else in web v1 stays: bank files, append-only Rules, the redactor, grounding, purge, snapshots, Profile versions, Project notes, `remember`/`recall` tools. Where this spec and web v1 disagree, this spec wins.

## Problem Statement

The 2026-10-15 retro of the live bank (`~/.local/share/pig/enderbloom/memory/users/`) found that Memory retains almost nothing:

1. In four days it stored 9 observations and 0 Rules. The first nightly scanned 637,175 events, kept 36 user prompts and stored 1.
2. Ingestion is a regex classifier for correction phrases (`ClassifyHeuristic`). A stated preference or decision that is not phrased as a correction never matches. Several stored items are chatter ("no like idle for like, 2 people and an ai running") and the kinds are wrong.
3. A Rule needs the same subject key in two Sessions. Subject keys are the first three topic tokens, so they are junk (`man.want.quota`, `love.dude.same`) and never recur. Even correct input cannot reach the queue.
4. Every durable thing needs a human approval before anything uses it. The pending Profile patch is four days old and unseen.
5. Recall is keyword-only (FTS5). A paraphrase of a stored item does not find it.
6. The nightly job did not run on Oct 14 or Oct 15. It fires only in the 03:00 hour and only when the Daemon is idle, and a busy Daemon skips the day silently.

Prior art (mnemopi in `can1357/oh-my-pi`, and Mnemosyne in `mnemosyne-oss/mnemosyne`) ingests the opposite way: an LLM extracts persistent items from every turn, with no human gate, stores them with importance, dedups by exact text and embedding similarity, and recalls with a hybrid of vector, keyword and importance scores. Humans delete what they dislike.

## Solution

Memory keeps three tiers. Only the first is new in how it is filled.

- **Memories** (new). Short items extracted by a cheap LLM from every settled turn, stored automatically with a verbatim quote, importance, and an embedding. No approval. A human can forget, park or pin any Memory, and can promote one to a Rule. Memories are what auto-recall injects first, after the Profile and Rules.
- **Rules** (unchanged). Human-approved, append-only, anchored. They remain the promotion path to `AGENTS.md` and Project notes. A Memory that recurs becomes a candidate Rule.
- **Profile** (unchanged data model, new approval path). Patches are proposed by the nightly job and approved with a one-tap notification and an inline card, the way sudo requests work.

A fourth store, **episodes**, keeps the redacted text of each user turn so a search can find what was said before extraction existed or when it missed. Episodes are searchable through `recall` only and are never auto-injected.

The goal stays less repeated correction (UX). It is measured by the labelled extraction set and by the forget rate (see Measurement).

## Design rules

These replace web v1 rules 1 to 4 where they differ. Rules 5 to 8 of web v1 stand.

1. **Never delete, with a soft forget.** No row is deleted outside a human purge. `forgotten` is a status, not a delete. A forgotten Memory is excluded from recall, injection, dedup matching and Rule candidates, and stays in the bank until purge.
2. **Grounding stays.** The extractor must return a verbatim `quote` for every item. Go checks that the quote is a substring of a `sender=user` Event of the scanned range, after redaction. Anything else is dropped without a second model call.
3. **Sender typing stays.** The extractor reads only `sender=user` prompt, steer and follow-up Events. The previous assistant message is given as labelled context, never as quotable text.
4. **Human gate, narrowed.** A human approval is still required for a Rule, a Profile patch and a Project note. A Memory needs no approval. Automatic Memories are private to the bank of the Event sender.
5. **Redact at every egress.** The redactor runs before text reaches the extractor, the embedder, the consolidator or any other model.
6. **Memory never blocks a turn.** Extraction, embedding and recall run off the turn path. Any failure skips that step and records a ledger row or a notice Event.
7. **Cheap by construction.** One extractor call per settled turn batch, one embedding call per new Memory. Target cost under $0.01 per day for 200 user messages.

## Data model

New migrations (append-only, never edit a shipped one). All new tables follow web v1 conventions: `id INTEGER PRIMARY KEY AUTOINCREMENT`, FTS5 external-content with insert, delete and update triggers and an identifier-split copy, `BEFORE DELETE` abort triggers.

- `memories`: id, session_id, event_id, author, kind (`preference`, `instruction`, `decision`, `fact`), text (at most 300 chars), quote, importance REAL (0 to 1), veracity (`stated`, `inferred`), mention_count (default 1), status (`active`, `parked`, `superseded`, `forgotten`), superseded_by (id or null), pinned (0 or 1), embedding BLOB (little-endian float32, null until computed), embedding_model TEXT, extractor_json (model, effort, prompt version), observed_commit, created_at, last_seen_at.
- `episodes`: id, session_id, event_range (first and last Event ID), author, text (redacted user turn text, at most 4 KB), importance (0.65), embedding BLOB, embedding_model, consolidated_at (null until folded), created_at.
- FTS5 over `memories.text` + `memories.quote` and over `episodes.text`.
- `rules_guard`-style update guard on `memories`: an UPDATE may change only `status`, `superseded_by`, `pinned`, `importance`, `mention_count`, `last_seen_at`, `embedding`, `embedding_model`. Text, quote, author and provenance never change; an edit is a new row that supersedes the old one.
- Purge covers both new tables and scrubs their FTS and snapshots like the existing tables.

`observations` and `queue` stay. Existing observations are not migrated; the Memory page keeps listing them under "Legacy observations" and a human can promote one to a Memory with the Memory page action. The heuristic classifier remains only as a fallback (see Extraction).

## Ingestion

Runs at turn end, off the turn path, using the existing cursor (`Cursor{offset,size,headHash}` per `events.jsonl`, reads to the last newline, resets on a changed head or a shrunk file).

1. **Trigger.** When a Session settles (idle status Event), enqueue it. At most one scan in flight per Session. Debounce 3 seconds so a steer burst becomes one scan. The nightly catch-up reads from the same cursor for Sessions missed (Daemon restart, crash); for a Session not live it waits for the settle window (10 minutes) or an ended Session.
2. **Select.** Read new `sender=user` prompt, steer and follow-up Events. Drop injections, handoffs, grouped Subagent results and sudo results (`sender=system`), slash commands, and empty or under-12-character text. Cap each Event at 4 KB and each scan at 15 KB of text (keep the newest). Redact.
3. **Episodes.** Store one `episodes` row per kept Event, importance 0.65. This always happens, even when extraction is off or fails.
4. **Extract.** One call per scan to the configured extractor. Input: the kept user texts, numbered, plus the last assistant message of the turn (at most 1 KB, redacted) labelled `context only, never quote it`. Temperature 0. Output is JSON only:

```
{"items":[{"kind":"preference|instruction|decision|fact","text":"...","quote":"...","event":N,"stated":true}]}
```

   At most 5 items per scan. Empty `items` is the normal answer.
5. **Validate (Go).** Drop an item when `kind` is unknown, `text` is empty or over 300 characters, `quote` is not a substring of Event `event`, or the redactor changes `text`. `stated=false` stores veracity `inferred`.
6. **Importance on write (fixed, not learned).** Stated plain item 0.7; stated with durable language ("always", "never", "from now on", "in this repo", "we decided") 0.85; inferred 0.5. Importance never decays at write time; decay is applied at recall.
7. **Dedup and supersession** (see below), then insert, then enqueue embedding.
8. **Ledger.** One `ingest` ledger row per scan with counts (scanned, kept, extracted, dropped_ungrounded, stored, duplicates, superseded), model, latency and cost.

### Extractor

Default: `inclusionai/ling-3.0-flash` through OpenRouter chat completions, `reasoning.effort = "low"`, `temperature = 0`, `max_tokens = 4000`. It was chosen by the 2026-10-15 benchmark: on 35 labelled messages it kept 11 to 12 of 15 memories with 0 false alarms at about $0.0135 per 1,000 messages and 1.6 s, against Haiku 5.5 at 8 of 15 and $0.055. `tencent/hy3` (thinking off) is the tested runner-up. Settings: `memory_extract_model`, `memory_extract_effort` (`off`, `low`, `medium`), admin-only, host-global, shown in Settings → Memory. An empty model disables LLM extraction. The OpenRouter key is the same one `find` and Jev use.

Fallback chain, first that answers wins: configured model, then the heuristic classifier (`ClassifyHeuristic`) storing at importance 0.4 with veracity `inferred`, then nothing (episodes still stored). A 429 or timeout retries once after 2 seconds, then falls through.

Prompt (version `x1`, stored in `extractor_json`; the test set decides edits):

```
You extract long-term memories about a user from messages they sent to a coding agent.
Extract ONLY persistent, non-transient items: standing preferences, rules for future work,
decisions with their choice, and durable project or environment facts.
Ignore one-off task requests, bug reports, questions, status, and chat.
Write each item as one self-contained sentence (max 200 chars) that keeps the user's meaning
exactly, never inverted. Quote the user's own words that support it, verbatim, in "quote".
If nothing qualifies return {"items":[]}.
Return only JSON: {"items":[{"kind":"preference|instruction|decision|fact","text":"...","quote":"...","event":N,"stated":true}]}
```

The benchmark prompt had no `quote` field. Ticket 02 re-runs the labelled set with the shipped prompt before merge.

### Dedup and supersession

Mirrors Mnemosyne and mnemopi, kept deterministic:

- Same normalized text (`NormalizeQuote`, negations kept) in the same bank: no insert, `mention_count + 1`, `last_seen_at` updated, importance raised by 0.05 up to 0.95.
- Embedding cosine at or above 0.95 and a shared-token overlap: duplicate, same bump.
- Cosine at or above 0.88 with two or more shared significant tokens and opposite polarity (existing `Relation` negation and antonym test): `contradicts`. The newer Memory supersedes the older one (`status=superseded`, `superseded_by` set) in the same transaction when both come from the same author; the older stays recallable only through `recall` with `include_superseded`.
- Cosine at or above 0.88 without a polarity split: `refines`. Both stay active; the older one is not injected when the newer has equal or higher importance and both appear in one recall.
- Before embeddings exist (null vector) only the normalized-text and `Relation` checks run.

## Embeddings

- Provider: OpenRouter `/api/v1/embeddings`, one call per new Memory or Episode, batched up to 32. Default model `baai/bge-m3` (1024 dims, $0.01 per million tokens). Ticket 04 compares `baai/bge-m3`, `openai/text-embedding-3-small` and `qwen/qwen3-embedding-4b` on the labelled set and pins the default. Setting: `memory_embedding_model`, admin-only. Empty disables vectors and recall falls back to FTS5 alone.
- Storage: `embedding` BLOB of little-endian float32, plus `embedding_model`. Compute is asynchronous after the write transaction (worker per Daemon, bounded queue). The Memory is usable before its vector exists.
- Search: brute-force cosine in Go over active Memories, no cgo and no extension. Cap 50,000 vectors per bank; beyond that the nightly job warns and archives Episodes first. Query embedding has a 800 ms budget, a 256-entry LRU, and falls back to FTS5 on timeout.
- Model change: the nightly `reembed` stage recomputes every row whose `embedding_model` differs from the setting. Recall ignores vectors from another model.
- Redacted text only goes to the embedder.

## Recall and injection

Replaces web v1 Retrieval and injection for Memories. Rules, anchors, Project notes, the Profile preamble and the fence stay as written there.

- Candidates: FTS5 top 40 over Memories (existing quoted-token query) unioned with vector top 40.
- Score per candidate: `0.5 * cosine + 0.3 * fts + 0.2 * importance`, where `fts` is BM25 min-max normalized within the candidate set and a missing term is 0. Weights are settings with these defaults, renormalized to sum 1. Without a vector the weights renormalize over the other two.
- Kind decay multiplier `0.7 + 0.3 * decay`: preference and instruction never decay (decay = 1); decision half-life 180 days; fact half-life 90 days; `pinned` never decays. Veracity multiplier 1.0 stated, 0.8 inferred.
- Gates: drop candidates with cosine under 0.30 and no FTS hit; drop `forgotten`, `superseded`, `parked`.
- Merge with Rules by reciprocal rank fusion (k=60) exactly as web v1 does for Rules and notes; BM25 scores from separate tables are never compared.
- Block: Profile first, then Rules (at most 5, with anchor flags), then Memories (at most 5), at most 8 items total, bounded at 700 tokens, inside the existing fence. Memory lines are `- <text> (<kind>, <date>)`.
- Create and compress inject Profile, Rules and Memories. Resume injects the Profile only. Per-turn recall is out of scope (see Out of Scope).
- Every injection writes the `injection` Event with Rule IDs and Memory IDs, as before. A user correction that contradicts an injected Memory in the same Session bumps that Memory's counter `contradicted` in the ledger and shows on the Memory page.
- `recall` (agent tool) searches Memories, Rules, Project notes and Episodes with the same scoring; Episodes score with importance 0.65 and are labelled `episode`.

## Nightly job (sleep)

Runs when due and the Daemon is idle, and on demand from the Memory page. Stages, each with a `ledger` row:

1. Snapshot (unchanged).
2. Catch-up ingest from the cursor for settled Sessions the live path missed.
3. Reembed any Memory or Episode with a missing or stale vector.
4. Fold: Episodes older than 12 hours and not `consolidated_at` are summarized per Session by one cheap-LLM call into at most 5 Memories (same validation, `veracity=inferred`, importance 0.6) and marked consolidated. Originals stay.
5. Consolidate: per bank, one cheap-LLM call reads the last 48 hours of Memories and their embedding neighbours and returns merge proposals (`merge`, `generalize`) or `none`. A valid `generalize` over two or more Memories becomes a pending `rule` queue item. A `merge` supersedes the sources by a new Memory. Go validates ids, counts, text length and redaction as in the web v1 consolidate stage.
6. Promote candidates: a Memory with `mention_count >= 2` across two or more Sessions, importance at least 0.7, kind `preference`, `instruction` or `decision`, and no live Rule with cosine at or above 0.88, becomes a pending `rule` queue item with its quotes. This replaces the subject-key recurrence test. Clusters use cosine at or above 0.80 instead of subject keys.
7. Profile patch proposals (unchanged prompt and `none` sentinel). Each pending patch raises a notification (see Approval).
8. Anchor check (unchanged).
9. Summary with counts: memories stored, forgotten, pinned, contradicted, queue size, approval latency.

### Scheduling fix

`nightlyDue` becomes: run when no `nightly_summary` ledger row exists since the most recent 03:00 local (a missed day catches up the next time the Daemon is idle, at any hour). A due run that finds the Daemon busy writes one `nightly_skipped_busy` row per day and retries every minute. The last-run marker is read from the ledger, so a restart does not forget it.

## Approval and control

- **Memory page** (Memory section of the browser). Adds a Memories tab: newest first, filter by kind and status, search, importance and mention count, source quote and Session link. Actions: Forget, Park, Pin, Promote to Rule (creates a pending `rule` queue item with the Memory as its source), Edit (creates a superseding Memory). Legacy observations and Episodes get read-only tabs. A header counter shows "N new since last visit".
- **Profile patches** show as an inline card in the Session that is open and as a push notification with Approve and Reject, matching the sudo request flow (`push.go`). Approval records `approved_by`. Rejection keeps the item as a label.
- Rules in the queue keep the existing approval card; the Memory page counter shows queue size and the oldest pending age. A pending item older than 48 hours raises one reminder notification.
- A daily digest notification ("learned N things today, tap to review") is an opt-in user preference, off by default.

## Settings

All admin-only and host-global, in Settings → Memory, saved through the existing Revert/Save bar:

- `memory_extract_model` (default `inclusionai/ling-3.0-flash`) and `memory_extract_effort` (default `low`).
- `memory_embedding_model` (default pinned by ticket 04).
- `memory_recall_weights` (vector, fts, importance; default 0.5, 0.3, 0.2).
- An on/off switch for automatic extraction. Default on when an OpenRouter key is configured, off otherwise. The page states that redacted user turns leave the host for the extractor and the embedder.

The Decider mode, Jev and `memory_model` settings stay for the legacy confirm path and the consolidate stages; Memory v2 does not remove them.

## Testing Decisions

- A labelled extraction set lives in `emberloop-web/memory/testdata/extract_eval.json`: 15 positive messages with their expected concept, 20 negative messages (tasks, bugs, questions), URLs and secrets stripped. A gated live test (`TestLiveExtract`, skipped by `-skip TestLive` in the repo check) runs the shipped prompt and model and prints recall and false alarms. Acceptance for merge: at least 10 of 15 caught and at most 1 of 20 false alarms.
- Deterministic tests use a `ScriptedExtractor` and a `FakeEmbedder`: validation (ungrounded quote dropped, over-long text dropped, unknown kind dropped, redactor change dropped), importance rules, dedup and supersession thresholds, soft-forget exclusion everywhere, update-guard column limits, purge scrubbing of both new tables, re-embed on model change, hybrid score weights and renormalization, FTS-only fallback, kind decay, promote-candidate rule, scheduler catch-up and the busy-skip ledger row.
- The `Memory` interface gains methods for Memories and Episodes; the fake and the bank both satisfy `contract_test.go`.
- Browser tests cover the Memories tab, actions, the counter and the settings controls. API tests cover the new endpoints.
- The replay eval from web v1 (ticket 30) gains a recall-with-Memories arm; the A/B recall-off switch stays.

## Measurement

- Extraction: recall and false alarms on the labelled set per model and prompt version; re-run on any prompt or model change.
- Production proxies from the ledger: Memories stored per day, share forgotten within 7 days (target under 20 percent), share promoted to Rules, injection hit rate (Memories injected per create), contradiction counter.
- Cost: extractor and embedder cost per day from the ledger rows; alert on the Memory page above $0.05 per day.

## Out of Scope

- Per-turn recall on every prompt (mnemopi's `agent_start`). Add it only if the replay arm shows create-time recall is too late.
- Local embeddings (ONNX or llama.cpp) and sqlite-vec. Go-side brute force is enough below 50,000 vectors.
- Knowledge-graph triples, Weibull decay by 21 types, tier degradation of old Episodes, MMR diversification.
- Sharing Memories across Users; Memories stay private to the bank of the Event sender.
- Auto-promoting a Memory to a Rule, a Profile line or a Project note without a human.
- The ScaleDown `extract` span model as the primary extractor (tested: fragment spans, false alarms on task requests). A hint pass is a later experiment.

## Open Questions

1. Default on for automatic extraction when an OpenRouter key exists, or off until an admin opts in? The spec assumes on, because the retro cause is that nothing is retained; user turns do leave the host.
2. Should the Episode text be kept after folding, or pruned after 30 days? The spec keeps everything until purge.
3. Embedding default: pin after the ticket 04 bake-off.

## Ticket map (written to `issues/`)

1. Memories and Episodes schema, update guard, FTS, purge, fake and contract tests.
2. Extractor interface, prompt `x1`, validation, labelled set and live test, fallback chain.
3. Turn-end trigger, debounce, selection, redaction, episodes, `ingest` ledger rows.
4. Embedder interface, OpenRouter client, async worker, re-embed, bake-off and default model.
5. Dedup and supersession over Memories (text, embedding, polarity).
6. Hybrid recall, kind decay, gates, injection block with Memories, `recall` tool update.
7. Nightly: scheduler catch-up and skipped-busy row, fold, consolidate, promote candidates, reembed stages.
8. Profile patch notification and inline approval card.
9. Memory page: Memories tab, actions, counter, legacy tabs.
10. Settings: extractor, embedding, weights, on/off switch and notes.
11. Replay eval arm and cost/forget-rate measurements; ADR 0004 for the narrowed human gate.
