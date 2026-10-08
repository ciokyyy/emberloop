package web

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding"
)

// PigBackend drives pig through the embedded Go SDK (pinned in go.mod).
// Every Session runs stripped: no built-in tools, no extension tools, only
// the ExtraTools passed here plus the Daemon's spawn tool. Spawn is how an
// agent creates Subagents: the tool is bound per Session to its own id, so
// the Daemon always knows the parent. Scout Sessions get no spawn tool and
// no write tools: they are read-only.
type PigBackend struct {
	mu       sync.Mutex
	sessions map[string]*pigSession
	// AgentDir overrides the pig config directory (default
	// coding.DefaultAgentDir()). Tests point it at a directory holding a
	// test-faux models.json.
	AgentDir string
	extra    []agent.AgentTool
	// Spawn creates Subagents under one parent. The Daemon wires it to
	// its own Spawn; nil keeps Sessions tool-free.
	Spawn func(parentID string, agents []SpawnAgent) (string, []SessionMeta, error)
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

// sessionTools is the stripped loadout for one Session: the configured
// ExtraTools plus the Daemon spawn tool bound to this Session's id, so the
// Daemon knows the parent of anything it spawns. Scouts keep the ExtraTools
// without spawn: nothing they call can write.
func (b *PigBackend) sessionTools(meta SessionMeta) []agent.AgentTool {
	b.mu.Lock()
	defer b.mu.Unlock()
	tools := append([]agent.AgentTool(nil), b.extra...)
	if b.Spawn != nil && !meta.Scout {
		tools = append(tools, &spawnAgentTool{parent: meta.ID, spawn: b.Spawn})
	}
	return tools
}

// spawnAgentTool is the Daemon spawn tool as a pig ExtraTool. One instance
// serves one Session: Execute spawns Subagents under the bound parent and
// answers with the Group and its Session ids.
type spawnAgentTool struct {
	parent string
	spawn  func(parentID string, agents []SpawnAgent) (string, []SessionMeta, error)
}

func (t *spawnAgentTool) Name() string  { return "spawn" }
func (t *spawnAgentTool) Label() string { return "spawn subagents" }
func (t *spawnAgentTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{
		Name:        t.Name(),
		Description: "Spawn Subagents that work in parallel and report back as one grouped result. Each agent takes a prompt and a type (worker or scout); scouts are read-only.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agents": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":       "object",
						"properties": map[string]any{"prompt": map[string]any{"type": "string"}, "type": map[string]any{"type": "string"}},
						"required":   []string{"prompt"},
					},
				},
			},
			"required": []string{"agents"},
		},
	}
}

func (t *spawnAgentTool) Execute(_ context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	var req struct {
		Agents []SpawnAgent `json:"agents"`
	}
	if err := json.Unmarshal(params, &req); err != nil || len(req.Agents) == 0 {
		return agent.AgentToolResult{}, fmt.Errorf("spawn: agents is required")
	}
	gid, kids, err := t.spawn(t.parent, req.Agents)
	if err != nil {
		return agent.AgentToolResult{}, err
	}
	ids := make([]string, 0, len(kids))
	for _, k := range kids {
		ids = append(ids, k.ID)
	}
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "spawned group " + gid + ": " + strings.Join(ids, " ")}},
	}, nil
}

func (t *spawnAgentTool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeSequential }

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
	tools := b.sessionTools(meta)
	b.mu.Lock()
	if _, ok := b.sessions[meta.ID]; ok {
		b.mu.Unlock()
		return fmt.Errorf("pig: session %s already exists", meta.ID)
	}
	agentDir := b.AgentDir
	b.mu.Unlock()
	svcs, model, err := buildPigServices(agentDir, tools, meta)
	if err != nil {
		return err
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
		ExtraTools:         tools,
	})
	if err != nil {
		_ = rt.Close()
		svcs.Close()
		return fmt.Errorf("pig: session: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.sessions[meta.ID]; ok {
		b.closePig(&pigSession{sess: sess, rt: rt, svcs: svcs, unsub: sess.Subscribe(func(agent.AgentEvent) {})})
		return fmt.Errorf("pig: session %s already exists", meta.ID)
	}
	b.subscribe(meta.ID, sess, rt, svcs, emit)
	return nil
}

