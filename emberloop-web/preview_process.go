package web

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// previewPort reserves distinct ephemeral ports for design Sessions until
// they end. The short listener probe ensures the port was available at pick.
func (d *Daemon) previewPort(sessionID string) (int, error) {
	d.previewMu.Lock()
	defer d.previewMu.Unlock()
	if port := d.previewPorts[sessionID]; port != 0 {
		return port, nil
	}
	if d.previewPorts == nil {
		d.previewPorts = make(map[string]int)
		d.previewReserved = make(map[int]string)
	}
	for attempts := 0; attempts < 32; attempts++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		port := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		if d.previewReserved[port] != "" || d.persistedPreviewPortInUse(sessionID, port) {
			continue
		}
		d.previewReserved[port] = sessionID
		d.previewPorts[sessionID] = port
		return port, nil
	}
	return 0, fmt.Errorf("could not reserve a preview port")
}
func (d *Daemon) previewPortFor(sessionID string) (int, bool) {
	d.previewMu.Lock()
	port := d.previewPorts[sessionID]
	d.previewMu.Unlock()
	if port != 0 {
		return port, true
	}
	d.mu.Lock()
	s := d.sessions[sessionID]
	d.mu.Unlock()
	if s != nil && s.meta.DesignSession && s.meta.PreviewPort > 0 {
		return s.meta.PreviewPort, true
	}
	return 0, false
}
func (d *Daemon) persistedPreviewPortInUse(sessionID string, port int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, s := range d.sessions {
		if id != sessionID && s.meta.DesignSession && s.meta.PreviewPort == port && s.meta.Status != "ended" {
			return true
		}
	}
	return false
}

func (d *Daemon) releasePreviewPort(sessionID string) {
	d.previewMu.Lock()
	defer d.previewMu.Unlock()
	port := d.previewPorts[sessionID]
	delete(d.previewPorts, sessionID)
	delete(d.previewReserved, port)
}

func (d *Daemon) stopPreview(sessionID string, workDir string, design bool) {
	d.previewMu.Lock()
	port := d.previewPorts[sessionID]
	delete(d.previewPorts, sessionID)
	delete(d.previewReserved, port)
	d.previewMu.Unlock()
	if design {
		stopPreviewProcesses(sessionID, workDir)
	}
}

// previewSessionEnv marks the processes a design Session's tools start: the
// dev server `.emberloop/preview` launches and everything it spawns inherits
// the marker, so one /proc scan finds the whole tree.
const previewSessionEnv = "EL_PREVIEW_SESSION"

// stopPreviewProcesses stops every process marked as one design Session's in
// its worktree.
func stopPreviewProcesses(sessionID, workDir string) {
	stopMarkedProcesses([]string{previewSessionEnv}, sessionID, workDir)
}

// stopMarkedProcesses uses a cgroup-free /proc scan constrained by Session
// identity, a marker environment and the worktree cwd, so shared-worktree
// Sessions stay isolated. keys name the markers a process may carry: a preview
// server and a background command are marked differently and both die with
// their Session.
func stopMarkedProcesses(keys []string, sessionID, workDir string) {
	root, err := filepath.EvalSymlinks(workDir)
	if err != nil || root == "" {
		return
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	type process struct {
		pid     int
		started string
	}
	var pids []process
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() || !markedProcessMatches(pid, keys, sessionID, root) {
			continue
		}
		started, _, ok := processStartTime(pid)
		if ok {
			pids = append(pids, process{pid, started})
		}
	}
	for _, p := range pids {
		_ = syscall.Kill(p.pid, syscall.SIGTERM)
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		alive := false
		for _, p := range pids {
			started, state, ok := processStartTime(p.pid)
			if ok && started == p.started && state != "Z" {
				alive = true
				break
			}
		}
		if !alive {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, p := range pids {
		started, state, ok := processStartTime(p.pid)
		if ok && started == p.started && state != "Z" && markedProcessMatches(p.pid, keys, sessionID, root) {
			_ = syscall.Kill(p.pid, syscall.SIGKILL)
		}
	}
}

// previewProcessMatches reports whether pid carries the preview marker of
// sessionID and runs under root.
func previewProcessMatches(pid int, sessionID, root string) bool {
	return markedProcessMatches(pid, []string{previewSessionEnv}, sessionID, root)
}

// markedProcessMatches reports whether pid carries any of the marker keys set
// to sessionID and runs under root.
func markedProcessMatches(pid int, keys []string, sessionID, root string) bool {
	base := filepath.Join("/proc", strconv.Itoa(pid))
	env, err := os.ReadFile(filepath.Join(base, "environ"))
	if err != nil {
		return false
	}
	marked := false
	for _, key := range keys {
		if hasEnvValue(env, key, sessionID) {
			marked = true
			break
		}
	}
	if !marked {
		return false
	}
	cwd, err := os.Readlink(filepath.Join(base, "cwd"))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, cwd)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// processStartTime reads Linux proc stat's starttime field, which distinguishes
// a surviving PID from one recycled while waiting for SIGTERM to take effect.
func processStartTime(pid int) (started, state string, ok bool) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", "", false
	}
	end := strings.LastIndexByte(string(raw), ')')
	if end < 0 {
		return "", "", false
	}
	fields := strings.Fields(string(raw[end+1:]))
	if len(fields) <= 19 {
		return "", "", false
	}
	return fields[19], fields[0], true
}

func hasEnvValue(env []byte, key, value string) bool {
	want := key + "=" + value
	for _, item := range strings.Split(string(env), "\x00") {
		if item == want {
			return true
		}
	}
	return false
}
