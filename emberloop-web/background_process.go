package web

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// backgroundSessionEnv marks a process the Daemon runs for one Session outside
// its turn. The marker travels to its children, so the /proc sweep that stops a
// design Session's dev server catches an escaped grandchild too.
const backgroundSessionEnv = "EL_BACKGROUND_SESSION"

const (
	// backgroundMaxBytes caps the combined output kept per process: enough for
	// a server's startup log, small enough that a chatty process cannot grow
	// the Daemon.
	backgroundMaxBytes = 128 << 10
	// backgroundReadLines is the tail a read returns when none is asked for.
	backgroundReadLines = 40
	// backgroundMaxProcesses caps how many run at once for one Session.
	backgroundMaxProcesses = 8
	// backgroundStartWait is how long start waits for the first output or an
	// exit, so a command that dies at once reports why in the same turn.
	backgroundStartWait = 750 * time.Millisecond
	// backgroundStopGrace is how long a process group gets between SIGTERM and
	// SIGKILL.
	backgroundStopGrace = 300 * time.Millisecond
	// backgroundWatchHeartbeat keeps an idle stream alive through proxies.
	backgroundWatchHeartbeat = 15 * time.Second
)

// The refusals a background route maps to status codes.
var (
	errNoSession          = errors.New("no such session")
	errSessionEnded       = errors.New("session ended")
	errBackgroundCommand  = errors.New("command is required")
	errTooManyBackgrounds = errors.New("too many background commands")
)

// outputRing keeps the last max bytes of a process's combined output. A write
// that passes the cap drops whole lines from the front, so a tail never starts
// mid-line.
type outputRing struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func newOutputRing(max int) *outputRing { return &outputRing{max: max} }

func (r *outputRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf.Write(p)
	if r.buf.Len() > r.max {
		b := r.buf.Bytes()
		cut := len(b) - r.max
		if i := bytes.IndexByte(b[cut:], '\n'); i >= 0 {
			cut += i + 1
		} else {
			cut = len(b)
		}
		rest := append([]byte(nil), b[cut:]...)
		r.buf.Reset()
		r.buf.Write(rest)
	}
	return len(p), nil
}

// tail returns the last lines of output; lines <= 0 returns everything kept.
func (r *outputRing) tail(lines int) string {
	r.mu.Lock()
	b := append([]byte(nil), r.buf.Bytes()...)
	r.mu.Unlock()
	if lines <= 0 || len(b) == 0 {
		return string(b)
	}
	parts := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}

func (r *outputRing) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Len()
}

// backgroundProcess is one command the Daemon runs for a Session outside its
// turn: a dev server, a watcher, a long test run. The agent starts it with the
// background tool and reads or stops it later; the browser shows the same list.
type backgroundProcess struct {
	id      string
	command string
	session string
	pid     int
	started time.Time
	out     *outputRing

	mu     sync.Mutex
	exited bool
	exit   string
	gone   bool
}

// finish records how the process ended. The first caller wins: cmd.Wait runs
// once, and stop consults the recorded state.
func (p *backgroundProcess) finish(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.exited {
		return
	}
	p.exited = true
	switch {
	case err == nil:
		p.exit = "exit 0"
	default:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			p.exit = "exit " + strconv.Itoa(ee.ExitCode())
		} else {
			p.exit = err.Error()
		}
	}
}

func (p *backgroundProcess) state() (exited bool, exit string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exited, p.exit
}

