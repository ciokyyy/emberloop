package web

// Restart drain coverage: a prod `el restart` never restarted its Daemon.
// The drain loop already forces the restart past idle, so the wedge was
// past the drain: Shutdown waited on turn-end mining with no timeout, and
// nothing reported what the drain waited on. These tests pin the blockers
// (busy Session, outstanding Group, running Run, starting Slot, live scan),
// the bounded Shutdown, the stuck-request re-arm, and the state endpoint.
import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The restart state names every drain blocker: the busy Session, the fresh
// Group with an undelivered result, the running Run, the starting slot and
// the live turn-end scan. Settling each clears it.
func TestRestartStateNamesBlockers(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.sessions["busy"] = &session{live: true, busy: true, last: time.Now()}
	d.sessions["kid"] = &session{live: true, last: time.Now()}
	d.groups["g1"] = &spawnGroup{ID: "g1", Parent: "busy", Members: []string{"kid"}, Deliver: "group", fresh: true}
	d.runs["r1"] = &runState{ID: "r1", Status: runStatusRunning, notify: make(chan struct{})}
	d.starting = 1
	d.mineWG.Add(1)
	d.mineMu.Lock()
	d.mineActive++
	d.mineMu.Unlock()

	st := d.RestartState()
	if st.Requested {
		t.Error("requested = true before any request")
	}
	if !equalStrings(st.Sessions, []string{"busy"}) {
		t.Errorf("sessions = %v, want [busy]", st.Sessions)
	}
	if !equalStrings(st.Groups, []string{"g1"}) {
		t.Errorf("groups = %v, want [g1]", st.Groups)
	}
	if !equalStrings(st.Runs, []string{"r1"}) {
		t.Errorf("runs = %v, want [r1]", st.Runs)
	}
	if st.Starting != 1 {
		t.Errorf("starting = %d, want 1", st.Starting)
	}
	if !st.Mining {
		t.Error("mining = false with a scan running")
	}

	if !d.RequestRestart() {
		t.Fatal("RequestRestart returned false")
	}
	st = d.RestartState()
	if !st.Requested || st.Since.IsZero() {
		t.Errorf("requested state = %+v, want requested with a time", st)
	}
	if st.Drain != (5 * time.Minute).String() {
		t.Errorf("drain = %q, want the 5m default", st.Drain)
	}
	// The request pauses the running Run, so it leaves the blockers.
	if !equalStrings(st.Runs, nil) {
		t.Errorf("runs after request = %v, want none (paused)", st.Runs)
	}

	d.mu.Lock()
	d.sessions["busy"].busy = false
	d.groups["g1"].Delivered = true
	d.starting = 0
	d.mu.Unlock()
	d.mineWG.Done()
	d.mineMu.Lock()
	d.mineActive--
	d.mineMu.Unlock()
	st = d.RestartState()
	if len(st.Sessions) != 0 || len(st.Groups) != 0 || st.Starting != 0 || st.Mining {
		t.Errorf("blockers after settling = %+v, want none", st)
	}
	if !d.Idle(0) {
		t.Error("Daemon should be idle once every blocker settles")
	}
}

// Shutdown never waits out a hung turn-end scan: it logs the bound and
// closes the Sessions, so the restart reaches Exec past the drain.
func TestShutdownBoundsHungMining(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.mineWait = 20 * time.Millisecond
	d.mineWG.Add(1) // a scan that never finishes
	defer d.mineWG.Done()
	d.sessions["s1"] = &session{live: true, meta: SessionMeta{ID: "s1", Harness: "fake"}}

	done := make(chan struct{})
	go func() { d.Shutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown blocked past its mining bound; the restart would never reach Exec")
	}
	d.mu.Lock()
	live := d.sessions["s1"].live
	d.mu.Unlock()
	if live {
		t.Error("Shutdown left the Session live")
	}
	raw, err := os.ReadFile(filepath.Join(d.stateDir, "enderbloom", restartLogName))
	if err != nil {
		t.Fatalf("restart.log: %v", err)
	}
	if !strings.Contains(string(raw), "mining still running") {
		t.Errorf("restart.log = %q, want the mining bound line", raw)
	}
}

