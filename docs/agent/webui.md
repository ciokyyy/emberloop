# Web UI (Emberloop)

The web UI is a local Go Daemon with the UI embedded in the binary. `el update` rebuilds the UI and the binary together; never build either by hand for prod.

## Two instances

| | Prod | Dev |
|---|---|---|
| URL | http://127.0.0.1:8787, and from other tailnet devices http://100.94.50.96:8787 | http://127.0.0.1:7941 |
| Run by | systemd user unit `emberloop-web` | `scripts/web-dev.sh` (foreground, in a background pane) |
| Binary | `~/.local/bin/el-web` | `go run` from the checkout |
| State | `~/.local/share/pig` | `~/.local/share/pig-dev` (`EL_WEB_DEV_STATE` overrides) |

The product name is Emberloop. The on-disk state subtree keeps its internal name: Sessions, Groups, Runs, Projects and push data live under `<state-dir>/enderbloom/`, and that path is unchanged.

Never start the Daemon with the default `-addr` (random port): it picks a random port and the URL changes on every start.

## Update prod

`el update` (see `el://docs/update`) does the whole prod update: it rebuilds
the UI unless its source is unchanged, rebuilds `el-web` with version stamps
into `~/.local/bin/el-web`, keeps the previous set, and rolls back to it when
the new binary fails its health check.

Do nothing else. The Daemon checks its own binary every minute (`-restart-check`). When the file changed and the Daemon is idle (no turn running, nothing queued, no Run driving), it closes live agent processes and re-executes itself with the same pid and address. When Runs are running, the Daemon does not wait for them: it takes the graceful restart below, so an update never waits on tickets and the Runs carry on after it. Idle Sessions reload on the next prompt.

Check it: `curl -s -H "Authorization: Bearer $(cat ~/.local/share/pig/enderbloom/daemon.token)" http://127.0.0.1:8787/api/health` must print `{"ok":true}`, and `el-web --version` names the build. The UI's bottom-right stamp (`ui <hash> · be <commit>`) names the builds a browser runs; a stale browser offers `update available — reload`.

If you run inside a Session of this Daemon, the restart drops your own agent process once the turn ends. The next prompt reloads the Session. Tell the user before you update.

`GET /api/version` answers the Daemon's `{version, commit, uiHash}`; `GET /api/health` answers `{"ok":true}`. Both need the machine token or a login cookie (see Accounts and API auth).

## Accounts and API auth

- Every `/api` route, SSE stream included, answers 401 without a login cookie (`el_session`) or the machine token as `Authorization: Bearer`. Only `/api/auth/signup|login|logout`, `/api/auth/me` and `/api/auth/state` answer without one; `state` gives only `{empty, signup_enabled}` for the sign-in screen.
- The first signup becomes the admin. Sign up the admin yourself right after a fresh start, before anyone else reaches the Daemon.
- The admin switches `signup_enabled` in Settings; off refuses signup.
- Reset a password: `el-web user set-password <name>` (prod; add `-state-dir ~/.local/share/pig-dev` for dev). It reads the new password from a terminal with echo off, or one line from a pipe. It signs that User out everywhere; a running Daemon sees the change at once.
- Machine token: `<state-dir>/enderbloom/daemon.token`, new on every start, owner-only. `el restart` reads it. It is no User: it cannot create Sessions or Runs. The Daemon's own agents get it as `EL_WEB_TOKEN`.
- Sessions and Runs record the User who created them. Sessions and Runs with no owner are hidden; nothing migrates them.

## Shared Claude login

All Users share one Claude login through a static setup token, so parallel
claude adapters never race each other's OAuth refresh (the failure looked
like `Failed to refresh OAuth token: another Claude Code process is
refreshing it`). Per-User `CLAUDE_CONFIG_DIR` isolation of transcripts and
settings is unchanged; only the login is shared.