// group stops the process group the Daemon started, SIGTERM first and SIGKILL
// after the grace period. It never signals a recycled pid: the process group id
// is the direct child's pid, which stays unreaped until cmd.Wait returns.
func (p *backgroundProcess) group() error {
	p.mu.Lock()
	reaped := p.gone
	p.mu.Unlock()
	if reaped || p.pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-p.pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(backgroundStopGrace)
	for time.Now().Before(deadline) {
		if exited, _ := p.state(); exited {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(-p.pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// backgroundView is one process as the API and the browser see it.
type backgroundView struct {
	ID        string `json:"id"`
	Command   string `json:"command"`
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
	Running   bool   `json:"running"`
	Exit      string `json:"exit,omitempty"`
	Output    string `json:"output,omitempty"`
}

func (p *backgroundProcess) view(lines int, withOutput bool) backgroundView {
	exited, exit := p.state()
	v := backgroundView{
		ID:        p.id,
		Command:   p.command,
		PID:       p.pid,
		StartedAt: p.started.Format(time.RFC3339),
		Running:   !exited,
		Exit:      exit,
	}
	if withOutput {
		v.Output = p.out.tail(lines)
	}
	return v
}

// backgroundForLocked returns the Session's process map, creating it on first
// use. Callers hold d.bgMu.
func (d *Daemon) backgroundForLocked(sessionID string) map[string]*backgroundProcess {
	if d.backgrounds == nil {
		d.backgrounds = map[string]map[string]*backgroundProcess{}
	}
	m, ok := d.backgrounds[sessionID]
	if !ok {
		m = map[string]*backgroundProcess{}
		d.backgrounds[sessionID] = m
	}
	return m
}

// startBackground runs command for one Session in its workdir, in a process
// group of its own so stop reaches whatever it spawns. It returns the record
// once the first output or exit has landed, so a command that fails at once
// reports why instead of looking healthy.
func (d *Daemon) startBackground(sessionID, command string) (*backgroundProcess, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, errBackgroundCommand
	}
	sessionID = strings.TrimSpace(sessionID)
	d.mu.Lock()
	s, ok := d.sessions[sessionID]
	ended := ok && s.meta.Status == "ended"
	var workDir string
	if ok {
		workDir = s.meta.WorkDir
	}
	d.mu.Unlock()
	if !ok {
		return nil, errNoSession
	}
	if ended {
		return nil, errSessionEnded
	}

	d.bgMu.Lock()
	m := d.backgroundForLocked(sessionID)
	if len(m) >= backgroundMaxProcesses {
		d.bgMu.Unlock()
		return nil, fmt.Errorf("%w: at most %d run at once for one session, stop one first", errTooManyBackgrounds, backgroundMaxProcesses)
	}
	proc := &backgroundProcess{
		id:      backgroundID(),
		command: command,
		session: sessionID,
		started: time.Now().UTC(),
		out:     newOutputRing(backgroundMaxBytes),
	}
	m[proc.id] = proc
	d.bgMu.Unlock()

	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), backgroundSessionEnv+"="+sessionID)
	// A design Session's reserved preview port reaches the command, so
	// .emberloop/preview finds the port the Daemon told the browser about.
	if port, ok := d.previewPortFor(sessionID); ok {
		cmd.Env = append(cmd.Env, "PORT="+strconv.Itoa(port))
	}
	cmd.Stdout, cmd.Stderr = proc.out, proc.out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		d.bgMu.Lock()
		delete(m, proc.id)
		d.bgMu.Unlock()
		return nil, err
	}
	proc.pid = cmd.Process.Pid
	// The child is reaped here, so p.gone marks the pid as no longer ours to
	// signal before any group kill can touch a recycled one.
	go func() {
		err := cmd.Wait()
		proc.mu.Lock()
		proc.gone = true
		proc.mu.Unlock()
		proc.finish(err)
		d.wakeSession(sessionID)
	}()

	deadline := time.Now().Add(backgroundStartWait)
	for time.Now().Before(deadline) {
		if exited, _ := proc.state(); exited || proc.out.len() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	d.wakeSession(sessionID)
	return proc, nil
}

// listBackgrounds returns one Session's processes, oldest first.
func (d *Daemon) listBackgrounds(sessionID string) []*backgroundProcess {
	d.bgMu.Lock()
	m := d.backgrounds[sessionID]
	out := make([]*backgroundProcess, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	d.bgMu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].started.Before(out[j].started) })
	return out
}

// backgroundByID returns one process of one Session.
func (d *Daemon) backgroundByID(sessionID, ref string) (*backgroundProcess, bool) {
	d.bgMu.Lock()
	defer d.bgMu.Unlock()
	p, ok := d.backgrounds[sessionID][ref]
	return p, ok
}

// stopBackground stops one process and keeps it in the list for its output.
func (d *Daemon) stopBackground(sessionID, ref string) bool {
	p, ok := d.backgroundByID(sessionID, ref)
	if !ok {
		return false
	}
	_ = p.group()
	d.wakeSession(sessionID)
	return true
}

// stopSessionBackgrounds stops everything a Session started and forgets it, so
// an ended Session leaves no server behind. Processes started by an earlier
// Daemon run are caught by the marker sweep through their worktree.
func (d *Daemon) stopSessionBackgrounds(sessionID, workDir string) {
	d.bgMu.Lock()
	procs := d.backgrounds[sessionID]
	delete(d.backgrounds, sessionID)
	d.bgMu.Unlock()
	for _, p := range procs {
		_ = p.group()
	}
	stopBackgroundProcesses(sessionID, workDir)
}

