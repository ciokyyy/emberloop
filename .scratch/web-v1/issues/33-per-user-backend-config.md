# 33: Per-User backend config dirs

**What to build:** Give each User a separate config directory for backends that load user-level memory or instructions on their own (claude: `CLAUDE_CONFIG_DIR`; check whether pig and grok load anything). The Daemon starts each handle with the owner's directory. Verify what each harness loads and write the result in this ticket.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] A claude Session of User A does not see files in User B's config directory
- [ ] A note in this ticket lists what pig and grok load by themselves and what was done about it
- [ ] Tests at the Daemon seam assert the environment passed to the fake harness per owner
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes

## Note: what each harness loads by itself

Checked on this host: pig 0.4.0 (`pig --help`, pinned SDK v0.4.0 source), claude CLI 2.1.296 (SDK 0.3.293, adapter `claude-agent-acp`). grok is not installed here: its facts come from `emberloop-web/grokacp.go` and its tests and are unverified.

**claude.** `CLAUDE_CONFIG_DIR` (default `~/.claude`) holds the login (`.credentials.json`), `.claude.json` (the CLI creates it per dir), `projects/` (transcripts, needed by session/load), `sessions/`, `backups/`, and user `CLAUDE.md`, memory, settings, skills and plugins. The adapter already set `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1`, `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` and `settingSources: []`, so no CLAUDE.md or memory loaded before this change.
Done: each owner's claude Sessions get `CLAUDE_CONFIG_DIR=<stateDir>/enderbloom/users/<userId>/config/claude`. Only the host `.credentials.json` is linked in. Checked: `claude -p` (haiku, one call) with such a dir answered, and wrote `.claude.json`, `projects/` and `sessions/` only inside that dir.

**pig.** `PIG_CODING_AGENT_DIR` moves the agent dir (`PIG_HOME` moves the whole root). Pig loads by itself from the agent dir: `AGENTS.md`/`CLAUDE.md` (agent dir first, then the WorkDir chain), `SYSTEM.md`, `APPEND_SYSTEM.md`, `skills/`, `prompts/`, `themes/`, `extensions/`, and the `skills`, `prompts` and `extensions` path lists in `settings.json`. From HOME it loads `~/.agents/skills/`.
Before this change, pig-acp linked every host agent-dir entry into each Session, and the SDK backend read the host agent dir. Host instructions and skills reached every owner.
Done: pig Sessions (pig-acp and SDK) use `users/<userId>/config/pig`. That dir links only shared state: `auth.json`, `models.json`, `models-store.json`, `keybindings.json`, `trust.json`, `sessions/`, `extensions/`, `bin/`, `npm/`, `git/`. Its `settings.json` is the host file without `skills` and `prompts`. The pig-acp shim and the SDK session pass no skills and no prompt templates (`--no-skills --no-prompt-templates`; `ownerResources`). WorkDir-chain context files still load: they are project files.
Not isolated: `trust.json` links the host's project trust decisions into every owner dir (approvals, not instructions). `extensions/` stays host code.

**grok.** Each Session already gets its own HOME and GROK_HOME under `<stateDir>/enderbloom/grok-acp/<id>/`. Only `auth.json` is copied in. `config.toml` turns memory, subagents and `[compat.claude]` scans off. Nothing to isolate in grok itself.
Done: the Daemon's global context file for grok Sessions was the host pig agent dir. It now comes from the owner's pig dir, like claude and pig.

**Remaining gaps.** Context files in ancestors of the WorkDir (AGENTS.md or CLAUDE.md above the User home) still load for every owner. Claude Sessions created before this change keep their transcripts in the host config dir, so they should fail to resume (inferred from CLI code, not run live); pig Sessions keep resuming. No migration, per spec. Host `~/.pig/agent/AGENTS.md` no longer reaches any Session; copy it into the owner's pig dir to keep it for that User.