- One-time setup (owner): run `claude setup-token` and store the token with
  `el-web claude-token set` (prod; add `-state-dir ~/.local/share/pig-dev`
  for dev). It reads the token once with echo off, or one line from a pipe,
  and writes `<state-dir>/enderbloom/claude-oauth-token` owner-only (0600).
  Alternatively set `EL_CLAUDE_OAUTH_TOKEN` (or `CLAUDE_CODE_OAUTH_TOKEN`) on
  the Daemon; env beats the file. Then restart the Daemon (`el restart`).
- Yearly rotation: the setup token lasts one year. Before it expires, run
  `claude setup-token` again, store the new token the same way, and restart.
- What it cannot do: a token login has no Remote Control and no claude.ai
  connectors. Anything needing those still needs an interactive `/login`.
- Without a token the Daemon falls back to linking the host
  `~/.claude/.credentials.json` into each User dir, and repairs one
  divergence per start: an owner copy that differs from the host and is older
  than the host re-links (a newer copy is left alone — it may hold a fresher
  refresh). A refresh race still fails the turn, but the transcript names the
  fix and the prompt retries once after ~2s.

## Service control

- Status: `systemctl --user status emberloop-web`
- Restart: `el restart` (`--dev` for port 7941, `--wait` to block until the Daemon is back; never `--wait` inside a Daemon Session). It POSTs `/api/restart` with the machine token (`EL_WEB_TOKEN`, else the `daemon.token` file of the state dir; `pig-dev` for `--dev`); `systemctl --user reload emberloop-web` (SIGUSR1) does the same. This is the only restart an agent runs. The Daemon pauses every running Run and marks it `resumeOnRestart`, stops each Run Worker and Reviewer right after its current tool call finishes (at once when no tool runs; their queued messages are saved, nothing re-prompts them), waits up to `-restart-drain` (default 5m) for the other Sessions to finish their turns, closes agent processes, and re-executes itself. The new Daemon resumes the marked Runs on its own, with the same Worker and Reviewer Sessions in the same worktrees; a Session whose turn the restart cut short gets a continue prompt at once. A Run still running on disk after a kill or crash resumes the same way; a Run paused by hand stays paused. A Daemon that boots 4 times within 5 minutes stops resuming Runs on its own (see `restart.log`); resume them by hand once the crash is fixed. Idle Sessions reload on the next prompt. `el restart` prints the pending request and what still blocks the drain (`GET /api/restart` answers the same state); when the drain expires the restart is forced regardless of idle, and a repeat past drain plus a minute re-arms a stuck request. Every step is logged timestamped to stderr and `<state-dir>/enderbloom/restart.log`, so an exec failure stays visible there.
- Never run `systemctl restart|stop`, `kill` or a manual `el-web` launch against the prod Daemon, least of all from a Session inside it: the restart drops your own agent process and cuts Run Workers mid-turn (they resume on the next Daemon, but `el restart` drains first and loses less).
- Logs: `el logs` (`-n N`, `-f`, `--since TIME`, `--grep PATTERN`); it wraps `journalctl --user -u emberloop-web`. The dev Daemon has no journal: read its pane.
- The unit file is `~/.config/systemd/user/emberloop-web.service`. It runs `el-web -addr 127.0.0.1:8787` with `Restart=always`. Linger is on, so it starts at boot.

## Dev loop

- Start: `scripts/web-dev.sh` through the background tool, never with bash. Rebuild the UI first (`npx vite build` in `emberloop-web/web`), then restart the pane, because `go run` embeds `web/dist` at compile time.
- Browser suite: `cd emberloop-web/web && npm run test:e2e`. It starts its own throwaway Daemon and needs no key.
- Component tests: `cd emberloop-web/web && npx vitest run`. Go tests: `cd emberloop-web && go test ./...`.

## URLs and fast session load

Real paths (Daemon SPA fallback serves `index.html`; missing assets and
`/api/*` still 404): `/s/<id>` Session with optional `?at=<seq>` scroll
target, `/runs`, `/runs/<runId>`, `/runs/<runId>/<ticketId>`, `/settings`,
`/` (last viewed from localStorage `eb.last`, else newest). Unknown ids show
Session-not-found, never a random Session. Push notification taps open
`/s/<sessionId>`.

