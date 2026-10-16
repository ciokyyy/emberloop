// COOPERATE mode: fuzzy natural-language tool calling.
//
// When enabled (--cooperate flag, X-Fake-Cooperate header, or
// FAKELLM_COOPERATE/FAKE_COOPERATE env), a prompt with no script directives
// that mentions an available tool triggers exactly one call to the
// best-matching tool. After the tool result arrives the reply is
// DONE:<result>, same as the TOOL: directive loop.
//
// Matching rules (documented for benchmark authors):
//  1. Case-insensitive everywhere: "Echo", "ECHO" and "echo" are equal.
//  2. Namespace-agnostic: tool names are split on any run of non-alphanumeric
//     characters ("-", "_", ".", ":", "/") into segments; the match key is
//     the normalised full name plus its last segment (the base). So the
//     mention "echo" matches "echo", "acpbench-stub_echo",
//     "mcp__acpbench_stub__echo", "acpbench_stub__echo",
//     "acpbench-stub__echo" and "functions.echo" (all have base "echo").
//  3. A tool matches when the lowercased prompt contains the full tool name,
//     the normalised tool name, or the base segment as a whole word or
//     substring. Full-name hits outrank normalised hits, which outrank
//     base-segment hits; ties break towards the longest tool name.
//  4. "slow" is special: when the prompt contains "slow" (any case) the
//     best slow-named tool wins regardless of other mentions, so "run the
//     slow tool for 200ms" calls the slow tool with ms 200.
//  5. Fire-once: if any tool result is already present in the conversation
//     history, no further cooperate call is emitted; the reply is
//     DONE:<result>. Multi-turn conversations therefore cannot loop.
//
// Default args: string-ish schema fields get "hello" (text, message, input,
// prompt, query, cmd, path, ...); ms-ish numeric fields (ms, millis,
// duration, delay, sleep, timeout, wait) get 100; other numbers get 1;
// booleans get false. Without a schema the args are {"text":"hello"}.
// The slow tool always gets {"ms": N} with N from the first integer in the
// prompt or 5000.
package main

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var coopIntRe = regexp.MustCompile(`\d+`)

// normTool lowers and unifies separators: '-', '.', ':', '/' become '_',
// runs of '_' collapse to one.
func normTool(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevU := false
	for _, r := range s {
		var c rune
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			c = r
		default:
			c = '_'
		}
		if c == '_' {
			if prevU {
				continue
			}
			prevU = true
		} else {
			prevU = false
		}
		b.WriteRune(c)
	}
	return strings.Trim(b.String(), "_")
}

// toolBase is the last segment after splitting on non-alphanumerics.
func toolBase(name string) string {
	n := normTool(name)
	if i := strings.LastIndex(n, "_"); i >= 0 {
		return n[i+1:]
	}
	return n
}

// matchCooperate finds the best available tool for a natural-language prompt.
// Returns "" when nothing matches.
func matchCooperate(prompt string, toolNames []string) string {
	lower := strings.ToLower(prompt)
	if len(toolNames) == 0 {
		return ""
	}
	// Slow preference: a "slow" prompt goes to the best slow tool.
	if strings.Contains(lower, "slow") {
		best := ""
		for _, tn := range toolNames {
			ln := strings.ToLower(tn)
			if strings.Contains(ln, "slow") || toolBase(tn) == "slow" {
				if len(tn) > len(best) {
					best = tn
				}
			}
		}
		if best != "" {
			return best
		}
	}
	best, bestScore := "", -1
	normPrompt := normTool(prompt)
	for _, tn := range toolNames {
		score := -1
		ln := strings.ToLower(tn)
		nn := normTool(tn)
		base := toolBase(tn)
		switch {
		case tn != "" && strings.Contains(lower, ln):
			score = 300 + len(tn)
		case nn != "" && strings.Contains(normPrompt, nn):
			score = 200 + len(nn)
		case base != "" && containsWord(lower, base):
			score = 100 + len(base)
		case base != "" && strings.Contains(normPrompt, base):
			score = 50 + len(base)
		}
		if score > bestScore {
			best, bestScore = tn, score
		}
	}
	return best
}

// containsWord reports whether base appears as a whole word in lower.
func containsWord(lower, base string) bool {
	if base == "" {
		return false
	}
	for _, tok := range strings.FieldsFunc(lower, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if tok == base {
			return true
		}
	}
	return false
}

// schemaProps extracts {"properties": {...}} from a tool definition that may
// carry function.parameters, input_schema, or parameters wrappers.
func schemaProps(raw json.RawMessage) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	for _, wrap := range []string{"function", "input_schema", "parameters"} {
		_ = wrap
	}
	// Unwrap {"function": {"parameters": {...}}} etc.
	var probes []json.RawMessage
	probes = append(probes, raw)
	if f, ok := m["function"]; ok {
		probes = append(probes, f)
		var fm map[string]json.RawMessage
		if json.Unmarshal(f, &fm) == nil {
			if p, ok := fm["parameters"]; ok {
				probes = append(probes, p)
			}
		}
	}
	if p, ok := m["input_schema"]; ok {
		probes = append(probes, p)
	}
	if p, ok := m["parameters"]; ok {
		probes = append(probes, p)
	}
	for _, pr := range probes {
		var o struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(pr, &o); err == nil && len(o.Properties) > 0 {
			return o.Properties
		}
	}
	return nil
}

// cooperateArgs builds default JSON args for the chosen tool.
func cooperateArgs(toolName string, props map[string]json.RawMessage, prompt string) string {
	// Slow tool: {"ms": N}.
	if strings.Contains(strings.ToLower(toolName), "slow") || toolBase(toolName) == "slow" {
		ms := 5000
		if m := coopIntRe.FindString(prompt); m != "" {
			if v, err := strconv.Atoi(m); err == nil {
				ms = v
			}
		}
		enc, _ := json.Marshal(map[string]any{"ms": ms})
		return string(enc)
	}
	if len(props) > 0 {
		args := map[string]any{}
		for name, praW := range props {
			ln := strings.ToLower(name)
			var pt struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(praW, &pt)
			switch pt.Type {
			case "integer", "number":
				switch {
				case strings.Contains(ln, "ms") || strings.Contains(ln, "millis") ||
					strings.Contains(ln, "duration") || strings.Contains(ln, "delay") ||
					strings.Contains(ln, "sleep") || strings.Contains(ln, "timeout") ||
					strings.Contains(ln, "wait"):
					args[name] = 100
				default:
					args[name] = 1
				}
			case "boolean":
				args[name] = false
			default:
				args[name] = "hello"
			}
		}
		if enc, err := json.Marshal(args); err == nil {
			return string(enc)
		}
	}
	return `{"text":"hello"}`
}
