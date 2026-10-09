package tools

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// requireBwrap returns the bwrap binary the sandbox runner resolves, or
// skips the test with a clear message when bwrap is absent. Ticket 16's
// sandbox tests reuse this helper.
func requireBwrap(t *testing.T) string {
	t.Helper()
	path, err := ResolveBinary("bwrap")
	if err != nil {
		t.Skipf("bwrap not installed: %v; skipping bubblewrap sandbox test", err)
	}
	return path
}

func runSandboxed(t *testing.T, cfg SandboxConfig, command string, args ...string) SandboxResult {
	t.Helper()
	ctx := context.Background()
	res, err := RunSandbox(ctx, cfg, command, args...)
	if err != nil {
		t.Fatalf("RunSandbox(%s %v) = %v", command, args, err)
	}
	return res
}

func TestSandboxDeniesWrites(t *testing.T) {
	bin := requireBwrap(t)
	_ = bin
	worktree := t.TempDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	outside := fmt.Sprintf("/etc/el-sandbox-probe-%d", os.Getpid())
	homeProbe := filepath.Join(home, fmt.Sprintf(".emberloop-sandbox-probe-%d", os.Getpid()))
	t.Cleanup(func() {
		os.Remove(outside)
		os.Remove(homeProbe)
		os.Remove(filepath.Join(worktree, "probe.txt"))
	})
	cfg := SandboxConfig{Worktree: worktree, Timeout: 30 * time.Second}
	// Each probe is the bare touch: its exit code is the sandbox's answer.
	probes := map[string]string{
		"worktree": filepath.Join(worktree, "probe.txt"),
		"outside":  outside,
		"home":     homeProbe,
	}
	for name, target := range probes {
		res := runSandboxed(t, cfg, "touch", target)
		if res.Code == 0 {
			t.Errorf("%s write succeeded inside the sandbox: stdout=%q stderr=%q", name, res.Stdout, res.Stderr)
		}
	}
	for _, p := range []string{filepath.Join(worktree, "probe.txt"), outside, homeProbe} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("sandboxed write created %s", p)
		}
	}
}

