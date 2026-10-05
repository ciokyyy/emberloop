# 01: TUI module and live spec list

**What to build:** `wick-pig tui` opens a bubbletea TUI in the current pane, from its own Go module so the core stays dependency-free. The Runs tab lists specs grouped by repo (current repo first), including specs with no run yet, each with running/ended status and ticket progress (`merged/total`), refreshed every second. Arrow and vim keys move, a click selects, `tab` switches tabs, `q` quits. The data comes from a `Snapshot(stateDir, herdr)` read model.

**Blocked by:** run-records 01, run-records 02

**Status:** ready-for-agent

- [ ] Separate module; `wick-pig tui` starts it (exec or installed binary, decided and documented here)
- [ ] `Snapshot` returns repos → specs with run status and progress; tested against fixture state dirs
- [ ] Running is decided by the orchestrator pane being alive (fake herdr in tests)
- [ ] Keyboard and mouse selection; 1-second polling
- [ ] A few model tests feeding key and mouse messages
