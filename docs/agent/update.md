# Update el

"Update" from the user means the full suite below: steps 1, 2 and 4, every time, without asking which parts. Run it after finishing a change too when the user asked for the update.

Run from the checkout (the repo whose root holds `AGENTS.md`). Do the steps in order; stop and report on the first failure.

1. Binaries and plugin: `cd emberloop && el install`.
   - Rebuilds `el`, `el-mcp` and `el-tui` into the directory of the running `el` binary (normally `~/.local/bin`), links the herdr compress plugin, refreshes the `el://docs` pages from `docs/agent/`, then runs `el deps`.
   - Refuses while any worker, reviewer or scout agent is live and names them. Tell the user; `--force` installs anyway and running sessions keep the old binary until they restart. Never pass `--force` unasked.
2. Web UI and Daemon (not covered by step 1): follow `el://docs/webui`. It builds the UI, then the binary into `~/.local/bin/el-web`; the prod Daemon (systemd unit `el-web`, port 7940) restarts itself when idle.
3. Skills library, only when asked: `el skills install`.
4. Verify with `el://docs/verify`.

Never commit as part of an update.
