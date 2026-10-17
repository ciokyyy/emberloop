package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// The Daemon-hosted tools for the single el MCP server. Groups, delivery and
// scout policy live in the web Daemon, so under it the el server serves these
// thin HTTP proxies instead of owning anything: spawn, steer, check, cancel
// and submit, bound to one Session through the environment (the same two
// variables the sudo gate reads). They register only for the web roles that
// allow them (see el's role matrix) and only when the binding is set; without
// it the roles resolve exactly as they do for the herdr harness. A proxy is
// never script-callable: submitting, spawning, steering, peeking or stopping
// from inside execute would bypass the harness that owns the turn, so
// NewExecute skips every DaemonProxy.

// WebDaemonBinding is the Daemon one el-mcp process calls back: the loopback
// URL and the Session id every call acts under (itself for submit, the parent
// for spawn, the recorded parent for steer, check and cancel).
func WebDaemonBinding() (daemonURL, sessionID string, ok bool) {
	daemonURL, sessionID = os.Getenv(SudoGateDaemonEnv), os.Getenv(SudoGateSessionEnv)
	if daemonURL == "" || sessionID == "" {
		return "", "", false
	}
	return strings.TrimSuffix(daemonURL, "/"), sessionID, true
}

// DaemonProxy is implemented by the Daemon-hosted tools. NewExecute refuses
// them to scripts; the MCP server still serves them as plain tools.
type DaemonProxy interface{ DaemonProxy() bool }

// IsDaemonProxy reports whether t is hosted by the web Daemon rather than
// implemented here.
func IsDaemonProxy(t Tool) bool {
	p, ok := t.(DaemonProxy)
	return ok && p.DaemonProxy()
}

