package web

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding"
)

// PigBackend drives pig through the embedded Go SDK (pinned in go.mod).
// Every Session runs stripped: no built-in tools, no extension tools, only
// the ExtraTools passed here. That slice is the emberloop MCP tool set; it
// stays empty until ticket 06 serves the spawn tool.
type PigBackend struct {
	mu       sync.Mutex
	sessions map[string]*pigSession
	// AgentDir overrides the pig config directory (default
	// coding.DefaultAgentDir()). Tests point it at a directory holding a
	// test-faux models.json.
	AgentDir string
	extra    []agent.AgentTool
}

type pigSession struct {
	ctx    context.Context
	cancel context.CancelFunc
	sess   *coding.Session
	rt     *coding.Runtime
	svcs   *coding.Services
	unsub  func()
}

func NewPigBackend(extra ...agent.AgentTool) *PigBackend {
	return &PigBackend{sessions: map[string]*pigSession{}, extra: extra}
}

var pigModelsOnce sync.Once
var pigModelsCache []string

// PigModels lists every pig model as "provider/id", sorted. It reads the
// same generated catalog the pig binary lists, so the Daemon's choices can
// never name a model the harness cannot run.
func PigModels() []string {
	pigModelsOnce.Do(func() {
		var out []string
		for _, m := range ai.ListModels("") {
			out = append(out, m.Provider+"/"+m.ID)
		}
		sort.Strings(out)
		pigModelsCache = out
	})
	return pigModelsCache
}

func (b *PigBackend) Models() []string { return PigModels() }

// CreateSession builds the SDK stack (Services, Runtime, Session) for one
// Daemon Session and subscribes the normalized Event stream. An unknown
// model or unusable working directory returns an error for a 400.
func (b *PigBackend) CreateSession(meta SessionMeta, emit func(kind, text, status string)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.sessions[meta.ID]; ok {
		return fmt.Errorf("pig: session %s already exists", meta.ID)
	}
	agentDir := b.AgentDir
	if agentDir == "" {
		agentDir = coding.DefaultAgentDir()
	}
	svcs, err := coding.NewServices(coding.ServicesOptions{CWD: meta.WorkDir, AgentDir: agentDir})
	if err != nil {
		return fmt.Errorf("pig: services: %w", err)
	}
	// BuildModel still builds unknown specs, so the listing check owns the
	// rejection: only catalog models (and the test-faux provider) create.
	if _, ok := ai.LookupModel(meta.Model); !ok && !strings.HasPrefix(meta.Model, "test-faux/") {
		svcs.Close()
		return fmt.Errorf("pig: unknown model %q", meta.Model)
	}
	model, err := coding.BuildModel(meta.Model, svcs)
	if err != nil {
		svcs.Close()
		return fmt.Errorf("pig: model %q: %w", meta.Model, err)
	}
	rt, err := coding.NewRuntime(coding.RuntimeOptions{Services: svcs})
	if err != nil {
		svcs.Close()
		return fmt.Errorf("pig: runtime: %w", err)
	}
	sess, err := rt.New(coding.SessionStartOptions{
		Model:              model,
		SkipBuiltinTools:   true,
		SkipExtensionTools: true,
		ExtraTools:         b.extra,
	})
	if err != nil {
		_ = rt.Close()
		svcs.Close()
		return fmt.Errorf("pig: session: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	unsub := sess.Subscribe(func(e agent.AgentEvent) {
		for _, ne := range mapAgentEvent(e) {
			emit(ne.kind, ne.text, ne.status)
		}
	})
	b.sessions[meta.ID] = &pigSession{ctx: ctx, cancel: cancel, sess: sess, rt: rt, svcs: svcs, unsub: unsub}
	return nil
}

// OnPrompt runs one turn to completion. Chunks and tool calls already
// streamed through the subscription; this records only the status edges.
func (b *PigBackend) OnPrompt(sessionID, text string, appendEvent func(kind, text, status string)) {
	b.mu.Lock()
	ps, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok {
		appendEvent("error", "pig: unknown session", "")
		return
	}
	appendEvent("status", "", "working")
	if _, err := ps.sess.Send(ps.ctx, text); err != nil {
		appendEvent("error", err.Error(), "")
		appendEvent("status", "", "failed")
		return
	}
	appendEvent("status", "", "idle")
}

