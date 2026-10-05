package antigravity

import "strings"

func asMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
func asStr(v any) string         { s, _ := v.(string); return s }
func toLower(s string) string    { return strings.ToLower(s) }
func hasPrefix(s, p string) bool { return strings.HasPrefix(s, p) }

// sanitize replaces invalid UTF-8 (the wire form of a lone surrogate).
func sanitize(s string) string { return strings.ToValidUTF8(s, "�") }

func userParts(content any) []any {
	if s, ok := content.(string); ok {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		return []any{map[string]any{"text": sanitize(s)}}
	}
	var parts []any
	items, _ := content.([]any)
	for _, it := range items {
		m := asMap(it)
		switch asStr(m["type"]) {
		case "text":
			if t := asStr(m["text"]); t != "" {
				parts = append(parts, map[string]any{"text": sanitize(t)})
			}
		case "image":
			if d := asStr(m["data"]); d != "" {
				parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": asStr(m["mimeType"]), "data": d}})
			}
		}
	}
	return parts
}

// assistantParts converts an assistant message. Signed history (thinking and
// signatures) is only replayed to the model that produced it.
func assistantParts(msg map[string]any, signed bool) []any {
	var parts []any
	blocks, _ := msg["content"].([]any)
	for _, b := range blocks {
		m := asMap(b)
		switch asStr(m["type"]) {
		case "thinking":
			if t := asStr(m["thinking"]); signed && t != "" {
				p := map[string]any{"text": sanitize(t), "thought": true}
				if sig := asStr(m["thinkingSignature"]); sig != "" {
					p["thoughtSignature"] = sig
				}
				parts = append(parts, p)
			}
		case "text":
			if t := asStr(m["text"]); strings.TrimSpace(t) != "" {
				p := map[string]any{"text": sanitize(t)}
				if sig := asStr(m["textSignature"]); signed && sig != "" {
					p["thoughtSignature"] = sig
				}
				parts = append(parts, p)
			}
		case "toolCall":
			args := m["arguments"]
			if args == nil {
				args = map[string]any{}
			}
			p := map[string]any{"functionCall": map[string]any{"name": asStr(m["name"]), "args": args, "id": asStr(m["id"])}}
			if sig := asStr(m["thoughtSignature"]); signed && sig != "" {
				p["thoughtSignature"] = sig
			}
			parts = append(parts, p)
		}
	}
	return parts
}

func toolResultResponse(msg map[string]any) map[string]any {
	var texts []string
	items, _ := msg["content"].([]any)
	for _, it := range items {
		if m := asMap(it); asStr(m["type"]) == "text" {
			texts = append(texts, asStr(m["text"]))
		}
	}
	text := strings.Join(texts, "\n")
	if b, _ := msg["isError"].(bool); b {
		if text == "" {
			text = "Error"
		}
		return map[string]any{"error": text}
	}
	return map[string]any{"output": text}
}

func sameTarget(msg map[string]any, provider, model string) bool {
	if provider == "" || model == "" {
		return true
	}
	return asStr(msg["provider"]) == provider && asStr(msg["model"]) == model
}