// WebProxyTool returns the Daemon proxy tool called name, or nil for anything
// else. The tool reads its binding from the environment on every call.
func WebProxyTool(name string) Tool {
	switch name {
	case "spawn":
		return webProxy{name: "spawn", desc: "Spawn Subagents that work in parallel and report back. Each agent takes a prompt, an optional name for its card, and a type (worker or scout); scouts are read-only. Deliver selects the parent notices: 'group' (the default) sends one grouped result when the last member settles, 'each' sends one notice per child as it settles. Returned ids can be steered with the steer tool, peeked at with check, or stopped with cancel.", schema: webProxySchema(map[string]any{
			"deliver": map[string]any{"type": "string", "enum": []string{"group", "each"}},
			"agents": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":       "object",
					"properties": map[string]any{"prompt": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"}, "type": map[string]any{"type": "string"}},
					"required":   []string{"prompt"},
				},
			},
		}, "agents"), run: webProxySpawn}
	case "steer":
		return webProxy{name: "steer", desc: "Message a Subagent you spawned by its Session id. A running worker is steered at its next safe step without interrupting your turn; an idle one starts as a follow-up; a released one resumes first. Only your own children; scouts are read-only. See also check (peek without messaging) and cancel (stop a turn).", schema: webProxySchema(map[string]any{
			"id":      map[string]any{"type": "string"},
			"message": map[string]any{"type": "string"},
		}, "id", "message"), run: webProxySteer}
	case "check":
		return webProxy{name: "check", desc: "Peek at Subagents you spawned without messaging them: with no id lists your own children, with an id shows one child's status plus its recent events. Results already arrive as one grouped result, so do not poll; use check only to peek. Only your own children.", schema: webProxySchema(map[string]any{
			"id": map[string]any{"type": "string"},
		}), readOnly: true, run: webProxyCheck}
	case "cancel":
		return webProxy{name: "cancel", desc: "Stop a Subagent you spawned: interrupts its current turn without touching your own; kill also ends its Session. Only your own children; scouts can be stopped but not messaged. Prefer check to see what it is doing first.", schema: webProxySchema(map[string]any{
			"id":   map[string]any{"type": "string"},
			"kill": map[string]any{"type": "boolean"},
		}, "id"), run: webProxyCancel}
	case "submit":
		// One tool, two shapes: workers send status/completed/report and
		// Dispositions, reviewers send verdict/tip/ran/findings. No field is
		// required at the schema level because neither shape's required fields
		// fit the other; Execute below and the Daemon route validate each shape.
		// The reviewer properties mirror the pig reviewer submit ExtraTool schema
		// and ValidateReviewerSubmit in emberloop-web: the three live in different
		// modules (el core never imports the web Daemon), so a schema change must
		// be made in each.
		return webProxy{name: "submit", desc: "Submit your result to the Daemon when the work is complete: a worker reports done with completed checkbox texts and dispositions for findings (blocker/major dispositions are required); a reviewer reports verdict pass or changes with tip, ran and findings. Every turn ends with a submit; a turn that ends without one is re-prompted.", schema: webProxySchema(map[string]any{
			"status":       map[string]any{"type": "string"},
			"completed":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"report":       map[string]any{"type": "string"},
			"verdict":      map[string]any{"type": "string", "enum": []string{"pass", "changes"}},
			"tip":          map[string]any{"type": "string", "description": "Reviewer only: the full 40-character commit SHA you judged. Never a summary or finding text; put those in findings."},
			"ran":          map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}, "exitCode": map[string]any{"type": "integer"}}, "required": []string{"command"}}},
			"findings":     map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"file": map[string]any{"type": "string"}, "line": map[string]any{"type": "integer"}, "severity": map[string]any{"type": "string", "enum": []string{"blocker", "major", "minor"}}, "problem": map[string]any{"type": "string"}}, "required": []string{"file", "severity", "problem"}}},
			"dispositions": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"findingId": map[string]any{"type": "string"}, "disposition": map[string]any{"type": "string", "enum": []string{"fixed", "declined", "deferred"}}, "reason": map[string]any{"type": "string"}, "ticket": map[string]any{"type": "string"}}, "required": []string{"findingId", "disposition"}}},
		}), run: webProxySubmit}
	case "profile":
		return webProxy{name: "profile", desc: "Read your standing Profile (memory://profile), or propose a patch to it. A patch is add, edit or retire operations by line ID and always waits for human approval in the settings view; it never applies directly. Call propose only after the User explicitly asks to change the Profile.", schema: webProxySchema(map[string]any{
			"action": map[string]any{"type": "string", "enum": []string{"read", "propose"}},
			"ops": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":       "object",
					"properties": map[string]any{"op": map[string]any{"type": "string"}, "id": map[string]any{"type": "string"}, "text": map[string]any{"type": "string"}, "tag": map[string]any{"type": "string"}},
					"required":   []string{"op"},
				},
			},
			"reason": map[string]any{"type": "string"},
		}, "action"), run: webProxyProfile}
	case "remember":
		return webProxy{name: "remember", desc: "Propose a Rule for the User's memory bank. The text is the note to keep (at most 300 chars), the reason says why it matters, and the quote is the User's verbatim words it came from (defaults to the text). It always waits for human approval in the queue; it never applies directly. Only top-level Sessions may propose; workers, scouts, Reviewers and Run Sessions are refused. Put Project knowledge in the nearest AGENTS.md instead.", schema: webProxySchema(map[string]any{
			"text":   map[string]any{"type": "string"},
			"reason": map[string]any{"type": "string"},
			"quote":  map[string]any{"type": "string"},
		}, "text", "reason"), run: webProxyRemember}
	case "recall":
		return webProxy{name: "recall", desc: "Search the User's memory bank and Project notes. Returns matching Rules, notes, observations and Memories. Superseded Memories are excluded unless include_superseded is true. Every role searches only the caller's User bank.", schema: webProxySchema(map[string]any{
			"query":              map[string]any{"type": "string"},
			"include_superseded": map[string]any{"type": "boolean", "description": "Include superseded Memories in results; off by default."},
		}, "query"), readOnly: true, run: webProxyRecall}
	}
	return nil
}

