package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"example.com/wick"
	"example.com/wick/tools"
)

const contextUsage = `usage: wick-pig context [--harness h] [--dir d] <session-id>

Print one session's context numbers: model, context tokens against the
window, compress threshold, outstanding tool calls and top tools.`

// runContext serves `wick-pig context`: the core slice verifiable without
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
	names := wick.HarnessNames()
	if *harness != "" {
		if wick.Registered(*harness) == nil {
			return fmt.Errorf("unknown harness %q (have %s)\n%s", *harness, strings.Join(names, ", "), contextUsage)
		}
		names = []string{*harness}
	}
	cwd, _ := os.Getwd()
	settings := tools.EffectiveSettings(cwd)
	for _, name := range names {
		h := wick.Registered(name)
		path := wick.FindSession(h.SessionRoot(wick.Session{Dir: *dir}), id)
		if path == "" {
			continue
		}
		return wick.WriteContextReport(out, name, id, path, settings)
	}
	return fmt.Errorf("no transcript for session %q\n%s", id, contextUsage)
}
