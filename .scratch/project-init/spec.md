# Project init and validate

## Problem Statement

Spec and ticket paths are hardcoded: the Runs view scans for `spec.md`, and the Driver expects an `issues/` folder beside it. Skills and tools cannot tell where a project keeps its specs. A malformed spec or ticket is found only when a Run starts, so a bad `Blocked by` line costs a failed Run. A project with a different layout has no way to say so.

## Solution

Each project declares its layout once in `.emberloop/settings.json`: `specs` (the specs directory), `tickets` (the ticket subfolder name) and `pipeline` (false silences all of this). `el init --specs .scratch --tickets issues` validates the choice and writes the file. The project file overrides the global `~/.emberloop/settings.json`. One shared ticket parser package serves the Driver and the Daemon. Spec and ticket files are validated when an agent writes them, and again at the end of a turn. The skills (`to-spec`, `to-tickets`, `implement`, `wayfinder`) read the settings instead of hardcoding paths.

## User Stories

1. As a user, I want `el init` to write the project settings, so that the layout is declared once.
2. As a user, I want `el init` to refuse a missing or non-directory `--specs` path and a `--tickets` value that is not a plain folder name, so that a typo never lands in the file.
3. As a user, I want `el init` to keep the other keys already in `.emberloop/settings.json`, so that `orchestrator_paths` survives.
4. As a user, I want the project settings to override the global ones, so that one project can differ.
5. As a user, I want `"pipeline": false` to silence validation and the init message, so that a project that does not use specs is left alone.
6. As a user, I want the Driver and the Daemon to parse tickets with the same code, so that the Runs preview and a real Run never disagree.
7. As a user, I want a warning on the tool result when an agent writes a bad spec or ticket, so that the agent fixes it at once.
8. As a user, I want the warning to name the file and line, so that the fix is quick.
9. As a user, I want a turn that edited spec files to be re-validated at its end, with one re-prompt carrying the errors, so that an agent gets one chance to fix them.
10. As a user, I want a Session to need me when the errors remain after the re-prompt, so that nothing loops forever.
11. As a user, I want cycles, unknown blockers and duplicate ticket numbers reported across files, so that a broken graph never starts.
12. As a user, I want a tool that needs spec or ticket paths to say "project not initiated: ask the user, then run `el init`" once per Session when the settings are missing, so that the agent asks me.
13. As a user, I want those tools to read the settings fresh on every call, so that they react the moment the file exists.
14. As a user, I want the skills to read `specs` and `tickets` from the settings, so that they write to the right place.
15. As a user, I want the Runs view to list specs from the `specs` directory when it is set, so that discovery matches the declared layout.

## Implementation Decisions

- Settings keys: `specs` (path relative to the project root), `tickets` (folder name, default `issues`), `pipeline` (bool, default true). Project file wins over the global file per key. Unknown keys round-trip.
- Settings are read fresh on every call. No cache.
- The shared ticket parser package is extracted from `emberloop/driver.go` (`readDriverTickets`, `driverBlockedValue`, `parseBlockerRefs`, `resolveDriverRefs`). The Driver keeps its public behaviour and error strings; the Daemon preview switches to the package.
- `ValidateSpec` checks one file: a ticket needs a number in its file name, one `Blocked by` line that parses, and a status line. A spec needs a title. Output is a list of problems, each with file, line and text.
- Cross-file checks run on the tickets directory: duplicate numbers, unknown blockers, cycles.
- The write and edit tools append validation warnings to their result for paths under the specs directory. The warning never blocks the write.
- The turn-end gate re-validates the spec files edited in the turn. Errors trigger one re-prompt with the list. Errors after the re-prompt move the Session to needs-you.
- Tools that need the paths return the "project not initiated" message once per Session, then stay quiet.
- `el init` is the only writer of the three keys.

## Testing Decisions

- Test through external behaviour: the CLI command, the tool results, the Daemon API.
- The parser package keeps the Driver's existing table tests, moved with the code.
- `el init` tests use a temp project directory and check the file bytes.
- Gate tests use the fake Backend through the Daemon API.
- The repo check (`.emberloop/check`) must pass at the end.

## Out of Scope

- A UI form for init. The agent runs `el init`.
- Migrating archived specs under `.scratch/archive/`.
- Changing ticket status handling or the Run lifecycle.
- Shiki, ANSI stripping and other tool-output polish.

## Further Notes

- Source plan: `.scratch/plans/web-overhaul.md` item 3.
- Known failing test `TestDoxTreeIndexIsComplete` is fixed in ticket 05.
