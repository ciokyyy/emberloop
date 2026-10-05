package antigravity

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

const (
	defaultProject   = "rising-fact-p41fc"
	providerID       = "google-antigravity"
	trailingUsageTTL = time.Second
)

var sessions = newSessionStore()

// ---- request ----

// request carries its fields in the order the agy CLI sends them.
type request struct {
	Contents          []any             `json:"contents"`
	SystemInstruction any               `json:"systemInstruction,omitempty"`
	Tools             []any             `json:"tools,omitempty"`
	ToolConfig        any               `json:"toolConfig,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	GenerationConfig  any               `json:"generationConfig,omitempty"`
	SessionID         string            `json:"sessionId,omitempty"`
}

type envelope struct {
	Project     string  `json:"project"`
	RequestID   string  `json:"requestId"`
	Request     request `json:"request"`
	Model       string  `json:"model"`
	UserAgent   string  `json:"userAgent"`
	RequestType string  `json:"requestType"`
}

func sessionKey(transcript, options map[string]any) string {
	if id := asStr(options["sessionId"]); id != "" {
		return id
	}
	if msgs, _ := transcript["messages"].([]any); len(msgs) > 0 {
		if ts := asMap(msgs[0])["timestamp"]; ts != nil {
			return fmt.Sprintf("message:%v", ts)
		}
	}
	return "__default__"
}

func numberOpt(v any) (int, bool) {
	f, ok := v.(float64)
	return int(f), ok
}

func buildEnvelope(model, transcript, options map[string]any, project string) (envelope, string) {
	id := asStr(model["id"])
	wire, tiered := resolveWireModel(id, asStr(options["reasoning"]))
	if r, _ := model["reasoning"].(bool); !r {
		tiered = false
	}
	req := request{Contents: convertMessages(sliceOf(transcript["messages"]), asStr(model["provider"]), id)}
	req.Tools = convertTools(sliceOf(transcript["tools"]))
	if sp := strings.TrimSpace(asStr(transcript["systemPrompt"])); sp != "" {
		req.SystemInstruction = map[string]any{"parts": []any{map[string]any{"text": sanitize(asStr(transcript["systemPrompt"]))}}}
	}
	if len(req.Tools) > 0 {
		req.ToolConfig = map[string]any{"functionCallingConfig": map[string]any{"mode": "VALIDATED"}}
	}
	gen := map[string]any{}
	if tiered {
		gen["thinkingConfig"] = map[string]any{"includeThoughts": true, "thinkingBudget": wire.Budget}
	}
	if n, ok := numberOpt(options["maxTokens"]); ok && n > 0 {
		gen["maxOutputTokens"] = n
	} else if n, ok := numberOpt(model["maxTokens"]); ok && n > 0 {
		gen["maxOutputTokens"] = n
	}
	if len(gen) > 0 {
		req.GenerationConfig = gen
	}
	key := sessionKey(transcript, options)
	sess, ts := sessions.begin(key)
	requestID, labels := agentMetadata(sess, req.Contents, wire.Name, ts)
	req.Labels = labels
	req.SessionID = sess.numericSessionID
	if project == "" {
		project = defaultProject
	}
	return envelope{Project: project, RequestID: requestID, Request: req, Model: wire.Name, UserAgent: "antigravity", RequestType: "agent"}, key
}

func sliceOf(v any) []any { s, _ := v.([]any); return s }

// ---- response ----

type chunk struct {
	Candidates []struct {
		Content struct {
			Parts []part `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Usage *struct {
		Prompt   *int `json:"promptTokenCount"`
		Cached   *int `json:"cachedContentTokenCount"`
		Output   *int `json:"candidatesTokenCount"`
		Thoughts *int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
	Error          json.RawMessage `json:"error"`
	PromptFeedback *struct {
		BlockReason  string `json:"blockReason"`
		BlockMessage string `json:"blockReasonMessage"`
	} `json:"promptFeedback"`
}

type part struct {
	Text             string `json:"text"`
	Thought          bool   `json:"thought"`
	ThoughtSignature string `json:"thoughtSignature"`
	FunctionCall     *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Args any    `json:"args"`
	} `json:"functionCall"`
}

