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

const contextUsage = `usage: el context [--harness h] [--dir d] <session-id>

Print one session's context numbers: model, context tokens against the
window, compress threshold, outstanding tool calls and top tools.`

// runContext serves `el context`: the core slice verifiable without
// a header strip or a supervisor. dir overrides the session directory a
// harness that takes one reads from; without --harness every registered
// harness is tried in name order.
func runContext(argv []string, out io.Writer) error {
	fs := flag.NewFlagSet("context", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	harness := fs.String("harness", "", "pig or claude")
	dir := fs.String("dir", "", "session directory (harnesses that take one)")
	if err := fs.Parse(argv); err != nil {
		return fmt.Errorf("%v\n%s", err, contextUsage)
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("context needs a session id\n%s", contextUsage)
	}
	id := fs.Arg(0)
	names := el.HarnessNames()
	if *harness != "" {
		if el.Registered(*harness) == nil {
			return fmt.Errorf("unknown harness %q (have %s)\n%s", *harness, strings.Join(names, ", "), contextUsage)
		}
		names = []string{*harness}
	}
	cwd, _ := os.Getwd()
	settings := tools.EffectiveSettings(cwd)
	for _, name := range names {
		h := el.Registered(name)
		path := el.FindSession(h.SessionRoot(el.Session{Dir: *dir}), id)
		if path == "" {
			continue
		}
		return el.WriteContextReport(out, name, id, path, settings)
	}
	return fmt.Errorf("no transcript for session %q\n%s", id, contextUsage)
}
