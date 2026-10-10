package herdrreport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// The agent log. A spawned pig agent runs in a pane nobody reads, and when its
// provider call dies the session file simply stops: no error record, nothing
// to say why. This writes one JSON line per lifecycle event to
// ~/.pig/state/emberloop/agentlogs/<label>-<session>.log, so a stalled agent
// leaves its last event, its last provider response and its last error behind.
// It never answers an event and never fails the session: every write is best
// effort. High-rate events (message_update, provider_stream_event) are left
// out on purpose, since each one costs the host a round trip.

// logEvents is every event the log records.
var logEvents = []string{
	sdk.EventSessionStart,
	sdk.EventSessionShutdown,
	sdk.EventBeforeAgentStart,
	sdk.EventAgentStart,
	sdk.EventAgentEnd,
	sdk.EventAgentBeforeSettle,
	sdk.EventAgentSettled,
	sdk.EventTurnStart,
	sdk.EventTurnEnd,
	sdk.EventMessageStart,
	sdk.EventMessageEnd,
	sdk.EventToolExecutionStart,
	sdk.EventToolExecutionEnd,
	sdk.EventBeforeProviderRequest,
	sdk.EventAfterProviderResponse,
	sdk.EventSessionCompactFailed,
	sdk.EventModelSelect,
}

// bulkKeys hold whole transcripts or request bodies. The log keeps their size,
// not their content.
var bulkKeys = map[string]bool{
	"payload": true, "context": true, "contextMessages": true,
	"messages": true, "systemPrompt": true, "entries": true,
}

const maxLogString = 400

type agentLog struct {
	mu   sync.Mutex
	path func() string
	file *os.File
}

func newAgentLog(pathFn func() string) *agentLog { return &agentLog{path: pathFn} }

// write appends one record. A failure is dropped: the log must never disturb
// the session it watches.
func (l *agentLog) write(event string, data map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		p := l.path()
		if p == "" {
			return
		}
		if os.MkdirAll(filepath.Dir(p), 0o700) != nil {
			return
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return
		}
		l.file = f
	}
	rec := map[string]any{"t": time.Now().UTC().Format(time.RFC3339Nano), "event": event}
	if len(data) > 0 {
		rec["data"] = summarize(data)
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	_, _ = l.file.Write(append(line, '\n'))
}

// summarize shrinks an event body to what explains a stall: scalars, short
// strings, and the size of anything bulky.
func summarize(data map[string]any) map[string]any {
	out := make(map[string]any, len(data))
	for k, v := range data {
		out[k] = shrink(k, v, 0)
	}
	return out
}

func shrink(key string, v any, depth int) any {
	if bulkKeys[key] {
		b, _ := json.Marshal(v)
		return map[string]any{"bytes": len(b)}
	}
	switch x := v.(type) {
	case string:
		if len(x) > maxLogString {
			return x[:maxLogString] + "…"
		}
		return x
	case map[string]any:
		if depth >= 3 {
			return "{…}"
		}
		m := make(map[string]any, len(x))
		for k, vv := range x {
			m[k] = shrink(k, vv, depth+1)
		}
		return m
	case []any:
		if depth >= 3 || len(x) > 8 {
			b, _ := json.Marshal(x)
			return map[string]any{"items": len(x), "bytes": len(b)}
		}
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = shrink("", vv, depth+1)
		}
		return out
	default:
		return v
	}
}

// logPath is the file this agent logs to: the label el gave it, then the
// session id so two agents with one label never share a file.
func logPath(label, sessionID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	name := label
	if sessionID != "" {
		name += "-" + sessionID
	}
	return filepath.Join(home, ".pig", "state", "emberloop", "agentlogs", name+".log")
}

// registerLog records every event in logEvents. The handlers answer nothing.
func registerLog(r eventRegistrar, l *agentLog) {
	for _, ev := range logEvents {
		ev := ev
		r.OnEvent(ev, func(_ sdk.Context, data map[string]any) (any, error) {
			l.write(ev, data)
			return nil, nil
		})
	}
}
