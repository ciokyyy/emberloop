package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// Handoff compress: the Daemon writes one Handoff document from the Event log
// (the summarizer, with a deterministic fallback; see summarize.go), starts a
// fresh harness handle for the same Session, sends the Handoff as its first
// prompt, and releases the old handle once that turn starts cleanly. The
// Session ID, Event log and metadata stay.

// handoffBackend is the seam a Backend implements to swap its handle.
type handoffBackend interface {
	// StartHandle starts a fresh handle for the Session and returns its
	// resume token. The returned release drops the old handle.
	StartHandle(meta SessionMeta, emit func(kind, text, status string)) (token string, release func(), err error)
}

// HandoffEvent is the JSON Text of a "compressed" Event. Retries counts the
// summarizer repair retries. TransientRetries and StartRetries count retries
// of network, 5xx and rate-limit failures (compress_retry.go).
type HandoffEvent struct {
	Number           int    `json:"number"`
	TokensBefore     int    `json:"tokensBefore"`
	Model            string `json:"model"` // compress model that wrote the Handoff
	Fallback         string `json:"fallback"`
	Retries          int    `json:"retries"`
	TransientRetries int    `json:"transientRetries"`
	StartRetries     int    `json:"startRetries"`
	Error            string `json:"error,omitempty"`
}

func (h HandoffEvent) text() string {
	b, _ := json.Marshal(h)
	return string(b)
}

// CompressBlockedEvent is the JSON Text of a "compress_blocked" Event: a compress
// stopped on a credit or auth failure. Action names the Retry action that the
// Event and the banner both offer.
type CompressBlockedEvent struct {
	Number           int    `json:"number"`
	Model            string `json:"model"`
	TransientRetries int    `json:"transientRetries"`
	StartRetries     int    `json:"startRetries"`
	Error            string `json:"error"`
	Action           string `json:"action"`
}

func (c CompressBlockedEvent) text() string {
	b, _ := json.Marshal(c)
	return string(b)
}

// handoffFor returns the Session's Backend when it swaps handles.
func (d *Daemon) handoffFor(id string) (handoffBackend, *session) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return nil, nil
	}
	hb, _ := d.backendForLocked(s.meta.Harness).(handoffBackend)
	return hb, s
}

const (
	handoffPromptsKept = 3
	handoffFieldCap    = 4000
)

// handoffHeader opens every Handoff text.
const handoffHeader = "# Handoff\n\nThis Session continues from an earlier context. Read this, then carry on.\n"

func capText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return headOf(s, n) + "\n[cut]"
}

// handoffPrompt is the text of a prompt Event the Handoff carries, or false
// when it carries none. Every prompt but a Handoff counts, whatever its sender:
// a Worker, Reviewer or Subagent task is a system prompt, and the Task must name
// it. A system prompt loses its fenced memory blocks, so an injected block alone
// carries nothing.
func handoffPrompt(e Event) (string, bool) {
	if e.Kind != "prompt" || e.Handoff {
		return "", false
	}
	text := e.Text
	if e.Sender != senderUser {
		text = dropMemoryBlocks(text)
	}
	if strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}

// buildHandoff writes the deterministic Handoff: the first prompt the Handoff
// carries as the Task, the last ones, files touched, last assistant message and
// open tool errors (see handoffPrompt).
func buildHandoff(events []Event) string {
	var prompts []string
	files := map[string]bool{}
	var fileOrder []string
	var toolErrs []string
	errIdx := map[string]int{} // tool id -> index in toolErrs, -1 once resolved
	last := ""
	inReply := false
	for _, e := range events {
		if e.Kind != "assistant_chunk" {
			if e.Kind != "usage" && e.Kind != "status" {
				inReply = false
			}
		}
		switch e.Kind {
		case "prompt":
			if text, ok := handoffPrompt(e); ok {
				prompts = append(prompts, text)
			}
		case "assistant_chunk":
			if !inReply {
				last = ""
				inReply = true
			}
			last += e.Text
		case "tool_call_started", "tool_call_updated":
			var tc ToolCall
			if json.Unmarshal([]byte(e.Text), &tc) != nil {
				continue
			}
			var in map[string]any
			if json.Unmarshal([]byte(tc.Input), &in) == nil {
				for _, k := range []string{"path", "file_path", "filePath"} {
					if p, ok := in[k].(string); ok && p != "" && !files[p] {
						files[p] = true
						fileOrder = append(fileOrder, p)
					}
				}
			}
			if tc.Status == "failed" {
				toolErrs = append(toolErrs, tc.Name+": "+capText(tc.Output, 300))
				if tc.ID != "" {
					errIdx[tc.ID] = len(toolErrs) - 1
				}
			} else if i, ok := errIdx[tc.ID]; ok && tc.ID != "" && i >= 0 {
				toolErrs[i] = ""
				errIdx[tc.ID] = -1
			}
		}
	}
	var b strings.Builder
	b.WriteString(handoffHeader)
	if len(prompts) > 0 {
		b.WriteString("\n## Task\n" + capText(prompts[0], handoffFieldCap) + "\n")
		start := len(prompts) - handoffPromptsKept
		if start < 1 {
			start = 1
		}
		if start < len(prompts) {
			b.WriteString("\n## Recent user prompts\n")
			for _, p := range prompts[start : len(prompts)-1] {
				b.WriteString("- " + capText(p, 1000) + "\n")
			}
		}
		if len(prompts) > 1 {
			b.WriteString("\n## Last user prompt (verbatim)\n" + prompts[len(prompts)-1] + "\n")
		}
	}
	if len(fileOrder) > 0 {
		b.WriteString("\n## Files touched\n")
		for _, f := range fileOrder {
			b.WriteString("- " + f + "\n")
		}
	}
	if last != "" {
		b.WriteString("\n## Last assistant message\n" + capText(dropMemoryBlocks(last), handoffFieldCap) + "\n")
	}
	header := false
	for _, e := range toolErrs {
		if e == "" {
			continue
		}
		if !header {
			b.WriteString("\n## Failed tool calls\n")
			header = true
		}
		b.WriteString("- " + e + "\n")
	}
	return b.String()
}

