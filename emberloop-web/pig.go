package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding"

	"example.com/emberloop-web/memory"
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
	Spawn func(parentID string, agents []SpawnAgent, deliver string) (string, []SessionMeta, error)
	// SteerChild messages one Subagent of the calling Session. The Daemon
	// wires it to its SteerChild; nil keeps Sessions steer-free.
	SteerChild func(parentID, childID, text string) (string, error)
	// DaemonURL is the Daemon base URL a Run worker's submit tool posts to;
	// empty leaves Run workers without submit.
	DaemonURL string
	// DaemonToken is the machine token the submit callback sends (see auth.go).
	DaemonToken string
	// CheckChild peeks at the calling Session's Subagents. The Daemon wires
	// it to its CheckChildren (empty child) and CheckChild; nil keeps
	// Sessions check-free.
	CheckChild func(parentID, childID string) (string, error)
	// CancelChild stops one Subagent of the calling Session. The Daemon
	// wires it to its CancelChild; nil keeps Sessions cancel-free.
	CancelChild func(parentID, childID string, kill bool) (string, error)
	// ReadProfile reads the Session owner's Profile (memory://profile).
	// The Daemon wires it to its ReadProfileForSession; nil hides the read.
	ReadProfile func(sessionID string) (string, error)
	// ProposeProfile enqueues a Profile patch for human approval. The Daemon
	// wires it to its ProposeProfileFromSession; nil hides the propose.
	// It always creates a queue item, never a Profile version, and only on
	// an explicit User request mentioning the Profile.
	ProposeProfile func(sessionID string, patch memory.ProfilePatch, reason string) (int64, error)
	// Remember proposes a Rule for the Session owner's bank. The Daemon wires
	// it to its RememberFromSession; nil hides the propose. It always creates
	// a queue item for human approval, never a Rule, needs a reason, is
	// rate-limited per Session, and only runs on top-level Sessions.
	Remember func(sessionID, text, reason, quote string) (int64, error)
	// Recall searches the Session owner's bank and the Project notes. The
	// Daemon wires it to its RecallForSession; nil hides the search. Every
	// role may search, including scouts, workers, Reviewers and Run Sessions.
	Recall func(sessionID, query string) (string, error)
	// SudoGate builds the per-Session gate that routes sudo bash commands to
	// user approval; nil runs them unasked.
	SudoGate func(sessionID string) func(ctx context.Context, root, command string) (string, bool)
	// SharedTools serves the el tools package (the same tools
	// el-mcp serves claude) to every Session, by role.
	SharedTools bool
	// LogDir, when set, receives one pig-<session>.log per Session: every SDK
	// event, each stall decision and each recovery, one JSON line each. A
	// worker whose provider stream dies leaves no error in its transcript, so
	// this is the only trace of where it stopped.
	LogDir string
}

type pigSession struct {
	// lastEvent is the unix-nano time of the newest SDK event; tools counts
	// tool executions in flight. The stall watchdog reads both.
	lastEvent atomic.Int64
	tools     atomic.Int32
	// textTail is true while the newest SDK output is assistant text with no
	// tool call after it: a turn that goes silent then has already answered.
	textTail atomic.Bool
	// interrupted is set by Interrupt; the turn then ends idle, not failed.
	interrupted atomic.Bool
	ctx         context.Context
	cancel      context.CancelFunc
	sess        *coding.Session
	rt          *coding.Runtime
	svcs        *coding.Services
	unsub       func()
	// meta and emit rebuild the stack in place when the SDK wedges inside a
	// provider stream (see recoverWedged).
	meta SessionMeta
	emit func(kind, text, status string)
	log  *pigLog
}

func NewPigBackend(extra ...agent.AgentTool) *PigBackend {
	return &PigBackend{sessions: map[string]*pigSession{}, extra: extra}
}

// sessionTools is the stripped loadout for one Session: the configured
// ExtraTools plus the Daemon tools its el role serves (elRoleFor), bound to
// this Session's id, so the Daemon knows the parent of anything it spawns,
// messages, peeks at or stops. Web gets spawn, steer, check, cancel, profile
// and remember; web-worker and run-worker get spawn (run-worker also submit);
// every role with a wired backend gets recall, scouts included. Workers,
// scouts and Reviewers get recall only (ticket 18); only top-level Sessions
// propose Profile patches and Rules.
func (b *PigBackend) sessionTools(meta SessionMeta) []agent.AgentTool {
	b.mu.Lock()
	defer b.mu.Unlock()
	role := elRoleFor(meta)
	tools := append([]agent.AgentTool(nil), b.extra...)
	if b.SharedTools {
		var gate func(ctx context.Context, root, command string) (string, bool)
		if b.SudoGate != nil && role != "scout" {
			gate = b.SudoGate(meta.ID)
		}
		if shared, err := elToolsFor(meta, gate); err == nil {
			tools = append(tools, shared...)
		} else {
			fmt.Fprintln(os.Stderr, "el-web: shared tools:", err)
		}
	}
	if role == "run-worker" && b.DaemonURL != "" {
		tools = append(tools, &submitAgentTool{session: meta.ID, daemonURL: b.DaemonURL, daemonToken: b.DaemonToken})
	}
	if role != "scout" && b.Spawn != nil {
		tools = append(tools, &spawnAgentTool{parent: meta.ID, spawn: b.Spawn})
	}
	if role == "web" {
		if b.SteerChild != nil {
			tools = append(tools, &steerAgentTool{parent: meta.ID, steer: b.SteerChild})
		}
		if b.CheckChild != nil {
			tools = append(tools, &checkAgentTool{parent: meta.ID, check: b.CheckChild})
		}
		if b.CancelChild != nil {
			tools = append(tools, &cancelAgentTool{parent: meta.ID, cancel: b.CancelChild})
		}
		// Reviewers get the filtered Profile by injection only; they never
		// propose patches or Rules.
		if !isReviewerSession(meta) && (b.ReadProfile != nil || b.ProposeProfile != nil) {
			tools = append(tools, &profileAgentTool{session: meta.ID, read: b.ReadProfile, propose: b.ProposeProfile})
		}
		// Only top-level Sessions propose Rules through remember; workers,
		// scouts, Reviewers and Run Sessions get recall only.
		if !isReviewerSession(meta) && b.Remember != nil {
			tools = append(tools, &rememberAgentTool{session: meta.ID, remember: b.Remember})
		}
	}
	// Every role searches memory, scouts included: recall never writes and the
	// bank is always the Session owner's, so no other User's notes ever show.
	if b.Recall != nil {
		tools = append(tools, &recallAgentTool{session: meta.ID, recall: b.Recall})
	}
	return tools
}