func webProxySchema(properties map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// webProxy is one Daemon-hosted tool: its binding comes from the environment
// on every call, so a long-lived server follows a re-exec unchanged.
type webProxy struct {
	name, desc string
	schema     map[string]any
	readOnly   bool
	run        func(ctx context.Context, daemonURL, sessionID string, args json.RawMessage) (string, error)
}

func (t webProxy) Name() string           { return t.name }
func (t webProxy) Description() string    { return t.desc }
func (t webProxy) Schema() map[string]any { return t.schema }
func (t webProxy) ReadOnly() bool         { return t.readOnly }
func (t webProxy) DaemonProxy() bool      { return true }

func (t webProxy) Execute(ctx context.Context, _ string, args json.RawMessage) (string, error) {
	daemonURL, sessionID, ok := WebDaemonBinding()
	if !ok {
		return "", fmt.Errorf("%s: not bound to a Daemon session (%s and %s are required)", t.name, SudoGateSessionEnv, SudoGateDaemonEnv)
	}
	return t.run(ctx, daemonURL, sessionID, args)
}

// webProxyClient bounds every Daemon round-trip: the calls run on the MCP
// server's goroutines, but a hung Daemon must not hang a tool call forever.
var webProxyClient = &http.Client{Timeout: 60 * time.Second}

func webProxyDo(ctx context.Context, req *http.Request) (*http.Response, error) {
	if tok := os.Getenv(SudoGateTokenEnv); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return webProxyClient.Do(req.WithContext(ctx))
}

func webProxyPost(ctx context.Context, url, ctype string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	return webProxyDo(ctx, req)
}

func webProxyGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return webProxyDo(ctx, req)
}

func webProxyDelete(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}
	return webProxyDo(ctx, req)
}

