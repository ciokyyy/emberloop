package web

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
)

func TestDesignSessionMCPGetsUniquePortsOnlyForDesignRoles(t *testing.T) {
	d := &Daemon{}
	b := &ClaudeBackend{mcpCommand: "/bin/echo", browserEnv: d.sessionToolEnv}
	for i := 0; i < 2; i++ {
		id := "design-" + strconv.Itoa(i)
		if _, err := d.previewPort(id); err != nil {
			t.Fatal(err)
		}
		servers, err := b.mcpServersFor(id, "run-worker")
		if err != nil {
			t.Fatal(err)
		}
		port := envValue(servers[0].Env, "PORT")
		if port == "" {
			t.Fatal("design Session has no PORT")
		}
		if i == 0 {
			t.Setenv("FIRST_DESIGN_PORT", port)
		} else if port == os.Getenv("FIRST_DESIGN_PORT") {
			t.Fatal("parallel Sessions got the same port")
		}
	}
	restarted := &Daemon{sessions: map[string]*session{"restored": {meta: SessionMeta{ID: "restored", DesignSession: true, PreviewPort: 45678}}}}
	servers, err := (&ClaudeBackend{mcpCommand: "/bin/echo", browserEnv: restarted.sessionToolEnv}).mcpServersFor("restored", "run-worker")
	if err != nil {
		t.Fatal(err)
	}
	if got := envValue(servers[0].Env, "PORT"); got != "45678" {
		t.Fatalf("restored Session PORT=%q, want persisted port", got)
	}
	for _, id := range []string{"ordinary-worker", "ordinary-reviewer"} {
		servers, err := b.mcpServersFor(id, "run-worker")
		if err != nil {
			t.Fatal(err)
		}
		if got := envValue(servers[0].Env, "PORT"); got != "" {
			t.Fatalf("non-design Session unexpectedly has PORT=%s", got)
		}
	}
}

func TestFailedDesignSessionStartReleasesPort(t *testing.T) {
	d := NewDaemon(tempDir(t), nil)
	meta := SessionMeta{ID: "failed-design", Harness: "missing", WorkDir: tempDir(t), Status: "idle", DesignSession: true}
	if _, err := d.startSession(meta); err == nil {
		t.Fatal("expected missing backend to fail Session start")
	}
	if _, ok := d.previewPortFor(meta.ID); ok {
		t.Fatal("failed Session retained its preview port")
	}
}

func envValue(env [][2]string, key string) string {
	for _, item := range env {
		if item[0] == key {
			return item[1]
		}
	}
	return ""
}

func TestStopPreviewProcessesInWorktree(t *testing.T) {
	root := tempDir(t)
	cmd := exec.Command("sh", "-c", "exec sleep 30")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "EL_PREVIEW_SESSION=session-1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopPreviewProcesses("session-1", root)
	if err := cmd.Wait(); err == nil {
		t.Fatal("expected preview process to be terminated")
	}
}

func TestStopPreviewProcessesLeavesOtherSessionInSharedWorktree(t *testing.T) {
	root := tempDir(t)
	cmd := exec.Command("sh", "-c", "exec sleep 30")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "EL_PREVIEW_SESSION=session-2")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
	stopPreviewProcesses("session-1", root)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("another Session's process was killed: %v", err)
	}
}
