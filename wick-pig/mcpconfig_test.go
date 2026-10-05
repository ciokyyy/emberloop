package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// claudeHerdr plays a claude session for the review path: it answers the herdr
// calls and writes the transcript claude would write, so WaitReply can finish.
func claudeHerdr(home, report string) func(string, []string) Result {
	return func(cmd string, args []string) Result {
		if len(args) < 2 {
			return Result{Stdout: "{}"}
		}
		switch args[1] {
		case "split":
			return Result{Stdout: `{"result":{"pane":{"pane_id":"p9"}}}`}
		case "run":
			w := strings.Fields(args[3])
			var id string
			for i, a := range w {
				if a == "--session-id" {
					id = w[i+1]
				}
			}
			dir := filepath.Join(home, ".claude", "projects", "wt")
			_ = os.MkdirAll(dir, 0o755)
			_ = os.WriteFile(filepath.Join(dir, "t_"+id+".jsonl"), []byte(
				`{"type":"assistant","message":{"stop_reason":"end_turn","content":[{"type":"text","text":"`+report+`"}]}}`+"\n"), 0o644)
		}
		return Result{Stdout: "{}"}
	}
}

// mcpConfigArg is the path claude was told to read its MCP config from, found
// in the launch line the caller typed into the pane.
func mcpConfigArg(t *testing.T, calls [][]string) string {
	t.Helper()
	for _, c := range calls {
		for _, field := range strings.Fields(strings.Join(c, " ")) {
			if path := strings.Trim(field, `"`); strings.HasSuffix(path, ".json") &&
				strings.Contains(path, string(os.PathSeparator)+"mcp"+string(os.PathSeparator)) {
				return path
			}
		}
	}
	t.Fatalf("no --mcp-config in the launch: %v", calls)
	return ""
}

func TestReadOnlyLeavesNoClaudeMCPConfigBehind(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WICK_READONLY_HARNESS", "claude")

	rec := &recorder{reply: claudeHerdr(home, "looks fine")}
	if _, err := ReadOnly(rec.run, "", "", "look at it"); err != nil {
		t.Fatal(err)
	}

	cfg := mcpConfigArg(t, rec.calls)
	if !strings.HasPrefix(cfg, filepath.Dir(statePath)+string(os.PathSeparator)) {
		t.Errorf("claude config %s lives outside wick's state dir %s", cfg, filepath.Dir(statePath))
	}
	if _, err := os.Stat(cfg); !os.IsNotExist(err) {
		t.Errorf("claude MCP config %s outlived the review (err=%v)", cfg, err)
	}
}

func TestFinishLeavesNoClaudeMCPConfigBehind(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	if err := save(map[string]Worker{"01-x": {
		ID: "01-x", Repo: "/repo", Base: "main", Branch: "implement/01-x", Worktree: "/repo/wt",
	}}); err != nil {
		t.Fatal(err)
	}

	// The launch the spawn tool built for this worker.
	launch, err := PrepareLaunch("claude", Session{Role: "work", Label: "01-x", Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := ""
	for i := 0; i+1 < len(strings.Fields(launch.Line)); i++ {
		if strings.Fields(launch.Line)[i] == "--mcp-config" {
			cfg = strings.Fields(launch.Line)[i+1]
		}
	}
	if cfg == "" {
		t.Fatalf("no --mcp-config in %s", launch.Line)
	}
	if _, err := os.Stat(cfg); err != nil {
		t.Fatalf("worker launch did not write its config: %v", err)
	}

	rec := &recorder{reply: func(string, []string) Result { return Result{Stdout: "{}"} }}
	if _, err := Finish(rec.run, "01-x", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg); !os.IsNotExist(err) {
		t.Errorf("claude MCP config %s outlived the worker (err=%v)", cfg, err)
	}
}
