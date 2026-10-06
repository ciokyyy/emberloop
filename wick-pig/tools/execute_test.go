package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeExecuteTool is one native tool the execute bridge can serve in tests: it
// records the root and args it was called with and answers a canned string.
type fakeExecuteTool struct {
	name   string
	result string
	seen   *fakeExecuteSeen
}

type fakeExecuteSeen struct {
	root string
	args json.RawMessage
}

func (f fakeExecuteTool) Name() string { return f.name }
func (f fakeExecuteTool) Description() string {
	return "fake " + f.name + " for execute tests"
}
func (f fakeExecuteTool) Schema() map[string]any {
	return obj(map[string]any{
		"path": map[string]any{"type": "string", "description": "A path."},
	}, "path")
}
func (f fakeExecuteTool) Execute(_ context.Context, root string, args json.RawMessage) (string, error) {
	if f.seen != nil {
		f.seen.root = root
		f.seen.args = args
	}
	return f.result, nil
}

// requirePython skips the test with a clear message when no system python3 is
// installed: execute runs real python3 per the spec.
func requirePython(t *testing.T) string {
	t.Helper()
	py, err := pythonPath()
	if err != nil {
		t.Skipf("python3 not installed, skipping execute test: %v", err)
	}
	return py
}

// Seam: the execute tool is registered beside the other file tools.
func TestExecuteRegisteredInAll(t *testing.T) {
	found := false
	for _, tl := range All() {
		if tl.Name() == "execute" {
			found = true
		}
	}
	if !found {
		t.Fatal("tools.All() must contain execute")
	}
}

// Seam: one tool call — a script calls tools.read and only what it prints
// enters the result.
func TestExecuteScriptCallsRead(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	var seen fakeExecuteSeen
	ex := NewExecute([]Tool{fakeExecuteTool{name: "read", result: "file-bytes", seen: &seen}}, "test execute", false)
	root := t.TempDir()
	out, err := ex.Execute(context.Background(), root, json.RawMessage(`{"code":"import tools\nprint(tools.read(path=\"a.txt\"))"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "file-bytes") {
		t.Fatalf("script result = %q, want the read result", out)
	}
	if seen.root != root {
		t.Errorf("read ran with root %q, want %q", seen.root, root)
	}
	var args map[string]any
	if err := json.Unmarshal(seen.args, &args); err != nil || args["path"] != "a.txt" {
		t.Errorf("read args = %s, want path a.txt", seen.args)
	}
}

// errExecuteTool always fails, so a script sees a ToolError.
type errExecuteTool struct{ name, msg string }

func (f errExecuteTool) Name() string        { return f.name }
func (f errExecuteTool) Description() string { return "failing " + f.name }
func (f errExecuteTool) Schema() map[string]any {
	return obj(map[string]any{})
}
func (f errExecuteTool) Execute(context.Context, string, json.RawMessage) (string, error) {
	return "", errors.New(f.msg)
}

// Seam: a refused or failing tool call raises a catchable ToolError; an
// uncaught exception returns the traceback tail with the exit status.
func TestExecuteFailingToolRaisesToolError(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	ex := NewExecute([]Tool{errExecuteTool{name: "git", msg: "git boom"}}, "test execute", false)
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(`{"code":"import tools\nprint(tools.git(args=[\"log\"]))"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "ToolError") || !strings.Contains(out, "git boom") {
		t.Fatalf("uncaught ToolError must come back as a traceback naming the cause: %q", out)
	}
	if !strings.Contains(out, "[exit status 1]") {
		t.Fatalf("a failing script must name its exit status: %q", out)
	}
}

func TestExecuteToolErrorIsCatchable(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	ex := NewExecute([]Tool{errExecuteTool{name: "git", msg: "git boom"}}, "test execute", false)
	code := "import tools\ntry:\n    tools.git(args=[\"log\"])\nexcept tools.ToolError as e:\n    print(\"caught:\", e)\n"
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(mustExecuteArgs(t, code)))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "caught: git boom") {
		t.Fatalf("a caught ToolError must print: %q", out)
	}
	if strings.Contains(out, "[exit status") {
		t.Fatalf("a script that caught its error exits 0: %q", out)
	}
}

func TestExecuteUnknownToolRaisesToolError(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	ex := NewExecute([]Tool{fakeExecuteTool{name: "read", result: "x"}}, "test execute", false)
	code := "import tools\ntry:\n    tools.nosuchtool()\nexcept tools.ToolError as e:\n    print(\"caught:\", e)\n"
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(mustExecuteArgs(t, code)))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "caught:") || !strings.Contains(out, "nosuchtool") {
		t.Fatalf("an unknown tool must raise ToolError naming it: %q", out)
	}
}