// A repeat older than the drain plus a minute re-arms a stuck request and
// reports true again; a repeat inside the window stays a no-op.
func TestRequestRestartRearmsStuckRequest(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	now := time.Now()
	d.clock = func() time.Time { return now }

	if !d.RequestRestart() {
		t.Fatal("first RequestRestart returned false")
	}
	<-d.RestartCh()
	if d.RequestRestart() {
		t.Fatal("repeat inside the window returned true, want a no-op false")
	}
	now = now.Add(defaultRestartDrain + time.Minute + time.Second)
	if !d.RequestRestart() {
		t.Fatal("repeat past drain+1m returned false, want a re-arm true")
	}
	select {
	case <-d.RestartCh():
	case <-time.After(time.Second):
		t.Fatal("re-arm did not re-signal the watcher")
	}
	if !d.RestartRequested() {
		t.Error("request not pending after the re-arm")
	}
}

// GET /api/restart reports the pending request and its blockers; POST keeps
// answering 202 with new and the same state.
func TestHandleRestartReportsState(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	get := func(path string) (int, RestartState) {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var st RestartState
		if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
			t.Fatalf("GET %s decode: %v", path, err)
		}
		return resp.StatusCode, st
	}
	if code, st := get("/api/restart"); code != http.StatusOK || st.Requested {
		t.Fatalf("GET before request = %d %+v, want 200 unrequested", code, st)
	}

	d.sessions["busy"] = &session{live: true, busy: true, last: time.Now()}
	resp, err := http.Post(srv.URL+"/api/restart", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var post struct {
		Restart string       `json:"restart"`
		New     bool         `json:"new"`
		State   RestartState `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&post); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted || !post.New || !post.State.Requested {
		t.Fatalf("POST = %d %+v, want 202 new and requested", resp.StatusCode, post)
	}
	if code, st := get("/api/restart"); code != http.StatusOK || !st.Requested || !equalStrings(st.Sessions, []string{"busy"}) {
		t.Fatalf("GET after request = %d %+v, want 200 requested with [busy]", code, st)
	}

	resp, err = http.Post(srv.URL+"/api/restart", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var repeat struct {
		New bool `json:"new"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repeat); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if repeat.New {
		t.Error("repeat POST new = true, want false")
	}
}

// Every restart step lands timestamped in the state dir's restart.log, so an
// Exec failure stays visible after stdout went to a dead socket.
func TestLogRestartWritesRestartLog(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.LogRestart("hello %s", "world")
	d.LogRestart("second line")
	raw, err := os.ReadFile(filepath.Join(d.stateDir, "enderbloom", restartLogName))
	if err != nil {
		t.Fatalf("restart.log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "hello world") || !strings.Contains(lines[1], "second line") {
		t.Errorf("restart.log = %q, want two appended lines", raw)
	}
}

// A re-arm consumed after a failed exec's cancel re-pauses the resumed Runs
// without queueing another drain cycle behind itself.
func TestReassertRestartRepausesWithoutSignaling(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.runs["r1"] = &runState{ID: "r1", Status: runStatusRunning, notify: make(chan struct{})}
	if !d.RequestRestart() {
		t.Fatal("RequestRestart returned false")
	}
	<-d.RestartCh()
	// A failed exec's cancel clears the request and resumes the Run.
	d.mu.Lock()
	d.restartReq = false
	d.runs["r1"].Status = runStatusRunning
	d.mu.Unlock()

	d.ReassertRestart()
	if !d.RestartRequested() {
		t.Fatal("request not re-established")
	}
	d.mu.Lock()
	status := d.runs["r1"].Status
	d.mu.Unlock()
	if status != runStatusPaused {
		t.Errorf("run status = %q, want paused again", status)
	}
	select {
	case <-d.RestartCh():
		t.Error("re-assert signaled the watcher; cycles would queue forever")
	default:
	}
}
