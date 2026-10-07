# Spec: one global settings file with a harness and model per role

Status: done (merged to master)

## Problem Statement

Every launch site picks its own harness and model. The orchestrator takes `--harness`/`--model` flags, workers take agent-type frontmatter or spawn arguments, reviewers inherit the worker's model and a harness from an environment variable, and scouts fall back to pig. There is no single place where I say "orchestrate with this, implement with that, review code with this other thing". The only settings file holds the find decision model and threshold, and it lives in a state directory I never look at, not beside my skills in `~/.emberloop/`.

## Solution

`~/.emberloop/settings.json` is the one user config. It holds a default `{harness, model, effort}` for each role slot: orchestrator, implementer, code-review, spec-review, scout, and the find decision model (model only), plus the find threshold. Every launch site resolves its harness, model and effort through one function with a fixed precedence: CLI flag, then agent-type frontmatter or spawn argument, then the settings file, then the built-in default. The code reviewer and the spec reviewer get their own slots instead of copying the worker's model. Each harness can list its models and the effort levels each model accepts, so the CLI and the TUI offer only valid choices. The old settings file is still read as a fallback for one version.

## User Stories

1. As a user, I want one settings file in `~/.emberloop/`, so that my config lives next to my skills and not in machine state.
2. As a user, I want to set a default harness and model for the orchestrator, so that `emberloop implement` uses them without flags.
3. As a user, I want to set a default harness and model for implementers, so that every worker the orchestrator spawns uses them.
4. As a user, I want a separate harness and model for code review, so that standards review can run on a cheaper or different model than the worker.
5. As a user, I want a separate harness and model for spec review, so that spec review can run on a stronger model than code review.
6. As a user, I want a default harness and model for scouts, so that fact-finding runs on a cheap fast model.
7. As a user, I want the find decision model and threshold in the same file, so that every setting is in one place.
8. As a user, I want a CLI flag to still win over the file, so that I can override a default for one run.
9. As a user, I want agent-type frontmatter and spawn arguments to still win over the file, so that a specialised agent type keeps its own choice.
10. As a user, I want an unset slot to fall back to the built-in default, so that an empty or missing file still works.
11. As a user with an existing settings file in the old location, I want it to keep working, so that upgrading loses nothing.
12. As a user, I want `emberloop settings` to show and set every slot, so that I can edit without the TUI.
13. As a user, I want an unknown harness name rejected when I set it, so that a typo fails at once and not at the next spawn.
14. As a user, I want the reviewers to keep the model I set even on a non-pig harness, so that a claude reviewer gets my model instead of none.
15. As the TUI, I want to load and save settings through the same functions as the CLI, so that both editors agree on the format.
16. As a user, I want to set a reasoning effort per slot, so that I can run reviews on high effort and scouts on low.
17. As a user, I want effort stored in the harness's own vocabulary (`xhigh` for claude and pig, `off`/`minimal` only for pig), so that nothing is lost in translation.
18. As a user, I want an effort the model doesn't support rejected on save, so that a bad combination fails early.
19. As the TUI, I want each harness to list its models with their supported effort levels, so that pickers only offer valid choices.
20. As a user, I want the model list cached and refreshable, so that pickers open instantly but stay current.
21. As a user, I want models listed without effort baked into their id (an id like `some-model-high` shown as `some-model` with effort `high`), so that model and effort are always chosen separately.

## Implementation Decisions

- The existing settings type grows a roles map keyed by slot name (`orchestrator`, `implementer`, `code-review`, `spec-review`, `scout`), each `{harness, model, effort}`; `decision_model` and `threshold` keep their keys. Unknown keys are preserved on save so the file can grow (later specs add more).
- The settings path moves to `~/.emberloop/settings.json`; the `EMBERLOOP_SETTINGS` override stays. If the new file is absent, the old state-dir file is read; saving always writes the new file.
- One resolver: `Resolve(slot, flags, typeOrSpawn) → {harness, model, effort}`, applied per field with precedence flag > type/spawn arg > settings > built-in. Every launch site calls it: top-level session, worker spawn, scout spawn, both reviewers. Agent-type frontmatter and spawn entries gain an `effort` field; the CLI gains `--effort`.
- Effort is stored in the harness's own words; there is no cross-harness scale. Each harness turns it into its own flag: pig `--thinking <level>`, claude `--effort <level>`.
- The harness interface gains `Models() → [{id, label, efforts[]}]`, the single source for pickers and validation:
  - pig: parses `pig --list-models`; models with thinking get pig's levels (off, minimal, low, medium, high, xhigh, max), others none.
  - claude: sends the `initialize` control request over `claude -p --input-format stream-json --output-format stream-json` with no prompt (no tokens spent) and reads `models[]`: `value`, `displayName`, `supportedEffortLevels`. Verified against Claude Code 2.1.289.
- Model lists are cached per harness in the state directory with a timestamp; `emberloop settings models [--refresh]` and the TUI's refresh key rebuild them. Validation on save uses the cache, refreshing once if a value is missing from it.
- Reviewers stop copying the worker's model; they resolve their own slot. The rule that drops the model for a non-pig reviewer harness goes: a model set for that slot is meant for that harness.
- The `EMBERLOOP_READONLY_HARNESS` environment variable is kept as a high-precedence override for compatibility (treated like a flag).
- `emberloop settings` gains `role <slot> harness <name>`, `role <slot> model <id>` and `role <slot> effort <level>`, and printing shows every slot with where each value came from (file or default). Changing a slot's harness keeps model and effort if the new harness supports them, otherwise clears them.
- Harness names are validated against the harness registry on save.

## Testing Decisions

- Good tests drive the public surface: the resolver and the `emberloop settings` command against a temporary `EMBERLOOP_SETTINGS` file, asserting resolved values and file contents, not internals.
- Resolver table test covering each precedence layer per slot, including empty file and missing file.
- Fallback test: only the old file exists, values are read; a save writes the new file.
- `Models()`: pig parsed from captured `--list-models` output; claude parsed from a captured `initialize` control response, run through a fake `Exec`; validation rejects an unsupported effort.
- Launch-site tests reuse the existing fake-herdr and `BuildCommand` tests to assert a spawned worker, scout and each reviewer gets the resolved harness and model.
- Prior art: the existing settings command tests (temp `EMBERLOOP_SETTINGS`), and the spawn and review tests with fake herdr.

## Out of Scope

- The TUI settings tab (spec: emberloop TUI).
- A price table; prices are not computed in v1.
- Web search tool: needs its own grill session before any spec.

## Further Notes

Spec 1 of 5 from the TUI grill: settings, harnesses, run records and archive, TUI, stats. Can run in parallel with the harnesses spec.
