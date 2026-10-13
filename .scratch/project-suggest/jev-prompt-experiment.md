# Jev project-pick prompt experiment (measured 2026-10-13)

Goal: find the best question wording for picking a Project from a user's FIRST
message with Jev, with measured accuracy.

## Bottom line

**v3 (name + path + one-line gist) wins clearly: top-1 accuracy 0.920 (23/25),
false-positive rate on none-prompts 0.000 (0/6), mean latency ~290ms, mean cost
~$0.000038/call.** The two v3 misses both predict `none` with low top
probabilities (0.44, 0.43), so the recommended accept gate abstains on both and
the accepted set is perfect. Recommended ship wording is v3 with accept gate
**top >= 0.5 AND (top - second) >= 0.2** (measured accepted-accuracy 1.000 at
0.80 coverage); abstain means ask the user which project they mean.

## Setup (what the previous attempt got wrong)

The previous worker spoke the wrong endpoint (`jev-api.org/api/v1/decisions`)
with a `choice` question type through the memory Decider. The real Jev here is
OpenRouter's `POST https://openrouter.ai/api/v1/systemone`, model
`typesafe/jev-1.13`, via `emberloop/tools/systemone` (`NewOpenRouter`,
`Request{Model, State, Questions map[string]Question{Type: noul, ...}}`,
`Response.Answers[id].Noul`). The test
(`emberloop-web/jev_projectpick_live_test.go`, package `web`, imports
`example.com/emberloop/tools/systemone` through the existing
`replace example.com/emberloop => ../emberloop`) follows the find cascade's
conventions (`emberloop/tools/cascade.go` stage2, ~lines 740-800, and stage3
~895-930): each question names its candidate in backticks, points at its State
entry (`` see `first_prompt` / `projects` ``), and carries true/false Criteria.
Key comes from `systemone.APIKey(ctx)` (OPENROUTER_API_KEY else pig auth) and
travels only in the Authorization header; it is never printed, logged, or
written. Gate is `EL_LIVE_JEV_PICK=1` only (skips otherwise, so the repo check
never touches the network).

Design: ONE call per prompt, one noul question per project (id = project name)
plus a `none` noul question; prediction = argmax over all answers. 25 labelled
prompts x 3 variants = **75 calls used of the HARD 100-call budget** (counter
in the test aborts past 100; one retry per call at most).

## Projects and gists

From `~/.local/share/pig/enderbloom/projects.json` (4 projects) and each
project's own docs:

| Project | Path | One-line gist (source) |
|---|---|---|
| cioky | /home/cioky/Projects/cioky | Empty directory with no files yet; a blank parked project slot (`ls` verified: zero files) |
| tarek | /home/cioky/Projects/tarek | Italian lead-research notes on Wengo.it fortune-teller (cartomanti) profiles in data/result*.md (`data/result_00.md`) |
| emberloop | /home/cioky/Projects/emberloop | Go harness for parallel agent implement/review loops over pig and claude sessions (repo README first lines) |
| scintilla-chat-v2 | /home/cioky/Projects/scintilla-chat-v2 | Scintilla tarot-reader marketplace monorepo: SvelteKit apps under apps/, Turbo plus oxlint gate (`CONTEXT.md`, `apps/` layout) |
| none | — | None of these projects: a general question or something else entirely |

Prompt set (25): 4 explicit-name, 5 path-mention, 8 implicit-topic, 3
ambiguous/dual-plausible (kept in totals), 6 none/general (one hard negative:
"Explain how SQLite FTS5 ranking works in general", adjacent to emberloop's
memory engine). `emberloop-web/` and `installer/` prompts label emberloop
(both live inside that checkout).

## Measured results (75 live calls, model typesafe/jev-1.13)

| Variant | Top-1 accuracy | None-FP | Mean latency | Mean cost/call | Best accept gate (offline sweep) |
|---|---|---|---|---|---|
| v1 names-only | 0.640 (16/25) | 0.000 (0/6) | 312ms | $0.000033 | top>=0.9: 1.000 acc, 0.32 coverage |
| v2 name+path | 0.640 (16/25) | 0.000 (0/6) | 311ms | $0.000035 | top>=0.9: 0.917 acc, 0.48 coverage |
| v3 name+gist | **0.920 (23/25)** | 0.000 (0/6) | 290ms | $0.000038 | top>=0.5 + margin>=0.2: **1.000 acc, 0.80 coverage** |

Usage was reported on all 75 calls. Total spend ≈ $0.0027. Full run completed
in ~23s (all sequential).

v3 accept-gate sweep (offline, no extra calls; margin = top - second):

| Gate | Accepted-accuracy | Abstention |
|---|---|---|
| top>=0.0 (no gate) | 0.920 (23/25) | 0.000 |
| margin>=0.3 | 1.000 (20/20) | 0.200 |
| top>=0.5 (any margin) | 1.000 (20/20) | 0.200 |
| top>=0.6 (any margin) | 1.000 (20/20) | 0.200 |
| top>=0.7 (any margin) | 1.000 (19/19) | 0.240 |
| top>=0.9 (any margin) | 1.000 (16/16) | 0.360 |

v1/v2 never clear 1.000 below top>=0.9, and there only at ~0.3-0.5 coverage;
their margin sweep barely moves (e.g. v1 top>=0.7: 0.875 at 0.36 abstention
for every margin). Paths add nothing over names; the gist is the whole game.

## Miss analysis

Every miss except one is an under-prediction to `none` (the safe direction: it
asks the user instead of starting work in the wrong repo). No variant ever
false-positives on a general question (none-FP 0.000 across the board).

- v3's only 2 misses: `installer/install.sh fails...` (want emberloop, top 0.44)
  and `Speed up the test suite...` (want scintilla-chat-v2, top 0.43, ambiguous
  by design). Both abstain under the recommended gate.
- v1/v2 miss the same implicit-topic prompts harder (tarek Cleo 0.80/0.91 top
  for `none`; cioky wiki 0.77/0.84 for `none`): without the gist the judge
  answers `none` even when fairly confident. One v2-only cross-fire:
  `Add a README documenting the project setup.` went to scintilla-chat-v2 at
  0.24 (ambiguous prompt, near-tie 0.24 vs 0.22).

## Recommended final wording + thresholds

State: `{"first_prompt": "<verbatim first message>", "projects": {<name>:
"<name> (<path>): <gist>"}}` with the gists above. One noul question per
project:

> Is the user's first message (see `first_prompt`) a task that belongs in the
> `<name>` project (see its entry in `projects`)?
> true: The first message asks about, changes, or directly concerns work in that project
> false: The message concerns a different project, or is a general question unrelated to any listed project

plus the `none` question:

> Is the user's first message (see `first_prompt`) unrelated to every listed
> project (see `projects`), so it belongs in none of them?
> true: The message is a general question or concerns something else entirely
> false: The message asks about, changes, or directly concerns work in one of the listed projects

Decide argmax; **accept iff top >= 0.5 AND top - second >= 0.2, else abstain
(ask the user)**. Measured: 1.000 accepted-accuracy at 0.80 coverage. If the
project list ever exceeds ~7, pre-filter to the top seven by token overlap
before adding `none`, and record the filter with the result. Re-pin the model
before shipping: thresholds were measured against `typesafe/jev-1.13`
(`systemone.JevModelPinned`), not the floating alias.

## To re-run

    cd emberloop-web && EL_LIVE_JEV_PICK=1 go test -count=1 -v -timeout 25m -run TestLiveJevProjectPick .

Skips without the env var (and without a key). Verification without
credentials stays `go vet ./...` plus `go test ./...`.
