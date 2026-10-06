# AGENTS.md — herdr-wick (herdr plugin)

## Purpose

The herdr plugin wick ships and links at install: one action,
`wick.compress`, that signals the supervisor for the focused pane. The key
binding sends the focused pane ID through the action context; `compress.sh`
execs `wick-pig request-compress` for it, whatever the harness. No slash
commands, no header logic here: queueing, the `compress queued` strip state
and the forced second press all live in `wick-pig` behind the exec seam.

## Ownership

Owns `herdr-plugin.toml` (the manifest: one compress action) and
`compress.sh` (the focused-pane signal). `wick-pig install` links this
directory (`herdr plugin link`) and its usage text carries the keybinding
line the user adds.

## Verification

 No check; the repo check covers `wick-pig/` and `wick-tui/`. The action's
 signal path is tested from `wick-pig/` (`plugin_signal_test.go` runs the
 shipped script with a stub binary).

## Child DOX Index

No child docs.
