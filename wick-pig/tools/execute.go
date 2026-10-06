// Execute runs a Python script that calls wick's native tools as functions
// (ticket 16; MCP tools join in ticket 17).
//
// Each call is a fresh `python3 -I` process with no state kept between calls.
// Wick creates a private temp directory holding a unix socket and a generated
// `tools` module, and puts the script beside it so `import tools` resolves
// through the script's own directory even under -I (which ignores
// PYTHONPATH). `tools.<name>(**kwargs)` sends one JSON line per call; wick
// runs the real tool and returns the result string, or an error the script
// sees as `tools.ToolError`. `tools.search` and `tools.help` answer from a
// spec baked into the module, with no round trip.
//
// The allowed tools are fixed at construction (NewExecute): exactly the
// role's resolved toolset minus execute itself and the orchestration tools,
// so a disabled tool is not callable and renames show under their new names.
// For review and scout the Python runs inside the bubblewrap sandbox with the
// bridge directory as the only writable bind; wick serves the socket outside
// it, so `check` can still write its own caches while the script can write
// nothing. The temp directory and socket are removed after every call, and a
// timeout kills the whole process group.
//
// Test hooks: WICK_PYTHON overrides the python3 path outright, and
// WICK_EXECUTE_TIMEOUT caps nothing (timeouts come from the call args).
package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultExecuteTimeout = 120 * time.Second
	maxExecuteTimeout     = 600 * time.Second
	// executeTraceTailLines caps the stderr a failing script contributes: the
	// verdict is at the traceback's tail.
	executeTraceTailLines = 30
	// executeFrameBytes bounds one bridge request line: a script passing a
	// whole file to write must still fit.
	executeFrameBytes = 8 << 20
)

// pythonPath resolves the interpreter execute runs: WICK_PYTHON wins for
// tests, otherwise the shared bin-dir-then-PATH resolver. A miss is a
// *SandboxUnavailableError naming `wick-pig deps`, the command that installs
// it.
func pythonPath() (string, error) {
	if p := os.Getenv("WICK_PYTHON"); p != "" {
		if isExecutable(p) {
			return p, nil
		}
		if abs, err := filepath.Abs(p); err == nil {
			if isExecutable(abs) {
				return abs, nil
			}
		}
		return "", &SandboxUnavailableError{Binary: "python3", Reason: fmt.Sprintf("WICK_PYTHON=%q is not executable", p)}
	}
	return ResolveBinary("python3")
}

// executeTool is the unbound placeholder tools.All serves: wick replaces it
// per role with a bound NewExecute carrying that role's allowed tools, so the
// MCP tool list and the stub generator cannot disagree. Served directly it
// refuses instead of running with no allowlist.
type executeTool struct{}

func (executeTool) Name() string { return "execute" }
func (executeTool) Description() string {
	return "Run a Python script (stdlib only) that calls wick tools as functions, so one call replaces many and only what the script prints enters context.\n\n" +
		"Behind execute: grep, git, check.\n\n" +
		"import tools\nprint(tools.grep(pattern=\"TODO\"))\n\n" +
		"tools.search(query) lists matching tools with signatures; tools.help(name) shows one tool. A failing tool call raises tools.ToolError."
}
func (executeTool) Schema() map[string]any {
	return obj(map[string]any{
		"code":    map[string]any{"type": "string", "description": "Python source to run."},
		"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default 120, maximum 600)."},
		"intent":  map[string]any{"type": "string", "description": "Optional: when output is stored, also return the chunks matching this intent inline."},
	}, "code")
}
func (executeTool) Execute(context.Context, string, json.RawMessage) (string, error) {
	return "", errors.New("execute is not bound to a role; it is replaced per role before serving")
}

// boundExecute is one role's execute tool: the allowed tools, the description
// listing them, and whether the Python runs sandboxed.
type boundExecute struct {
	allowed   []Tool
	byName    map[string]Tool
	desc      string
	sandboxed bool
}

// Orchestration names never reach a script: submitting or spawning from
// inside execute would bypass the harness that owns the turn.
var executeExcluded = map[string]bool{
	"execute": true, "spawn": true, "finish": true, "tell": true, "submit": true,
}