// buildPigServices builds the Services and model for one Daemon Session. An
// unknown model or unusable working directory returns an error for a 400.
// It holds no lock; callers snapshot AgentDir and ExtraTools first.
func buildPigServices(agentDir string, extra []agent.AgentTool, meta SessionMeta) (*coding.Services, *ai.Model, error) {
	if agentDir == "" {
		agentDir = coding.DefaultAgentDir()
	}
	svcs, err := coding.NewServices(coding.ServicesOptions{CWD: meta.WorkDir, AgentDir: agentDir})
	if err != nil {
		return nil, nil, fmt.Errorf("pig: services: %w", err)
	}
	// BuildModel still builds unknown specs, so the listing check owns the
	// rejection: only catalog models create. The test-faux provider passes
	// only with PIG_TEST_FAUX=1, so production never admits unlisted models.
	if _, ok := ai.LookupModel(meta.Model); !ok && !isTestFauxModel(meta.Model) {
		svcs.Close()
		return nil, nil, fmt.Errorf("pig: unknown model %q", meta.Model)
	}
	model, err := coding.BuildModel(meta.Model, svcs)
	if err != nil {
		svcs.Close()
		return nil, nil, fmt.Errorf("pig: model %q: %w", meta.Model, err)
	}
	return svcs, model, nil
}

// subscribe attaches the normalized Event stream and stores the stack.
// Callers hold b.mu.
func (b *PigBackend) subscribe(id string, sess *coding.Session, rt *coding.Runtime, svcs *coding.Services, emit func(kind, text, status string)) {
	ctx, cancel := context.WithCancel(context.Background())
	unsub := sess.Subscribe(func(e agent.AgentEvent) {
		for _, ne := range mapAgentEvent(e) {
			emit(ne.kind, ne.text, ne.status)
		}
	})
	b.sessions[id] = &pigSession{ctx: ctx, cancel: cancel, sess: sess, rt: rt, svcs: svcs, unsub: unsub}
}

// OnPrompt runs one turn to completion. Chunks and tool calls already
// streamed through the subscription; this records only the status edges.
// A close mid-turn drops the status edges: the Daemon owns the ended state
// and ended is terminal there, so a late idle or failed must not regress it.
func (b *PigBackend) OnPrompt(sessionID, text string, appendEvent func(kind, text, status string)) {
	b.mu.Lock()
	ps, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok {
		appendEvent("error", "pig: unknown session", "")
		return
	}
	appendEvent("status", "", "working")
	_, sendErr := ps.sess.Send(ps.ctx, text)
	if !b.alive(sessionID) {
		if sendErr != nil {
			appendEvent("error", sendErr.Error(), "")
		}
		return
	}
	if sendErr != nil {
		appendEvent("error", sendErr.Error(), "")
		appendEvent("status", "", "failed")
		return
	}
	appendEvent("status", "", "idle")
}

// Steer delivers text at the next tool boundary through the SDK queue.
// FollowUp delivers it after the turn. The Daemon routes working Sessions
// here and lists both through QueuedMessages; messages queued with no turn
// running buffer until the next Send.
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

// QueuedMessages lists the pig native queues oldest-first, steering ahead
// of follow-ups. The Daemon merges this with its own pending list.
func (b *PigBackend) QueuedMessages(sessionID string) []QueuedMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	ps, ok := b.sessions[sessionID]
	if !ok {
		return nil
	}
	var out []QueuedMessage
	for _, text := range ps.sess.GetSteeringMessages() {
		out = append(out, QueuedMessage{Mode: "steer", Text: text})
	}
	for _, text := range ps.sess.GetFollowUpMessages() {
		out = append(out, QueuedMessage{Mode: "followup", Text: text})
	}
	return out
}

