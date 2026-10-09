# 06: Memory sampler for harness process trees

**What to build:** Every 5 seconds, each session's own el process records the RSS of its harness process tree (the harness, `el-mcp`, and anything they started) per process: `{agent, at, pid, name, rss}`, appended to the agent's memory log in the archive. Samples are written whether or not the TUI is open, and stop when the session ends.

**Blocked by:** 01

**Status:** done

- [x] Finds the harness as its parent process and walks descendants via `/proc`
- [x] One row per process per sample; processes that vanish mid-walk are skipped
- [x] Test: a fake `/proc` tree with known pids, parents, names and RSS gives the expected rows
- [x] Sampling overhead is a handful of file reads per tick
