// Command emberloop-web eval runs the ticket-30 offline evals: replay
// (recall@5 and the repeat-correction rate over a Project's history and period,
// with an optional embedder comparison on the same cases), deciders (precision
// at the approve threshold for heuristic, llm, jev and laya on a hand-labelled
// file) and sample (building the hand-labelling set). All three read canned
// histories and print tabular output, never touching live Sessions.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"example.com/emberloop-web/memory"

	web "example.com/emberloop-web"
)

const evalUsage = "usage: emberloop-web eval replay [-state-dir DIR] [-project ID] [-since TIME] [-until TIME] [-embedder-scores FILE] [-write-embedder-template FILE]\n" +
	"       emberloop-web eval deciders -labels FILE [-scores FILE] [-threshold 0.7] [-state-dir DIR -queue-user USER] [-write-scores FILE]\n" +
	"       emberloop-web eval sample [-state-dir DIR] [-project ID] [-since TIME] [-until TIME] [-n 250] -labels-out FILE"

// runEvalCommand runs `emberloop-web eval ...`. It returns the exit status.
func runEvalCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, evalUsage)
		return 2
	}
	switch args[0] {
	case "replay":
		return runEvalReplay(args[1:], stdout, stderr)
	case "deciders":
		return runEvalDeciders(args[1:], stdout, stderr)
	case "sample":
		return runEvalSample(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, evalUsage)
		return 2
	}
}

// parseEvalTime parses a period bound: RFC 3339 or a YYYY-MM-DD date
// (midnight UTC). Empty means unbounded.
func parseEvalTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("parse time %q: want RFC 3339 or YYYY-MM-DD", s)
}