// NewExecute builds the execute tool for one role: allowed is the role's
// resolved toolset (both buckets, renames applied) and only what is not
// excluded stays callable. desc is served as the description; empty means the
// placeholder text. sandboxed runs the Python under bubblewrap.
func NewExecute(allowed []Tool, desc string, sandboxed bool) Tool {
	byName := map[string]Tool{}
	var kept []Tool
	for _, t := range allowed {
		if executeExcluded[t.Name()] {
			continue
		}
		if _, dup := byName[t.Name()]; dup {
			continue
		}
		byName[t.Name()] = t
		kept = append(kept, t)
	}
	if desc == "" {
		desc = executeTool{}.Description()
	}
	return boundExecute{allowed: kept, byName: byName, desc: desc, sandboxed: sandboxed}
}

func (e boundExecute) Name() string        { return "execute" }
func (e boundExecute) Description() string { return e.desc }
func (e boundExecute) Schema() map[string]any {
	return (executeTool{}).Schema()
}

// executeLauncher runs mainPath as __main__ with dir first on sys.path:
// -I implies -P (the script's directory is not prepended), so without this
// `import tools` cannot find the generated module beside the script. Argv
// carries the paths, so no quoting is needed.
const executeLauncher = "import sys; sys.path.insert(0, sys.argv[1]); import runpy; runpy.run_path(sys.argv[2], run_name='__main__')"

// ExecuteDescription lists the tools behind execute by name, with the grep
// example and the search/help/ToolError how-to. names are the execute bucket
// in serve order; main-list tools callable beside them are named in one line.
func ExecuteDescription(names []string) string {
	var b strings.Builder
	b.WriteString("Run a Python script (stdlib only) that calls wick tools as functions, so one call replaces many and only what the script prints enters context.\n\n")
	if len(names) == 0 {
		b.WriteString("No tools are behind execute for this role.\n")
	} else {
		fmt.Fprintf(&b, "Behind execute: %s.\n\n", strings.Join(names, ", "))
	}

	b.WriteString("import tools\nprint(tools.grep(pattern=\"TODO\"))\n\n")
	b.WriteString("tools.search(query) lists matching tools with signatures; tools.help(name) shows one tool. " +
		"A failing tool call raises tools.ToolError: catch it or the traceback comes back.")
	return b.String()
}

