# Web UI (Enderbloom Web)

The web UI is a local Go Daemon with the UI embedded in the binary. It is a separate program from `el`: `el install` does not rebuild it.

## Two instances

| | Prod | Dev |
|---|---|---|
| URL | http://127.0.0.1:7940, and from other tailnet devices http://100.94.50.96:7940 | http://127.0.0.1:7941 |
| Run by | systemd user unit `emberloop-web` | `scripts/web-dev.sh` (foreground, in a background pane) |
| Binary | `~/.local/bin/el-web` | `go run` from the checkout |
| State | `~/.local/share/pig` | `~/.local/share/pig-dev` (`EL_WEB_DEV_STATE` overrides) |

Never start the Daemon with the default `-addr` (random port): it picks a random port and the URL changes on every start.

## Update prod

Run from the checkout root. Do the steps in order and stop on the first failure.

1. Build the UI first. The Daemon embeds `web/dist`, so this must come before the Go build: `cd emberloop-web/web && npx vite build`.
2. Build the binary in place: `cd emberloop-web && go build -o ~/.local/bin/el-web ./cmd/emberloop-web`.
3. Do nothing else. The Daemon checks its own binary every minute (`-restart-check`). When the file changed and no Session is mid-turn and no Run is driving, it closes live agent processes and re-executes itself with the same pid and address. Idle Sessions reload on the next prompt.
4. Check it: `curl -sI http://127.0.0.1:7940 | head -1` must print `HTTP/1.1 200 OK`.

If you run inside a Session of this Daemon, the restart drops your own agent process once the turn ends. The next prompt reloads the Session. Tell the user before you rebuild.

Write the binary with `go build -o`, which replaces the file. Do not copy over a running binary with `cp`: that fails with "text file busy".

## Service control

- Status: `systemctl --user status emberloop-web`
- Force a restart (only when the self-restart did not happen, for example `-restart-check 0`): `systemctl --user restart emberloop-web`
- Logs: `journalctl --user -u emberloop-web -n 50`
- The unit file is `~/.config/systemd/user/emberloop-web.service`. It runs `el-web -addr 127.0.0.1:7940` with `Restart=always`. Linger is on, so it starts at boot.

## Dev loop

- Start: `scripts/web-dev.sh` through the background tool, never with bash. Rebuild the UI first (`npx vite build` in `emberloop-web/web`), then restart the pane, because `go run` embeds `web/dist` at compile time.
- Browser suite: `cd emberloop-web/web && npm run test:e2e`. It starts its own throwaway Daemon and needs no key.
- Component tests: `cd emberloop-web/web && npx vitest run`. Go tests: `cd emberloop-web && go test ./...`.

## Projects

Projects are recorded folders. A Session started in a Project runs with the Project path as its cwd. They live in `<state-dir>/enderbloom/projects.json`. Worker and Subagent Sessions in a worktree stay grouped under their parent Project.

Never commit as part of an update.

## Reach it from other devices (Tailscale)

The prod unit runs `el-web -addr 127.0.0.1:7940 -tailnet -allow-host cachy-cioky,cachy-cioky.tailef9ae.ts.net`.

- `127.0.0.1` alone is reachable only from this machine. `-tailnet` adds a second listener on this machine's Tailscale IPv4 (100.64.0.0/10) at the same port, so any device on the tailnet opens `http://<tailscale-ip>:7940` (`tailscale ip -4` prints it).
- Nothing listens on `0.0.0.0` or the LAN. The Daemon refuses a bind that is not loopback or a Tailscale address, and rejects any request whose `Host` or `Origin` is not loopback, a Tailscale IP, or on `-allow-host`.
- There is no login: anyone on the tailnet can use the Daemon. Never bind it to `0.0.0.0` or expose it with Funnel.
- The listener retries every 10 seconds, so it comes up even when Tailscale starts after the Daemon.
- HTTPS alternative: `sudo tailscale serve --bg --https=7940 http://127.0.0.1:7940`, then use the `-allow-host` name. Needs root once.

### Slow over Tailscale

- The Daemon gzips text assets (html, js, css, svg, json, wasm) when the client sends `Accept-Encoding: gzip`, and serves `assets/*` with `Cache-Control: public, max-age=31536000, immutable` (Vite hashes the names). `index.html` is `no-cache`. Check: `curl -sI -H 'Accept-Encoding: gzip' http://127.0.0.1:7940/assets/<file>.js` shows `Content-Encoding: gzip`.
- Check the path first: `tailscale status` and `tailscale ping <peer>` must say `direct`, not `relay`. Relayed (DERP) paths are slow. Mobile data often cannot go direct (CGNAT): enable UPnP/NAT-PMP on the router, keep IPv6 on, or run a Tailscale peer relay on a tailnet device with a public endpoint.
- `tailscale netcheck` shows UDP, port mapping and DERP latency. Prefer ethernet over wifi on the host.
- MagicDNS names can fail on this host; open the Daemon by Tailscale IP.

## Context compression

Pig Sessions compress in place with the SDK compaction: older history becomes a summary, recent turns stay, the Session id stays. Claude Sessions have no compaction hook.

- Manual: the Compress button under the chat, or `POST /api/sessions/<id>/compress` with `{"instructions": "..."}`. It answers 409 while the Session works.
- Auto: after a turn, when context is at or above `compress_threshold_percent` (default 80) and at least `compress_cooldown` turns (default 5) passed since the last compression. `auto_compress` (default true) turns it off.
- Settings page section "Context compression" writes `auto_compress`, `compress_threshold_percent`, `compress_cooldown` and `compress_instructions` to `~/.emberloop/settings.json`. Changes apply to the next turn.
- The transcript shows a "Context compressed" card with token counts and the summary; the meter under the chat refreshes.

## Push notifications

Settings page, section "Notifications": the Turn on button subscribes this browser. The Daemon pushes when a root chat turn finishes or fails. Subscriptions and the VAPID key live in `<state-dir>/enderbloom/push/`. `web/public/sw.js` shows the notification, so a UI update that touches it needs the normal update steps above.

- Browsers allow push only in a secure context: https, or http on localhost. A plain `http://<tailscale-ip>:7940` is not secure, so the button stays disabled there with a note. Use the HTTPS alternative under "Reach it from other devices" and open `https://<name>.ts.net:7940`.
- The Daemon needs outbound internet to reach the browser push service (FCM, Mozilla, Apple). The phone needs its own internet.
- Subscribe once per browser. A 404 or 410 from the push service drops the subscription.
