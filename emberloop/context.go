package el

import (
	"bufio"
	"fmt"
	"io"
)

// ContextSnapshot is what the context core reports for one session's
// transcript: the current context size under the shared formula, the
// model of the latest request, how many tool calls still wait for a
// result, how many finished (one per result: the auto-compress cooldown's
// input), and result sizes per tool name with the top three split out.
// Sizes are bytes of result text.
type ContextSnapshot struct {
	Model          string
	Input          int64
	Output         int64
	CacheRead      int64
	CacheWrite     int64
	ContextTokens  int64
	Outstanding    int
	CompletedCalls int
	ToolBytes      map[string]int64
	TopTools       []ToolSize
	Provider       string   // pig only: provider of the latest request
	Running        ToolCall // the newest call still waiting for its result
	Last           ToolCall // the newest call that has a result
}

// ToolCall is one tool call as the strip shows it: the tool, its key
// argument, and for a finished call the bytes of its result.
type ToolCall struct {
	Name  string
	Arg   string
	Bytes int64
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
	pending := map[string]ToolCall{}
	var order []string // pending ids, oldest first
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		events := ctxParser.ParseLine(line)
		if events.Usage != nil {
			u := events.Usage
			snap.Model = u.Model
			if u.Provider != "" {
				snap.Provider = u.Provider
			}
			snap.Input, snap.Output = u.Input, u.Output
			snap.CacheRead, snap.CacheWrite = u.CacheRead, u.CacheWrite
			snap.ContextTokens = ContextTokensOf(u.Input, u.Output, u.CacheRead, u.CacheWrite)
		}
		for _, c := range events.Calls {
			if c.ID == "" {
				continue
			}
			pending[c.ID] = ToolCall{Name: c.Name, Arg: c.Arg}
			order = append(order, c.ID)
		}
		for _, res := range events.Results {
			call, known := pending[res.ID]
			name := call.Name
			if !known {
				name = res.Name
			} else {
				delete(pending, res.ID)
			}
			if res.ID == "" {
				continue
			}
			snap.CompletedCalls++
			if name == "" {
				continue
			}
			snap.ToolBytes[name] += res.Bytes
			snap.Last = ToolCall{Name: name, Arg: call.Arg, Bytes: res.Bytes}
		}
	}
	if err := sc.Err(); err != nil {
		return ContextSnapshot{}, err
	}
	snap.Outstanding = len(pending)
	for i := len(order) - 1; i >= 0; i-- {
		if c, ok := pending[order[i]]; ok {
			snap.Running = c
			break
		}
	}
	snap.TopTools = TopTools(snap.ToolBytes, 3)
	return snap, nil
}