// lastUsedTokens reads the newest usage Event's used count.
func lastUsedTokens(events []Event) int {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind != "usage" {
			continue
		}
		var u struct {
			Used int `json:"used"`
		}
		if json.Unmarshal([]byte(events[i].Text), &u) == nil {
			return u.Used
		}
	}
	return 0
}

// runHandoff swaps the Session's handle, then runs the Handoff as the first
// turn on the new handle through the shared turn path. That turn skips the
// submit gate, auto-compress and the Reviewer verdict hook; Group delivery
// and finishTurn still run. The caller marked the Session busy; runHandoff
// settles it. The summarizer writes the body; writeHandoff picks the fallback step.
func (d *Daemon) runHandoff(id string, hb handoffBackend, model, instructions string) {
	settled := false
	defer func() {
		if !settled {
			d.appendEvent(id, "status", "", "idle")
			d.finishTurn(id)
			d.maybeDeliverGroup(id)
			d.runDeferredCompress(id)
		}
	}()

	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return
	}
	events := append([]Event(nil), s.events...)
	meta := s.meta
	d.mu.Unlock()
	dirs, err := d.ownerDirsFor(meta.Owner)
	if err != nil {
		d.appendEvent(id, "error", "compress failed: "+err.Error(), "")
		return
	}
	meta.Dirs = dirs

	number := 1
	for _, e := range events {
		if e.Kind == "compressed" {
			number++
		}
	}
	before := lastUsedTokens(events)
	d.appendEvent(id, "status", "", "working")
	note := func(reason string) { d.appendEvent(id, "compress_step", reason, "") }
	rs := &retryingSummarizer{next: d.summarizer, wait: d.backoffWait()}
	res := writeHandoff(rs, model, instructions, events, note)
	res.transient = rs.retries
	if res.blocked {
		d.blockCompress(id, CompressBlockedEvent{Number: number, Model: model, TransientRetries: res.transient, Error: res.err})
		return
	}
	text := res.text

	emit := func(kind, text, status string) { d.appendEvent(id, kind, text, status) }
	h := startHandle(meta, handleCompress)
	// Compress behaves like create: Profile, fresh recall, then the Handoff.
	if pb := d.profileBlockFor(meta); len(pb) > 0 {
		h.inject = append(pb, h.inject...)
	}
	var token string
	var release func()
	startRetries, err := retryTransient(func() error {
		var e error
		token, release, e = hb.StartHandle(meta, emit)
		return e
	}, d.backoffWait())
	if err != nil {
		if classifyCompressError(err) == failBlock {
			d.blockCompress(id, CompressBlockedEvent{Number: number, Model: model, TransientRetries: res.transient, StartRetries: startRetries, Error: err.Error()})
			return
		}
		d.appendEvent(id, "error", "compress failed: "+err.Error(), "")
		return
	}
	// The copy is best effort: a full disk never blocks the swap.
	_ = os.WriteFile(filepath.Join(d.sessionDir(id), fmt.Sprintf("handoff-%d.md", number)), []byte(text), 0o600)

	d.mu.Lock()
	if cur, ok := d.sessions[id]; ok {
		cur.meta.ResumeToken = token
		// A successful swap clears a block: held messages go on after the Handoff.
		cur.meta.CompressBlock = ""
		d.persistMeta(cur.meta)
		cur.turnsSinceCompress = 0
		cur.inject = h.inject
	}
	d.mu.Unlock()
	d.appendEvent(id, "compressed", HandoffEvent{Number: number, TokensBefore: before, Model: model, Fallback: res.fallback, Retries: res.retries, TransientRetries: res.transient, StartRetries: startRetries, Error: res.err}.text(), "")

	d.mu.Lock()
	d.appendEventExtraLocked(id, "prompt", text, "", "", 0, nil, func(e *Event) {
		e.Handoff = true
		systemSender.stamp(e)
	})
	be := d.backendForLocked(meta.Harness)
	d.mu.Unlock()

	// The old handle goes as soon as the first turn on the new one works. A
	// turn that ends without ever working releases it too, unless it reported
	// an error: then the old handle stays. An error after working changes
	// nothing: the old handle is already gone.
	var once sync.Once
	releaseOld := func() {
		once.Do(func() {
			if release != nil {
				release()
			}
		})
	}
	var failed atomic.Bool
	settled = true
	d.runTurnWith(id, be, text, nil, true, func(kind, _, status string) {
		switch {
		case kind == "status" && status == "working":
			releaseOld()
		case kind == "error":
			failed.Store(true)
		}
	})
	if !failed.Load() {
		releaseOld()
	}
}