// spawnAgentTool is the Daemon spawn tool as a pig ExtraTool. One instance
// serves one Session: Execute spawns Subagents under the bound parent and
// answers with the Group and its Session ids.
type spawnAgentTool struct {
	parent string
	spawn  func(parentID string, agents []SpawnAgent, deliver string) (string, []SessionMeta, error)
}

func (t *spawnAgentTool) Name() string  { return "spawn" }
func (t *spawnAgentTool) Label() string { return "spawn subagents" }
func (t *spawnAgentTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{
		Name:        t.Name(),
		Description: "Spawn Subagents that work in parallel and report back. Each agent takes a prompt, an optional name for its card, and a type (worker or scout); scouts are read-only. Deliver selects the parent notices: 'group' (the default) sends one grouped result when the last member settles, 'each' sends one notice per child as it settles. Returned ids can be steered with the steer tool, peeked at with check, or stopped with cancel.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"deliver": map[string]any{"type": "string", "enum": []string{"group", "each"}},
				"agents": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":       "object",
						"properties": map[string]any{"prompt": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"}, "type": map[string]any{"type": "string"}},
						"required":   []string{"prompt"},
					},
				},
			},
			"required": []string{"agents"},
		},
	}
}

// steerAgentTool is the Daemon steer tool as a pig ExtraTool. One instance
// serves one Session: Execute messages the caller's own Subagent by id. A
// running child is steered at its next safe step, an idle one starts as a
// follow-up, and a released one resumes first.
type steerAgentTool struct {
	parent string
	steer  func(parentID, childID, text string) (string, error)
}

func (t *steerAgentTool) Name() string  { return "steer" }
func (t *steerAgentTool) Label() string { return "message a subagent" }
func (t *steerAgentTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{
		Name:        t.Name(),
		Description: "Message a Subagent you spawned by its Session id. A running worker is steered at its next safe step without interrupting your turn; an idle one starts as a follow-up; a released one resumes first. Only your own children; scouts are read-only. See also check (peek without messaging) and cancel (stop a turn).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":      map[string]any{"type": "string"},
				"message": map[string]any{"type": "string"},
			},
			"required": []string{"id", "message"},
		},
	}
}
func (t *steerAgentTool) Execute(_ context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	var req struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" || req.Message == "" {
		return agent.AgentToolResult{}, fmt.Errorf("steer: id and message are required")
	}
	outcome, err := t.steer(t.parent, req.ID, req.Message)
	if err != nil {
		return agent.AgentToolResult{}, err
	}
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: outcome}},
	}, nil
}
func (t *steerAgentTool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeSequential }

// submitAgentTool is the Run worker submit tool as a pig ExtraTool. It posts
// to the same submit route the el MCP proxy uses, so a done submit starts the
// ticket's reviewers on every harness alike.
type submitAgentTool struct {
	session     string
	daemonURL   string
	daemonToken string
}

func (t *submitAgentTool) Name() string  { return "submit" }
func (t *submitAgentTool) Label() string { return "submit work" }
func (t *submitAgentTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{
		Name:        t.Name(),
		Description: "Submit your result to the Daemon when the ticket work is complete: status done with the completed checkbox texts. A done submit starts the reviewers.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":    map[string]any{"type": "string"},
				"completed": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"report":    map[string]any{"type": "string"},
			},
			"required": []string{"status"},
		},
	}
}
func (t *submitAgentTool) Execute(ctx context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	var req struct {
		Status    string   `json:"status"`
		Completed []string `json:"completed"`
		Report    string   `json:"report"`
	}
	if err := json.Unmarshal(params, &req); err != nil || req.Status == "" {
		return agent.AgentToolResult{}, fmt.Errorf("submit: status is required")
	}
	body, _ := json.Marshal(req)
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.daemonURL+"/api/sessions/"+t.session+"/submit", bytes.NewReader(body))
	if err != nil {
		return agent.AgentToolResult{}, fmt.Errorf("submit: %w", err)
	}
	hreq.Header.Set("Content-Type", "application/json")
	if t.daemonToken != "" {
		hreq.Header.Set("Authorization", "Bearer "+t.daemonToken)
	}
	resp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return agent.AgentToolResult{}, fmt.Errorf("submit: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		Submits int    `json:"submits"`
		Err     string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusAccepted {
		if out.Err == "" {
			out.Err = fmt.Sprintf("refused: %d", resp.StatusCode)
		}
		return agent.AgentToolResult{}, fmt.Errorf("submit: %s", out.Err)
	}
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: fmt.Sprintf("submitted %s (submit %d)", req.Status, out.Submits)}},
	}, nil
}
func (t *submitAgentTool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeSequential }

