package main

import (
	"fmt"
	"io"
	"os"

	"example.com/emberloop"
	"example.com/emberloop/tools"
)

const compressUsage = `usage: emberloop compress [--force] <pane>

Summarize the worker session in pane and restart it in the same pane with
the handoff as its first message. The handoff copy and the old transcript
are kept for audit and rollback. A failed summary leaves the old session
untouched. A press during a running tool call queues to the next boundary
and the header shows it; --force compresses past the running call.`

const requestCompressUsage = `usage: emberloop request-compress <pane>

The herdr compress key's signal for the focused pane, whatever the harness.
At a tool boundary it compresses at once. During a running tool call it
queues to the boundary and the header shows the queue; a second press while
the queue stands forces the compress now.`

// runCompress serves `emberloop compress`: the manual path over the
// supervisor restart, with the settings' compress model per harness.
func runCompress(argv []string, out io.Writer) error {
	force := false
	var panes []string
	for _, a := range argv {
		if a == "--force" || a == "-force" || a == "-f" {
			force = true
			continue
		}
		panes = append(panes, a)
	}
	if len(panes) != 1 {
		return fmt.Errorf("compress needs a pane\n%s", compressUsage)
	}
	cwd, _ := os.Getwd()
	next, err := emberloop.CompressPaneWithForce(emberloop.OSExec, panes[0], tools.EffectiveSettings(cwd), force)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "compressed %s as %s\n", panes[0], next)
	return nil
}

// runRequestCompress serves `emberloop request-compress`: the herdr plugin
// action's signal path over the same restart.
func runRequestCompress(argv []string, out io.Writer) error {
	if len(argv) != 1 {
		return fmt.Errorf("request-compress needs a pane\n%s", requestCompressUsage)
	}
	cwd, _ := os.Getwd()
	next, err := emberloop.RequestCompress(emberloop.OSExec, argv[0], tools.EffectiveSettings(cwd))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "compressed %s as %s\n", argv[0], next)
	return nil
}
