package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Prune stage of the Handoff writer. An Event log over pruneMinChars is first
// pruned by Relace Compact (a token-level pruning model, not a summarizer): it
// keeps the original wording of what matters and drops the rest in seconds.
// The pruned trace then goes through the usual path (fold when it is still too
// big, then the summarizer writes the Handoff sections). Any failure of the
// prune keeps the unpruned blocks: a prune never blocks a compress.

const (
	// pruneTargetTokens is the retained-context budget sent to Compact.
	pruneTargetTokens = 32000
	// pruneMinChars is the rendered log size below which pruning is skipped:
	// the summarizer reads that much directly.
	pruneMinChars = 160000
	// pruneTimeout bounds one Compact call; it normally takes seconds.
	pruneTimeout = 90 * time.Second
	// relaceProvider is the pig auth entry that holds the Relace key.
	relaceProvider = "relace"
	// defaultRelaceURL is the Relace model API host.
	defaultRelaceURL = "https://models.relace.ai"
	// pruneAgentModel is the agent_model hint Compact uses for tokenizing.
	pruneAgentModel = "gpt-5.5"
)

// errPruneOff is the failure of a prune that is turned off or has no key.
var errPruneOff = errors.New("prune is off or has no Relace key")

// tracePruner is the optional summarizer seam that shrinks an Event log
// before it is read. It returns the pruned trace as summary blocks.
type tracePruner interface {
	Prune(events []Event, targetTokens int) ([]summaryBlock, error)
}

// Prune forwards to the wrapped summarizer when it prunes and the setting
// allows it.
func (r *retryingSummarizer) Prune(events []Event, targetTokens int) ([]summaryBlock, error) {
	p, ok := r.next.(tracePruner)
	if !ok || r.noPrune {
		return nil, errPruneOff
	}
	return p.Prune(events, targetTokens)
}

// Prune is the production prune: Relace Compact over the Event log.
func (pigSummarizer) Prune(events []Event, targetTokens int) ([]summaryBlock, error) {
	return relacePruner{}.Prune(events, targetTokens)
}

// totalChars is the rendered size of blocks.
func totalChars(blocks []summaryBlock) int {
	n := 0
	for _, b := range blocks {
		n += b.size()
	}
	return n
}

// pruneBlocks returns the pruned blocks when sum prunes, the log is over
// pruneMinChars and the prune made it smaller. Otherwise it returns blocks as
// they are. A failure is noted, except a prune that is off or has no key.
func pruneBlocks(sum handoffSummarizer, events []Event, blocks []summaryBlock, note func(string)) []summaryBlock {
	p, ok := sum.(tracePruner)
	if !ok {
		return blocks
	}
	before := totalChars(blocks)
	if before <= pruneMinChars {
		return blocks
	}
	pruned, err := p.Prune(events, pruneTargetTokens)
	if err != nil {
		if !errors.Is(err, errPruneOff) {
			note("prune: Relace Compact failed, using the full log: " + err.Error())
		}
		return blocks
	}
	after := totalChars(pruned)
	if len(pruned) == 0 || after >= before {
		note(fmt.Sprintf("prune: Relace Compact did not shrink the log (%d to %d chars), using the full log", before, after))
		return blocks
	}
	note(fmt.Sprintf("prune: Relace Compact cut the log from %d to %d chars", before, after))
	return pruned
}

// relacePruner calls the Relace Compact endpoint.
type relacePruner struct {
	agentDir string // pig agent dir for the key; empty is the user's own
	baseURL  string // host override for tests; empty is EL_RELACE_URL or the default
	client   *http.Client
}

