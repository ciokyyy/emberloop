# Update el

"Update" from the user means the full suite below: steps 1, 2 and 3, every time, without asking which parts. Run it after finishing a change too when the user asked for the update.

Run from the checkout (the repo whose root holds `AGENTS.md`). Do the steps in order; stop and report on the first failure.

1. Everything: `cd emberloop-web && el update` (`el install` is the same command).
   - Skips the web build (`npm run build`) when the web source is unchanged since the last build; otherwise rebuilds the UI. Always rebuilds `el`, `el-mcp` and `el-web` with version stamps into the directory of the running `el` binary (normally `~/.local/bin`), keeps the previous set, and restores it when the new `el-web` fails its health check. Refreshes the `el://docs` pages from `docs/agent/`, then runs `el deps`.
   - Running sessions keep the old binary until they restart.
   - The prod Daemon (systemd unit `emberloop-web`, port 8787) notices the new binary and restarts itself when idle; with Runs running it takes the graceful restart instead (Workers and Reviewers stop after their current tool call and continue in the same Sessions and worktrees on the new Daemon), so an update never waits on tickets. Check it: `curl -s -H "Authorization: Bearer $(cat ~/.local/share/pig/enderbloom/daemon.token)" http://127.0.0.1:8787/api/health` must print `{"ok":true}`. The UI's bottom-right stamp (`ui <hash> · be <commit>`) names the builds; a stale browser offers `update available — reload`.
2. Skills library, only when asked: `el skills install`.
3. Verify with `el://docs/verify`.

If you run inside a Session of the prod Daemon, its self-restart drops your own agent process once the turn ends. The next prompt reloads the Session. Tell the user before you update.

Never commit as part of an update.
