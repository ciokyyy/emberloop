// Directives: deterministic script parsing for the fake LLM.
//
// The last user message drives the reply. Supported directives (case
// sensitive names, matched anywhere in the message):
//
//	ECHO:<text>            reply with <text> (rest of the line)
//	TOOL:<name>:<json>     emit a tool call of <name> with <json> args
//	                       (separate parallel calls with ";" or newlines)
//	SLOW:<ms>              delay <ms> between streamed chunks
//	HANG                   send one chunk, then hold the connection open
//	CRASHCONN              send partial bytes, then close the TCP connection
//	BIG:<kb>               reply with roughly <kb> kilobytes of text
//	FAULT:<class>[:p1[:p2]] failure injection; classes:
//	  429/500/502/503/529/401 (HTTP status; 429 carries Retry-After: 1;
//	  529 is the Anthropic overloaded_error envelope),
//	  SSE_ERROR (stream dies mid-way with a protocol error event),
//	  STALL:N:D (no bytes for D ms after N chunks, then resume),
//	  TTFB:MS (slow first byte), TRUNCATED (cut-off JSON),
//	  INVALID_SSE (non-SSE bytes), RESET (mid-stream TCP RST),
//	  EMPTY (zero content, stop), LENGTH (finish_reason length),
//	  DUPLICATE (every streamed chunk twice).
//
// Default reply is "OK". After a tool result arrives (and the new turn
// carries no TOOL: directive) the reply is "DONE:<result>".
package main

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// ToolDirective is one parsed TOOL:<name>:<args> request.
type ToolDirective struct {
	Name     string
	Args     json.RawMessage
	ArgsText string
}

// Directives is the parsed script for one turn.
type Directives struct {
	HasEcho bool
	Echo    string
	Tools   []ToolDirective
	SlowMs  int
	HasSlow bool
	Hang    bool
	Crash   bool
	BigKB   int
	HasBig  bool
	// Fault is the canonical failure-injection class, "" for none.
	Fault string
	// StallAfter/StallMs carry FAULT:STALL:N:D; TtfbMs carries FAULT:TTFB:MS.
	StallAfter int
	StallMs    int
	TtfbMs     int
	Raw        string
}

var (
	slowRe  = regexp.MustCompile(`SLOW:(\d+)`)
	bigRe   = regexp.MustCompile(`BIG:(\d+)`)
	hangRe  = regexp.MustCompile(`\bHANG\b`)
	crashRe = regexp.MustCompile(`CRASHCONN`)
	faultRe = regexp.MustCompile(`FAULT:([A-Za-z0-9_]+)(?::(\d+))?(?::(\d+))?`)
)

// ParseDirectives extracts the script from the last user message text.
func ParseDirectives(text string) Directives {
	d := Directives{Raw: text}
	if idx := strings.Index(text, "ECHO:"); idx >= 0 {
		rest := text[idx+len("ECHO:"):]
		if nl := strings.Index(rest, "\n"); nl >= 0 {
			rest = rest[:nl]
		}
		// Cut at ";" only when it starts another directive, so echo text
		// may itself contain semicolons.
		rest = cutAtDirective(rest)
		d.HasEcho = true
		d.Echo = strings.TrimSpace(rest)
	}
	parts := strings.Split(text, "\n")
	var segs []string
	for _, p := range parts {
		segs = append(segs, strings.Split(p, ";")...)
	}
	for _, s := range segs {
		s = strings.TrimSpace(s)
		ti := indexOfTool(s)
		if ti < 0 {
			continue
		}
		rest := strings.TrimSpace(s[ti+len("TOOL:"):])
		colon := strings.Index(rest, ":")
		if colon <= 0 {
			continue
		}
		name := strings.TrimSpace(rest[:colon])
		if name == "" || strings.ContainsAny(name, " \t\"'{}[]") {
			continue
		}
		argsText := strings.TrimSpace(rest[colon+1:])
		d.Tools = append(d.Tools, ToolDirective{
			Name:     name,
			Args:     canonicalArgs(argsText),
			ArgsText: argsText,
		})
	}
	if m := slowRe.FindStringSubmatch(text); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			d.SlowMs = v
			d.HasSlow = true
		}
	}
	if m := bigRe.FindStringSubmatch(text); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			d.BigKB = v
			d.HasBig = true
		}
	}
	d.Hang = hangRe.MatchString(text)
	d.Crash = crashRe.MatchString(text)
	if m := faultRe.FindStringSubmatch(text); m != nil {
		if c := canonicalFault(m[1]); c != "" {
			d.Fault = c
			switch c {
			case faultStall:
				d.StallAfter = 1
				d.StallMs = 500
				if m[2] != "" {
					if v, err := strconv.Atoi(m[2]); err == nil {
						d.StallAfter = v
					}
				}
				if m[3] != "" {
					if v, err := strconv.Atoi(m[3]); err == nil {
						d.StallMs = v
					}
				}
			case faultTTFB:
				d.TtfbMs = 300
				if m[2] != "" {
					if v, err := strconv.Atoi(m[2]); err == nil {
						d.TtfbMs = v
					}
				}
			}
		}
	}
	return d
}

func indexOfTool(s string) int {
	// Case-sensitive TOOL: marker.
	return strings.Index(s, "TOOL:")
}

// cutAtDirective truncates echo text at the next embedded directive.
func cutAtDirective(s string) string {
	for _, marker := range []string{";TOOL:", ";SLOW:", ";HANG", ";CRASHCONN", ";BIG:", " TOOL:", " SLOW:", " HANG", " CRASHCONN", " BIG:"} {
		if i := strings.Index(s, marker); i >= 0 {
			// Keep echo text before the marker; drop the separator.
			if strings.HasPrefix(marker, ";") {
				s = s[:i]
			} else {
				s = s[:i]
			}
		}
	}
	return s
}

func canonicalArgs(argsText string) json.RawMessage {
	t := strings.TrimSpace(argsText)
	if t == "" {
		return json.RawMessage(`{}`)
	}
	if json.Valid([]byte(t)) {
		var v any
		if err := json.Unmarshal([]byte(t), &v); err == nil {
			if enc, err := json.Marshal(v); err == nil {
				return json.RawMessage(enc)
			}
		}
		return json.RawMessage(t)
	}
	enc, _ := json.Marshal(map[string]string{"_text": t})
	return json.RawMessage(enc)
}

// ReplyText resolves the deterministic text reply for a turn that emits no
// tool calls. toolResults are the decoded tool outputs seen in this request.
func (d Directives) ReplyText(toolResults []string) string {
	if len(d.Tools) == 0 && len(toolResults) > 0 {
		return "DONE:" + strings.Join(toolResults, ";")
	}
	if d.HasBig {
		return bigReply(d.BigKB)
	}
	if d.HasEcho {
		return d.Echo
	}
	return "OK"
}

// bigReply builds a roughly kb-kilobyte deterministic payload.
func bigReply(kb int) string {
	if kb < 0 {
		kb = 0
	}
	const maxKB = 8192
	if kb > maxKB {
		kb = maxKB
	}
	const chunk = "0123456789abcdef"
	need := kb * 1024
	var b strings.Builder
	b.Grow(need + 16)
	b.WriteString("BIG:")
	for b.Len() < need {
		b.WriteString(chunk)
	}
	return b.String()[:need]
}
