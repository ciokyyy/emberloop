# What is the notification and reply channel from a VPS over Tailscale?

Type: research
Status: resolved
Blocked by:

## Question

The harness runs on a VPS reached over Tailscale. It must report status off-box, and — when an implementation genuinely needs an answer — deliver a question and route the reply back into the running worker. Which channel is simplest and actually reliable?

## Answer

**A relay is already running on this machine. Bidirectional works today, and the reply path needs no TUI awareness.**

### What exists on this box

`herdr-remote` — checkout at `~/herdr-remote`, running as `systemd --user herdr-relay.service` via `uv run`, listening on `127.0.0.1:8375`, polling the herdr CLI every 2s. A second unit, `herdr-tunnel.service`, runs `cloudflared` named tunnel `herdr-remote-omp`, mapping `omp-agent.koboscintilla.com` and `omp-agent.kobo-scintilla.com` to `127.0.0.1:8375`.

Verified live:

| probe | result |
|---|---|
| `http://127.0.0.1:8375/` | 401 Invalid token |
| `?token=<wrong>` | 401 |
| `https://omp-agent.koboscintilla.com/` | 401 — token enforced through the tunnel |
| `?token=<real>` WebSocket | authenticated, streamed `sessions`/`agents` frames, clean close 1000 |
| `/api/vapid-public-key` | 200 but `{"publicKey": ""}` — web push NOT configured |

Auth is `HERDR_RELAY_TOKEN` (64 hex chars), stored in `~/.config/herdr-remote/secrets.env` and duplicated in `config.env`, both `0600`. The relay hard-fails at import if it binds beyond loopback without a token — so a non-loopback bind is safe once the token is set.

### Outbound: a POST is enough

Any `POST /?d=<urlencoded json>` lands on the relay's `event_queue` and is broadcast to every connected WebSocket client. That is exactly what the `herdr.push` plugin does — and **`herdr.push` is currently dead**: `herdr plugin config-dir herdr.push` is an empty directory, so `HERDR_RELAY` is unset and `on_event.sh` exits at line 12. The plugin is installed and enabled; it just has nowhere to send. Pointing it at the relay is a config line.

Dependencies are only `curl` and system `python3`.

### Inbound: this is the important half

`herdr_relay.py:1354-1366` — `send_text` calls `herdr pane send-text <pane> <text>`, **unconditionally**. It needs only a known `pane_id` and ≤1000 characters. No agent detection, no TUI parsing, no prompt matching.

So the bidirectional requirement is satisfied by the dumb path: the harness POSTs a question, the human replies through whatever client, and the relay writes the answer straight into the pane's stdin — landing in the worker's own input.

### The one thing that does not transfer

`herdr_relay.py:621-665` `detect_question()` recognises a question only by scanning pane text for an option block containing an option labelled **"Other (type your own)"**, or "Done selecting". `respond` (`:1211-1255`) additionally requires `pane_is_omp`. Both are tied to the OMP/pi TUI's rendering.

**pig will not be detected.** Not a blocker — but any pig-side question surface must render in a shape `detect_question` recognises, or we accept that questions arrive as plain text a human reads and answers without the relay offering a one-tap reply.

Also noted at `:978-990`: for `status=blocked` the relay re-reads the pane and builds the message itself. **The question text must be rendered in the agent's pane, not merely POSTed.**

### What this does not settle

1. **Location.** The relay runs on this laptop, not the VPS. Either run one on the VPS, or expose this one over Tailscale. The relay already supports a non-loopback bind behind its token.
2. **Client.** WebSocket clients exist for this relay; web push is configured but has zero subscriptions and a blank public key. Whether the human's client is a phone browser, a desktop, or a Discord channel is still open.
3. **herdr-web is a separate, weaker thing.** `barnuri.herdr-web` 0.1.1 is disabled as a plugin and runs as `systemd --user herdr-web.service` on `127.0.0.1:7936`. It spawns a real PTY per WebSocket client and writes input into it, so it is bidirectional too — **but it has no auth at all**: `isAllowedOrigin` returns true when `Origin` is absent and accepts any same-host origin. It must never be bound publicly. Treat it as a local convenience, not a channel.