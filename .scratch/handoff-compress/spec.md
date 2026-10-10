# Handoff compress

## Problem Statement

A long Session fills its harness context window. Quality drops and cost rises. Today only a pig Session on the SDK Backend can compact, and it compacts in place with the harness's own summarizer. Claude Sessions and pig-over-ACP Sessions cannot compress at all. The user cannot choose which model summarizes, cannot control what the summary keeps, and gets no guarantee that the summary is usable.

## Solution

Compress a Session by Handoff. The Daemon asks a pig model, chosen by the user in the WebUI settings, to write one Handoff document from the Event log. The Daemon then starts a fresh harness handle for the same Session and sends the Handoff as its first prompt. The Session ID, the Event log and the UI history stay. Only the harness handle changes.

The Handoff recipe is the Matt Pocock handoff sections written in caveman ultra register. Code, paths, errors and IDs stay verbatim. Artifacts are referenced by path, never copied.

Compress never ends in an unusable summary. A fallback chain always produces a Handoff. The only hard stop is an unusable summarizer account (no credits, bad auth): the Session stops, the user is told, and a Retry button reruns the whole compress.

Compress runs from a button, or automatically at a context threshold.

## User Stories

1. As a user, I want a Compress button on a Session, so that I shrink its context on demand.
2. As a user, I want Compress to work for pig, pig-over-ACP and claude Sessions, so that no Backend is second class.
3. As a user, I want to choose the summarizer model in the WebUI settings, so that I control cost and quality.
4. As a user, I want only authed pig models in that choice, so that I cannot pick a model that fails.
5. As a user, I want Compress disabled with a reason when no compress model is set, so that I know what to fix.
6. As a user, I want the Session to keep its ID, title and full history in the UI, so that nothing seems lost.
7. As a user, I want a divider in the history where each compress happened, so that I see what the agent no longer holds.
8. As a user, I want the divider to show tokens before, the summarizer model and which fallback step produced the Handoff, so that I judge the quality.
9. As a user, I want the Handoff shown as a collapsed block, not as something I typed, so that the history stays honest.
10. As a user, I want the Handoff saved as a file in the Session directory, so that I audit what the agent was told.
11. As a user, I want my queued follow-ups to arrive after the Handoff, so that nothing I queued is dropped.
12. As a user, I want a held steer to degrade to a follow-up across a compress, so that it is never lost.
13. As a user, I want the context meter to reset after compress, so that it shows the new load.
14. As a user, I want auto-compress at a percent threshold, so that I do not watch the meter.
15. As a user, I want auto-compress only at a tool boundary or when idle, so that no turn is cut mid-token.
16. As a user, I want a cooldown after a compress, so that a Session does not compress in a loop.
17. As a user, I want compress to wait while a spawn Group is outstanding, so that parent and Subagent links survive.
18. As a user, I want a compress to survive a bad summary without my help, so that I never face a broken Session.
19. As a user, I want transient errors retried automatically with backoff, so that a network blip does not stop me.
20. As a user, I want a credit or auth failure to stop the Session with a clear banner, so that I know why it stopped.
21. As a user, I want a Retry button on that banner, so that I rerun compress after I fix the account.
22. As a user, I want a Change compress model action on that banner, so that I switch accounts in one click.
23. As a user, I want the old harness handle left untouched on a block, so that a failed compress loses nothing.
24. As a user, I want a very long Session to still compress, so that size is no reason to fail.
25. As a user, I want each fallback step recorded in the Event log with its reason, so that I debug a poor Handoff.
26. As an operator, I want a live probe per Backend, so that a harness change that breaks compress shows up.

## Implementation Decisions