Session open loads the last 6 turns (`GET /api/sessions/<id>?tail=6`,
compacted to the last `tool_call_updated` per tool id); older turns page in
via `?before=<seq>&turns=6` (`hasMore`/`firstSeq`). No query means the full
log, as before.

## Projects

Projects are recorded folders. A Session started in a Project runs with the Project path as its cwd. They live in `<state-dir>/enderbloom/projects.json`. Worker and Subagent Sessions in a worktree stay grouped under their parent Project.

Never commit as part of an update.

## Reach it from other devices (Tailscale)

The prod unit runs `el-web -addr 127.0.0.1:8787 -tailnet -allow-host cachy-cioky,cachy-cioky.tailef9ae.ts.net`.

- `127.0.0.1` alone is reachable only from this machine. `-tailnet` adds a second listener on this machine's Tailscale IPv4 (100.64.0.0/10) at the same port, so any device on the tailnet opens `http://<tailscale-ip>:8787` (`tailscale ip -4` prints it).
- Nothing listens on `0.0.0.0` or the LAN. The Daemon refuses a bind that is not loopback or a Tailscale address, and rejects any request whose `Host` or `Origin` is not loopback, a Tailscale IP, or on `-allow-host`.
- Login is per account (see Accounts and API auth), but the tailnet is still the perimeter: anyone on the tailnet can reach the sign-in page, and signup stays open until the admin turns `signup_enabled` off. Never bind it to `0.0.0.0` or expose it with Funnel.
- The listener retries every 10 seconds, so it comes up even when Tailscale starts after the Daemon.
- HTTPS alternative: `sudo tailscale serve --bg --https=8787 http://127.0.0.1:8787`, then use the `-allow-host` name. Needs root once.

### Slow over Tailscale

- The Daemon gzips text assets (html, js, css, svg, json, wasm) when the client sends `Accept-Encoding: gzip`, and serves `assets/*` with `Cache-Control: public, max-age=31536000, immutable` (Vite hashes the names). `index.html` is `no-cache`. Check: `curl -sI -H 'Accept-Encoding: gzip' http://127.0.0.1:8787/assets/<file>.js` shows `Content-Encoding: gzip`.
- Check the path first: `tailscale status` and `tailscale ping <peer>` must say `direct`, not `relay`. Relayed (DERP) paths are slow. Mobile data often cannot go direct (CGNAT): enable UPnP/NAT-PMP on the router, keep IPv6 on, or run a Tailscale peer relay on a tailnet device with a public endpoint.
- `tailscale netcheck` shows UDP, port mapping and DERP latency. Prefer ethernet over wifi on the host.
- MagicDNS names can fail on this host; open the Daemon by Tailscale IP.

## Context compression

Compress makes a Session's context smaller by Handoff restart on every Backend: a one-off pig call writes a Handoff from the Event log, a fresh harness handle takes it as first prompt, the Session id stays. The fake, pig SDK, pig-over-ACP, claude and grok Backends all swap this way.