// Steer delivers text at the next tool boundary through the SDK queue.
// FollowUp delivers it after the turn. The HTTP endpoints land in ticket 05.
func (b *PigBackend) Steer(sessionID, text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ps, ok := b.sessions[sessionID]; ok {
		ps.sess.QueueSteer(text, nil)
	}
}

func (b *PigBackend) FollowUp(sessionID, text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ps, ok := b.sessions[sessionID]; ok {
		ps.sess.QueueFollowUp(text, nil)
	}
}

// CloseSession cancels the turn, closes the SDK session and its runtime,
// and drops the handle. Repeated calls are safe.
func (b *PigBackend) CloseSession(sessionID string) {
	b.mu.Lock()
	ps, ok := b.sessions[sessionID]
	if ok {
		delete(b.sessions, sessionID)
	}
	b.mu.Unlock()
	if !ok {
		return
	}
	ps.cancel()
	if ps.unsub != nil {
		ps.unsub()
	}
	_ = ps.sess.Close()
	_ = ps.rt.Close()
	ps.svcs.Close()
}

// ToolNames reports the active loadout. Stripped Sessions offer no native
// tools, so this holds only the configured MCP tools.
func (b *PigBackend) ToolNames(sessionID string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ps, ok := b.sessions[sessionID]
	if !ok {
		return nil
	}
	var out []string
	for _, t := range ps.sess.Tools() {
		out = append(out, t.Name())
	}
	sort.Strings(out)
	return out
}

type normEvent struct {
	kind, text, status string
}

// mapAgentEvent normalizes one SDK event. Message text and thinking deltas
// become chunks; tool-call frames and tool execution become tool_call
// Events. Turn, message lifecycle, queue and settle Events carry no
// transcript content: pig has no plan event type, and the queue UI lands in
// ticket 05.
func mapAgentEvent(e agent.AgentEvent) []normEvent {
	switch e := e.(type) {
	case agent.MessageUpdateEvent:
		switch se := e.AssistantMessageEvent.(type) {
		case ai.TextDeltaEvent:
			if se.Delta == "" {
				return nil
			}
			return []normEvent{{kind: "assistant_chunk", text: se.Delta}}
		case ai.ThinkingDeltaEvent:
			if se.Delta == "" {
				return nil
			}
			return []normEvent{{kind: "thinking_chunk", text: se.Delta}}
		case ai.ToolCallStartEvent:
			return []normEvent{{kind: "tool_call_started", text: partialToolName(e.Message, se.ContentIndex)}}
		case ai.ToolCallDeltaEvent:
			return []normEvent{{kind: "tool_call_updated", text: se.Delta}}
		case ai.ToolCallEndEvent:
			args, _ := json.Marshal(se.ToolCall.Arguments)
			return []normEvent{{kind: "tool_call_updated", text: se.ToolCall.Name + " " + string(args)}}
		}
	case agent.ToolExecutionStartEvent:
		return []normEvent{{kind: "tool_call_started", text: e.ToolName}}
	case agent.ToolExecutionUpdateEvent:
		return []normEvent{{kind: "tool_call_updated", text: toolResultText(e.PartialResult)}}
	case agent.ToolExecutionEndEvent:
		text := toolResultText(e.Result)
		if e.IsError {
			text = "error: " + text
		}
		return []normEvent{{kind: "tool_call_updated", text: text}}
	}
	return nil
}

func partialToolName(msg agent.AgentMessage, idx int) string {
	if msg.Assistant == nil {
		return ""
	}
	blocks := msg.Assistant.Content
	if idx >= 0 && idx < len(blocks) {
		if call, ok := blocks[idx].(ai.ToolCall); ok {
			return call.Name
		}
	}
	for _, block := range blocks {
		if call, ok := block.(ai.ToolCall); ok {
			return call.Name
		}
	}
	return ""
}

func toolResultText(r agent.AgentToolResult) string {
	out := ""
	for _, c := range r.Content {
		if t, ok := c.(ai.TextContent); ok {
			out += t.Text
		}
	}
	return out
}