func TestSandboxDeniesNetwork(t *testing.T) {
	requireBwrap(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	probe := fmt.Sprintf("exec 3<>/dev/tcp/127.0.0.1/%d", ln.Addr().(*net.TCPAddr).Port)
	cfg := SandboxConfig{Worktree: t.TempDir(), Timeout: 30 * time.Second}

	res := runSandboxed(t, cfg, "bash", "-c", probe)
	if res.Code == 0 {
		t.Fatalf("sandboxed command opened a network connection: stdout=%q stderr=%q", res.Stdout, res.Stderr)
	}

	// Positive control: the same probe works outside the sandbox, so the
	// listener is real and the failure above is the sandbox, not the probe.
	out, err := exec.Command("bash", "-c", probe).CombinedOutput()
	if err != nil {
		t.Fatalf("unsandboxed probe failed, listener may be broken: %v %s", err, out)
	}
}

func TestSandboxAllowsBridgeAndReads(t *testing.T) {
	requireBwrap(t)
	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktree, "hello.txt"), []byte("hello-worktree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bridge := t.TempDir()
	cfg := SandboxConfig{Worktree: worktree, BridgeDir: bridge, Timeout: 30 * time.Second}

	script := fmt.Sprintf("cat %q && cat /etc/hostname && echo intobridge > %q",
		filepath.Join(worktree, "hello.txt"), filepath.Join(bridge, "out.txt"))
	res := runSandboxed(t, cfg, "bash", "-c", script)
	if res.Code != 0 {
		t.Fatalf("sandboxed read/bridge script failed: code=%d stdout=%q stderr=%q", res.Code, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "hello-worktree") {
		t.Errorf("worktree read missing from output: %q", res.Stdout)
	}
	host, _ := os.ReadFile("/etc/hostname")
	if len(host) > 0 && !strings.Contains(res.Stdout, strings.TrimSpace(string(host))) {
		t.Errorf("/etc/hostname read missing from output: %q", res.Stdout)
	}
	got, err := os.ReadFile(filepath.Join(bridge, "out.txt"))
	if err != nil || string(got) != "intobridge\n" {
		t.Errorf("bridge write = %q, err = %v", got, err)
	}
}

func TestSandboxTimeoutKillsProcessGroup(t *testing.T) {
	requireBwrap(t)
	cfg := SandboxConfig{Worktree: t.TempDir(), Timeout: time.Second}
	start := time.Now()
	_, err := RunSandbox(context.Background(), cfg, "bash", "-c", "sleep 300 & sleep 300")
	elapsed := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout err = %v, want a timed-out error", err)
	}
	if elapsed > 15*time.Second {
		t.Fatalf("took %v: children held the pipe", elapsed)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, _ := exec.Command("pgrep", "-f", "sleep 300").CombinedOutput()
		if len(out) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("sandboxed children survived the timeout: %s", out)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestSandboxMissingBwrapIsTypedError(t *testing.T) {
	t.Setenv("EL_BWRAP", filepath.Join(t.TempDir(), "no-such-bwrap"))
	cfg := SandboxConfig{Worktree: t.TempDir(), BridgeDir: t.TempDir(), Timeout: 30 * time.Second}
	_, err := RunSandbox(context.Background(), cfg, "bash", "-c", "echo should-never-run")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var unavailable *SandboxUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("err type = %T, want *SandboxUnavailableError", err)
	}
	if !strings.Contains(err.Error(), "el deps") {
		t.Errorf("err = %q, want it to name el deps", err)
	}
	if !IsSandboxUnavailable(err) {
		t.Errorf("IsSandboxUnavailable(%v) = false, want true", err)
	}
}

func TestSandboxForcedSelfTestFailureRefuses(t *testing.T) {
	requireBwrap(t)
	t.Setenv("EL_SANDBOX_FORCE_FAIL", "1")
	worktree := t.TempDir()
	bridge := t.TempDir()
	canary := filepath.Join(bridge, "canary.txt")
	cfg := SandboxConfig{Worktree: worktree, BridgeDir: bridge, Timeout: 30 * time.Second}
	_, err := RunSandbox(context.Background(), cfg, "bash", "-c",
		fmt.Sprintf("echo ran > %q", canary))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var unavailable *SandboxUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("err type = %T, want *SandboxUnavailableError", err)
	}
	if !strings.Contains(err.Error(), "el deps") {
		t.Errorf("err = %q, want it to name el deps", err)
	}
	if _, statErr := os.Stat(canary); statErr == nil {
		t.Error("command ran despite the failed self-test: it must never run unsandboxed")
	}
}

func TestResolveBinaryPrefersBinDir(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "bwrap")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_BIN_DIR", dir)
	got, err := ResolveBinary("bwrap")
	if err != nil {
		t.Fatal(err)
	}
	if got != fake {
		t.Fatalf("ResolveBinary = %q, want %q", got, fake)
	}
}

func TestResolveBinaryMissingNamesDeps(t *testing.T) {
	t.Setenv("EL_BIN_DIR", filepath.Join(t.TempDir(), "empty-bin"))
	t.Setenv("PATH", t.TempDir())
	_, err := ResolveBinary("no-such-tool-xyz")
	if !IsSandboxUnavailable(err) || !strings.Contains(err.Error(), "el deps") {
		t.Fatalf("err = %v, want the typed error naming el deps", err)
	}
}

func TestSandboxRejectsOverlappingBridge(t *testing.T) {
	requireBwrap(t)
	worktree := t.TempDir()
	inside := filepath.Join(worktree, "sub")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"same":   worktree,
		"inside": inside,
		"above":  filepath.Dir(worktree),
	}
	for name, bridge := range cases {
		_, err := RunSandbox(context.Background(),
			SandboxConfig{Worktree: worktree, BridgeDir: bridge, Timeout: 30 * time.Second},
			"bash", "-c", "exit 0")
		if err == nil || !strings.Contains(err.Error(), "overlap") {
			t.Errorf("%s bridge: err = %v, want the overlap refusal", name, err)
		}
	}
}

func TestSandboxRejectsBadBridge(t *testing.T) {
	requireBwrap(t)
	missing := filepath.Join(t.TempDir(), "no-such-dir")
	_, err := RunSandbox(context.Background(),
		SandboxConfig{Worktree: t.TempDir(), BridgeDir: missing, Timeout: 30 * time.Second},
		"bash", "-c", "exit 0")
	if err == nil || !strings.Contains(err.Error(), "bridge") {
		t.Errorf("missing bridge: err = %v, want the bridge error, not bwrap's exit 1", err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = RunSandbox(context.Background(),
		SandboxConfig{Worktree: t.TempDir(), BridgeDir: file, Timeout: 30 * time.Second},
		"bash", "-c", "exit 0")
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("file bridge: err = %v, want the not-a-directory error", err)
	}
}
