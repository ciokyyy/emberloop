package wick

// Strip wiring for launched and top-level sessions (ticket 04): the source
// comes from the launch command's own environment, so spawned panes and the
// user's session share one path.

import (
	"os"
	"strings"

	"example.com/wick/tools"
)

// LaunchStripSource builds the strip's snapshot source from a launch
// command: the agent identity and harness in its environment, and pig's
// session dir in its arguments. The chip shows the herdr pane name for the
// agent type (PaneNameForType: implementer, spec-reviewer, code-reviewer),
// so herdr and the strip read one name source; an empty type renders as
// the orchestrator.
func LaunchStripSource(cmd Command) HeaderSource {
	env := map[string]string{}
	for _, e := range cmd.Env {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = v
		}
	}
	dir := ""
	for i, a := range cmd.Args {
		if a == "--session-dir" && i+1 < len(cmd.Args) {
			dir = cmd.Args[i+1]
		}
	}
	return HeaderSource{
		AgentID:    env["WICK_AGENT_ID"],
		AgentType:  env["WICK_AGENT_TYPE"],
		AgentName:  PaneNameForType(env["WICK_AGENT_TYPE"]),
		Harness:    env["WICK_HARNESS"],
		SessionID:  env["WICK_SESSION_ID"],
		SessionDir: dir,
	}
}

// WrapLaunch runs a launch command under the header strip: same binary,
// argv and env, on a terminal sized for the strip. Without a terminal, or
// with the strip disabled, the command runs plainly exactly as before.
func WrapLaunch(cmd Command) error {
	if HeaderStripDisabled() {
		return RunPlainCommand(cmd.Bin, cmd.Args, cmd.Env, os.Stdin, os.Stdout, os.Stderr)
	}
	cwd, _ := os.Getwd()
	return WrapChild(WrapConfig{
		Source:   LaunchStripSource(cmd),
		Settings: tools.EffectiveSettings(cwd),
		Bin:      cmd.Bin,
		Args:     cmd.Args,
		Env:      cmd.Env,
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
	})
}

// WrapTopLevel runs a top-level session command under the header strip:
// same binary, argv and env, on a terminal sized for the strip. Without a
// terminal, or with the strip disabled, the command runs plainly exactly
// as before.
func WrapTopLevel(cmd Command, src HeaderSource) error {
	if HeaderStripDisabled() {
		return RunPlainCommand(cmd.Bin, cmd.Args, cmd.Env, os.Stdin, os.Stdout, os.Stderr)
	}
	cwd, _ := os.Getwd()
	return WrapChild(WrapConfig{
		Source:   src,
		Settings: tools.EffectiveSettings(cwd),
		Bin:      cmd.Bin,
		Args:     cmd.Args,
		Env:      cmd.Env,
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
	})
}
