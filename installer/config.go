package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// mergeMCPServer sets mcpServers.emberloop in a pig mcp.json and keeps every
// other key. Empty input starts a new file.
func mergeMCPServer(raw []byte, command string) ([]byte, error) {
	doc := map[string]any{}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("mcp.json is not valid JSON: %w", err)
		}
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	// The project's former name registered "wick-mcp" as "wick". That entry
	// fails to start in every el session, so it goes.
	if old, ok := servers["wick"].(map[string]any); ok {
		if cmd, _ := old["command"].(string); filepath.Base(cmd) == "wick-mcp" {
			delete(servers, "wick")
		}
	}
	servers["el"] = map[string]any{
		"command":     command,
		"exposure":    "direct",
		"description": "el's own tools: read, edit, write, bash, find, background, worker orchestration",
		"timeout":     600,
	}
	doc["mcpServers"] = servers
	return json.MarshalIndent(doc, "", "  ")
}

// mergeGraphEnabled sets graph.enabled in an el settings file and
// keeps every other key.
func mergeGraphEnabled(raw []byte, enabled bool) ([]byte, error) {
	doc := map[string]any{}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("settings.json is not valid JSON: %w", err)
		}
	}
	graph, _ := doc["graph"].(map[string]any)
	if graph == nil {
		graph = map[string]any{}
	}
	graph["enabled"] = enabled
	doc["graph"] = graph
	return json.MarshalIndent(doc, "", "  ")
}

var goWorkReplace = regexp.MustCompile(`(?m)^(replace\s+\S+\s+=>\s+).*$`)

// rewriteGoWorkSDK points the replace directive of an extension go.work at
// the local PiG SDK directory.
func rewriteGoWorkSDK(src, sdkDir string) string {
	return goWorkReplace.ReplaceAllString(src, "${1}"+sdkDir)
}