// ClearQueued drops the pig native queues before delivery and reports how
// many messages were dropped.
func (b *PigBackend) ClearQueued(sessionID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	ps, ok := b.sessions[sessionID]
	if !ok {
		return 0
	}
	steering, followUp := ps.sess.ClearQueue()
	return len(steering) + len(followUp)
}

// CloseSession cancels the turn, closes the SDK session and its runtime,
// and drops the handle. Repeated calls are safe. A concurrent release that
// already replaced the entry leaves the new handle alone.
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
	b.closePig(ps)
}

// closePig tears down one SDK stack outside the backend lock.
func (b *PigBackend) closePig(ps *pigSession) {
	if ps.cancel != nil {
		ps.cancel()
	}
	if ps.unsub != nil {
		ps.unsub()
	}
	_ = ps.sess.Close()
	_ = ps.rt.Close()
	ps.svcs.Close()
}

func (b *PigBackend) alive(sessionID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.sessions[sessionID]
	return ok
}

// ReleaseSession captures the pig Session id, then closes the SDK stack so
// an idle Session holds no process. The id reloads through Runtime.Resume.
func (b *PigBackend) ReleaseSession(sessionID string) string {
	b.mu.Lock()
	ps, ok := b.sessions[sessionID]
	if ok {
		delete(b.sessions, sessionID)
	}
	b.mu.Unlock()
	if !ok {
		return ""
	}
	token := ps.sess.ID()
	b.closePig(ps)
	return token
}

// ResumeToken reports the live pig Session id for persistence.
func (b *PigBackend) ResumeToken(sessionID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ps, ok := b.sessions[sessionID]; ok {
		return ps.sess.ID()
	}
	return ""
}

// ResumeOK holds when a saved pig Session id exists to reload.
func (b *PigBackend) ResumeOK(meta SessionMeta) (bool, string) {
	if meta.ResumeToken == "" {
		return false, "pig Session has no saved agent to load (it started before resume support); history is read-only"
	}
	return true, ""
}

// ResumeSession rebuilds the SDK stack and reloads the saved pig Session.
// Slow construction runs outside the backend lock; a handle that appears
// meanwhile is adopted instead of replaced.
func (b *PigBackend) ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error {
	if meta.ResumeToken == "" {
		return fmt.Errorf("pig: no saved session to resume")
	}
	b.mu.Lock()
	if _, ok := b.sessions[meta.ID]; ok {
		b.mu.Unlock()
		return nil
	}
	b.mu.Unlock()
	tools := b.sessionTools(meta)
	b.mu.Lock()
	agentDir := b.AgentDir
	b.mu.Unlock()
	svcs, model, err := buildPigServices(agentDir, tools, meta)
	if err != nil {
		return err
	}
	rt, err := coding.NewRuntime(coding.RuntimeOptions{Services: svcs})
	if err != nil {
		svcs.Close()
		return fmt.Errorf("pig: runtime: %w", err)
	}
	sess, err := rt.Resume(meta.ResumeToken, coding.SessionStartOptions{
		Model:              model,
		SkipBuiltinTools:   true,
		SkipExtensionTools: true,
		ExtraTools:         tools,
	})
	if err != nil {
		_ = rt.Close()
		svcs.Close()
		return fmt.Errorf("pig: resume: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.sessions[meta.ID]; ok {
		b.closePig(&pigSession{sess: sess, rt: rt, svcs: svcs, unsub: sess.Subscribe(func(agent.AgentEvent) {})})
		return nil
	}
	b.subscribe(meta.ID, sess, rt, svcs, emit)
	return nil
}

func isTestFauxModel(model string) bool {
	return os.Getenv("PIG_TEST_FAUX") == "1" && strings.HasPrefix(model, "test-faux/")
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
