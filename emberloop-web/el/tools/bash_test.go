package tools

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBashRunsInRootAndReportsExit(t *testing.T) {
	dir := t.TempDir()
	out, err := run(t, bashTool{}, dir, map[string]any{"command": "pwd; echo oops >&2; exit 3"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, dir) || !strings.Contains(out, "oops") || !strings.Contains(out, "[exit status 3]") {
		t.Fatalf("out = %q", out)
	}
}

func TestBashTimeoutKillsChildren(t *testing.T) {
	start := time.Now()
	out, err := run(t, bashTool{}, t.TempDir(), map[string]any{"command": "sleep 30 & sleep 30", "timeout": 1})
	if err != nil || !strings.Contains(out, "timed out") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if time.Since(start) > 8*time.Second {
		t.Fatalf("took %v: children held the pipe", time.Since(start))
	}
}

func TestSanitizeStripsWithoutTruncating(t *testing.T) {
	got := sanitize("\x1b[31mred\x1b[0m\r\n\n\n\n\nx" + strings.Repeat("y", 40000))
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\r") || strings.Contains(got, "truncated") {
		t.Fatalf("got %q...", got[:40])
	}
	if len(got) < 40000 {
		t.Fatalf("sanitize truncated %d bytes; oversize output goes to the overflow store in Execute", len(got))
	}
}

func TestRTKRewriteLeavesRtkAndEmptyAlone(t *testing.T) {
	for _, c := range []string{"", "  ", "rtk git status"} {
		if got := RTKRewrite(t.Context(), c); got != c {
			t.Errorf("RTKRewrite(%q) = %q", c, got)
		}
	}
}

func TestRTKRewriteWrapsAKnownCommand(t *testing.T) {
	if _, err := exec.LookPath("rtk"); err != nil {
		t.Skip("rtk not installed")
	}
	if got := RTKRewrite(t.Context(), "git status"); got != "rtk git status" {
		t.Fatalf("got %q", got)
	}
}

// Seam: bash uses the overflow store above the threshold instead of
// truncating: a handle comes back and the full output is pageable.
func TestBashLargeOutputStoredWithHandle(t *testing.T) {
	t.Setenv("EL_STATE_DIR", t.TempDir())
	t.Setenv("EL_OVERFLOW_THRESHOLD_BYTES", "100")
	out, err := run(t, bashTool{}, t.TempDir(), map[string]any{"command": "seq 1 50"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ctx:") || !strings.Contains(out, "lines") {
		t.Fatalf("large bash output not stored:\n%s", out)
	}
	if strings.Contains(out, "\n25\n") {
		t.Fatalf("stored summary must not carry the full middle:\n%s", out)
	}
}
