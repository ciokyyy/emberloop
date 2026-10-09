# 03: Resolver used by top-level sessions and workers, with effort flags

**What to build:** One resolver gives `{harness, model, effort}` for a slot with precedence CLI flag > agent-type frontmatter or spawn argument > settings > built-in default, per field. `el` and `el implement` use it (orchestrator slot for implement); `--effort` is a new CLI flag. Spawned workers use it (implementer slot); agent-type frontmatter and spawn entries accept `effort`. Each harness turns effort into its own flag: pig `--thinking`, claude `--effort`.

**Blocked by:** 01

**Status:** done

- [x] Resolver table test covering every precedence layer for each field, empty and missing file
- [x] `el implement` with no flags uses the orchestrator slot
- [x] Worker spawn uses the implementer slot unless type or spawn args say otherwise
- [x] `effort` accepted in frontmatter and spawn entries; `--effort` on the CLI
- [x] pig command gets `--thinking <level>`, claude gets `--effort <level>` (asserted via `BuildCommand` tests)
- [x] Worker record stores the effort