func (p relacePruner) host() string {
	if p.baseURL != "" {
		return strings.TrimRight(p.baseURL, "/")
	}
	if u := strings.TrimSpace(os.Getenv("EL_RELACE_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultRelaceURL
}

// key is the Relace API key: the environment first, then the pig auth entry.
func (p relacePruner) key() string {
	if k := strings.TrimSpace(os.Getenv("RELACE_API_KEY")); k != "" {
		return k
	}
	cred, err := pigCredentials(p.agentDir).Read(context.Background(), relaceProvider)
	if err != nil || cred == nil {
		return ""
	}
	return strings.TrimSpace(cred.Key)
}

// Prune sends the Event log to Relace Compact and returns the pruned trace as
// summary blocks. With no key it returns errPruneOff.
func (p relacePruner) Prune(events []Event, targetTokens int) ([]summaryBlock, error) {
	key := p.key()
	if key == "" {
		return nil, errPruneOff
	}
	trace := eventsToTrace(events)
	if len(trace) == 0 {
		return nil, errors.New("no trace to prune")
	}
	body, err := json.Marshal(map[string]any{
		"messages":      trace,
		"target_tokens": targetTokens,
		"agent_model":   pruneAgentModel,
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pruneTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.host()+"/v1/code/compact", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := p.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("relace compact: HTTP %d: %s", resp.StatusCode, capText(string(raw), 200))
	}
	var out struct {
		Messages []traceMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("relace compact: bad reply: %w", err)
	}
	return traceBlocks(out.Messages), nil
}

// traceMessage is one chat-completions message of the trace.
type traceMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []traceToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type traceToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// text is the message content as a string: a JSON string, or the text parts
// of a content array.
func (m traceMessage) text() string {
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(m.Content, &parts) == nil {
		var b strings.Builder
		for _, part := range parts {
			b.WriteString(part.Text)
		}
		return b.String()
	}
	return ""
}

func traceString(s string) json.RawMessage {
	raw, _ := json.Marshal(s)
	return raw
}

// eventsToTrace renders the Event log as a chat-completions trace in log
// order: prompts as user messages, model text as assistant messages and each
// tool call (last state) as an assistant tool call followed by its result.
// Thinking, status and usage Events are left out.
func eventsToTrace(events []Event) []traceMessage {
	var trace []traceMessage
	var reply strings.Builder
	toolAt := map[string]int{}
	flush := func() {
		if text := strings.TrimSpace(reply.String()); text != "" {
			trace = append(trace, traceMessage{Role: "assistant", Content: traceString(dropMemoryBlocks(text))})
		}
		reply.Reset()
	}
	toolPair := func(tc ToolCall) []traceMessage {
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", len(trace))
		}
		args := tc.Input
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		out := dropMemoryBlocks(tc.Output)
		if tc.Status == "failed" {
			out = "[failed] " + out
		}
		call := traceToolCall{ID: id, Type: "function"}
		call.Function.Name = tc.Name
		call.Function.Arguments = dropMemoryBlocks(args)
		return []traceMessage{
			{Role: "assistant", Content: traceString(""), ToolCalls: []traceToolCall{call}},
			{Role: "tool", ToolCallID: id, Content: traceString(out)},
		}
	}
	for _, e := range events {
		switch e.Kind {
		case "assistant_chunk":
			reply.WriteString(e.Text)
		case "prompt":
			flush()
			text := e.Text
			if e.Sender == "system" || e.Handoff {
				text = "[SYSTEM] " + dropMemoryBlocks(text)
			}
			trace = append(trace, traceMessage{Role: "user", Content: traceString(text)})
		case "error":
			flush()
			trace = append(trace, traceMessage{Role: "user", Content: traceString("[ERROR] " + dropMemoryBlocks(e.Text))})
		case "tool_call_started", "tool_call_updated":
			flush()
			var tc ToolCall
			if json.Unmarshal([]byte(e.Text), &tc) != nil || tc.Name == "" {
				break
			}
			pair := toolPair(tc)
			if i, ok := toolAt[tc.ID]; ok && tc.ID != "" {
				trace[i], trace[i+1] = pair[0], pair[1]
				break
			}
			if tc.ID != "" {
				toolAt[tc.ID] = len(trace)
			}
			trace = append(trace, pair...)
		}
	}
	flush()
	return trace
}

// traceBlocks renders a pruned trace back into summary blocks, one per message
// and one per tool call with its result.
func traceBlocks(msgs []traceMessage) []summaryBlock {
	var blocks []summaryBlock
	results := map[string]string{}
	for _, m := range msgs {
		if m.Role == "tool" {
			results[m.ToolCallID] = m.text()
		}
	}
	for _, m := range msgs {
		switch m.Role {
		case "user":
			if text := m.text(); strings.TrimSpace(text) != "" {
				blocks = append(blocks, summaryBlock{text: "USER:\n" + text})
			}
		case "assistant":
			if text := strings.TrimSpace(m.text()); text != "" {
				blocks = append(blocks, summaryBlock{text: "ASSISTANT:\n" + text})
			}
			for _, call := range m.ToolCalls {
				blocks = append(blocks, summaryBlock{
					text:   fmt.Sprintf("TOOL %s\ninput: %s\noutput: ", call.Function.Name, call.Function.Arguments),
					result: capHeadTail(results[call.ID], toolResultCap),
				})
			}
		}
	}
	return blocks
}