- Manual: the Compress button under the chat, or `POST /api/sessions/<id>/compress` with `{"instructions": "..."}`. It answers 409 while the Session works, and with the reason while the compress model is unset or has no pig auth. During a spawn Group of the Session it answers 202 `{"waiting":"group"}` and compresses once the Group reports.
- Compress model: Settings page, "Context compression", "Compress model" picks `compress_model` from the authed pig models only. Until it is set, Compress stays disabled and shows why.
- Auto: after every settled turn (never a Handoff turn), a Session at or past `compress_threshold_percent` or the absolute `compress_threshold_tokens` cap (default 300000, 0 means off) compresses by Handoff through the same swap. The percent gate alone never fires on 1M-window claude Sessions (80% is 800k tokens, far past anything seen); the token cap is what makes auto-compress work there, while smaller pig windows keep tripping the percent gate first. It fires only at a tool boundary, never mid-turn; a working Session skips to the next turn end and an outstanding Group defers until it reports. `compress_cooldown` counts turns (non-Handoff prompts) since the last compress; the first compress is never cooled. With no compress model auto runs nothing and the Session `compressNote`/Settings warning names the reason; a blocked Session never auto-compresses until Retry or dismiss.
- Settings page section "Context compression" writes `compress_model`, `auto_compress`, `compress_threshold_percent`, `compress_threshold_tokens`, `compress_cooldown` and `compress_instructions` to `~/.emberloop/settings.json`. Changes apply to the next turn.
- The transcript shows a divider with the Handoff number, model, fallback step and retries. Oversize input adds one step Event per fold or fallback, with its reason, before the divider. The meter under the chat seeds an estimated usage Event at Session start and at prompt dispatch (Backend window: pig SDK, catalog or known adapter size; used from log chars at four per token), so it shows immediately instead of waiting for the first turn end; pig emits a zero-used reading when only the window is known (e.g. right after compaction), and the ACP Backends (claude, pig-acp, grok) emit a fallback reading from their known window when the adapter sends no usage_update. The meter always renders, read-only on scout, ended and unresumable Sessions; only Compress and the composer stay gated.
- Retries: a summarizer call or a new-handle start that fails with a network, 5xx or rate-limit error retries after 2s, 8s and 30s. A summarizer failure that outlasts them falls back to the deterministic Handoff; a start failure that outlasts them ends the compress with an error. The divider shows the retry counts.
- Blocked: a credit or auth failure (402, 401, quota) does not retry. The Session blocks: the banner "compress needs pig credits" offers Retry (runs the whole compress again), Change compress model (opens Settings) and Dismiss. The transcript keeps a `compress_blocked` card with the same Retry button. The old handle stays. New messages queue and wait until Retry succeeds or the banner is dismissed. A compress waiting on a Group is dropped by the block; a Retry while a Group still runs answers 409.
- Dismiss: `POST /api/sessions/<id>/compress/dismiss` clears the block without a compress; the held messages then run on the old handle. It answers 409 while the Session works or no block is set.

## Push notifications

Settings page, section "Notifications": the Turn on button subscribes this browser. The Daemon pushes when a root chat turn finishes or fails. Subscriptions and the VAPID key live in `<state-dir>/enderbloom/push/`. `web/public/sw.js` shows the notification, so a UI update that touches it needs the normal update steps above.

- Browsers allow push only in a secure context: https, or http on localhost. A plain `http://<tailscale-ip>:8787` is not secure, so the button stays disabled there with a note. Use the HTTPS alternative under "Reach it from other devices" and open `https://<name>.ts.net:8787`.
- The Daemon needs outbound internet to reach the browser push service (FCM, Mozilla, Apple). The phone needs its own internet.
- Subscribe once per browser. A 404 or 410 from the push service drops the subscription.

## Disk hygiene

Parallel Runs leave bulk behind (Go build cache, stale temp dirs, abandoned worktrees), so a Daemon janitor trims them every 10 minutes and the Daemon refuses new work when the disk is nearly full.

- Go cache cap: global setting `disk_go_cache_cap_gb` (default 8 GiB, 0 disables trimming), admin-only in Settings under Concurrency. Past the cap the janitor deletes cache files oldest-mtime-first until under 80% of the cap, never touching files younger than 10 minutes.
- Temp sweep: entries in `os.TempDir()` starting with `go-build`, `el-web-test-` or `el-test-` and older than 6h are removed.
- Worktree reap: `.el-worktrees/<name>` dirs of finished (done/abandoned) Runs older than 24h go through `git worktree remove --force` + `prune`, only when Run state names no Session using them and no Worker/Reviewer check runs in that repo.
- Guard: free space below 10% or 5 GiB (whichever is larger) refuses new Sessions, Runs, Tickets and worktrees with 507 and names what the janitor can free; below 20% it pushes one warning per 6h and the header shows a one-line banner.
- `GET /api/disk` (admin only) answers `{freeBytes, totalBytes, cacheBytes, low, critical, last}` with the last janitor result; `GET /api/health` stays exactly `{"ok":true}`.
