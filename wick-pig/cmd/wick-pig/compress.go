package main

import (
	"fmt"
	"io"
	"os"

	"example.com/wick"
	"example.com/wick/tools"
)

const compressUsage = `usage: wick-pig compress <pane>

Summarize the worker session in pane and restart it in the same pane with
the handoff as its first message. The handoff copy and the old transcript
are kept for audit and rollback. A failed summary leaves the old session
untouched.`

// runCompress serves `wick-pig compress`: the manual path over the
// supervisor restart, with the settings' compress model per harness.
func runCompress(argv []string, out io.Writer) error {
	if len(argv) != 1 {
		return fmt.Errorf("compress needs a pane\n%s", compressUsage)
	}
	cwd, _ := os.Getwd()
	next, err := wick.CompressPane(wick.OSExec, argv[0], tools.EffectiveSettings(cwd))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "compressed %s as %s\n", argv[0], next)
	return nil
}
