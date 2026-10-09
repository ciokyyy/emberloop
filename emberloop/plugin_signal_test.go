package el

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Seam (ticket 08): the plugin action's signal — compress.sh sends the
// focused pane from the action context to `el request-compress`. A
// fake el on the path records the call; no live herdr runs.

// pluginScript is the shipped action, resolved from this package: the
// herdr-emberloop directory beside the el module.
func pluginScript(t *testing.T) string {
	t.Helper()
	script := filepath.Join("..", "herdr-emberloop", "compress.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("shipped compress action missing: %v", err)
	}
	return script
}

// fakeElPig plants a stub binary recording its argv, one call per line,
// and returns its path for EL_PIG_BIN.
func fakeElPig(t *testing.T) (bin, calls string) {
	t.Helper()
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls")
	stub := filepath.Join(dir, "el")
	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + calls + "\n"
	if err := os.WriteFile(stub, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return stub, calls
}

func runCompressAction(t *testing.T, contextJSON string) (string, int, string) {
	t.Helper()
	bin, calls := fakeElPig(t)
	cmd := exec.Command("bash", pluginScript(t))
	cmd.Env = append(os.Environ(),
		"EL_PIG_BIN="+bin,
		`HERDR_PLUGIN_CONTEXT_JSON=`+contextJSON,
	)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatalf("action run: %v", err)
		}
	}
	raw, _ := os.ReadFile(calls)
	return strings.TrimSpace(string(raw)), code, string(out)
}

// The action signals the focused pane's compress through request-compress.
func TestCompressActionSignalsFocusedPane(t *testing.T) {
	isolateState(t)
	got, code, _ := runCompressAction(t, `{"focused_pane_id":"pane-9"}`)
	if code != 0 {
		t.Fatalf("action exit = %d, want 0", code)
	}
	if got != "request-compress pane-9" {
		t.Errorf("action signal = %q, want %q", got, "request-compress pane-9")
	}
}

// Without a focused pane the action fails naming it and signals nothing.
func TestCompressActionWithoutFocusedPaneFails(t *testing.T) {
	isolateState(t)
	got, code, out := runCompressAction(t, `{}`)
	if code == 0 {
		t.Error("an action with no focused pane must fail")
	}
	if !strings.Contains(out, "focused pane") {
		t.Errorf("the error must name the focused pane, got %q", out)
	}
	if got != "" {
		t.Errorf("a failed action must signal nothing, got %q", got)
	}
}

// Malformed action context fails cleanly: no traceback, no signal.
func TestCompressActionMalformedContextFails(t *testing.T) {
	isolateState(t)
	got, code, out := runCompressAction(t, `{"focused_pane_id":`)
	if code == 0 {
		t.Error("malformed context must fail")
	}
	if strings.Contains(out, "Traceback") {
		t.Errorf("malformed context must not leak a traceback, got %q", out)
	}
	if !strings.Contains(out, "focused pane") {
		t.Errorf("the error must name the focused pane, got %q", out)
	}
	if got != "" {
		t.Errorf("a failed action must signal nothing, got %q", got)
	}
}
