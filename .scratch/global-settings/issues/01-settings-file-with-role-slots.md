# 01: Settings file in ~/.wick-pig with role slots

**What to build:** `~/.wick-pig/settings.json` is the user's config. It holds `decision_model`, `threshold`, and a roles map with `{harness, model, effort}` for `orchestrator`, `implementer`, `code-review`, `spec-review`, `scout`. If only the old state-dir settings file exists it is read; saving always writes the new file; unknown keys survive a save. `wick-pig settings` prints every slot with its source (file or default) and sets them with `role <slot> harness|model|effort <value>`. Harness names are checked against the registry. Changing a slot's harness clears model and effort (full validation comes in 02).

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] New default path `~/.wick-pig/settings.json`; `WICK_SETTINGS` still overrides
- [ ] Old file read as a fallback when the new one is absent; save writes the new path
- [ ] Unknown keys preserved across load/save
- [ ] `wick-pig settings role <slot> harness|model|effort <value>` works; unknown slot or harness rejected
- [ ] Print shows all slots, decision model and threshold with their source
- [ ] Existing `decision-model`, `threshold`, `models` subcommands keep working
- [ ] Tests against a temporary `WICK_SETTINGS`, following the existing settings command tests