func mustExecuteArgs(t *testing.T, code string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Seam: discovery — tools.search lists matching names with signatures and
// tools.help shows one tool, without spending a tool turn.
func TestExecuteSearchAndHelp(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	ex := NewExecute([]Tool{
		fakeExecuteTool{name: "read", result: "x"},
		fakeExecuteTool{name: "grep", result: "y"},
	}, "test execute", false)
	code := "import tools\nprint(tools.search(\"rea\"))\nprint(tools.help(\"grep\"))\n"
	out, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(mustExecuteArgs(t, code)))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "read(") {
		t.Fatalf("search must list the matching tool with its signature: %q", out)
	}
	if strings.Contains(out, "grep(") && !strings.Contains(out, "fake grep") {
		t.Fatalf("search for rea must not list grep: %q", out)
	}
	if !strings.Contains(out, "fake grep for execute tests") {
		t.Fatalf("help must show the tool description: %q", out)
	}
}

// Seam: the description lists the tools behind execute by name and carries a
// tools.grep example.
func TestExecuteDescriptionListsTools(t *testing.T) {
	desc := ExecuteDescription([]string{"grep", "git", "check"})
	for _, want := range []string{"grep", "git", "check", "tools.grep"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("execute description must name %q:\n%s", want, desc)
		}
	}
}

