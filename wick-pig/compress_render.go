package wick

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// RenderTranscript renders one session transcript as plain text in full:
// every user message, assistant text, tool call and result, oldest first,
// nothing truncated. The summarizer sends this text, so a handoff can quote
// code, paths, errors and ids verbatim. A new harness adds a case here
// beside its context parser.
func RenderTranscript(harness string, r io.Reader) (string, error) {
	switch harness {
	case "pig":
		return renderPigTranscript(r), nil
	case "claude":
		return renderClaudeTranscript(r), nil
	default:
		return "", fmt.Errorf("no transcript render for harness %q", harness)
	}
}

// renderPigTranscript reads pig's transcript: user text, assistant text and
// toolCall blocks with their arguments, toolResult messages with the call id
// and the tool name on them.
func renderPigTranscript(r io.Reader) string {
	var out []string
	scanLines(r, func(rec map[string]any) {
		m, _ := rec["message"].(map[string]any)
		if m == nil {
			return
		}
		role, _ := m["role"].(string)
		switch role {
		case "user":
			if text := textBlocks(m["content"]); text != "" {
				out = append(out, "[user]\n"+text)
			}
		case "assistant":
			model, _ := m["model"].(string)
			if text := textBlocks(m["content"]); text != "" {
				out = append(out, "[assistant "+model+"]\n"+text)
			}
			for _, b := range contentBlocks(m["content"]) {
				if b["type"] != "toolCall" {
					continue
				}
				id, _ := b["id"].(string)
				name, _ := b["name"].(string)
				line := "[tool " + name + " " + id + "]"
				if args := blockJSON(b, "id", "type", "name"); args != "" && args != "{}" {
					line += "\n" + args
				}
				out = append(out, line)
			}
		case "toolResult":
			callID, _ := m["toolCallId"].(string)
			name, _ := m["toolName"].(string)
			out = append(out, "[result "+name+" "+callID+"]\n"+textBlocks(m["content"]))
		}
	})
	return strings.Join(out, "\n")
}

// renderClaudeTranscript reads claude's transcript: user strings and
// tool_result blocks keyed by the call id, assistant text and tool_use blocks
// with their input.
func renderClaudeTranscript(r io.Reader) string {
	var out []string
	scanLines(r, func(rec map[string]any) {
		m, _ := rec["message"].(map[string]any)
		if m == nil {
			return
		}
		role, _ := m["role"].(string)
		model, _ := m["model"].(string)
		switch role {
		case "user":
			switch c := m["content"].(type) {
			case string:
				if strings.TrimSpace(c) != "" {
					out = append(out, "[user]\n"+c)
				}
			default:
				for _, b := range contentBlocks(m["content"]) {
					switch b["type"] {
					case "tool_result":
						id, _ := b["tool_use_id"].(string)
						out = append(out, "[result "+id+"]\n"+claudeBlockText(b["content"]))
					case "text":
						if text, _ := b["text"].(string); strings.TrimSpace(text) != "" {
							out = append(out, "[user]\n"+text)
						}
					}
				}
			}
		case "assistant":
			if text := textBlocks(m["content"]); text != "" {
				out = append(out, "[assistant "+model+"]\n"+text)
			}
			for _, b := range contentBlocks(m["content"]) {
				if b["type"] != "tool_use" {
					continue
				}
				id, _ := b["id"].(string)
				name, _ := b["name"].(string)
				line := "[tool " + name + " " + id + "]"
				if input := valueJSON(b["input"]); input != "" && input != "{}" && input != "null" {
					line += "\n" + input
				}
				out = append(out, line)
			}
		}
	})
	return strings.Join(out, "\n")
}

// scanLines calls fn for every line that decodes as a JSON object; garbage
// lines are skipped, the way the context core skips shapes it cannot read.
func scanLines(r io.Reader, fn func(map[string]any)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var rec map[string]any
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			continue
		}
		fn(rec)
	}
}

// contentBlocks is the message's content array, or nil for any other shape.
func contentBlocks(content any) []map[string]any {
	blocks, _ := content.([]any)
	var out []map[string]any
	for _, b := range blocks {
		if m, _ := b.(map[string]any); m != nil {
			out = append(out, m)
		}
	}
	return out
}

// textBlocks joins the text blocks of message content the way a result
// reads: each block's text, one per line. Both harnesses shape text the
// same way.
func textBlocks(content any) string {
	var parts []string
	for _, b := range contentBlocks(content) {
		if b["type"] == "text" {
			if s, _ := b["text"].(string); s != "" {
				parts = append(parts, s)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// claudeBlockText reads a tool_result payload: a plain string, or blocks
// whose text fields join.
func claudeBlockText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	return textBlocks(content)
}

// blockJSON encodes the block's remaining fields: the call arguments (pig)
// or the tool input (claude). skip drops the routing fields already on the
// label line.
func blockJSON(b map[string]any, skip ...string) string {
	rest := map[string]any{}
dropped:
	for k, v := range b {
		for _, s := range skip {
			if k == s {
				continue dropped
			}
		}
		rest[k] = v
	}
	if len(rest) == 0 {
		return ""
	}
	raw, err := json.Marshal(rest)
	if err != nil {
		return ""
	}
	return string(raw)
}

// valueJSON encodes one value: a tool input that arrives unwrapped.
func valueJSON(v any) string {
	if v == nil {
		return ""
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}
