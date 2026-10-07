# 01: Settings file in ~/.emberloop with role slots

**What to build:** `~/.emberloop/settings.json` is the user's config. It holds `decision_model`, `threshold`, and a roles map with `{harness, model, effort}` for `orchestrator`, `implementer`, `code-review`, `spec-review`, `scout`. If only the old state-dir settings file exists it is read; saving always writes the new file; unknown keys survive a save. `emberloop settings` prints every slot with its source (file or default) and sets them with `role <slot> harness|model|effort <value>`. Harness names are checked against the registry. Changing a slot's harness clears model and effort (full validation comes in 02).

**Blocked by:** None (can start immediately)

**Status:** done

- [x] New default path `~/.emberloop/settings.json`; `EMBERLOOP_SETTINGS` still overrides
- [x] Old file read as a fallback when the new one is absent; save writes the new path
- [x] Unknown keys preserved across load/save
- [x] `emberloop settings role <slot> harness|model|effort <value>` works; unknown slot or harness rejected
- [x] Print shows all slots, decision model and threshold with their source
- [x] Existing `decision-model`, `threshold`, `models` subcommands keep working
- [x] Tests against a temporary `EMBERLOOP_SETTINGS`, following the existing settings command tests