// checkAgentTool is the Daemon check tool as a pig ExtraTool. One instance
// serves one Session: Execute peeks at the caller's own Subagents, listing
// them all when id is empty, else one child's summary plus recent Events.
type checkAgentTool struct {
	parent string
	check  func(parentID, childID string) (string, error)
}

func (t *checkAgentTool) Name() string  { return "check" }
func (t *checkAgentTool) Label() string { return "peek at subagents" }
func (t *checkAgentTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{
		Name:        t.Name(),
		Description: "Peek at Subagents you spawned without messaging them: with no id lists your own children, with an id shows one child's status plus its recent events. Results already arrive as one grouped result, so do not poll; use check only to peek. Only your own children.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string"},
			},
		},
	}
}
func (t *checkAgentTool) Execute(_ context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return agent.AgentToolResult{}, fmt.Errorf("check: bad arguments")
	}
	outcome, err := t.check(t.parent, req.ID)
	if err != nil {
		return agent.AgentToolResult{}, err
	}
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: outcome}},
	}, nil
}
func (t *checkAgentTool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeSequential }

// cancelAgentTool is the Daemon cancel tool as a pig ExtraTool. One instance
// serves one Session: Execute stops the caller's own Subagent mid-turn
// without touching the caller, ending its Session too when kill is set.
type cancelAgentTool struct {
	parent string
	cancel func(parentID, childID string, kill bool) (string, error)
}

func (t *cancelAgentTool) Name() string  { return "cancel" }
func (t *cancelAgentTool) Label() string { return "stop a subagent" }
func (t *cancelAgentTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{
		Name:        t.Name(),
		Description: "Stop a Subagent you spawned: interrupts its current turn without touching your own; kill also ends its Session. Only your own children; scouts can be stopped but not messaged. Prefer check to see what it is doing first.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":   map[string]any{"type": "string"},
				"kill": map[string]any{"type": "boolean"},
			},
			"required": []string{"id"},
		},
	}
}
func (t *cancelAgentTool) Execute(_ context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	var req struct {
		ID   string `json:"id"`
		Kill bool   `json:"kill"`
	}
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" {
		return agent.AgentToolResult{}, fmt.Errorf("cancel: id is required")
	}
	outcome, err := t.cancel(t.parent, req.ID, req.Kill)
	if err != nil {
		return agent.AgentToolResult{}, err
	}
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: outcome}},
	}, nil
}
func (t *cancelAgentTool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeSequential }

// profileAgentTool is the Daemon profile tool as a pig ExtraTool. One
// instance serves one top-level Session: read returns memory://profile, and
// propose enqueues a patch for human approval. It never writes a version
// itself, and only runs after an explicit User request mentioning the
// Profile; workers, scouts and Reviewers never see it.
type profileAgentTool struct {
	session string
	read    func(sessionID string) (string, error)
	propose func(sessionID string, patch memory.ProfilePatch, reason string) (int64, error)
}

func (t *profileAgentTool) Name() string  { return "profile" }
func (t *profileAgentTool) Label() string { return "read or propose profile" }
func (t *profileAgentTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{
		Name:        t.Name(),
		Description: "Read your standing Profile (memory://profile), or propose a patch to it. A patch is add, edit or retire operations by line ID and always waits for human approval in the settings view; it never applies directly. Call propose only after the User explicitly asks to change the Profile.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{"type": "string", "enum": []string{"read", "propose"}},
				"ops": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"op":   map[string]any{"type": "string"},
							"id":   map[string]any{"type": "string"},
							"text": map[string]any{"type": "string"},
							"tag":  map[string]any{"type": "string"},
						},
						"required": []string{"op"},
					},
				},
				"reason": map[string]any{"type": "string"},
			},
			"required": []string{"action"},
		},
	}
}
func (t *profileAgentTool) Execute(_ context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	var req struct {
		Action string             `json:"action"`
		Ops    []memory.ProfileOp `json:"ops"`
		Reason string             `json:"reason"`
	}
	if err := json.Unmarshal(params, &req); err != nil || (req.Action != "read" && req.Action != "propose") {
		return agent.AgentToolResult{}, fmt.Errorf("profile: action must be read or propose")
	}
	if req.Action == "read" {
		if t.read == nil {
			return agent.AgentToolResult{}, fmt.Errorf("profile: read is not available")
		}
		text, err := t.read(t.session)
		if err != nil {
			return agent.AgentToolResult{}, err
		}
		if text == "" {
			text = "(no profile saved)"
		}
		return agent.AgentToolResult{
			Content: []ai.ToolResultMessageContent{ai.TextContent{Text: text}},
		}, nil
	}
	if t.propose == nil {
		return agent.AgentToolResult{}, fmt.Errorf("profile: propose is not available")
	}
	if len(req.Ops) == 0 {
		return agent.AgentToolResult{}, fmt.Errorf("profile: ops are required to propose")
	}
	if strings.TrimSpace(req.Reason) == "" {
		return agent.AgentToolResult{}, fmt.Errorf("profile: reason is required")
	}
	id, err := t.propose(t.session, memory.ProfilePatch{Ops: req.Ops}, req.Reason)
	if err != nil {
		return agent.AgentToolResult{}, err
	}
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: fmt.Sprintf("proposed profile patch %d for human approval", id)}},
	}, nil
}
func (t *profileAgentTool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeSequential }

