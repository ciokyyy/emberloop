package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeMCPServerKeepsOthers(t *testing.T) {
	in := `{"mcpServers":{"other":{"command":"x"},"wick":{"command":"wick-mcp"}},"keep":1}`
	out, err := mergeMCPServer([]byte(in), "/bin/el-mcp")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string]map[string]any
	var flat map[string]any
	_ = json.Unmarshal(out, &doc)
	_ = json.Unmarshal(out, &flat)
	if doc["mcpServers"]["other"]["command"] != "x" || flat["keep"].(float64) != 1 {
		t.Fatalf("other keys lost: %s", out)
	}
	if _, ok := doc["mcpServers"]["wick"]; ok {
		t.Fatalf("old wick entry kept: %s", out)
	}
	if doc["mcpServers"]["el"]["command"] != "/bin/el-mcp" {
		t.Fatalf("entry missing: %s", out)
	}
	if _, err := mergeMCPServer([]byte("{nope"), "x"); err == nil {
		t.Fatal("invalid JSON must fail")
	}
	if out, err := mergeMCPServer(nil, "x"); err != nil || !strings.Contains(string(out), "el") {
		t.Fatalf("empty input: %v %s", err, out)
	}
}

func TestMergeGraphEnabled(t *testing.T) {
	out, err := mergeGraphEnabled([]byte(`{"a":1,"graph":{"binary":"b"}}`), false)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(out, &doc)
	g := doc["graph"].(map[string]any)
	if g["enabled"] != false || g["binary"] != "b" || doc["a"].(float64) != 1 {
		t.Fatalf("merge lost keys: %s", out)
	}
}

func TestRewriteGoWorkSDK(t *testing.T) {
	src := "go 1.27.1\n\nuse .\n\nreplace github.com/x/sdk => /home/other/.pig/state/pigsdk/sdk\n"
	got := rewriteGoWorkSDK(src, "/home/me/.pig/state/pigsdk/sdk")
	if !strings.Contains(got, "=> /home/me/.pig/state/pigsdk/sdk\n") || strings.Contains(got, "other") {
		t.Fatalf("rewrite: %q", got)
	}
}