func (e boundExecute) Execute(ctx context.Context, root string, raw json.RawMessage) (string, error) {
	var p struct {
		Code    string `json:"code"`
		Timeout int    `json:"timeout"`
		Intent  string `json:"intent"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("invalid execute parameters: %w", err)
	}
	if strings.TrimSpace(p.Code) == "" {
		return "", errors.New("code is required")
	}
	timeout := defaultExecuteTimeout
	if p.Timeout > 0 {
		timeout = min(time.Duration(p.Timeout)*time.Second, maxExecuteTimeout)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	py, err := pythonPath()
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "wick-execute-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	sockPath := filepath.Join(dir, "sock")
	stub, err := buildToolsModule(sockPath, e.allowed)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "tools.py"), []byte(stub), 0o644); err != nil {
		return "", err
	}
	mainPath := filepath.Join(dir, "main.py")
	if err := os.WriteFile(mainPath, []byte(p.Code), 0o644); err != nil {
		return "", err
	}

	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		return "", fmt.Errorf("execute bridge: %w", err)
	}
	defer lis.Close()
	serveCtx, stopServe := context.WithCancel(ctx)
	defer stopServe()
	var wg sync.WaitGroup
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				serveExecuteConn(serveCtx, root, e.byName, conn)
			}()
		}
	}()

	var stdout, stderr string
	var code int
	if e.sandboxed {
		res, serr := RunSandbox(ctx, SandboxConfig{Worktree: root, BridgeDir: dir, Timeout: timeout}, py, "-I", "-c", executeLauncher, dir, mainPath)
		stdout, stderr = res.Stdout, res.Stderr
		if ctx.Err() == context.DeadlineExceeded {
			return executeResult(root, p.Code, p.Intent, stdout, stderr, 0, true, timeout)
		}
		if serr != nil {
			return "", serr
		}
		code = res.Code
	} else {
		cmd := exec.CommandContext(ctx, py, "-I", "-c", executeLauncher, dir, mainPath)
		cmd.Dir = root
		// A timeout must reach the children too, or the wait hangs on a pipe
		// they still hold. Own process group, killed as a group, like bash.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
		cmd.WaitDelay = 2 * time.Second
		var outBuf, errBuf strings.Builder
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		runErr := cmd.Run()
		stdout, stderr = outBuf.String(), errBuf.String()
		if ctx.Err() == context.DeadlineExceeded {
			return executeResult(root, p.Code, p.Intent, stdout, stderr, 0, true, timeout)
		}
		if runErr != nil {
			var exit *exec.ExitError
			if errors.As(runErr, &exit) {
				code = exit.ExitCode()
			} else {
				return "", runErr
			}
		}
	}
	_ = lis.Close()
	stopServe()
	wg.Wait()
	return executeResult(root, p.Code, p.Intent, stdout, stderr, code, false, timeout)
}

// executeResult formats one run: stdout then stderr (a failing run keeps the
// traceback tail), through the bash sanitiser and the overflow store, with
// the exit status named on failure.
func executeResult(root, code, intent, stdout, stderr string, exitCode int, timedOut bool, timeout time.Duration) (string, error) {
	stderrPart := stderr
	if exitCode != 0 && !timedOut {
		stderrPart = tailLines(stderr, executeTraceTailLines)
	}
	text := stdout
	if strings.TrimSpace(stderrPart) != "" {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += stderrPart
	}
	text = sanitize(text)
	if summary, stored, merr := MaybeOverflow(root, overflowSession(), executeSource(code), "execute", text, intent); merr != nil {
		text = truncateHeadTail(text, 15*1024, 15*1024) +
			fmt.Sprintf("\n[overflow store unavailable: %v]", merr)
	} else if stored {
		text = summary
	}
	if timedOut {
		return text + fmt.Sprintf("\n[timed out after %v]", timeout), nil
	}
	if exitCode != 0 {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return text + fmt.Sprintf("[exit status %d]", exitCode), nil
	}
	return text, nil
}

// executeSource labels a stored execute output with the script's first line,
// truncated so the label stays one line.
func executeSource(code string) string {
	line := strings.TrimSpace(code)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if len(line) > 80 {
		line = line[:80] + "..."
	}
	return "execute " + line
}

// tailLines keeps the last n lines of s.
func tailLines(s string, n int) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// executeRequest is one `tools.<name>(**kwargs)` call arriving over the
// socket; executeResponse carries the result string or the ToolError text.
type executeRequest struct {
	ID   int64          `json:"id"`
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

type executeResponse struct {
	ID     int64  `json:"id"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// serveExecuteConn answers one script's bridge calls until it goes away: each
// line runs the named tool through the role's resolved instance and the reply
// carries the result string, or the error the script raises as ToolError.
func serveExecuteConn(ctx context.Context, root string, byName map[string]Tool, conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), executeFrameBytes)
	w := bufio.NewWriter(conn)
	for sc.Scan() {
		var req executeRequest
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			writeExecuteResponse(w, executeResponse{ID: req.ID, Error: fmt.Sprintf("bad bridge request: %v", err)})
			continue
		}
		t, ok := byName[req.Tool]
		if !ok {
			writeExecuteResponse(w, executeResponse{ID: req.ID, Error: fmt.Sprintf("unknown tool %q: it is not available in execute for this role", req.Tool)})
			continue
		}
		args, err := json.Marshal(req.Args)
		if err != nil {
			writeExecuteResponse(w, executeResponse{ID: req.ID, Error: fmt.Sprintf("bad arguments for %q: %v", req.Tool, err)})
			continue
		}
		res, err := t.Execute(ctx, root, args)
		if err != nil {
			writeExecuteResponse(w, executeResponse{ID: req.ID, Error: err.Error()})
			continue
		}
		writeExecuteResponse(w, executeResponse{ID: req.ID, Result: res})
	}
}

func writeExecuteResponse(w *bufio.Writer, resp executeResponse) {
	raw, err := json.Marshal(resp)
	if err != nil {
		return
	}
	raw = append(raw, '\n')
	_, _ = w.Write(raw)
	_ = w.Flush()
}

// toolSpec is what the stub bakes in for discovery: the signature search
// matches on and the description help shows.
type toolSpec struct {
	Signature   string `json:"sig"`
	Description string `json:"desc"`
}

// executeSpecs renders one spec per allowed tool: the signature from its JSON
// schema (required first, then the rest alphabetically) and its description.
func executeSpecs(allowed []Tool) map[string]toolSpec {
	specs := make(map[string]toolSpec, len(allowed))
	for _, t := range allowed {
		specs[t.Name()] = toolSpec{Signature: signatureOf(t), Description: t.Description()}
	}
	return specs
}

