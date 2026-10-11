package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/emberloop-web/testsupport"
)

// The Daemon re-executes itself, same pid, once its binary is replaced and it
// is idle.
func TestRestartsWhenBinaryChanges(t *testing.T) {
	dir := testsupport.TempDir(t)
	bin := filepath.Join(dir, "el-web")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, "-addr", "127.0.0.1:0", "-state-dir", filepath.Join(dir, "state"), "-restart-check", "100ms")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	wait := func(want string) {
		t.Helper()
		deadline := time.After(20 * time.Second)
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatalf("daemon exited before %q", want)
				}
				if strings.Contains(l, want) {
					return
				}
			case <-deadline:
				t.Fatalf("no %q line", want)
			}
		}
	}
	wait("el-web on")

	// Replace the binary with a new file, as go build does.
	raw, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	next := bin + ".new"
	if err := os.WriteFile(next, append(raw, 0), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(next, bin); err != nil {
		t.Fatal(err)
	}
	wait("restarting")
	wait("el-web on")
}
