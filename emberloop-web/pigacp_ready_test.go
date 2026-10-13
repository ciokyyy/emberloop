package web

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A pig Session start waits for its el server's ready file, so the first
// prompt never reaches a model that has no tools yet. A Session whose agent
// dir this Backend never prepared does not wait.
func TestWaitMCPReadyWaitsForTheReadyFile(t *testing.T) {
	t.Parallel()
	b := &PigACPBackend{ClaudeBackend: &ClaudeBackend{}, stateDir: tempDir(t)}

	start := time.Now()
	b.waitMCPReady("unprepared")
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("an unprepared Session waited %s, want no wait", time.Since(start))
	}

	path := b.readyFile("s1")
	if want := filepath.Join(b.stateDir, "enderbloom", "pig-acp", "s1", "mcp-ready"); path != want {
		t.Fatalf("ready file = %q, want %q", path, want)
	}
	b.readyFiles = map[string]string{"s1": path}
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, []byte("ready\n"), 0o600)
	}()
	start = time.Now()
	b.waitMCPReady("s1")
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond || elapsed > 5*time.Second {
		t.Fatalf("waited %s, want about 150ms (until the file appeared)", elapsed)
	}
	if _, ok := b.readyFiles["s1"]; ok {
		t.Fatal("ready entry not consumed")
	}
}
