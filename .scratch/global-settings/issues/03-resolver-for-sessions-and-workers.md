# 03: Resolver used by top-level sessions and workers, with effort flags

**What to build:** One resolver gives `{harness, model, effort}` for a slot with precedence CLI flag > agent-type frontmatter or spawn argument > settings > built-in default, per field. `wick-pig` and `wick-pig implement` use it (orchestrator slot for implement); `--effort` is a new CLI flag. Spawned workers use it (implementer slot); agent-type frontmatter and spawn entries accept `effort`. Each harness turns effort into its own flag: pig `--thinking`, claude `--effort`.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] Resolver table test covering every precedence layer for each field, empty and missing file
- [ ] `wick-pig implement` with no flags uses the orchestrator slot
- [ ] Worker spawn uses the implementer slot unless type or spawn args say otherwise
- [ ] `effort` accepted in frontmatter and spawn entries; `--effort` on the CLI
- [ ] pig command gets `--thinking <level>`, claude gets `--effort <level>` (asserted via `BuildCommand` tests)
- [ ] Worker record stores the effort
