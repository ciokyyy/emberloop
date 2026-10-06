package wick

import (
	"encoding/json"
	"sort"
	"strings"
)

// ContextCall is one tool call a transcript line makes.
type ContextCall struct {
	ID   string
	Name string
}

// ContextResult is one tool result a transcript line carries, with its
// text size in bytes.
type ContextResult struct {
	ID    string
	Name  string
	Bytes int64
}

// RequestUsage is one request's token breakdown with the model that ran
// it: the shared formula's input for that transcript line.
type RequestUsage struct {
	Model      string
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
}

// ContextEvents is one transcript line's context activity: the tool calls
// it makes, the results it carries, and the latest request's usage when the
// line reports one. One parser reports all four, so a new harness plugs in
// with a single registration.
type ContextEvents struct {
	Calls   []ContextCall
	Results []ContextResult
	Usage   *RequestUsage
}

// ContextParser reads context activity off one harness's transcript lines:
// the latest request's usage and model, the tool calls made, and the
// results received. A new harness implements this and registers it; the
// shared formula in context.go does the rest.
type ContextParser interface {
	ParseLine(line string) ContextEvents
}

var contextParsers = map[string]ContextParser{}

func init() {
	contextParsers["pig"] = pigContextParser{}
	contextParsers["claude"] = claudeContextParser{}
}

// RegisterContextParser adds a harness's context parser. Registering a
// name twice replaces the old one.
func RegisterContextParser(harness string, p ContextParser) {
	contextParsers[harness] = p
}

// ContextParserFor returns the parser for a harness, or false for one
// with no context wick reads.
func ContextParserFor(harness string) (ContextParser, bool) {
	p, ok := contextParsers[harness]
	return p, ok
}

// TopTools orders per-tool sizes largest first, ties by name, and keeps
// the first n. An empty size never ranks.
func TopTools(sizes map[string]int64, n int) []ToolSize {
	out := make([]ToolSize, 0, len(sizes))
	for name, size := range sizes {
		if name == "" || size <= 0 {
			continue
		}
		out = append(out, ToolSize{Name: name, Bytes: size})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// textSize joins text blocks the way a result reads: each block's text,
// one per line.
func textSize(parts []string) int64 {
	return int64(len(strings.Join(parts, "\n")))
}

// pigContextParser reads pig's transcript: assistant content blocks of
// type toolCall make calls, messages of role toolResult carry results
// with the call id and the tool name on them.
type pigContextParser struct{}

func (pigContextParser) ParseLine(line string) ContextEvents {
	var rec struct {
		Message *struct {
			Role       string `json:"role"`
			ToolCallID string `json:"toolCallId"`
			ToolName   string `json:"toolName"`
			Provider   string `json:"provider"`
			Model      string `json:"model"`
			Usage      *struct {
				Input      int64 `json:"input"`
				Output     int64 `json:"output"`
				CacheRead  int64 `json:"cacheRead"`
				CacheWrite int64 `json:"cacheWrite"`
			} `json:"usage"`
			Content []struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(line), &rec) != nil || rec.Message == nil {
		return ContextEvents{}
	}
	m := rec.Message
	var out ContextEvents
	if m.Role == "assistant" {
		if m.Usage != nil {
			out.Usage = &RequestUsage{Model: m.Model,
				Input: m.Usage.Input, Output: m.Usage.Output,
				CacheRead: m.Usage.CacheRead, CacheWrite: m.Usage.CacheWrite}
		}
		for _, b := range m.Content {
			if b.Type == "toolCall" {
				out.Calls = append(out.Calls, ContextCall{ID: b.ID, Name: b.Name})
			}
		}
		return out
	}
	if m.Role == "toolResult" {
		var parts []string
		for _, b := range m.Content {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		out.Results = append(out.Results, ContextResult{ID: m.ToolCallID, Name: m.ToolName, Bytes: textSize(parts)})
	}
	return out
}

// claudeContextParser reads claude's transcript: assistant content blocks
// of type tool_use make calls, user content blocks of type tool_result
// carry results keyed by the call id. A result names no tool, so the
// core matches it to the earlier call.
type claudeContextParser struct{}

func (claudeContextParser) ParseLine(line string) ContextEvents {
	var rec struct {
		Type    string `json:"type"`
		Message *struct {
			Role  string `json:"role"`
			Model string `json:"model"`
			Usage *struct {
				Input      int64 `json:"input_tokens"`
				Output     int64 `json:"output_tokens"`
				CacheRead  int64 `json:"cache_read_input_tokens"`
				CacheWrite int64 `json:"cache_creation_input_tokens"`
			} `json:"usage"`
			Content []struct {
				Type    string `json:"type"`
				ID      string `json:"id"`
				Name    string `json:"name"`
				ToolUse string `json:"tool_use_id"`
				Content any    `json:"content"`
				Text    string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(line), &rec) != nil || rec.Message == nil {
		return ContextEvents{}
	}
	var out ContextEvents
	if rec.Type == "assistant" && rec.Message.Usage != nil {
		u := rec.Message.Usage
		out.Usage = &RequestUsage{Model: rec.Message.Model,
			Input: u.Input, Output: u.Output,
			CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}
	}
	for _, b := range rec.Message.Content {
		switch b.Type {
		case "tool_use":
			if rec.Message.Role == "assistant" {
				out.Calls = append(out.Calls, ContextCall{ID: b.ID, Name: b.Name})
			}
		case "tool_result":
			out.Results = append(out.Results, ContextResult{ID: b.ToolUse, Bytes: claudeResultSize(b.Content)})
		}
	}
	return out
}

// claudeResultSize measures a tool_result payload: a plain string, or a
// list of blocks whose text fields join.
func claudeResultSize(content any) int64 {
	switch c := content.(type) {
	case string:
		return int64(len(c))
	case []any:
		var parts []string
		for _, b := range c {
			if m, _ := b.(map[string]any); m["type"] == "text" {
				if s, _ := m["text"].(string); s != "" {
					parts = append(parts, s)
				}
			}
		}
		return textSize(parts)
	}
	return 0
}