// readSSE sends each decoded chunk on out and closes it at end of body. The
// Antigravity envelope wraps every chunk in {"response": ...}.
func readSSE(body io.Reader, out chan<- chunk, errc chan<- error) {
	defer close(out)
	r := bufio.NewReaderSize(body, 64*1024)
	var data []string
	flush := func() {
		if len(data) == 0 {
			return
		}
		payload := strings.TrimSpace(strings.Join(data, "\n"))
		data = nil
		if payload == "" || payload == "[DONE]" {
			return
		}
		var wrapped struct {
			Response json.RawMessage `json:"response"`
		}
		raw := []byte(payload)
		if json.Unmarshal(raw, &wrapped) == nil && len(wrapped.Response) > 0 {
			raw = wrapped.Response
		}
		var c chunk
		if json.Unmarshal(raw, &c) == nil {
			out <- c
		}
	}
	for {
		line, err := r.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimSpace(line[5:]))
		}
		if err != nil {
			flush()
			if err != io.EOF {
				errc <- err
			}
			return
		}
	}
}

func embeddedFailure(c chunk) string {
	if len(c.Error) > 0 && string(c.Error) != "null" {
		var e struct {
			Code    any    `json:"code"`
			Status  string `json:"status"`
			Message string `json:"message"`
		}
		if json.Unmarshal(c.Error, &e) != nil {
			return "Antigravity stream error: " + string(c.Error)
		}
		var labels []string
		if e.Status != "" {
			labels = append(labels, e.Status)
		}
		if e.Code != nil {
			labels = append(labels, fmt.Sprintf("code %v", e.Code))
		}
		s := "Antigravity stream error"
		if len(labels) > 0 {
			s += " (" + strings.Join(labels, ", ") + ")"
		}
		if e.Message != "" {
			s += ": " + e.Message
		}
		return s
	}
	if f := c.PromptFeedback; f != nil && (f.BlockReason != "" || f.BlockMessage != "") {
		s := "Antigravity prompt blocked"
		if f.BlockReason != "" {
			s += " (" + f.BlockReason + ")"
		}
		if f.BlockMessage != "" {
			s += ": " + f.BlockMessage
		}
		return s
	}
	return ""
}

// ---- assistant message under construction ----

type block struct {
	kind, text, sig, id, name string
	args                      any
}

type message struct {
	provider, model, api string
	blocks               []*block
	input, output        int
	cacheRead            int
	stopReason           string
	errMsg               string
	ts                   int64
}

func (m *message) snapshot() map[string]any {
	content := make([]any, 0, len(m.blocks))
	for _, b := range m.blocks {
		switch b.kind {
		case "text":
			e := map[string]any{"type": "text", "text": b.text}
			if b.sig != "" {
				e["textSignature"] = b.sig
			}
			content = append(content, e)
		case "thinking":
			e := map[string]any{"type": "thinking", "thinking": b.text}
			if b.sig != "" {
				e["thinkingSignature"] = b.sig
			}
			content = append(content, e)
		case "toolCall":
			content = append(content, m.toolCall(b))
		}
	}
	out := map[string]any{
		"role": "assistant", "content": content, "api": m.api, "provider": m.provider, "model": m.model,
		"usage": map[string]any{
			"input": m.input, "output": m.output, "cacheRead": m.cacheRead, "cacheWrite": 0,
			"totalTokens": m.input + m.output + m.cacheRead,
			"cost":        map[string]any{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0, "total": 0},
		},
		"stopReason": m.stopReason, "timestamp": m.ts,
	}
	if m.errMsg != "" {
		out["errorMessage"] = m.errMsg
	}
	return out
}