// rememberAgentTool is the Daemon remember tool as a pig ExtraTool. One
// instance serves one top-level Session: Execute grounds the quote in a User
// message of the Session and enqueues a Rule candidate for human approval.
// It never writes a Rule itself; workers, scouts, Reviewers and Run Sessions
// never see it.
type rememberAgentTool struct {
	session  string
	remember func(sessionID, text, reason, quote string) (int64, error)
}

func (t *rememberAgentTool) Name() string  { return "remember" }
func (t *rememberAgentTool) Label() string { return "propose a rule" }
func (t *rememberAgentTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{
		Name:        t.Name(),
		Description: "Propose a Rule for the User's memory bank. The text is the note to keep (at most 300 chars), the reason says why it matters, and the quote is the User's verbatim words it came from (defaults to the text). It always waits for human approval in the queue; it never applies directly. Only top-level Sessions may propose. Put Project knowledge in the nearest AGENTS.md instead.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":   map[string]any{"type": "string"},
				"reason": map[string]any{"type": "string"},
				"quote":  map[string]any{"type": "string"},
			},
			"required": []string{"text", "reason"},
		},
	}
}
func (t *rememberAgentTool) Execute(_ context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	var req struct {
		Text   string `json:"text"`
		Reason string `json:"reason"`
		Quote  string `json:"quote"`
	}
	if err := json.Unmarshal(params, &req); err != nil || strings.TrimSpace(req.Text) == "" {
		return agent.AgentToolResult{}, fmt.Errorf("remember: text is required")
	}
	if strings.TrimSpace(req.Reason) == "" {
		return agent.AgentToolResult{}, fmt.Errorf("remember: reason is required")
	}
	if t.remember == nil {
		return agent.AgentToolResult{}, fmt.Errorf("remember: propose is not available")
	}
	id, err := t.remember(t.session, req.Text, req.Reason, req.Quote)
	if err != nil {
		return agent.AgentToolResult{}, err
	}
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: fmt.Sprintf("proposed rule %d for human approval", id)}},
	}, nil
}
func (t *rememberAgentTool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeSequential }

// recallAgentTool is the Daemon recall tool as a pig ExtraTool. One instance
// serves one Session of any role: Execute searches the Session owner's bank
// and the Project notes. It never writes and never returns another User's
// notes.
type recallAgentTool struct {
	session string
	recall  func(sessionID, query string) (string, error)
}

func (t *recallAgentTool) Name() string  { return "recall" }
func (t *recallAgentTool) Label() string { return "search memory" }
func (t *recallAgentTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{
		Name:        t.Name(),
		Description: "Search the User's memory bank and the Project notes for the query. Returns matching Rules and notes, best match first, with a verify flag when the anchored code changed since the note was observed. Every role may search; the bank is always the caller's User bank, so no other User's notes ever show.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
			"required": []string{"query"},
		},
	}
}
func (t *recallAgentTool) Execute(_ context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	var req struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(params, &req); err != nil || strings.TrimSpace(req.Query) == "" {
		return agent.AgentToolResult{}, fmt.Errorf("recall: query is required")
	}
	if t.recall == nil {
		return agent.AgentToolResult{}, fmt.Errorf("recall: search is not available")
	}
	text, err := t.recall(t.session, req.Query)
	if err != nil {
		return agent.AgentToolResult{}, err
	}
	if strings.TrimSpace(text) == "" {
		text = "(no matching rules)"
	}
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: text}},
	}, nil
}
func (t *recallAgentTool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeSequential }

