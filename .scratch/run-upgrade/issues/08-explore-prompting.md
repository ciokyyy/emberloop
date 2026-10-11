# 08: Explore prompting

**What to build:** Tool descriptions and the Worker and Reviewer prompts steer exploration to `find` and `execute` before repeated bash grep, and a script replays web-v1 exploration patterns to count calls with and without the guidance.

**Blocked by:** find-graph-index (07)

**Status:** ready-for-agent

- [x] Descriptions and prompts updated
- [x] Measurement script prints call counts for the replayed patterns
- [x] Result recorded in this ticket's note
- [x] `.emberloop/check` passes

Note: `scripts/explore-replay.sh` holds three web-v1 call-list fixtures (A locate-concept, B 5-marker sweep, C grep+read fan-out) and computes counts from the lists: 11->2, 5->1, 8->1; total 24->4 (saved 20, 83% fewer). Guidance is one shared const in worker/reviewer role prompts plus `find`/`execute`/`bash`/`grep` descriptions.
