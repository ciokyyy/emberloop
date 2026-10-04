# What must exist on the VPS before any run can start?

Type: task
Status: open
Blocked by:

## Question

The destination runs on a VPS reached over Tailscale, and workers execute there. Nothing exists there yet. This ticket is the manual work that has to happen before any decision downstream can be made from evidence rather than assumption.

1. **herdr server.** The relay polls the herdr CLI and the harness drives panes, so a herdr server must run on the VPS. Headless, `systemd --user` or system unit, surviving reboot. Which machine, which unit, which config.
2. **The relay.** It currently runs on this laptop. Decide: a second relay on the VPS, or bind this one to a Tailscale address. The relay already refuses to bind beyond loopback without `HERDR_RELAY_TOKEN`, so the token is the only new secret.
3. **Both harnesses installed and authenticated.** `pig` and `claude`, each with working model access from the VPS. `pig` has no API key configured anywhere yet — `/login` is required and may mean OAuth, which does not survive a headless box without thought.
4. **Repo access.** Clone or bind the target repos, with push credentials. Decide whether the harness clones per run or works from a long-lived checkout with worktrees.
5. **Worktree bootstrap.** A fresh worktree has no `node_modules`. Decide who installs dependencies, whether a shared package-manager store is configured (this is the same class of problem as the verification gate in ticket 06, and the two may share one mechanism), and what the cold-start cost is.
6. **Secrets.** Provider keys, git credentials, relay token. Where they live, who rotates them, and what a worker can read.

Ticket 02 gives facts 1 and 2 for this laptop; everything else is unproven. Resolving this produces the environment the rest of the map reasons about.