// runEvalReplay prints recall@5 and the repeat-correction rate for a
// Project's history over a period. With -embedder-scores it re-ranks the same
// FTS5 top-20 per case and prints the embedder recall@5 and the gap.
// With -write-embedder-template it writes the scores-file skeleton (one empty
// map per replay case) for a remote-embedder run to fill in: embeddings are
// out of scope in v1, so the embedder stays external and the file is the seam.
func runEvalReplay(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("eval replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stateDir := fs.String("state-dir", defaultStateDir(), "pig state directory the Daemon uses")
	project := fs.String("project", "", "project ID to replay (empty replays every session)")
	sinceRaw := fs.String("since", "", "period start, RFC 3339 or YYYY-MM-DD (inclusive, empty is unbounded)")
	untilRaw := fs.String("until", "", "period end, RFC 3339 or YYYY-MM-DD (exclusive, empty is unbounded)")
	embedderPath := fs.String("embedder-scores", "", "JSON embedder scores file (Event key to rule text to score)")
	templatePath := fs.String("write-embedder-template", "", "write the embedder scores-file skeleton for the replay cases and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, evalUsage)
		return 2
	}
	since, err := parseEvalTime(*sinceRaw)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 2
	}
	until, err := parseEvalTime(*untilRaw)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 2
	}
	if !since.IsZero() && !until.IsZero() && !since.Before(until) {
		fmt.Fprintln(stderr, "emberloop-web: since must be before until")
		return 2
	}
	sessions, err := web.LoadReplaySessions(*stateDir, *project, since, until)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	bank, cleanup, err := web.NewReplayBank()
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	defer cleanup()
	newBank := func() memory.Memory { return bank }
	if *templatePath != "" {
		res, cases, err := web.ReplayEvaluate(sessions, newBank)
		if err != nil {
			fmt.Fprintln(stderr, "emberloop-web:", err)
			return 1
		}
		_ = res
		skeleton := map[string]map[string]float64{}
		for _, c := range cases {
			skeleton[c.SessionID+":"+c.EventID] = map[string]float64{}
		}
		raw, _ := json.MarshalIndent(skeleton, "", "  ")
		if err := os.WriteFile(*templatePath, append(raw, '\n'), 0o644); err != nil {
			fmt.Fprintln(stderr, "emberloop-web:", err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote embedder template for %d cases to %s\n", len(cases), *templatePath)
		return 0
	}
	if *embedderPath == "" {
		res, _, err := web.ReplayEvaluate(sessions, newBank)
		if err != nil {
			fmt.Fprintln(stderr, "emberloop-web:", err)
			return 1
		}
		fmt.Fprint(stdout, web.FormatReplayReport(res, len(sessions), 0, 0, 0, false))
		return 0
	}
	embedder, err := web.LoadEmbedderScores(*embedderPath)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	fts, emb, gap, res, err := web.ReplayCompare(sessions, newBank, embedder)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	fmt.Fprint(stdout, web.FormatReplayReport(res, len(sessions), fts, emb, gap, true))
	return 0
}

// runEvalDeciders scores each Decider on a labelling file at the approve
// threshold. Labels are JSONL (id, text, durable, optional context and
// kind): hand labels from -labels plus, with -queue-user, the decided rows
// of that User's queue (pending rows are unjudged and skipped). Scores are
// JSON mapping decider to label ID to durable probability; heuristic also
// runs live from the text, so it scores even without a scores file, while
// llm, jev and laya report unscored (n/a) for labels they never scored.
// With -write-scores the command writes the scores file back with the live
// heuristic scores merged in, so the file grows: run once for the heuristic
// baseline, then fill llm from model runs and jev/laya once ticket 21 lands
// their endpoint client, and re-run to compare.
func runEvalDeciders(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("eval deciders", flag.ContinueOnError)
	fs.SetOutput(stderr)
	labelsPath := fs.String("labels", "", "JSONL labelling file (required)")
	scoresPath := fs.String("scores", "", "JSON decider scores file")
	threshold := fs.Float64("threshold", 0.7, "approve threshold (confirmHigh)")
	stateDir := fs.String("state-dir", defaultStateDir(), "pig state directory the Daemon uses (for -queue-user)")
	queueUser := fs.String("queue-user", "", "also score the decided rows of this User's queue (user ID)")
	writeScores := fs.String("write-scores", "", "write the scores file with live heuristic scores merged in")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *labelsPath == "" {
		fmt.Fprintln(stderr, evalUsage)
		return 2
	}
	if *threshold < 0 || *threshold > 1 {
		fmt.Fprintln(stderr, "emberloop-web: threshold must be between 0 and 1")
		return 2
	}
	raw, err := os.ReadFile(*labelsPath)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	labels, err := memory.ParseDeciderLabels(raw)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	hand := len(labels)
	var queued []memory.DeciderLabel
	if *queueUser != "" {
		queued, err = web.QueueLabelsForUser(*stateDir, *queueUser)
		if err != nil {
			fmt.Fprintln(stderr, "emberloop-web:", err)
			return 1
		}
		seen := map[string]bool{}
		for _, l := range labels {
			seen[l.ID] = true
		}
		for _, l := range queued {
			if !seen[l.ID] {
				labels = append(labels, l)
				seen[l.ID] = true
			}
		}
	}
	byDecider := map[string]memory.DeciderScores{}
	if *scoresPath != "" {
		sraw, err := os.ReadFile(*scoresPath)
		if err != nil {
			fmt.Fprintln(stderr, "emberloop-web:", err)
			return 1
		}
		byDecider, err = memory.ParseDeciderScores(sraw)
		if err != nil {
			fmt.Fprintln(stderr, "emberloop-web:", err)
			return 1
		}
	}
	evals := memory.ScoreEvalDeciders(labels, byDecider, *threshold)
	fmt.Fprint(stdout, web.FormatDeciderReport(evals, *threshold, hand, len(queued)))
	if *writeScores != "" {
		live := memory.DeciderScores{}
		if cur, ok := byDecider["heuristic"]; ok {
			for id, p := range cur {
				live[id] = p
			}
		}
		for _, l := range labels {
			live[l.ID] = memory.HeuristicScore(l.Text)
		}
		byDecider["heuristic"] = live
		out, _ := json.MarshalIndent(byDecider, "", "  ")
		if err := os.WriteFile(*writeScores, append(out, '\n'), 0o644); err != nil {
			fmt.Fprintln(stderr, "emberloop-web:", err)
			return 1
		}
	}
	return 0
}

// runEvalSample builds the hand-labelling set: up to -n user turns from the
// period with heuristic hits oversampled first, written as JSONL for hand
// labelling. Each row carries the live heuristic hint, but the human sets
// durable: the hint oversamples, never labels. The file stays on the host.
func runEvalSample(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("eval sample", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stateDir := fs.String("state-dir", defaultStateDir(), "pig state directory the Daemon uses")
	project := fs.String("project", "", "project ID to sample (empty samples every session)")
	sinceRaw := fs.String("since", "", "period start, RFC 3339 or YYYY-MM-DD (inclusive, empty is unbounded)")
	untilRaw := fs.String("until", "", "period end, RFC 3339 or YYYY-MM-DD (exclusive, empty is unbounded)")
	n := fs.Int("n", 250, "turns to emit (heuristic hits first, then misses to fill)")
	labelsOut := fs.String("labels-out", "", "JSONL labelling file to write (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *labelsOut == "" {
		fmt.Fprintln(stderr, evalUsage)
		return 2
	}
	if *n < 1 {
		fmt.Fprintln(stderr, "emberloop-web: n must be at least 1")
		return 2
	}
	since, err := parseEvalTime(*sinceRaw)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 2
	}
	until, err := parseEvalTime(*untilRaw)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 2
	}
	if !since.IsZero() && !until.IsZero() && !since.Before(until) {
		fmt.Fprintln(stderr, "emberloop-web: since must be before until")
		return 2
	}
	sessions, err := web.LoadReplaySessions(*stateDir, *project, since, until)
	if err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	turns := web.SampleLabelCandidates(sessions, *n)
	var b strings.Builder
	for _, t := range turns {
		row, _ := json.Marshal(map[string]any{
			"id": t.ID, "text": t.Text, "context": t.Context,
			"durable": false, "heuristic": t.Heuristic,
		})
		b.Write(row)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(*labelsOut, []byte(b.String()), 0o644); err != nil {
		fmt.Fprintln(stderr, "emberloop-web:", err)
		return 1
	}
	hits := 0
	for _, t := range turns {
		if t.Heuristic > 0 {
			hits++
		}
	}
	fmt.Fprintf(stdout, "wrote %d turns (%d heuristic hits) to %s; set durable by hand\n", len(turns), hits, *labelsOut)
	return 0
}
