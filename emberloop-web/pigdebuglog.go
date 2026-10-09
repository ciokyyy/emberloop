package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

// pigLog is the per-Session trace of the embedded pig stack: one JSON line per
// SDK event, stall decision and recovery. A nil pigLog (no LogDir) drops every
// write, and a failed write is ignored: the trace must never disturb a turn.
type pigLog struct {
	mu   sync.Mutex
	file *os.File
	// updates counts per-token events since the last logged event, and last
	// is the time of the previous record: together they show how long the
	// stream sat silent and how much it said before going quiet.
	updates int
	chars   int // blocks in the partial assistant message
	last    time.Time
}

func newPigLog(dir, id string) *pigLog {
	if dir == "" {
		return nil
	}
	if os.MkdirAll(dir, 0o700) != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, "pig-"+id+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil
	}
	return &pigLog{file: f, last: time.Now()}
}

func (l *pigLog) write(kind string, data map[string]any) {
	if l == nil {
		return
	}
	now := time.Now()
	rec := map[string]any{"t": now.UTC().Format(time.RFC3339Nano), "kind": kind}
	l.mu.Lock()
	rec["gapMs"] = now.Sub(l.last).Milliseconds()
	l.last = now
	if l.updates > 0 {
		rec["updatesBefore"] = l.updates
		rec["blocksNow"] = l.chars
		l.updates, l.chars = 0, 0
	}
	l.mu.Unlock()
	if len(data) > 0 {
		rec["data"] = data
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.file.Write(append(line, '\n'))
}

// event records one SDK event by Go type name. Per-token updates are skipped
// (one line per token would bury the stall), and the normalized text stays
// short.
func (l *pigLog) event(e agent.AgentEvent) {
	if l == nil {
		return
	}
	name := fmt.Sprintf("%T", e)
	d := map[string]any{"type": name}
	switch v := e.(type) {
	case agent.MessageUpdateEvent:
		// Per-token: count only, so the next record shows what arrived.
		l.mu.Lock()
		l.updates++
		if v.Message.Assistant != nil {
			l.chars = len(v.Message.Assistant.Content)
		}
		l.mu.Unlock()
		return
	case agent.MessageStartEvent:
		d["role"] = messageRole(v.Message)
	case agent.MessageEndEvent:
		d["role"] = messageRole(v.Message)
		if a := v.Message.Assistant; a != nil {
			d["stopReason"] = string(a.StopReason)
			d["rawStopReason"] = a.RawStopReason
			d["errorMessage"] = a.ErrorMessage
			d["model"] = a.Provider + "/" + a.ModelID
			d["thinkingLevel"] = string(a.ThinkingLevel)
			d["blocks"] = describeBlocks(a.Content)
			if a.Usage != nil {
				d["usage"] = a.Usage
			}
			if len(a.Diagnostics) > 0 {
				d["diagnostics"] = a.Diagnostics
			}
		}
	case agent.ToolExecutionStartEvent:
		d["tool"], d["callId"], d["args"] = v.ToolName, v.ToolCallID, clip(string(v.Args), 200)
	case agent.ToolExecutionEndEvent:
		d["tool"], d["callId"], d["isError"], d["durMs"] = v.ToolName, v.ToolCallID, v.IsError, v.Duration.Milliseconds()
	case agent.TurnEndEvent:
		d["turn"], d["toolResults"] = v.TurnIndex, len(v.ToolResults)
	}
	for _, ne := range mapAgentEvent(e) {
		if ne.kind == "assistant_chunk" || ne.kind == "thinking_chunk" {
			continue
		}
		d["kind"], d["status"], d["text"] = ne.kind, ne.status, clip(ne.text, 200)
		break
	}
	l.write("sdk_event", d)
}

func messageRole(m agent.AgentMessage) string {
	switch {
	case m.Assistant != nil:
		return "assistant"
	case m.ToolResult != nil:
		return "toolResult"
	case m.User != nil:
		return "user"
	case m.System != nil:
		return "system"
	}
	return "custom"
}

// describeBlocks lists an assistant message's content blocks as "type:len"
// so a message cut off before its tool calls shows as text only.
func describeBlocks(blocks []ai.AssistantContentBlock) []string {
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		switch v := b.(type) {
		case ai.TextContent:
			out = append(out, fmt.Sprintf("text:%d", len(v.Text)))
		case ai.ThinkingContent:
			out = append(out, fmt.Sprintf("thinking:%d", len(v.Thinking)))
		default:
			out = append(out, fmt.Sprintf("%T", b))
		}
	}
	return out
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
