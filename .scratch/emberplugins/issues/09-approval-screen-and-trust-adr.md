# 09: Approval screen and trust ADR

**What to build:** A new or changed Emberplugin does nothing until the user approves its declared permissions on a one-time screen. The screen lists every permission group and says plainly that plugin scripts and servers run with the user's OS rights, and the permission system gates only the Daemon API. A changed permission set needs a fresh approval. Sudo and file writes always need approval. An ADR records the origin-isolation and no-sandbox decisions.

**Blocked by:** 03 (Token, permissions and LLM call)

**Status:** ready-for-agent

- [ ] A plugin starts unapproved; tokens, jobs, servers and Sessions refuse until approval
- [ ] The UI approval screen lists permissions and the OS-rights statement
- [ ] A permission set change flips the plugin back to unapproved
- [ ] The default read plus LLM grant is shown separately from groups that need explicit approval
- [ ] `sudo` and `files` need approval even when declared
- [ ] ADR added under `docs/adr/` for own-origin iframe, token-only access and the no-OS-sandbox trust model
- [ ] Tests cover approve, revoke, changed set, and refusal paths
