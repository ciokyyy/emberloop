package el

// Strip wiring for launched and top-level sessions (ticket 04): the source
// comes from the launch command's own environment, so spawned panes and the
// user's session share one path.

import (
	"io"
	"os"
	"strings"

	"example.com/emberloop/tools"
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
		AgentID:    env["EL_AGENT_ID"],
		AgentType:  env["EL_AGENT_TYPE"],
		AgentName:  PaneNameForType(env["EL_AGENT_TYPE"]),
		Harness:    env["EL_HARNESS"],
		SessionID:  env["EL_SESSION_ID"],
		SessionDir: dir,
	}
}

// wrapConfigFor runs a launch command under the header strip with the input
// queue's control channel served: same binary, argv and env, on a terminal
// sized for the strip. The channel is what lets tell and the shared delivery
// path reach the child through the queue instead of herdr send-text.
func wrapConfigFor(cmd Command, src HeaderSource, s tools.Settings) WrapConfig {
	return WrapConfig{
		Source: src, Settings: s,
		Bin: cmd.Bin, Args: cmd.Args, Env: cmd.Env,
		Stdin: os.Stdin, Stdout: os.Stdout,
		ServeCtrl: true,
	}
}

// WrapLaunch runs a launch command under the header strip: same binary,
// argv and env, on a terminal sized for the strip, with the child's stderr
// merged into the terminal. Without a terminal, or with the strip disabled,
// the command runs plainly exactly as before.
func WrapLaunch(cmd Command) error { return WrapLaunchWithChildStderr(cmd, nil) }

// WrapLaunchWithChildStderr runs a launch command under the header strip
// with the child's stderr on its own pipe: stdin and stdout stay on the
// PTY, so the strip is drawn, while stderr streams to childStderr (the
// pig tools log). A nil childStderr merges stderr into the PTY, as
// WrapLaunch does. Without a terminal, or with the strip disabled, the
// command runs plainly with childStderr as its error sink. Stderr stays
// unset, so wrapper warnings never land in the child's log.
func WrapLaunchWithChildStderr(cmd Command, childStderr io.Writer) error {
	if HeaderStripDisabled() {
		stderr := childStderr
		if stderr == nil {
			stderr = os.Stderr
		}
		return RunPlainCommand(cmd.Bin, cmd.Args, cmd.Env, os.Stdin, os.Stdout, stderr)
	}
	cwd, _ := os.Getwd()
	cfg := wrapConfigFor(cmd, LaunchStripSource(cmd), tools.EffectiveSettings(cwd))
	cfg.ChildStderr = childStderr
	return WrapChild(cfg)
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
	cfg := wrapConfigFor(cmd, src, tools.EffectiveSettings(cwd))
	cfg.InputRow = false // input row disabled; code kept (archive/input-row)
	return WrapChild(cfg)
}