// wakeSession releases every watcher of one Session: the event stream and the
// background stream both re-read the state they serve.
func (d *Daemon) wakeSession(sessionID string) {
	d.mu.Lock()
	s, ok := d.sessions[sessionID]
	var notify chan struct{}
	if ok {
		notify = s.notify
		s.notify = make(chan struct{})
	}
	d.mu.Unlock()
	if ok && notify != nil {
		close(notify)
	}
}

// backgroundID is the short handle the tool and the browser use for a process.
func backgroundID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

// handleSessionBackground serves a Session's background processes: the list,
// one process with its output tail, a start and a stop. rest is the path after
// "background", empty for the collection.
func (d *Daemon) handleSessionBackground(w http.ResponseWriter, r *http.Request, sessionID, rest string) {
	d.mu.Lock()
	_, ok := d.sessions[sessionID]
	d.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch {
	case rest == "" && r.Method == http.MethodGet:
		if r.URL.Query().Get("watch") == "1" {
			d.watchBackground(w, r, sessionID)
			return
		}
		lines, withOutput := backgroundTailQuery(r)
		out := make([]backgroundView, 0)
		for _, p := range d.listBackgrounds(sessionID) {
			out = append(out, p.view(lines, withOutput))
		}
		writeJSON(w, http.StatusOK, map[string]any{"processes": out})
	case rest == "" && r.Method == http.MethodPost:
		var req struct {
			Command string `json:"command"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		proc, err := d.startBackground(sessionID, req.Command)
		if err != nil {
			code := http.StatusInternalServerError
			switch {
			case errors.Is(err, errNoSession):
				code = http.StatusNotFound
			case errors.Is(err, errSessionEnded):
				code = http.StatusConflict
			case errors.Is(err, errBackgroundCommand), errors.Is(err, errTooManyBackgrounds):
				code = http.StatusBadRequest
			}
			writeJSON(w, code, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"process": proc.view(10, true), "output": proc.out.tail(10)})
	case rest == "" && r.Method == http.MethodDelete:
		procs := d.listBackgrounds(sessionID)
		for _, p := range procs {
			_ = p.group()
		}
		if len(procs) > 0 {
			d.wakeSession(sessionID)
		}
		writeJSON(w, http.StatusOK, map[string]any{"stopped": len(procs)})
	case r.Method == http.MethodGet:
		p, ok := d.backgroundByID(sessionID, rest)
		if !ok {
			http.NotFound(w, r)
			return
		}
		lines, _ := backgroundTailQuery(r)
		if lines == 0 {
			lines = backgroundReadLines
		}
		writeJSON(w, http.StatusOK, map[string]any{"process": p.view(lines, true)})
	case r.Method == http.MethodDelete:
		if !d.stopBackground(sessionID, rest) {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"stopped": 1})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// backgroundTailQuery reads ?tail=N and ?lines=N: withOutput is false only when
// the caller asked for no output at all.
func backgroundTailQuery(r *http.Request) (lines int, withOutput bool) {
	q := r.URL.Query()
	tail, lineArg := q.Get("tail"), q.Get("lines")
	if tail == "" && lineArg == "" {
		return 0, false
	}
	lines = backgroundReadLines
	if n, err := strconv.Atoi(lineArg); err == nil && n > 0 {
		lines = n
	}
	if n, err := strconv.Atoi(tail); err == nil && n > 0 {
		lines = n
	}
	return lines, true
}

// watchBackground streams one Session's process list whenever it changes, so a
// browser panel shows a start, an exit and a live tail without polling.
func (d *Daemon) watchBackground(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	heartbeat := time.NewTicker(backgroundWatchHeartbeat)
	defer heartbeat.Stop()
	lines, _ := backgroundTailQuery(r)
	if lines == 0 {
		lines = backgroundReadLines
	}
	for {
		out := make([]backgroundView, 0)
		for _, p := range d.listBackgrounds(sessionID) {
			out = append(out, p.view(lines, true))
		}
		body, err := json.Marshal(map[string]any{"processes": out})
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", body)
		if flusher != nil {
			flusher.Flush()
		}
		d.mu.Lock()
		s, ok := d.sessions[sessionID]
		var ch chan struct{}
		if ok {
			ch = s.notify
		}
		d.mu.Unlock()
		if !ok {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
		case <-ch:
		}
	}
}

// stopBackgroundProcesses stops every process marked as one Session's in its
// worktree: the sweep of a Session that ended, and the one a restarted Daemon
// runs for processes it no longer holds in memory.
func stopBackgroundProcesses(sessionID, workDir string) {
	stopMarkedProcesses([]string{backgroundSessionEnv}, sessionID, workDir)
}