- Compress is Handoff restart on every Backend. The Daemon keeps the Session ID and Event log. It rotates the harness resume token. This replaces the pig-only in-place compact. After this work, every Backend reports Sessions as compressible.
- The summarizer is a one-off pig call with no tools. It never runs inside the Session being compressed.
- `compressModel` is one global Daemon setting, edited in the WebUI settings panel. Its choices come from the authed pig catalog. Unset, or no pig auth: compress is disabled with a reason, and auto-compress shows a warning. There is no per-harness override.
- Summarizer input is the serialized Event log. Tool results are cut to head and tail (about 2000 chars). Reads that a later event superseded may be elided. Input that exceeds the summarizer window is folded in budgeted chunks: summarize each chunk, then summarize the summaries. A chunk that still overflows is halved down to a floor. At the floor, tool results are cut harder (head 500 chars). Past that, the deterministic Handoff applies.
- Handoff sections follow the pinned terminal recipe: Pocock handoff plus caveman ultra. The last user prompt is included verbatim. An open task gets a continue line. No raw tail of recent messages is kept.
- Fallback chain, in order: (1) repair retries, up to 2, with a prompt that restates the required sections; (2) accept non-empty text and wrap it under the required headings; (3) a deterministic Handoff built without an LLM: first user prompt, last 3 user prompts, files touched from tool calls, last assistant message, open tool errors. Step 3 always succeeds.
- Retry policy: transient errors (network, 5xx, rate limit) retry 3 times with backoff of 2s, 8s and 30s. Credit and auth errors (402, 401, quota) do not retry automatically. They block the Session. Any other error falls through the fallback chain. Retry counts are fields on the `compressed` Event.
- Block state: after a credit or auth error the Session stops and shows a banner "compress needs pig credits". Actions: Retry (reruns the whole compress from the start) and Change compress model. The old harness handle is untouched. A new prompt does not run until compress succeeds or the user dismisses the banner. A `compress blocked` Event is appended, and it carries the same Retry action.
- Swap order: summarize first. Then start the new harness handle and send the Handoff. Release the old handle only after the first turn starts cleanly. If the new handle fails to start, retry per the policy, then block as above.
- Handoff delivery is the first prompt, as inline text. It is flagged as a Handoff in the Event log, so the UI renders a collapsed block. A copy is stored in the Session directory per compress.
- Event log: pre-compress Events stay. One `compressed` Event is a divider with: compress number, tokens before, summarizer model, fallback step used, retry counts. Fallback and fold steps append Events with their reason.
- Queued follow-ups and held steers move to the new handle in order, after the Handoff prompt. A held steer degrades to a follow-up.
- Compress waits while a spawn Group of the Session is outstanding. Group, Steer and Subagent links live in the Daemon and survive the swap.
- Triggers: manual button, and auto when used/size reaches the existing `compress_threshold_percent` (default 80) with the existing `compress_cooldown`. Auto fires only at a tool boundary or when idle. The existing `auto_compress` and `compress_instructions` settings keep their meaning; instructions are added to the summarizer prompt.
- New domain terms **Handoff** and **Compress** go in `GLOSSARY.md`.

## Testing Decisions

- One seam: the Daemon HTTP API (create Session, prompt, compress, read Session and Events), driven against the fake Backend and a fake summarizer. Good tests assert external behavior: the Event log, the Session response, the prompts the Backend received. They do not assert internal function calls.
- Fake summarizer modes cover: good output, empty output, wrong format, transient error then success, credit error, oversize input. The fake Backend gets a failing-start mode.
- Prior art: the existing Daemon tests for the pig in-place compact, resume, and the queue and steer paths.
- Live probes in `scripts/` run compress against real pig, pig-over-ACP and claude Sessions. They need live agents and are not part of the check.
- UI logic (divider, banner, settings) is tested at the colocated `lib/*.js` level, as for other views.

## Out of Scope

- Dynamic-context-pruning style request rewriting. It needs a hook per harness.
- headroom or any tool-output compression layer. A later idea: deterministic tool-output truncation in the Daemon before the summarizer.
- A raw recent-message tail kept beside the Handoff.
- Per-harness compress models.
- Changes to the terminal `emberloop/` compress.
- Using `caveman-compress` as a runtime tool. Only its register is borrowed.

## Further Notes

- Prior art in this repo: `.scratch/archive/context-compress/spec.md` (terminal build: pig one-off, restart in place, pinned recipe). The web build reuses its recipe and sanity checks.
- Prior art elsewhere: OMP (method cascade, skips handoff on overflow), pi (summary sections, never cut at a tool result), Codex and Claude Code (handoff-style summary prompt), Roo (summarize with the same model; this spec chooses a separate pig model on purpose).