func (t *spawnAgentTool) Execute(_ context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	var req struct {
		Agents  []SpawnAgent `json:"agents"`
		Deliver string       `json:"deliver,omitempty"`
	}
	if err := json.Unmarshal(params, &req); err != nil || len(req.Agents) == 0 {
		return agent.AgentToolResult{}, fmt.Errorf("spawn: agents is required")
	}
	gid, kids, err := t.spawn(t.parent, req.Agents, req.Deliver)
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

func (b *PigBackend) Models() []string {
	b.mu.Lock()
	dir := b.AgentDir
	b.mu.Unlock()
	return PigAvailableModels(dir)
}

// PigAvailableModels lists the catalog models whose providers hold usable
// auth (a stored credential in <agentDir>/auth.json, an environment key, or
// an OAuth login), checked through the SDK's own CheckProviderAuth, so the
// picker never offers a model the harness cannot run. The test-faux provider
// counts as available only with PIG_TEST_FAUX=1, mirroring creation.
func PigAvailableModels(agentDir string) []string {
	creds := pigCredentials(agentDir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	authed := make(map[string]bool, len(ai.ListProviders()))
	for _, p := range ai.ListProviders() {
		authed[p] = pigProviderAuthed(ctx, p, creds)
	}
	var out []string
	for _, m := range ai.ListModels("") {
		if authed[m.Provider] {
			out = append(out, m.Provider+"/"+m.ID)
		}
	}
	sort.Strings(out)
	return out
}

// pigCredentials reads the auth store under agentDir, or an empty store when
// it cannot be read. An empty agentDir is the user's pig agent dir.
func pigCredentials(agentDir string) ai.CredentialStore {
	if agentDir == "" {
		agentDir = coding.DefaultAgentDir()
	}
	if store, err := ai.NewAuthStorage(filepath.Join(agentDir, "auth.json")); err == nil {
		return store
	}
	return emptyCredentialStore{}
}

// pigProviderAuthed reports whether the SDK can run the provider now. A nil
// check (or error) means no usable auth: no stored credential, no environment
// key, no OAuth login.
func pigProviderAuthed(ctx context.Context, provider string, creds ai.CredentialStore) bool {
	if provider == "test-faux" {
		return os.Getenv("PIG_TEST_FAUX") == "1"
	}
	auth, err := ai.BuiltinProviderAuth(provider)
	if err != nil {
		return false
	}
	check, err := ai.CheckProviderAuth(ctx, provider, auth, creds, ai.DefaultProviderAuthContext())
	if err != nil {
		return false
	}
	return check != nil
}

// emptyCredentialStore holds no credentials, so only ambient environment auth
// checks pass. It covers an unreadable auth.json without refusing the listing.
type emptyCredentialStore struct{}

func (emptyCredentialStore) Read(context.Context, string) (*ai.Credential, error) {
	return nil, nil
}
func (emptyCredentialStore) List(context.Context) ([]ai.CredentialInfo, error) {
	return nil, nil
}
func (emptyCredentialStore) Modify(_ context.Context, _ string, _ func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
	return nil, nil
}
func (emptyCredentialStore) Delete(context.Context, string) error { return nil }

// pigEfforts are the thinking levels a pig Session accepts: the levels
// pig --thinking takes.
var pigEfforts = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

func validPigEffort(e string) bool {
	if e == "" {
		return true
	}
	for _, v := range pigEfforts {
		if v == e {
			return true
		}
	}
	return false
}

// buildHandle builds a fresh SDK stack (Services, Runtime, Session) for one
// Daemon Session. It holds no lock; callers snapshot AgentDir and ExtraTools
// first. The model and thinking level come from the record.
func (b *PigBackend) buildHandle(meta SessionMeta, tools []agent.AgentTool, agentDir string) (*coding.Session, *coding.Runtime, *coding.Services, error) {
	svcs, model, err := buildPigServices(agentDir, tools, meta)
	if err != nil {
		return nil, nil, nil, err
	}
	rt, err := coding.NewRuntime(coding.RuntimeOptions{Services: svcs})
	if err != nil {
		svcs.Close()
		return nil, nil, nil, fmt.Errorf("pig: runtime: %w", err)
	}
	resources, err := ownerResourcesExtra(svcs, rolePromptFor(meta))
	if err != nil {
		_ = rt.Close()
		svcs.Close()
		return nil, nil, nil, fmt.Errorf("pig: resources: %w", err)
	}
	sess, err := rt.New(coding.SessionStartOptions{
		Model:              model,
		ThinkingLevel:      ai.ThinkingLevel(meta.Effort),
		SkipBuiltinTools:   true,
		SkipExtensionTools: true,
		ExtraTools:         tools,
		ResourceLoader:     resources,
	})
	if err != nil {
		_ = rt.Close()
		svcs.Close()
		return nil, nil, nil, fmt.Errorf("pig: session: %w", err)
	}
	return sess, rt, svcs, nil
}

// agentDirFor snapshots the pig config directory for one Session record.
func (b *PigBackend) agentDirFor(meta SessionMeta) string {
	b.mu.Lock()
	agentDir := b.AgentDir
	b.mu.Unlock()
	if meta.Dirs.pig != "" {
		agentDir = meta.Dirs.pig
	}
	return agentDir
}

// CreateSession builds the SDK stack (Services, Runtime, Session) for one
// Daemon Session and subscribes the normalized Event stream. An unknown
// model or unusable working directory returns an error for a 400.
func (b *PigBackend) CreateSession(meta SessionMeta, emit func(kind, text, status string)) error {
	if !validPigEffort(meta.Effort) {
		return fmt.Errorf("pig: unknown effort %q (want off, minimal, low, medium, high, xhigh or max)", meta.Effort)
	}
	tools := b.sessionTools(meta)
	b.mu.Lock()
	if _, ok := b.sessions[meta.ID]; ok {
		b.mu.Unlock()
		return fmt.Errorf("pig: session %s already exists", meta.ID)
	}
	b.mu.Unlock()
	sess, rt, svcs, err := b.buildHandle(meta, tools, b.agentDirFor(meta))
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.sessions[meta.ID]; ok {
		b.closePig(&pigSession{sess: sess, rt: rt, svcs: svcs, unsub: sess.Subscribe(func(agent.AgentEvent) {})})
		return fmt.Errorf("pig: session %s already exists", meta.ID)
	}
	b.subscribe(meta.ID, sess, rt, svcs, emit)
	b.sessions[meta.ID].meta = meta
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name()+"/"+fmt.Sprint(t.ExecutionMode()))
	}
	b.sessions[meta.ID].log.write("create", map[string]any{"model": meta.Model, "effort": meta.Effort, "workDir": meta.WorkDir, "scout": meta.Scout, "parent": meta.ParentID, "extraTools": names})
	return nil
}

// ownerResources loads a Session's resources from its agent dir without
// ambient skills or prompt templates. Pig reads ~/.agents/skills from HOME,
// which no owner owns. Context files from the WorkDir chain still load. extra
// carries the role's memory/DOX text (prompts.go, one place per role) as an
// appended system-prompt section; empty appends nothing (summarizer calls).
func ownerResources(svcs *coding.Services) (coding.ResourceLoader, error) {
	return ownerResourcesExtra(svcs, "")
}

