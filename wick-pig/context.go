package wick

import (
	"bufio"
	"fmt"
	"io"
)

// ContextSnapshot is what the context core reports for one session's
// transcript: the current context size under the shared formula, the
// model of the latest request, how many tool calls still wait for a
// result, and result sizes per tool name with the top three split out.
// Sizes are bytes of result text.
type ContextSnapshot struct {
	Model         string
	Input         int64
	Output        int64
	CacheRead     int64
	CacheWrite    int64
	ContextTokens int64
	Outstanding   int
	ToolBytes     map[string]int64
	TopTools      []ToolSize
}

// ToolSize is one tool's total result bytes.
type ToolSize struct {
	Name  string
	Bytes int64
}

// ContextTokensOf is the one shared formula every harness feeds: the
// latest request's input plus cache read plus cache write plus that
// message's output. Never a cumulative sum.
func ContextTokensOf(input, output, cacheRead, cacheWrite int64) int64 {
	return input + output + cacheRead + cacheWrite
}

// SummarizeContext folds a transcript into a snapshot. The harness picks
// one context parser; a new harness plugs in by registering one.
func SummarizeContext(harness string, r io.Reader) (ContextSnapshot, error) {
	ctxParser, ok := ContextParserFor(harness)
	if !ok {
		return ContextSnapshot{}, fmt.Errorf("no context parser for harness %q", harness)
	}
	snap := ContextSnapshot{ToolBytes: map[string]int64{}}
	pending := map[string]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		events := ctxParser.ParseLine(line)
		if events.Usage != nil {
			u := events.Usage
			snap.Model = u.Model
			snap.Input, snap.Output = u.Input, u.Output
			snap.CacheRead, snap.CacheWrite = u.CacheRead, u.CacheWrite
			snap.ContextTokens = ContextTokensOf(u.Input, u.Output, u.CacheRead, u.CacheWrite)
		}
		for _, c := range events.Calls {
			if c.ID == "" {
				continue
			}
			pending[c.ID] = c.Name
		}
		for _, res := range events.Results {
			name, known := pending[res.ID]
			if !known {
				name = res.Name
			} else {
				delete(pending, res.ID)
			}
			if res.ID == "" || name == "" {
				continue
			}
			snap.ToolBytes[name] += res.Bytes
		}
	}
	if err := sc.Err(); err != nil {
		return ContextSnapshot{}, err
	}
	snap.Outstanding = len(pending)
	snap.TopTools = TopTools(snap.ToolBytes, 3)
	return snap, nil
}