// webProxySpawn creates one Subagent Session per requested agent under the
// bound parent. Over the cap the whole call is refused with no half group.
func webProxySpawn(ctx context.Context, daemonURL, parentID string, args json.RawMessage) (string, error) {
	var req struct {
		Agents []struct {
			Prompt string `json:"prompt"`
			Name   string `json:"name,omitempty"`
			Type   string `json:"type,omitempty"`
		} `json:"agents"`
		Deliver string `json:"deliver,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("spawn: bad arguments")
	}
	if len(req.Agents) == 0 {
		return "", fmt.Errorf("spawn: agents is required")
	}
	body, _ := json.Marshal(map[string]any{"agents": req.Agents, "deliver": req.Deliver})
	resp, err := webProxyPost(ctx, daemonURL+"/api/sessions/"+parentID+"/spawn", "application/json", body)
	if err != nil {
		return "", fmt.Errorf("spawn: %s", err.Error())
	}
	defer resp.Body.Close()
	var out struct {
		Group    string `json:"group"`
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
		Err string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusCreated {
		if out.Err == "" {
			out.Err = fmt.Sprintf("spawn refused: %d", resp.StatusCode)
		}
		return "", fmt.Errorf("spawn: %s", out.Err)
	}
	ids := make([]string, 0, len(out.Sessions))
	for _, s := range out.Sessions {
		ids = append(ids, s.ID)
	}
	return "spawned group " + out.Group + ": " + strings.Join(ids, " "), nil
}

// webProxySteer messages one Subagent through the same HTTP routes the UI
// uses: a working child takes steer, an idle or released one takes follow-up
// and starts (resuming first when parked). Only the recorded parent may
// message a child; scouts stay read-only.
func webProxySteer(ctx context.Context, daemonURL, parentID string, args json.RawMessage) (string, error) {
	var req struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("steer: bad arguments")
	}
	childID, text := strings.TrimSpace(req.ID), strings.TrimSpace(req.Message)
	if childID == "" || text == "" {
		return "", fmt.Errorf("steer: id and message are required")
	}
	meta, code, msg := webProxySessionMeta(ctx, daemonURL, childID)
	if code != 0 {
		return "", fmt.Errorf("steer: %s", msg)
	}
	if meta.ParentID != parentID {
		return "", fmt.Errorf("steer: not a child of this session")
	}
	if meta.Scout {
		return "", fmt.Errorf("steer: scout sessions are read-only")
	}
	if meta.Status == "ended" {
		return "", fmt.Errorf("steer: session ended")
	}
	mode := "followup"
	if meta.Status == "working" {
		mode = "steer"
	}
	body, _ := json.Marshal(map[string]any{"text": req.Message})
	resp, err := webProxyPost(ctx, daemonURL+"/api/sessions/"+childID+"/"+mode, "application/json", body)
	if err != nil {
		return "", fmt.Errorf("steer: %s", err.Error())
	}
	defer resp.Body.Close()
	var out struct {
		Seq      int64  `json:"seq"`
		Queued   bool   `json:"queued"`
		Position int    `json:"position"`
		Err      string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusAccepted {
		if out.Err == "" {
			out.Err = fmt.Sprintf("steer refused: %d", resp.StatusCode)
		}
		return "", fmt.Errorf("steer: %s", out.Err)
	}
	switch {
	case !out.Queued && mode == "steer":
		return fmt.Sprintf("started steer on %s (seq %d)", childID, out.Seq), nil
	case !out.Queued:
		return fmt.Sprintf("started follow-up on %s (seq %d)", childID, out.Seq), nil
	case mode == "followup":
		return fmt.Sprintf("queued as follow-up on %s (position %d)", childID, out.Position), nil
	case webProxySteerDegraded(ctx, daemonURL, childID, req.Message):
		return fmt.Sprintf("queued as follow-up on %s (position %d)", childID, out.Position), nil
	default:
		if out.Seq != 0 {
			return fmt.Sprintf("steered %s (queued natively)", childID), nil
		}
		return fmt.Sprintf("steered %s (queued at position %d)", childID, out.Position), nil
	}
}

// webProxyCheck peeks at the bound parent's own Subagents through the same
// session GET routes the UI uses: no id lists every child, an id shows one
// child's summary plus its recent events. Only the recorded parent may peek.
func webProxyCheck(ctx context.Context, daemonURL, parentID string, args json.RawMessage) (string, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("check: bad arguments")
	}
	if id := strings.TrimSpace(req.ID); id != "" {
		meta, events, live, code, msg := webProxySessionFull(ctx, daemonURL, id)
		if code != 0 {
			return "", fmt.Errorf("check: %s", msg)
		}
		if meta.ParentID != parentID {
			return "", fmt.Errorf("check: not a child of this session")
		}
		var b strings.Builder
		b.WriteString(webProxySummarizeChild(meta, events, live))
		for _, l := range webProxyRecentEventLines(events, 20) {
			b.WriteByte('\n')
			b.WriteString(l)
		}
		return b.String(), nil
	}
	kids, code, msg := webProxyChildren(ctx, daemonURL, parentID)
	if code != 0 {
		return "", fmt.Errorf("check: %s", msg)
	}
	if len(kids) == 0 {
		return "no subagents", nil
	}
	var b strings.Builder
	for i, k := range kids {
		if i > 0 {
			b.WriteByte('\n')
		}
		meta, events, live, code, msg := webProxySessionFull(ctx, daemonURL, k.ID)
		if code != 0 {
			return "", fmt.Errorf("check: %s", msg)
		}
		b.WriteString(webProxySummarizeChild(meta, events, live))
	}
	return b.String(), nil
}

// webProxyCancel stops the bound parent's own Subagent through the same
// interrupt and end routes the UI uses: the child's turn stops and its queue
// drops without touching the parent turn, and kill ends its Session like
// DELETE. Only the recorded parent may stop a child; scouts may be stopped,
// since a stop is not a message.
func webProxyCancel(ctx context.Context, daemonURL, parentID string, args json.RawMessage) (string, error) {
	var req struct {
		ID   string `json:"id"`
		Kill bool   `json:"kill"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("cancel: bad arguments")
	}
	childID := strings.TrimSpace(req.ID)
	if childID == "" {
		return "", fmt.Errorf("cancel: id is required")
	}
	meta, code, msg := webProxySessionMeta(ctx, daemonURL, childID)
	if code != 0 {
		return "", fmt.Errorf("cancel: %s", msg)
	}
	if meta.ParentID != parentID {
		return "", fmt.Errorf("cancel: not a child of this session")
	}
	if meta.Status == "ended" {
		return "", fmt.Errorf("cancel: session ended")
	}
	stopped, code, msg := webProxyInterrupt(ctx, daemonURL, childID)
	if code != 0 {
		return "", fmt.Errorf("cancel: %s", msg)
	}
	if !req.Kill {
		if stopped {
			return fmt.Sprintf("interrupted %s", childID), nil
		}
		return fmt.Sprintf("%s already idle", childID), nil
	}
	if code, msg := webProxyEndSession(ctx, daemonURL, childID); code != 0 {
		return "", fmt.Errorf("cancel: %s", msg)
	}
	return fmt.Sprintf("killed %s", childID), nil
}

