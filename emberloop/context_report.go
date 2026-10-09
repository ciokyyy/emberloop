package el

import (
	"fmt"
	"io"
	"os"
	"strings"

	"example.com/emberloop/tools"
)

// WriteContextReport prints one session's context numbers: the model, the
// context size against the window with the percent, the compress
// threshold, the outstanding tool calls and the top tools by result size.
// It is the `el context` command's output, so the slice is
// verifiable without a header strip or a supervisor.
func WriteContextReport(out io.Writer, harness, sessionID, transcriptPath string, s tools.Settings) error {
	if Registered(harness) == nil {
		return fmt.Errorf("unknown harness %q (have %s)", harness, strings.Join(HarnessNames(), ", "))
	}
	f, err := os.Open(transcriptPath)
	if err != nil {
		return fmt.Errorf("open transcript for %q: %w", sessionID, err)
	}
	defer f.Close()
	snap, err := SummarizeContext(harness, f)
	if err != nil {
		return err
	}
	window := SnapshotWindow(snap, s)
	threshold := s.EffectiveCompressThreshold(harness)
	percent := int64(0)
	if window > 0 {
		percent = snap.ContextTokens * 100 / window
	}
	model := snap.Model
	if model == "" {
		model = "-"
	}
	fmt.Fprintf(out, "session %s (%s)\n", sessionID, harness)
	fmt.Fprintf(out, "model: %s\n", model)
	fmt.Fprintf(out, "context: %d tokens (%d%% of %d window)\n", snap.ContextTokens, percent, window)
	fmt.Fprintf(out, "threshold: %d tokens\n", threshold)
	fmt.Fprintf(out, "outstanding tool calls: %d\n", snap.Outstanding)
	if len(snap.TopTools) == 0 {
		fmt.Fprintln(out, "top tools: (none)")
		return nil
	}
	names := make([]string, 0, len(snap.TopTools))
	for _, ts := range snap.TopTools {
		names = append(names, fmt.Sprintf("%s %d", ts.Name, ts.Bytes))
	}
	fmt.Fprintf(out, "top tools: %s\n", strings.Join(names, ", "))
	return nil
}
