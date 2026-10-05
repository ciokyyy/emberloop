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
		case "list":
			return Result{Stdout: `{"result":{"panes":[{"pane_id":"w1:p1","tab_id":"` + os.Getenv("HERDR_TAB_ID") + `"}]}}`}
		case "split":
			return Result{Stdout: `{"result":{"pane":{"pane_id":"p9"}}}`}
		case "run":
			// The pane runs `wick-pig exec <launch-file>`: read the launch the
			// way exec would, to find the session it starts.
			file, ok := execFileFromLine(args[3])
			if !ok {
				return Result{Stdout: "{}"}
			}
			cmd, err := readLaunch(file)
			if err != nil {
				return Result{Stdout: "{}"}
			}
			var id string
			for i, a := range cmd.Args {
				if a == "--session-id" && i+1 < len(cmd.Args) {
					id = cmd.Args[i+1]
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

// mcpConfigArg is the path claude was told to read its MCP config from: the
// --mcp-config in the launch file the pane's `wick-pig exec` line names.
func mcpConfigArg(t *testing.T, calls [][]string) string {
	t.Helper()
	for _, c := range calls {
		joined := strings.Join(c, " ")
		idx := strings.Index(joined, "wick-pig exec ")
		if idx < 0 {
			continue
		}
		file, ok := execFileFromLine(strings.TrimSpace(joined[idx:]))
		if !ok {
			continue
		}
		cmd, err := readLaunch(file)
		if err != nil {
			continue
		}
		for i, a := range cmd.Args {
			if a == "--mcp-config" && i+1 < len(cmd.Args) {
				return cmd.Args[i+1]
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
		ID: "01-x", Repo: "/repo", Base: "main", Branch: "implement/01-x", Worktree: "/repo/wt", SessionID: "sess-1",
	}}); err != nil {
		t.Fatal(err)
	}

	// The launch the spawn tool built for this worker: a real spawn always
	// sets the session ID (spawn.go), so the config is keyed by it and Finish
	// must remove the ID-keyed file, not just the label fallback.
	launch, err := PrepareLaunch("claude", Session{ID: "sess-1", Role: "work", Label: "01-x", Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	file, ok := execFileFromLine(launch.Line)
	if !ok {
		t.Fatalf("no launch file in %s", launch.Line)
	}
	cmd, err := readLaunch(file)
	if err != nil {
		t.Fatal(err)
	}
	cfg := ""
	for i, a := range cmd.Args {
		if a == "--mcp-config" && i+1 < len(cmd.Args) {
			cfg = cmd.Args[i+1]
		}
	}
	if cfg == "" {
		t.Fatalf("no --mcp-config in %+v", cmd.Args)
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