// webProxySubmit records the bound Session's own result through the same
// submit route Runs use: a worker reports done with completed checkbox texts
// and Dispositions, a reviewer sends verdict pass|changes with tip, ran and findings.
func webProxySubmit(ctx context.Context, daemonURL, sessionID string, args json.RawMessage) (string, error) {
	var req struct {
		Status       string           `json:"status"`
		Completed    []string         `json:"completed,omitempty"`
		Report       string           `json:"report,omitempty"`
		Verdict      string           `json:"verdict,omitempty"`
		Tip          string           `json:"tip,omitempty"`
		Ran          []map[string]any `json:"ran,omitempty"`
		Findings     []map[string]any `json:"findings,omitempty"`
		Dispositions []map[string]any `json:"dispositions,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("submit: bad arguments")
	}
	if strings.TrimSpace(req.Verdict) != "" {
		body, _ := json.Marshal(map[string]any{"verdict": req.Verdict, "tip": req.Tip, "ran": req.Ran, "findings": req.Findings})
		resp, err := webProxyPost(ctx, daemonURL+"/api/sessions/"+sessionID+"/submit", "application/json", body)
		if err != nil {
			return "", fmt.Errorf("submit: %s", err.Error())
		}
		defer resp.Body.Close()
		var out struct {
			Submits    int    `json:"submits"`
			Verdict    string `json:"verdict"`
			Stale      bool   `json:"stale"`
			CurrentTip string `json:"currentTip"`
			Err        string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode != http.StatusAccepted {
			if out.Err == "" {
				out.Err = fmt.Sprintf("submit refused: %d", resp.StatusCode)
			}
			return "", fmt.Errorf("submit: %s", out.Err)
		}
		status := req.Verdict
		if out.Verdict != "" {
			status = out.Verdict
		}
		msg := fmt.Sprintf("submitted %s (submit %d)", status, out.Submits)
		if out.Stale {
			if out.CurrentTip != "" {
				msg += fmt.Sprintf(" stale: tip is behind the branch tip (current %s); judge the current tip", out.CurrentTip)
			} else {
				msg += " stale: tip is behind the branch tip"
			}
		}
		return msg, nil
	}
	if strings.TrimSpace(req.Status) == "" {
		return "", fmt.Errorf("submit: status is required")
	}
	body, _ := json.Marshal(map[string]any{"status": req.Status, "completed": req.Completed, "report": req.Report, "dispositions": req.Dispositions})
	resp, err := webProxyPost(ctx, daemonURL+"/api/sessions/"+sessionID+"/submit", "application/json", body)
	if err != nil {
		return "", fmt.Errorf("submit: %s", err.Error())
	}
	defer resp.Body.Close()
	var out struct {
		Submits int    `json:"submits"`
		Err     string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusAccepted {
		if out.Err == "" {
			out.Err = fmt.Sprintf("submit refused: %d", resp.StatusCode)
		}
		return "", fmt.Errorf("submit: %s", out.Err)
	}
	return fmt.Sprintf("submitted %s (submit %d)", req.Status, out.Submits), nil
}

// webProxyProfile reads memory://profile or proposes a patch to it, through
// the same Session-bound routes the UI uses. Read returns the rendered
// Profile (filtered for Reviewers); propose always creates a queue item for
// human approval, never a version, and only after an explicit User request.
func webProxyProfile(ctx context.Context, daemonURL, sessionID string, args json.RawMessage) (string, error) {
	var req struct {
		Action string `json:"action"`
		Ops    []struct {
			Op   string `json:"op"`
			ID   string `json:"id,omitempty"`
			Text string `json:"text,omitempty"`
			Tag  string `json:"tag,omitempty"`
		} `json:"ops"`
		Reason string `json:"reason,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("profile: bad arguments")
	}
	switch req.Action {
	case "read":
		resp, err := webProxyGet(ctx, daemonURL+"/api/sessions/"+sessionID+"/profile")
		if err != nil {
			return "", fmt.Errorf("profile: %s", err.Error())
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			if msg := strings.TrimSpace(string(raw)); msg != "" {
				return "", fmt.Errorf("profile: %s", msg)
			}
			return "", fmt.Errorf("profile: read refused: %d", resp.StatusCode)
		}
		var out struct {
			Rendered string `json:"rendered"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return "", fmt.Errorf("profile: bad session response")
		}
		if strings.TrimSpace(out.Rendered) == "" {
			return "(no profile saved)", nil
		}
		return out.Rendered, nil
	case "propose":
		if len(req.Ops) == 0 {
			return "", fmt.Errorf("profile: ops are required to propose")
		}
		if strings.TrimSpace(req.Reason) == "" {
			return "", fmt.Errorf("profile: reason is required")
		}
		body, _ := json.Marshal(map[string]any{"ops": req.Ops, "reason": req.Reason})
		resp, err := webProxyPost(ctx, daemonURL+"/api/sessions/"+sessionID+"/profile/propose", "application/json", body)
		if err != nil {
			return "", fmt.Errorf("profile: %s", err.Error())
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusCreated {
			if msg := strings.TrimSpace(string(raw)); msg != "" {
				return "", fmt.Errorf("profile: %s", msg)
			}
			return "", fmt.Errorf("profile: refused: %d", resp.StatusCode)
		}
		var out struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return "", fmt.Errorf("profile: bad session response")
		}
		return fmt.Sprintf("proposed profile patch %d for human approval", out.ID), nil
	default:
		return "", fmt.Errorf("profile: action must be read or propose")
	}
}

// webProxyRemember proposes a Rule through the Session-bound remember route:
// the Daemon grounds the quote in a User message of the Session, checks the
// per-Session rate limit, and enqueues a Rule candidate for human approval.
// It never writes a Rule directly.
func webProxyRemember(ctx context.Context, daemonURL, sessionID string, args json.RawMessage) (string, error) {
	var req struct {
		Text   string `json:"text"`
		Reason string `json:"reason"`
		Quote  string `json:"quote,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("remember: bad arguments")
	}
	if strings.TrimSpace(req.Text) == "" {
		return "", fmt.Errorf("remember: text is required")
	}
	if strings.TrimSpace(req.Reason) == "" {
		return "", fmt.Errorf("remember: reason is required")
	}
	body, _ := json.Marshal(map[string]any{"text": req.Text, "reason": req.Reason, "quote": req.Quote})
	resp, err := webProxyPost(ctx, daemonURL+"/api/sessions/"+sessionID+"/remember", "application/json", body)
	if err != nil {
		return "", fmt.Errorf("remember: %s", err.Error())
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		if msg := strings.TrimSpace(string(raw)); msg != "" {
			return "", fmt.Errorf("remember: %s", msg)
		}
		return "", fmt.Errorf("remember: refused: %d", resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("remember: bad session response")
	}
	return fmt.Sprintf("proposed rule %d for human approval", out.ID), nil
}

// webProxyRecall searches the caller's User bank and the Project notes
// through the Session-bound recall route. Every role may search; the Daemon
// scopes the bank to the Session owner, so no other User's notes ever show.
func webProxyRecall(ctx context.Context, daemonURL, sessionID string, args json.RawMessage) (string, error) {
	var req struct {
		Query             string `json:"query"`
		IncludeSuperseded bool   `json:"include_superseded"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", fmt.Errorf("recall: bad arguments")
	}
	if strings.TrimSpace(req.Query) == "" {
		return "", fmt.Errorf("recall: query is required")
	}
	queryURL := daemonURL + "/api/sessions/" + sessionID + "/recall?query=" + url.QueryEscape(req.Query)
	if req.IncludeSuperseded {
		queryURL += "&include_superseded=true"
	}
	resp, err := webProxyGet(ctx, queryURL)
	if err != nil {
		return "", fmt.Errorf("recall: %s", err.Error())
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if msg := strings.TrimSpace(string(raw)); msg != "" {
			return "", fmt.Errorf("recall: %s", msg)
		}
		return "", fmt.Errorf("recall: refused: %d", resp.StatusCode)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("recall: bad session response")
	}
	if strings.TrimSpace(out.Text) == "" {
		return "(no matching rules)", nil
	}
	return out.Text, nil
}

// webProxySession is the small session metadata the Daemon's session routes
// answer with, enough to enforce the recorded-parent rule.
type webProxySession struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	ParentID  string    `json:"parentId"`
	Scout     bool      `json:"scout"`
	CreatedAt time.Time `json:"createdAt"`
}

// webProxyEvent is one session log event as the session GET route answers.
type webProxyEvent struct {
	Seq    int64  `json:"seq"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Status string `json:"status"`
	Mode   string `json:"mode"`
}

// webProxySessionMeta reads one session the way the UI does.
func webProxySessionMeta(ctx context.Context, daemonURL, id string) (webProxySession, int, string) {
	resp, err := webProxyGet(ctx, daemonURL+"/api/sessions/"+id)
	if err != nil {
		return webProxySession{}, -32000, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return webProxySession{}, http.StatusNotFound, "unknown session " + id
		}
		return webProxySession{}, resp.StatusCode, fmt.Sprintf("session lookup refused: %d", resp.StatusCode)
	}
	var out struct {
		Session webProxySession `json:"session"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return webProxySession{}, resp.StatusCode, "bad session response"
	}
	return out.Session, 0, ""
}

// webProxySessionFull reads one session with its events and liveness, the way
// the UI does.
func webProxySessionFull(ctx context.Context, daemonURL, id string) (webProxySession, []webProxyEvent, bool, int, string) {
	resp, err := webProxyGet(ctx, daemonURL+"/api/sessions/"+id)
	if err != nil {
		return webProxySession{}, nil, false, -32000, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return webProxySession{}, nil, false, http.StatusNotFound, "unknown session " + id
		}
		return webProxySession{}, nil, false, resp.StatusCode, fmt.Sprintf("session lookup refused: %d", resp.StatusCode)
	}
	var out struct {
		Session webProxySession `json:"session"`
		Events  []webProxyEvent `json:"events"`
		Live    bool            `json:"live"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return webProxySession{}, nil, false, resp.StatusCode, "bad session response"
	}
	return out.Session, out.Events, out.Live, 0, ""
}

// webProxyChildren lists the bound parent's own Subagents through the session
// list, oldest first.
func webProxyChildren(ctx context.Context, daemonURL, parentID string) ([]webProxySession, int, string) {
	resp, err := webProxyGet(ctx, daemonURL+"/api/sessions")
	if err != nil {
		return nil, -32000, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Sprintf("session list refused: %d", resp.StatusCode)
	}
	var all []webProxySession
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil, resp.StatusCode, "bad session list response"
	}
	var kids []webProxySession
	for _, m := range all {
		if m.ParentID == parentID {
			kids = append(kids, m)
		}
	}
	sort.Slice(kids, func(i, j int) bool { return kids[i].CreatedAt.Before(kids[j].CreatedAt) })
	return kids, 0, ""
}

// webProxyInterrupt stops one session's turn through the interrupt route,
// reporting whether a turn was running.
func webProxyInterrupt(ctx context.Context, daemonURL, id string) (bool, int, string) {
	resp, err := webProxyPost(ctx, daemonURL+"/api/sessions/"+id+"/interrupt", "application/json", nil)
	if err != nil {
		return false, -32000, err.Error()
	}
	defer resp.Body.Close()
	var out struct {
		Interrupted bool   `json:"interrupted"`
		Err         string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusOK {
		if out.Err == "" {
			out.Err = fmt.Sprintf("interrupt refused: %d", resp.StatusCode)
		}
		return false, resp.StatusCode, out.Err
	}
	return out.Interrupted, 0, ""
}

// webProxyEndSession ends one session through the same DELETE the UI serves.
func webProxyEndSession(ctx context.Context, daemonURL, id string) (int, string) {
	req, err := http.NewRequest(http.MethodDelete, daemonURL+"/api/sessions/"+id, nil)
	if err != nil {
		return -32000, err.Error()
	}
	resp, err := webProxyDo(ctx, req)
	if err != nil {
		return -32000, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Sprintf("end refused: %d", resp.StatusCode)
	}
	return 0, ""
}

// webProxySteerDegraded reports whether a queued steer landed as a follow-up
// (a claude steer the adapter could not inject degrades at delivery time).
func webProxySteerDegraded(ctx context.Context, daemonURL, childID, text string) bool {
	resp, err := webProxyGet(ctx, daemonURL+"/api/sessions/"+childID+"/queue")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var out struct {
		Queue []struct {
			Mode string `json:"mode"`
			Text string `json:"text"`
		} `json:"queue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false
	}
	for i := len(out.Queue) - 1; i >= 0; i-- {
		if out.Queue[i].Text == text {
			return out.Queue[i].Mode == "followup"
		}
	}
	return false
}

// webProxySummarizeChild renders one peek line: id, kind, status, the running
// tool and token count when the log shows them, and the last assistant reply
// tail.
func webProxySummarizeChild(meta webProxySession, events []webProxyEvent, live bool) string {
	kind := "worker"
	if meta.Scout {
		kind = "scout"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s): status=%s", webProxyShortID(meta.ID), kind, webProxyDisplayStatus(meta, live))
	if tool := webProxyCurrentTool(events); tool != "" {
		fmt.Fprintf(&b, " tool=%s", tool)
	}
	if toks, ok := webProxyTokens(events); ok {
		fmt.Fprintf(&b, " tokens=%d", toks)
	}
	if tail := webProxyAssistantTail(events, 300); tail != "" {
		fmt.Fprintf(&b, " reply=%s", tail)
	}
	return b.String()
}

// webProxyDisplayStatus names the peek status: ended wins, a session holding
// no agent process is released, otherwise the recorded turn status.
func webProxyDisplayStatus(meta webProxySession, live bool) string {
	if meta.Status == "ended" {
		return "ended"
	}
	if !live {
		return "released"
	}
	return meta.Status
}

// webProxyCurrentTool names the tool still running: the newest tool_call
// started whose id no later tool_call update completed or failed. Anything
// unparseable or settled reports nothing.
func webProxyCurrentTool(events []webProxyEvent) string {
	done := map[string]bool{}
	for _, e := range events {
		if e.Kind != "tool_call_updated" {
			continue
		}
		var tc struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(e.Text), &tc) != nil {
			continue
		}
		if tc.Status == "completed" || tc.Status == "failed" {
			done[tc.ID] = true
		}
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind != "tool_call_started" {
			continue
		}
		var tc struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal([]byte(events[i].Text), &tc) != nil || tc.Name == "" {
			return ""
		}
		if !done[tc.ID] {
			return tc.Name
		}
		return ""
	}
	return ""
}

// webProxyTokens reports the newest usage count in the log, false when none.
func webProxyTokens(events []webProxyEvent) (int64, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind != "usage" {
			continue
		}
		var u struct {
			Used int64 `json:"used"`
		}
		if json.Unmarshal([]byte(events[i].Text), &u) != nil {
			return 0, false
		}
		return u.Used, true
	}
	return 0, false
}

// webProxyAssistantTail joins the assistant chunks after the last prompt and
// keeps the last n characters, so a peek shows how the child answered.
func webProxyAssistantTail(events []webProxyEvent, n int) string {
	start := 0
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == "prompt" {
			start = i + 1
			break
		}
	}
	var b strings.Builder
	for _, e := range events[start:] {
		if e.Kind == "assistant_chunk" {
			b.WriteString(e.Text)
		}
	}
	tail := strings.Join(strings.Fields(b.String()), " ")
	if len(tail) > n {
		tail = tail[len(tail)-n:]
	}
	return tail
}

// webProxyRecentEventLines renders the last n events, one compact line each.
func webProxyRecentEventLines(events []webProxyEvent, n int) []string {
	if len(events) > n {
		events = events[len(events)-n:]
	}
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, webProxyEventLine(e))
	}
	return out
}

// webProxyEventLine compacts one event: its seq, kind, delivery mode when set,
// and the text flattened to one line (status events show their status).
func webProxyEventLine(e webProxyEvent) string {
	text := strings.Join(strings.Fields(e.Text), " ")
	if e.Kind == "status" {
		text = e.Status
	}
	if len(text) > 120 {
		text = text[:120] + "…"
	}
	if e.Mode != "" {
		return fmt.Sprintf("#%d %s/%s %s", e.Seq, e.Kind, e.Mode, text)
	}
	return fmt.Sprintf("#%d %s %s", e.Seq, e.Kind, text)
}

func webProxyShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