// ownerResourcesExtra is ownerResources with the role extra appended to the
// system prompt. Agent Sessions pass rolePromptFor(meta); one-off calls
// (summarizer, titles) pass "".
func ownerResourcesExtra(svcs *coding.Services, extra string) (coding.ResourceLoader, error) {
	opts := coding.DefaultResourceLoaderOptions{
		CWD:               svcs.CWD(),
		AgentDir:          svcs.AgentDir(),
		SettingsManager:   svcs.SettingsManager(),
		NoSkills:          true,
		NoPromptTemplates: true,
	}
	if extra != "" {
		opts.AppendSystemPrompt = []string{extra}
	}
	loader := coding.NewDefaultResourceLoader(opts)
	if err := loader.Reload(); err != nil {
		return nil, err
	}
	return loader, nil
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
	ps := &pigSession{ctx: ctx, cancel: cancel, sess: sess, rt: rt, svcs: svcs, emit: emit, log: newPigLog(b.LogDir, id)}
	ps.lastEvent.Store(time.Now().UnixNano())
	ps.log.write("subscribe", map[string]any{"pigSession": sess.ID()})
	unsub := sess.Subscribe(func(e agent.AgentEvent) {
		ps.lastEvent.Store(time.Now().UnixNano())
		ps.log.event(e)
		switch e.(type) {
		case agent.ToolExecutionStartEvent:
			ps.tools.Add(1)
			ps.textTail.Store(false)
		case agent.ToolExecutionEndEvent:
			ps.tools.Add(-1)
		}
		for _, ne := range mapAgentEvent(e) {
			switch ne.kind {
			case "assistant_chunk":
				ps.textTail.Store(true)
			case "thinking_chunk", "tool_call_started":
				ps.textTail.Store(false)
			}
			emit(ne.kind, ne.text, ne.status)
		}
		if _, ok := e.(agent.TurnEndEvent); ok {
			// Report usage as each model turn ends, not only when the whole
			// prompt ends: a long or failed worker turn would otherwise show
			// "no usage yet" for its whole life.
			if u := sess.ContextUsage(); u != nil && u.ContextWindow > 0 && u.Tokens != nil {
				emit("usage", usageText(*u.Tokens, u.ContextWindow), "")
			}
		}
	})
	ps.unsub = unsub
	b.sessions[id] = ps
}

// OnPrompt runs one turn to completion. Chunks and tool calls already
// streamed through the subscription; this records only the status edges.
// A close mid-turn drops the status edges: the Daemon owns the ended state
// and ended is terminal there, so a late idle or failed must not regress it.
func (b *PigBackend) OnPrompt(sessionID, text string, appendEvent func(kind, text, status string)) {
	b.OnPromptImages(sessionID, text, nil, appendEvent)
}

// OnPromptImages is OnPrompt with image attachments sent as content blocks.
func (b *PigBackend) OnPromptImages(sessionID, text string, images []Image, appendEvent func(kind, text, status string)) {
	b.mu.Lock()
	ps, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok {
		appendEvent("error", "pig: unknown session", "")
		return
	}
	appendEvent("status", "", "working")
	ps.log.write("prompt", map[string]any{"chars": len(text), "images": len(images), "text": clip(text, 200)})
	var sendErr error
	var turnCtx context.Context
	var endTurn context.CancelFunc
	// A model stream can go silent mid-turn (seen with opencode-go workers:
	// one status line, then nothing). Cancel it and ask the model to
	// continue, up to pigStallRetries times, before failing the turn. The
	// retry only runs when the stalled Send returned, so the Session is not
	// wedged inside the provider stream.
	for attempt := 0; ; attempt++ {
		turnCtx, endTurn = context.WithCancel(ps.ctx)
		ps.textTail.Store(false)
		stall := watchPigStall(turnCtx, ps)
		type sendResult struct{ err error }
		done := make(chan sendResult, 1)
		sess := ps.sess
		ps.log.write("send", map[string]any{"attempt": attempt, "chars": len(text)})
		go func(prompt string, imgs []Image) {
			var err error
			if len(imgs) == 0 {
				_, err = sess.Send(turnCtx, prompt)
			} else {
				_, err = sess.SendContent(turnCtx, userBlocks(prompt, imgs))
			}
			ps.log.write("send_returned", map[string]any{"attempt": attempt, "error": fmt.Sprint(err)})
			done <- sendResult{err}
		}(text, images)
		stalled, wedged := false, false
		select {
		case r := <-done:
			sendErr = r.err
		case <-stall:
			ps.log.write("stall", map[string]any{"attempt": attempt, "textTail": ps.textTail.Load(), "toolsInFlight": ps.tools.Load(), "idleMs": time.Since(time.Unix(0, ps.lastEvent.Load())).Milliseconds()})
			// The SDK can wedge inside a provider stream and ignore its
			// context, so cancel once and stop waiting: the Send goroutine is
			// abandoned and the stack is closed when the Session releases.
			endTurn()
			if ps.textTail.Load() {
				// The model already gave its final text and the provider stream
				// never closed (PiG awaitContinuation never resolves; seen with
				// opencode-go workers). The answer is complete, so end the turn
				// as done instead of failing a finished worker.
				select {
				case <-done:
				case <-time.After(3 * time.Second):
				}
				ps.log.write("stall_tail_done", map[string]any{"attempt": attempt, "note": "stream silent after text; turn ended as done, no stop event seen"})
				sendErr = nil
				break
			}
			stalled = true
			select {
			case <-done:
				sendErr = fmt.Errorf("stalled")
			case <-time.After(3 * time.Second):
				sendErr = fmt.Errorf("pig: model stream stalled for %s with no events; turn cancelled", pigStallTimeout)
				wedged = true
			}
		}
		if (stalled || wedged) && attempt < pigStallRetries && b.alive(sessionID) {
			appendEvent("error", fmt.Sprintf("pig: model stream stalled for %s with no events; asking it to continue (%d/%d)", pigStallTimeout, attempt+1, pigStallRetries), "")
			if wedged {
				// Send never returned, so this stack is stuck inside the
				// provider stream and a prompt to it would queue behind the
				// hang. Rebuild the stack from the saved pig Session and ask
				// the fresh one to continue.
				fresh, err := b.recoverWedged(sessionID)
				if err != nil {
					ps.log.write("recover_failed", map[string]any{"error": err.Error()})
					sendErr = fmt.Errorf("pig: model stream stalled for %s and the session could not be rebuilt: %v", pigStallTimeout, err)
					break
				}
				ps = fresh
			}
			text, images = "Your previous response stopped without finishing. Continue the task from where you left off.", nil
			endTurn()
			continue
		}
		if stalled {
			sendErr = fmt.Errorf("pig: model stream stalled for %s with no events; turn cancelled", pigStallTimeout)
		}
		break
	}
	endTurn()
	if ps.interrupted.Swap(false) {
		sendErr = nil
	}
	ps.log.write("turn_end", map[string]any{"error": fmt.Sprint(sendErr), "alive": b.alive(sessionID)})
	if !b.alive(sessionID) {
		if sendErr != nil {
			appendEvent("error", sendErr.Error(), "")
		}
		return
	}
	if sendErr != nil {
		appendEvent("error", sendErr.Error(), "")
		if used, size, ok := b.ContextUsage(sessionID); ok {
			appendEvent("usage", usageText(used, size), "")
		}
		appendEvent("status", "", "failed")
		return
	}
	if used, size, ok := b.ContextUsage(sessionID); ok {
		appendEvent("usage", usageText(used, size), "")
	}
	appendEvent("status", "", "idle")
}

