# 12: Claude connect through claude auth login

**What to build:** An admin connects Claude from the browser. The Daemon runs `claude auth login` in a pty; the UI shows the link and sends back the pasted code. The login lives only in `~/.claude`, written by Claude Code itself, and never goes to pig (spec Parts 2 to 6, Providers).

**Blocked by:** 09-providers-page-and-api-keys

**Status:** ready-for-agent

- [ ] Connect on the Claude row starts `claude auth login` in a pty and shows its link
- [ ] The pasted code goes to the pty; success refreshes the Claude Quota card
- [ ] Nothing is written to pig's `auth.json`
- [ ] Go tests with a fake `claude` binary cover success, a bad code and pty exit; vitest covers the panel
- [ ] Repo check and `npm test` pass