func (m *message) toolCall(b *block) map[string]any {
	e := map[string]any{"type": "toolCall", "id": b.id, "name": b.name, "arguments": b.args}
	if b.sig != "" {
		e["thoughtSignature"] = b.sig
	}
	return e
}

func (m *message) applyUsage(c chunk) {
	u := c.Usage
	if u == nil {
		return
	}
	if u.Cached != nil {
		m.cacheRead = *u.Cached
	}
	prompt := m.input + m.cacheRead
	if u.Prompt != nil {
		prompt = *u.Prompt
	}
	m.input = max(0, prompt-m.cacheRead)
	if u.Output != nil || u.Thoughts != nil {
		o := 0
		if u.Output != nil {
			o += *u.Output
		}
		if u.Thoughts != nil {
			o += *u.Thoughts
		}
		m.output = o
	}
}

// ---- the streamSimple handler ----

// goContext turns the SDK's request context into a context.Context.
func goContext(c sdk.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-c.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func streamSimple(c sdk.Context, model, transcript, options map[string]any) (*sdk.ModelEventStream, error) {
	stream := sdk.CreateAssistantMessageEventStream()
	ctx, cancel := goContext(c)
	go func() {
		defer cancel()
		run(ctx, stream, model, transcript, options)
	}()
	return stream, nil
}

func run(ctx context.Context, stream *sdk.ModelEventStream, model, transcript, options map[string]any) {
	msg := &message{
		provider: asStr(model["provider"]), model: asStr(model["id"]), api: asStr(model["api"]),
		stopReason: "stop", ts: time.Now().UnixMilli(),
	}
	push := func(e map[string]any) { stream.Push(e) }
	push(map[string]any{"type": "start", "partial": msg.snapshot()})

	fail := func(err error) {
		msg.stopReason, msg.errMsg = "error", err.Error()
		if ctx.Err() != nil {
			msg.stopReason = "aborted"
		}
		push(map[string]any{"type": "error", "reason": msg.stopReason, "error": msg.snapshot()})
	}

	token, project := parseAPIKey(asStr(options["apiKey"]))
	if token == "" {
		fail(errors.New("missing Antigravity OAuth access token"))
		return
	}
	env, key := buildEnvelope(model, transcript, options, project)
	body, err := json.Marshal(env)
	if err != nil {
		fail(err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cloudCodeBase+"/v1internal:streamGenerateContent?alt=sse", bytes.NewReader(body))
	if err != nil {
		fail(err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", harnessUserAgent())
	resp, err := httpClient.Do(req)
	if err != nil {
		fail(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		fail(fmt.Errorf("Antigravity request failed: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(b))))
		return
	}

	chunks, errc := make(chan chunk), make(chan error, 1)
	go readSSE(resp.Body, chunks, errc)

	var textIdx, thinkIdx = -1, -1
	var pendingSig, terminalReason string
	terminal := false
	gptOSS := strings.Contains(strings.ToLower(msg.model), "gpt-oss")
	var trailing <-chan time.Time

	endBlock := func(idx *int, kind, endType string) {
		if *idx == -1 {
			return
		}
		push(map[string]any{"type": endType, "contentIndex": *idx, "content": msg.blocks[*idx].text, "partial": msg.snapshot()})
		*idx = -1
	}
	closeText := func() { endBlock(&textIdx, "text", "text_end") }
	closeThink := func() { endBlock(&thinkIdx, "thinking", "thinking_end") }

loop:
	for {
		var c chunk
		var ok bool
		select {
		case <-ctx.Done():
			fail(errors.New("Request was aborted"))
			return
		case <-trailing:
			break loop
		case c, ok = <-chunks:
			if !ok {
				break loop
			}
		}
		if f := embeddedFailure(c); f != "" {
			fail(errors.New(f))
			return
		}
		msg.applyUsage(c)
		if terminal {
			if c.Usage != nil {
				break
			}
			continue
		}
		if len(c.Candidates) == 0 {
			continue
		}
		cand := c.Candidates[0]
		for _, p := range cand.Content.Parts {
			if !p.Thought && p.FunctionCall == nil && p.Text == "" && p.ThoughtSignature != "" {
				if textIdx != -1 {
					msg.blocks[textIdx].sig = p.ThoughtSignature
				} else {
					pendingSig = p.ThoughtSignature
				}
				continue
			}
			if p.Thought && p.Text == "" && p.ThoughtSignature != "" {
				pendingSig = p.ThoughtSignature
			}
			if fc := p.FunctionCall; fc != nil {
				closeText()
				closeThink()
				sig := p.ThoughtSignature
				if sig == "" {
					sig = pendingSig
				}
				pendingSig = ""
				id := fc.ID
				if id == "" {
					id = "call_" + uuid4()
				}
				args := fc.Args
				if args == nil {
					args = map[string]any{}
				}
				b := &block{kind: "toolCall", id: id, name: fc.Name, args: args, sig: sig}
				msg.blocks = append(msg.blocks, b)
				idx := len(msg.blocks) - 1
				push(map[string]any{"type": "toolcall_start", "contentIndex": idx, "partial": msg.snapshot()})
				push(map[string]any{"type": "toolcall_end", "contentIndex": idx, "toolCall": msg.toolCall(b), "partial": msg.snapshot()})
				msg.stopReason = "toolUse"
				continue
			}
			if p.Thought {
				if p.Text == "" {
					continue
				}
				closeText()
				if thinkIdx == -1 {
					msg.blocks = append(msg.blocks, &block{kind: "thinking", sig: p.ThoughtSignature})
					thinkIdx = len(msg.blocks) - 1
					push(map[string]any{"type": "thinking_start", "contentIndex": thinkIdx, "partial": msg.snapshot()})
				}
				b := msg.blocks[thinkIdx]
				b.text += p.Text
				if p.ThoughtSignature != "" {
					b.sig = p.ThoughtSignature
				}
				push(map[string]any{"type": "thinking_delta", "contentIndex": thinkIdx, "delta": p.Text, "partial": msg.snapshot()})
				continue
			}
			if p.Text != "" {
				closeThink()
				if textIdx == -1 {
					sig := p.ThoughtSignature
					if sig == "" {
						sig = pendingSig
					}
					pendingSig = ""
					msg.blocks = append(msg.blocks, &block{kind: "text", sig: sig})
					textIdx = len(msg.blocks) - 1
					push(map[string]any{"type": "text_start", "contentIndex": textIdx, "partial": msg.snapshot()})
				}
				b := msg.blocks[textIdx]
				b.text += p.Text
				if p.ThoughtSignature != "" {
					b.sig = p.ThoughtSignature
				}
				push(map[string]any{"type": "text_delta", "contentIndex": textIdx, "delta": p.Text, "partial": msg.snapshot()})
			}
		}
		if cand.FinishReason != "" {
			closeText()
			closeThink()
			if msg.stopReason != "toolUse" && cand.FinishReason == "MAX_TOKENS" {
				msg.stopReason = "length"
			}
			terminal, terminalReason = true, cand.FinishReason
			if gptOSS && c.Usage == nil {
				// gpt-oss reports usage in a trailing chunk; wait briefly for it.
				trailing = time.After(trailingUsageTTL)
				continue
			}
			break
		}
	}
	select {
	case err := <-errc:
		if !terminal {
			fail(err)
			return
		}
	default:
	}
	if ctx.Err() != nil {
		fail(errors.New("Request was aborted"))
		return
	}
	if !terminal {
		fail(errors.New("Antigravity stream ended without a terminal candidate response"))
		return
	}
	if len(msg.blocks) == 0 {
		fail(fmt.Errorf("Antigravity returned an empty response (%s)", terminalReason))
		return
	}
	if msg.stopReason == "stop" || msg.stopReason == "length" {
		sessions.complete(key)
	}
	push(map[string]any{"type": "done", "reason": msg.stopReason, "message": msg.snapshot()})
}