// Seam: timeout — the whole process group dies (children included) and the
// call comes back with what was printed so far, not a hang.
func TestExecuteTimeoutKillsTheProcess(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	ex := NewExecute(nil, "test execute", false)
	start := time.Now()
	out, err := ex.Execute(context.Background(), t.TempDir(),
		json.RawMessage(`{"code":"import subprocess, time\nsubprocess.Popen([\"sleep\", \"57\"])\ntime.sleep(60)","timeout":2}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if elapsed := time.Now().Sub(start); elapsed > 30*time.Second {
		t.Fatalf("a 2s timeout took %v; the process group was not killed", elapsed)
	}
	if !strings.Contains(out, "timed out") {
		t.Fatalf("a timed-out script must say so: %q", out)
	}
	leftover, err := exec.Command("pgrep", "-f", "sleep 57").Output()
	if err == nil {
		t.Fatalf("a child sleep survived the timeout: %s", leftover)
	}
}

// Seam: the temp directory and socket are removed after every call.
func TestExecuteRemovesItsTempDir(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	before, err := filepath.Glob(filepath.Join(os.TempDir(), "wick-execute-*"))
	if err != nil {
		t.Fatal(err)
	}
	ex := NewExecute([]Tool{fakeExecuteTool{name: "read", result: "x"}}, "test execute", false)
	if _, err := ex.Execute(context.Background(), t.TempDir(),
		json.RawMessage(mustExecuteArgs(t, "import tools\nprint(tools.read(path=\"a\"))"))); err != nil {
		t.Fatalf("execute: %v", err)
	}
	after, err := filepath.Glob(filepath.Join(os.TempDir(), "wick-execute-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("temp dirs before=%v after=%v; the bridge must be removed", before, after)
	}
}

// requireGit skips the test with a clear message when git is not installed.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := ResolveBinary("git"); err != nil {
		t.Skipf("git not installed, skipping execute test: %v", err)
	}
}

// Seam: one scripted call reaches the native toolkit — read, grep, git, find,
// and where the role allows them check and bash.
func TestExecuteNativeToolkit(t *testing.T) {
	requirePython(t)
	requireGit(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("the launch code is 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".wick"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".wick", "check"), []byte("echo check-ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	init := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	init("init")
	init("config", "user.email", "test@example.com")
	init("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("the launch code is 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	init("add", "notes.txt")
	init("commit", "-m", "add notes")

	ex := NewExecute([]Tool{readTool{}, grepTool{}, gitTool{}, findTool{lexicalOnly: true}, checkTool{}, bashTool{}}, "test execute", false)
	code := "import tools\n" +
		"print(tools.read(path=\"notes.txt\"))\n" +
		"print(tools.grep(pattern=\"launch code\"))\n" +
		"print(tools.git(args=[\"log\", \"--oneline\", \"-1\"]))\n" +
		"print(tools.find(query=\"launch code\", grep_keywords=[\"launch\"]))\n" +
		"print(tools.check())\n" +
		"print(tools.bash(command=\"echo bash-ok\"))\n"
	out, err := ex.Execute(context.Background(), root, json.RawMessage(mustExecuteArgs(t, code)))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"launch code is 42", "notes.txt", "add notes", "check-ok", "bash-ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("toolkit output must contain %q:\n%s", want, out)
		}
	}
}

// Seam: over-threshold output is stored and summarised; with intent the
// matching chunks come back inline.
func TestExecuteOverflowStoresAndSummarizes(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	root := t.TempDir()
	globalProjectEnv(t, root, `{}`, `{"overflow":{"threshold_bytes":200}}`)
	ex := NewExecute(nil, "test execute", false)
	var code strings.Builder
	code.WriteString("for i in range(50):\n    print(\"line\", i, \"filler filler filler filler\")\n")
	out, err := ex.Execute(context.Background(), root, json.RawMessage(mustExecuteArgs(t, code.String())))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "ctx:") {
		t.Fatalf("over-threshold output must be stored with a handle: %q", out)
	}
}

func TestExecuteIntentReturnsMatchingChunks(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	root := t.TempDir()
	globalProjectEnv(t, root, `{}`, `{"overflow":{"threshold_bytes":200}}`)
	ex := NewExecute(nil, "test execute", false)
	var code strings.Builder
	code.WriteString("for i in range(50):\n    print(\"line\", i, \"filler filler filler filler\")\nprint(\"the needle token zebraquux hides here\")\n")
	args, err := json.Marshal(map[string]string{"code": code.String(), "intent": "zebraquux"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := ex.Execute(context.Background(), root, args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "ctx:") {
		t.Fatalf("over-threshold output must be stored with a handle: %q", out)
	}
	if !strings.Contains(out, "zebraquux") {
		t.Fatalf("intent must return the matching chunks inline: %q", out)
	}
}

// requireSandbox skips the test with a clear message when bubblewrap is not
// installed: sandboxed execute needs it.
func requireSandbox(t *testing.T, worktree string) {
	t.Helper()
	if err := EnsureHealthy(worktree); err != nil {
		t.Skipf("bwrap not installed or unhealthy, skipping sandboxed execute test: %v", err)
	}
}

// Seam: sandboxed execute — the script cannot write files or reach the
// network, yet it can still call tools through the socket.
func TestExecuteSandboxedScriptIsConfinedButCallsTools(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	root := t.TempDir()
	requireSandbox(t, root)
	ex := NewExecute([]Tool{fakeExecuteTool{name: "read", result: "s prompted"}}, "test execute", true)
	code := "import tools\n" +
		"print(tools.read(path=\"x\"))\n" +
		"open(\"escape.txt\", \"w\").write(\"no\")\n"
	out, err := ex.Execute(context.Background(), root, json.RawMessage(mustExecuteArgs(t, code)))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "s prompted") {
		t.Fatalf("a sandboxed script must still reach tools: %q", out)
	}
	if !strings.Contains(out, "Permission denied") && !strings.Contains(out, "Read-only file system") {
		t.Fatalf("a sandboxed script must not write files: %q", out)
	}
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("the write must not have landed: %v", err)
	}
	if !strings.Contains(out, "[exit status 1]") {
		t.Fatalf("a script denied its write must fail: %q", out)
	}
}

func TestExecuteSandboxedScriptHasNoNetwork(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	root := t.TempDir()
	requireSandbox(t, root)
	ex := NewExecute(nil, "test execute", true)
	code := "import socket\ns = socket.socket()\ns.settimeout(5)\ns.connect((\"127.0.0.1\", 9))\nprint(\"reached\")\n"
	out, err := ex.Execute(context.Background(), root, json.RawMessage(mustExecuteArgs(t, code)))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.Contains(out, "reached") {
		t.Fatalf("a sandboxed script must not reach the network: %q", out)
	}
}

// Seam: a broken sandbox refuses naming `wick-pig deps` and never runs the
// script unsandboxed.
func TestExecuteBrokenSandboxRefuses(t *testing.T) {
	requirePython(t)
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	t.Setenv("WICK_SANDBOX_FORCE_FAIL", "1")
	ex := NewExecute([]Tool{fakeExecuteTool{name: "read", result: "x"}}, "test execute", true)
	_, err := ex.Execute(context.Background(), t.TempDir(),
		json.RawMessage(mustExecuteArgs(t, "print(\"must not run\")")))
	if err == nil || !strings.Contains(err.Error(), "wick-pig deps") {
		t.Fatalf("a broken sandbox must refuse naming wick-pig deps, got %v", err)
	}
}

// Seam: a missing python3 names `wick-pig deps`, the command that installs it.
func TestExecuteMissingPythonNamesDeps(t *testing.T) {
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	t.Setenv("WICK_PYTHON", filepath.Join(t.TempDir(), "no-python-here"))
	ex := NewExecute(nil, "test execute", false)
	_, err := ex.Execute(context.Background(), t.TempDir(), json.RawMessage(mustExecuteArgs(t, "print(1)")))
	if err == nil || !strings.Contains(err.Error(), "wick-pig deps") {
		t.Fatalf("a missing python3 must name wick-pig deps, got %v", err)
	}
}