// pigStallTimeout is how long a working turn may go without any SDK event
// while no tool runs. The SDK can hang inside a provider stream with no
// error (seen in the field: awaitContinuation never returns), and a hung
// turn would hold its Session working and its cap slot forever.
var pigStallTimeout = 90 * time.Second

// pigTailStallTimeout is the shorter silence allowed once the model has
// already written its final text: the provider stream then just failed to close.
var pigTailStallTimeout = 20 * time.Second

// pigStallTick is how often the watchdog checks for silence. Tests shrink
// it with the two timeouts.
var pigStallTick = 5 * time.Second

// pigStallRetries is how many times a stalled turn is nudged to continue.
const pigStallRetries = 2

// watchPigStall closes the returned channel when the stream goes quiet for
// pigStallTimeout with no tool in flight. It stops when the turn context
// ends.
func watchPigStall(turn context.Context, ps *pigSession) <-chan struct{} {
	fired := make(chan struct{})
	ps.lastEvent.Store(time.Now().UnixNano())
	go func() {
		t := time.NewTicker(pigStallTick)
		defer t.Stop()
		for {
			select {
			case <-turn.Done():
				return
			case <-t.C:
				idle := time.Since(time.Unix(0, ps.lastEvent.Load()))
				limit := pigStallTimeout
				if ps.textTail.Load() {
					limit = pigTailStallTimeout
				}
				if ps.tools.Load() == 0 && idle > limit {
					close(fired)
					return
				}
			}
		}
	}()
	return fired
}

// Steer delivers text at the next tool boundary through the SDK queue.
// FollowUp delivers it after the turn. The Daemon routes working Sessions
// here and lists both through QueuedMessages; messages queued with no turn
// running buffer until the next Send. It reports whether the message queued;
// false means no live handle held it and the Daemon must hold it itself.
func (b *PigBackend) Steer(sessionID, text string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ps, ok := b.sessions[sessionID]; ok {
		ps.sess.QueueSteer(text, nil)
		return true
	}
	return false
}

// SteerImages and FollowUpImages queue with image attachments. See Steer for
// the return.
func (b *PigBackend) SteerImages(sessionID, text string, images []Image) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ps, ok := b.sessions[sessionID]; ok {
		ps.sess.QueueSteer(text, imageContents(images))
		return true
	}
	return false
}

func (b *PigBackend) FollowUpImages(sessionID, text string, images []Image) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ps, ok := b.sessions[sessionID]; ok {
		ps.sess.QueueFollowUp(text, imageContents(images))
		return true
	}
	return false
}

func imageContents(images []Image) []ai.ImageContent {
	out := make([]ai.ImageContent, 0, len(images))
	for _, img := range images {
		out = append(out, ai.ImageContent{Data: img.Data, MimeType: strings.ToLower(img.Mime)})
	}
	return out
}

// Interrupt aborts the running SDK turn.
func (b *PigBackend) Interrupt(sessionID string) bool {
	b.mu.Lock()
	ps, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok {
		return false
	}
	ps.interrupted.Store(true)
	ps.sess.RequestAbort()
	return true
}

func (b *PigBackend) FollowUp(sessionID, text string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ps, ok := b.sessions[sessionID]; ok {
		ps.sess.QueueFollowUp(text, nil)
		return true
	}
	return false
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
	// A wedged SDK stack can block Close forever: bound the wait and leave
	// the rest to a background goroutine.
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		_ = ps.sess.Close()
		_ = ps.rt.Close()
		ps.svcs.Close()
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
	}
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

