package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"example.com/emberloop"
	"example.com/emberloop/tools"
)

const wrapUsage = `usage: emberloop wrap [--agent id] [--type t] [--name n] --harness h [--session id] [--dir d] [--compress-count n] -- <bin> [args...]

Run a command under the header strip: the child gets a terminal sized to
the pane minus the strip rows, and the strip shows its context use. Flag
values win; the EMBERLOOP_AGENT_ID, EMBERLOOP_AGENT_TYPE, EMBERLOOP_SESSION_ID and
EMBERLOOP_HARNESS environment carries the rest.`

// runWrap serves `emberloop wrap`: flags and environment become the strip's
// snapshot source, the rest becomes the child. Effective settings come from
// where the command runs. EMBERLOOP_HEADER_STRIP=0 runs the child plainly.
func runWrap(argv []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("wrap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	agent := fs.String("agent", "", "stable agent id for the strip hue and cost")
	agentType := fs.String("type", "", "agent type for the strip chip")
	name := fs.String("name", "", "strip chip label (default: the type)")
	harness := fs.String("harness", "", "pig or claude")
	session := fs.String("session", "", "harness session id for the transcript")
	dir := fs.String("dir", "", "session directory (harnesses that take one)")
	compressCount := fs.Int("compress-count", 0, "completed compressions")
	inputRow := fs.Bool("input-row", false, "draw emberloop input row above strip")
	if err := fs.Parse(argv); err != nil {
		return fmt.Errorf("%v\n%s", err, wrapUsage)
	}
	if *harness == "" {
		*harness = os.Getenv("EMBERLOOP_HARNESS")
	}
	if emberloop.Registered(*harness) == nil {
		return fmt.Errorf("unknown harness %q (have %s)\n%s", *harness, strings.Join(emberloop.HarnessNames(), ", "), wrapUsage)
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("wrap needs a command\n%s", wrapUsage)
	}
	if *agent == "" {
		*agent = os.Getenv("EMBERLOOP_AGENT_ID")
	}
	if *agentType == "" {
		*agentType = os.Getenv("EMBERLOOP_AGENT_TYPE")
	}
	if *name == "" {
		*name = *agentType
	}
	if *session == "" {
		*session = os.Getenv("EMBERLOOP_SESSION_ID")
	}
	cwd, _ := os.Getwd()
	settings := tools.EffectiveSettings(cwd)
	if emberloop.HeaderStripDisabled() {
		return emberloop.RunPlainCommand(fs.Arg(0), fs.Args()[1:], nil, stdin, stdout, os.Stderr)
	}
	return emberloop.WrapChild(wrapCmdConfig(*agent, *agentType, *name, *harness, *session, *dir, *compressCount, *inputRow || os.Getenv("EMBERLOOP_INPUT_ROW") == "1", settings, stdin, stdout, fs.Arg(0), fs.Args()[1:]))
}

// wrapCmdConfig builds the wrapper config for `emberloop wrap`: the strip's
// snapshot source plus the child, with the input queue's control channel
// served, so tell and the shared delivery path reach the child through the
// queue instead of herdr send-text.
func wrapCmdConfig(agent, agentType, name, harness, session, dir string, compressCount int, inputRow bool, settings tools.Settings, stdin io.Reader, stdout io.Writer, bin string, args []string) emberloop.WrapConfig {
	return emberloop.WrapConfig{
		Source: emberloop.HeaderSource{
			AgentID: agent, AgentType: agentType, AgentName: name,
			Harness: harness, SessionID: session, SessionDir: dir,
			CompressCount: compressCount,
		},
		Settings:  settings,
		Bin:       bin,
		Args:      args,
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    os.Stderr,
		ServeCtrl: true,
		InputRow:  inputRow,
	}
}