// convertMessages turns a pig transcript into Gemini contents. Tool results go
// back under role "model" when the call came from the target model itself, the
// shape the agy CLI sends.
func convertMessages(messages []any, provider, model string) []any {
	matches := map[string]bool{}
	for _, m := range messages {
		msg := asMap(m)
		if asStr(msg["role"]) != "assistant" {
			continue
		}
		ok := sameTarget(msg, provider, model)
		blocks, _ := msg["content"].([]any)
		for _, b := range blocks {
			if bm := asMap(b); asStr(bm["type"]) == "toolCall" {
				matches[asStr(bm["id"])] = ok
			}
		}
	}
	var contents []any
	for _, m := range messages {
		msg := asMap(m)
		switch asStr(msg["role"]) {
		case "user":
			if parts := userParts(msg["content"]); len(parts) > 0 {
				contents = append(contents, map[string]any{"role": "user", "parts": parts})
			}
		case "assistant":
			if parts := assistantParts(msg, sameTarget(msg, provider, model)); len(parts) > 0 {
				contents = append(contents, map[string]any{"role": "model", "parts": parts})
			}
		case "toolResult":
			role := "user"
			if matches[asStr(msg["toolCallId"])] {
				role = "model"
			}
			part := map[string]any{"functionResponse": map[string]any{
				"name": asStr(msg["toolName"]), "response": toolResultResponse(msg), "id": asStr(msg["toolCallId"]),
			}}
			if n := len(contents); n > 0 {
				last := asMap(contents[n-1])
				if asStr(last["role"]) == role && allFunctionResponses(last["parts"]) {
					last["parts"] = append(last["parts"].([]any), part)
					continue
				}
			}
			contents = append(contents, map[string]any{"role": role, "parts": []any{part}})
		}
	}
	return contents
}

func allFunctionResponses(v any) bool {
	parts, _ := v.([]any)
	for _, p := range parts {
		if _, ok := asMap(p)["functionResponse"]; !ok {
			return false
		}
	}
	return true
}

var unsupportedSchemaFields = map[string]bool{
	"additionalProperties": true, "$schema": true, "$id": true, "$comment": true, "$ref": true, "$defs": true,
	"definitions": true, "const": true, "contentMediaType": true, "contentEncoding": true, "if": true,
	"then": true, "else": true, "not": true, "patternProperties": true, "unevaluatedProperties": true,
	"unevaluatedItems": true, "dependentRequired": true, "dependentSchemas": true, "propertyNames": true,
	"minContains": true, "maxContains": true,
}

// toGeminiSchema rewrites a JSON Schema into the shape Antigravity's strict
// validation accepts: upper-case types, unsupported keywords dropped, required
// limited to declared properties, arrays always carrying items.
func toGeminiSchema(v any) any {
	in, ok := v.(map[string]any)
	if !ok {
		return v
	}
	props, _ := in["properties"].(map[string]any)
	out := map[string]any{}
	for k, val := range in {
		if unsupportedSchemaFields[k] {
			continue
		}
		switch {
		case k == "type":
			if s, ok := val.(string); ok {
				out[k] = strings.ToUpper(s)
			} else {
				out[k] = val
			}
		case k == "properties" && props != nil:
			np := map[string]any{}
			for pk, pv := range props {
				np[pk] = toGeminiSchema(pv)
			}
			out[k] = np
		case k == "items":
			out[k] = toGeminiSchema(val)
		case k == "anyOf" || k == "oneOf" || k == "allOf":
			if arr, ok := val.([]any); ok {
				na := make([]any, len(arr))
				for i, it := range arr {
					na[i] = toGeminiSchema(it)
				}
				out[k] = na
			} else {
				out[k] = val
			}
		case k == "required":
			arr, ok := val.([]any)
			if !ok || len(props) == 0 {
				out[k] = val
				continue
			}
			var keep []any
			for _, r := range arr {
				if s, ok := r.(string); ok {
					if _, declared := props[s]; declared {
						keep = append(keep, s)
					}
				}
			}
			if len(keep) > 0 {
				out[k] = keep
			}
		default:
			out[k] = val
		}
	}
	if out["type"] == "ARRAY" && out["items"] == nil {
		out["items"] = map[string]any{"type": "STRING"}
	}
	return out
}

func convertTools(tools []any) []any {
	if len(tools) == 0 {
		return nil
	}
	decls := make([]any, 0, len(tools))
	for _, t := range tools {
		m := asMap(t)
		decls = append(decls, map[string]any{
			"name": asStr(m["name"]), "description": asStr(m["description"]), "parameters": toGeminiSchema(m["parameters"]),
		})
	}
	return []any{map[string]any{"functionDeclarations": decls}}
}