// StartHandle starts a fresh SDK handle for the Session and returns its
// resume token. It implements handoffBackend: Handoff compress swaps to the
// new handle and releases the old one once the Handoff turn starts cleanly.
// The model and thinking level come from the Session record, so they survive
// the swap. Text-only queued native messages move to the new handle as
// follow-ups: a held steer degrades per the swap contract, so the Handoff
// leads. Image attachments never enter the native queue (the Daemon holds
// them in pending, which survives the swap); the SDK queue carries text only.
// The old handle stays alive until the returned release runs, so a failed swap
// leaves it untouched.
func (b *PigBackend) StartHandle(meta SessionMeta, emit func(kind, text, status string)) (string, func(), error) {
	if !validPigEffort(meta.Effort) {
		return "", nil, fmt.Errorf("pig: unknown effort %q (want off, minimal, low, medium, high, xhigh or max)", meta.Effort)
	}
	tools := b.sessionTools(meta)
	sess, rt, svcs, err := b.buildHandle(meta, tools, b.agentDirFor(meta))
	if err != nil {
		return "", nil, err
	}
	b.mu.Lock()
	old, hadOld := b.sessions[meta.ID]
	var steering, followUp []string
	if hadOld && old.sess != nil {
		steering = append([]string(nil), old.sess.GetSteeringMessages()...)
		followUp = append([]string(nil), old.sess.GetFollowUpMessages()...)
	}
	for _, text := range steering {
		sess.QueueFollowUp(text, nil)
	}
	for _, text := range followUp {
		sess.QueueFollowUp(text, nil)
	}
	b.subscribe(meta.ID, sess, rt, svcs, emit)
	b.sessions[meta.ID].meta = meta
	token := sess.ID()
	b.sessions[meta.ID].log.write("handoff", map[string]any{"pigSession": token, "model": meta.Model, "effort": meta.Effort})
	b.mu.Unlock()
	var release func()
	if hadOld {
		release = func() { b.closePig(old) }
	} else {
		release = func() {}
	}
	return token, release, nil
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
	if meta.Dirs.pig != "" {
		agentDir = meta.Dirs.pig
	}
	svcs, model, err := buildPigServices(agentDir, tools, meta)
	if err != nil {
		return err
	}
	rt, err := coding.NewRuntime(coding.RuntimeOptions{Services: svcs})
	if err != nil {
		svcs.Close()
		return fmt.Errorf("pig: runtime: %w", err)
	}
	resources, err := ownerResourcesExtra(svcs, rolePromptFor(meta))
	if err != nil {
		_ = rt.Close()
		svcs.Close()
		return fmt.Errorf("pig: resources: %w", err)
	}
	sess, err := rt.Resume(meta.ResumeToken, coding.SessionStartOptions{
		Model:              model,
		ThinkingLevel:      ai.ThinkingLevel(meta.Effort),
		SkipBuiltinTools:   true,
		SkipExtensionTools: true,
		ExtraTools:         tools,
		ResourceLoader:     resources,
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
	b.sessions[meta.ID].meta = meta
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
		}
	// Tool frames stream as partial JSON and repeat the call: only the
	// execution events carry a stable id, so only they become Events, in
	// the same {id,name,input,output,status} shape claude emits.
	case agent.ToolExecutionStartEvent:
		return []normEvent{{kind: "tool_call_started", text: pigToolJSON(e.ToolCallID, e.ToolName, string(e.Args), "", "running")}}
	case agent.ToolExecutionUpdateEvent:
		return []normEvent{{kind: "tool_call_updated", text: pigToolJSON(e.ToolCallID, e.ToolName, "", toolResultText(e.PartialResult), "running")}}
	case agent.ToolExecutionEndEvent:
		status := "completed"
		if e.IsError {
			status = "failed"
		}
		return []normEvent{{kind: "tool_call_updated", text: pigToolJSON(e.ToolCallID, e.ToolName, "", toolResultText(e.Result), status)}}
	}
	return nil
}

// pigToolJSON is the shared ToolCall Event text.
func pigToolJSON(id, name, input, output, status string) string {
	return ToolCall{ID: id, Name: name, Input: input, Output: output, Status: status}.Text()
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

// userBlocks builds the user content for a prompt with attachments: the text
// first (when any), then one image block each.
func userBlocks(text string, images []Image) []ai.UserContentBlock {
	out := make([]ai.UserContentBlock, 0, len(images)+1)
	if text != "" {
		out = append(out, ai.TextContent{Text: text})
	}
	for _, img := range images {
		out = append(out, ai.ImageContent{Data: img.Data, MimeType: strings.ToLower(img.Mime)})
	}
	return out
}

// recoverWedged replaces a stack stuck inside a provider stream. It takes the
// pig Session id from the stuck stack, closes that stack (bounded, so a
// wedged Close cannot hold the turn), and resumes the saved Session into a
// fresh one under the same Daemon id.
func (b *PigBackend) recoverWedged(id string) (*pigSession, error) {
	b.mu.Lock()
	old, ok := b.sessions[id]
	if ok {
		delete(b.sessions, id)
	}
	b.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("session closed")
	}
	tokenCh := make(chan string, 1)
	go func() { tokenCh <- old.sess.ID() }()
	var token string
	select {
	case token = <-tokenCh:
	case <-time.After(2 * time.Second):
	}
	if token == "" {
		token = old.meta.ResumeToken
	}
	if token == "" {
		b.closePig(old)
		return nil, fmt.Errorf("no saved pig Session to resume")
	}
	old.log.write("recover", map[string]any{"pigSession": token})
	b.closePig(old)
	meta := old.meta
	meta.ResumeToken = token
	if err := b.ResumeSession(meta, old.emit); err != nil {
		return nil, err
	}
	b.mu.Lock()
	fresh, ok := b.sessions[id]
	b.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("session closed during rebuild")
	}
	return fresh, nil
}