// signatureOf renders name plus its schema's properties: required in schema
// order, then the rest alphabetically, so the stub's search and help read the
// same across runs.
func signatureOf(t Tool) string {
	schema := t.Schema()
	props, _ := schema["properties"].(map[string]any)
	var required []string
	if req, ok := schema["required"].([]string); ok {
		required = req
	} else if req, ok := schema["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				required = append(required, s)
			}
		}
	}
	inReq := map[string]bool{}
	for _, r := range required {
		inReq[r] = true
	}
	var rest []string
	for name := range props {
		if !inReq[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return t.Name() + "(" + strings.Join(append(required, rest...), ", ") + ")"
}

// isPythonIdent reports whether name is a valid Python identifier and not a
// keyword: only such tools get a `def` in the stub (the rest stay reachable
// through tools.call and search/help).
func isPythonIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	switch name {
	case "def", "class", "return", "import", "from", "as", "pass", "raise",
		"None", "True", "False", "and", "or", "not", "if", "else", "elif",
		"for", "while", "try", "except", "finally", "with", "lambda", "global",
		"nonlocal", "assert", "del", "yield", "async", "await", "is", "in":
		return false
	}
	return true
}

// buildToolsModule renders the generated `tools` module: the ToolError type,
// the bridge client, one function per callable tool, and the search/help
// discovery over the baked specs.
func buildToolsModule(sockPath string, allowed []Tool) (string, error) {
	specRaw, err := json.Marshal(executeSpecs(allowed))
	if err != nil {
		return "", err
	}
	var fns strings.Builder
	for _, t := range allowed {
		name := t.Name()
		if !isPythonIdent(name) {
			fmt.Fprintf(&fns, "# %s is callable only through tools.call(%q, ...): it is not a valid Python name\n", name, name)
			continue
		}
		fmt.Fprintf(&fns, "def %s(**kwargs):\n    return _call(%q, kwargs)\n\n", name, name)
	}
	return fmt.Sprintf(`"""Wick native tools as functions: one call replaces many tool turns."""
import json as _json
import socket as _socket

class ToolError(Exception):
    """A wick tool call failed; the message is the tool's error."""

_SOCKET_PATH = %q
_SPECS = _json.loads(%q)

_sock = None
_reader = None
_next_id = 0

def _ensure():
    global _sock, _reader
    if _sock is None:
        s = _socket.socket(_socket.AF_UNIX, _socket.SOCK_STREAM)
        s.connect(_SOCKET_PATH)
        _sock = s
        _reader = s.makefile("r")
    return _sock, _reader

def _call(tool, args):
    global _next_id
    sock, reader = _ensure()
    _next_id += 1
    sock.sendall((_json.dumps({"id": _next_id, "tool": tool, "args": args}) + "\n").encode("utf-8"))
    line = reader.readline()
    if not line:
        raise ToolError("execute bridge closed")
    try:
        resp = _json.loads(line)
    except Exception as e:
        raise ToolError("bad bridge reply: %%s" %% e)
    if resp.get("error"):
        raise ToolError(resp["error"])
    return resp.get("result", "")

def call(tool, **kwargs):
    """Call any available tool by name."""
    return _call(tool, kwargs)

def __getattr__(name):
    # A tool the role does not have (disabled, or never allowed) still routes
    # through the bridge, so wick refuses it and the script sees a catchable
    # ToolError instead of an AttributeError.
    if name.startswith("__"):
        raise AttributeError(name)
    def _missing(**kwargs):
        return _call(name, kwargs)
    return _missing

%s
def search(query):
    """List the available tools matching query, with signatures."""
    q = (query or "").lower()
    out = []
    for name in sorted(_SPECS):
        spec = _SPECS[name]
        if q in name.lower() or q in spec["desc"].lower():
            first = spec["desc"].split("\n")[0]
            out.append("%%s%%s: %%s" %% (name, spec["sig"][len(name):], first))
    if not out:
        out.append("no tools match %%r; available: %%s" %% (query, ", ".join(sorted(_SPECS))))
    return "\n".join(out)

def help(name):
    """Show one tool's signature and description."""
    spec = _SPECS.get(name)
    if spec is None:
        return "unknown tool %%r; available: %%s" %% (name, ", ".join(sorted(_SPECS)))
    return "%%s%%s:\n%%s" %% (name, spec["sig"][len(name):], spec["desc"])
`, sockPath, string(specRaw), fns.String()), nil
}
