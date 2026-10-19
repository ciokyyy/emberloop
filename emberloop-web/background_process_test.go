package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// backgroundDaemon is a Daemon with one fake Session in a temp workdir, served
// through the authenticated routes the browser and the tool call.
func backgroundDaemon(t *testing.T) (*Daemon, *httptest.Server, string) {
	t.Helper()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	return d, srv, id
}

// backgroundStart starts one command and returns the process map it answered.
func backgroundStart(t *testing.T, base, sessionID, command string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"command": command})
	resp, err := http.Post(base+"/api/sessions/"+sessionID+"/background", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			// A refusal may be plain text: http.NotFound writes no JSON.
			out["raw"] = string(raw)
		}
	}
	return resp.StatusCode, out
}

// backgroundGet reads the collection (lines <= 0 omits output).
func backgroundGet(t *testing.T, base, sessionID string, lines int) []map[string]any {
	t.Helper()
	path := base + "/api/sessions/" + sessionID + "/background"
	if lines > 0 {
		path += "?tail=" + strconv.Itoa(lines)
	}
	resp, err := http.Get(path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", resp.StatusCode)
	}
	var out struct {
		Processes []map[string]any `json:"processes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Processes
}

func processID(t *testing.T, view map[string]any) string {
	t.Helper()
	proc, _ := view["process"].(map[string]any)
	id, _ := proc["id"].(string)
	if id == "" {
		t.Fatalf("start answered no process: %v", view)
	}
	return id
}

func processPID(t *testing.T, view map[string]any) int {
	t.Helper()
	proc, _ := view["process"].(map[string]any)
	pid, _ := proc["pid"].(float64)
	if pid <= 0 {
		t.Fatalf("start answered no pid: %v", view)
	}
	return int(pid)
}

// alive reports whether pid is still a live process: a zombie has exited but is
// not reaped yet, so kill(pid, 0) still succeeds for it.
func alive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	if end := strings.LastIndexByte(string(raw), ')'); end >= 0 {
		if fields := strings.Fields(string(raw)[end+1:]); len(fields) > 0 && fields[0] == "Z" {
			return false
		}
	}
	return true
}

// waitGone waits for pid to disappear.
func waitGone(t *testing.T, pid int) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// A start returns the first output in the same turn, keeps the process with its
// exit state, and a read serves the tail after it exited.
func TestBackgroundStartKeepsOutputAndExit(t *testing.T) {
	t.Parallel()
	_, srv, id := backgroundDaemon(t)
	code, out := backgroundStart(t, srv.URL, id, "printf 'first line\\n'; sleep 0.3; printf 'last line\\n'")
	if code != http.StatusOK {
		t.Fatalf("start status = %d: %v", code, out)
	}
	if got := out["output"]; !strings.Contains(got.(string), "first line") {
		t.Fatalf("start must return the first output, got %q", got)
	}

	// The process is kept after it exits, with its status and full output.
	deadline := time.Now().Add(5 * time.Second)
	var last []map[string]any
	for time.Now().Before(deadline) {
		last = backgroundGet(t, srv.URL, id, 0)
		if len(last) == 1 && last[0]["running"] == false {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(last) != 1 || last[0]["running"] != false {
		t.Fatalf("exited process = %v", last)
	}
	if got, _ := last[0]["exit"].(string); got != "exit 0" {
		t.Fatalf("exit = %q, want exit 0", got)
	}
	withTail := backgroundGet(t, srv.URL, id, 40)
	if got, _ := withTail[0]["output"].(string); !strings.Contains(got, "first line") || !strings.Contains(got, "last line") {
		t.Fatalf("tail = %q, want both lines", got)
	}
}

// A command that dies at once reports its status and output in the start reply,
// rather than looking like a healthy server.
func TestBackgroundStartReportsAnImmediateFailure(t *testing.T) {
	t.Parallel()
	_, srv, id := backgroundDaemon(t)
	code, out := backgroundStart(t, srv.URL, id, "echo boom >&2; exit 3")
	if code != http.StatusOK {
		t.Fatalf("start status = %d: %v", code, out)
	}
	proc, _ := out["process"].(map[string]any)
	if proc["running"] != false || proc["exit"] != "exit 3" {
		t.Fatalf("failed command = %v", proc)
	}
	if got, _ := out["output"].(string); !strings.Contains(got, "boom") {
		t.Fatalf("output = %q, want the stderr line", got)
	}
}

// Stop kills the whole process group, so whatever the command spawned dies too.
func TestBackgroundStopKillsTheProcessGroup(t *testing.T) {
	t.Parallel()
	_, srv, id := backgroundDaemon(t)
	code, out := backgroundStart(t, srv.URL, id, "sleep 60")
	if code != http.StatusOK {
		t.Fatalf("start status = %d: %v", code, out)
	}
	pid := processPID(t, out)
	ref := processID(t, out)
	del, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/sessions/"+id+"/background/"+ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stop status = %d", resp.StatusCode)
	}
	if !waitGone(t, pid) {
		t.Fatalf("pid %d survived the stop", pid)
	}
}

// Ending a Session stops the commands it started: a test server or watcher must
// not outlive its Session.
func TestBackgroundDiesWithTheSession(t *testing.T) {
	t.Parallel()
	_, srv, id := backgroundDaemon(t)
	code, out := backgroundStart(t, srv.URL, id, "sleep 60")
	if code != http.StatusOK {
		t.Fatalf("start status = %d: %v", code, out)
	}
	pid := processPID(t, out)
	deleteSession(t, srv.URL, id)
	if !waitGone(t, pid) {
		t.Fatalf("pid %d outlived its Session", pid)
	}
}

// The marker sweep stops a command this Daemon no longer holds, which is what a
// restart leaves behind.
func TestBackgroundMarkerSweepStopsOrphans(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	cmd := exec.Command("sleep", "60")
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), backgroundSessionEnv+"=session-1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	if !alive(pid) {
		t.Fatal("sweep fixture did not start")
	}
	// Diagnostic: which of the three conditions the sweep needs is missing.
	resolved, _ := filepath.EvalSymlinks(root)
	cwd, _ := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
	env, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
	t.Logf("root=%q resolved=%q cwd=%q marked=%v env has marker=%v", root, resolved, cwd,
		markedProcessMatches(pid, []string{backgroundSessionEnv}, "session-1", resolved),
		hasEnvValue(env, backgroundSessionEnv, "session-1"))
	stopBackgroundProcesses("session-1", root)
	if !waitGone(t, pid) {
		t.Fatalf("pid %d survived the marker sweep", pid)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("a swept process must have been signalled")
	}
	// The sweep is scoped to its Session: another Session's marker survives.
	other := exec.Command("sleep", "60")
	other.Dir = root
	other.Env = append(other.Environ(), backgroundSessionEnv+"=session-2")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(other.Process.Pid, syscall.SIGKILL) })
	stopBackgroundProcesses("session-1", root)
	if !alive(other.Process.Pid) {
		t.Fatal("the sweep killed another Session's process")
	}
}

// The cap is per Session, and a refused start leaves no record behind.
func TestBackgroundCapPerSession(t *testing.T) {
	t.Parallel()
	_, srv, id := backgroundDaemon(t)
	var pids []int
	defer func() {
		for _, pid := range pids {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}()
	for i := 0; i < backgroundMaxProcesses; i++ {
		code, out := backgroundStart(t, srv.URL, id, "sleep 60")
		if code != http.StatusOK {
			t.Fatalf("start %d status = %d: %v", i, code, out)
		}
		pids = append(pids, processPID(t, out))
	}
	code, out := backgroundStart(t, srv.URL, id, "sleep 60")
	if code != http.StatusBadRequest {
		t.Fatalf("start past the cap status = %d: %v", code, out)
	}
	if procs := backgroundGet(t, srv.URL, id, 0); len(procs) != backgroundMaxProcesses {
		t.Fatalf("cap left %d processes", len(procs))
	}
}

// An empty command and an ended Session are refused; an unknown process 404s.
func TestBackgroundRefusals(t *testing.T) {
	t.Parallel()
	_, srv, id := backgroundDaemon(t)
	if code, _ := backgroundStart(t, srv.URL, id, "   "); code != http.StatusBadRequest {
		t.Fatalf("empty command status = %d, want 400", code)
	}
	if code, _ := backgroundStart(t, srv.URL, "nope", "sleep 1"); code != http.StatusNotFound {
		t.Fatalf("unknown Session status = %d, want 404", code)
	}
	if resp, err := http.Get(srv.URL + "/api/sessions/" + id + "/background/deadbeef"); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("unknown process status = %d, want 404", resp.StatusCode)
		}
	}
	deleteSession(t, srv.URL, id)
	if code, _ := backgroundStart(t, srv.URL, id, "sleep 1"); code != http.StatusConflict {
		t.Fatalf("ended Session status = %d, want 409", code)
	}
}

// The watch stream carries the list on connect and again when a command starts,
// so the Processes panel updates without polling.
func TestBackgroundWatchStreamsChanges(t *testing.T) {
	t.Parallel()
	_, srv, id := backgroundDaemon(t)
	resp, err := http.Get(srv.URL + "/api/sessions/" + id + "/background?watch=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("watch status = %d", resp.StatusCode)
	}
	lines := bufio.NewScanner(resp.Body)
	readEvent := func() []map[string]any {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if !lines.Scan() {
				t.Fatalf("watch stream ended: %v", lines.Err())
			}
			text := lines.Text()
			if !strings.HasPrefix(text, "data: ") {
				continue
			}
			var out struct {
				Processes []map[string]any `json:"processes"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(text, "data: ")), &out); err != nil {
				t.Fatalf("watch payload: %v", err)
			}
			return out.Processes
		}
		t.Fatal("no watch event within the deadline")
		return nil
	}
	if first := readEvent(); len(first) != 0 {
		t.Fatalf("first watch event = %v, want no processes", first)
	}
	code, out := backgroundStart(t, srv.URL, id, "sleep 60")
	if code != http.StatusOK {
		t.Fatalf("start status = %d: %v", code, out)
	}
	defer syscall.Kill(processPID(t, out), syscall.SIGKILL)
	got := readEvent()
	if len(got) != 1 || got[0]["command"] != "sleep 60" {
		t.Fatalf("watch event after a start = %v", got)
	}
}
