# 30: Replay eval and Decider eval

**What to build:** Offline tools. (a) Replay: build memory from Sessions before Session N and measure whether the Rules that would be injected cover the corrections made in N (recall@5); compare FTS5 against a remote embedder on the same cases. (b) Repeat-correction rate over a period. (c) Decider eval: run `heuristic`, `llm`, `jev` and Laya on a hand-labelled set of 200 to 300 user turns, oversampling heuristic hits, and report precision at the approve threshold. (d) The Run A/B hook: one of N Workers with recall off.

**Blocked by:** 19, 20, 24

**Status:** ready-for-agent

- [ ] A command prints recall@5 and the repeat-correction rate for a Project's history
- [ ] The embedder comparison runs on the same cases and prints the gap
- [ ] A labelling file format and a script that scores each Decider on it exist; labels come from the queue and from hand labels
- [ ] The Run A/B option is a Run setting and is recorded in the Run file
- [ ] Unit tests cover the metrics on canned logs
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
