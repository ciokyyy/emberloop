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

// stopPreviewProcesses uses a cgroup-free /proc scan constrained by Session
// identity and worktree cwd, so shared-worktree Sessions stay isolated.
func stopPreviewProcesses(sessionID, workDir string) {
	root, err := filepath.EvalSymlinks(workDir)
	if err != nil || root == "" {
		return
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	var pids []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		env, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "environ"))
		if err != nil || !hasEnvValue(env, "EL_PREVIEW_SESSION", sessionID) {
			continue
		}
		cwd, err := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, cwd)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			pids = append(pids, pid)
		}
	}
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	if len(pids) == 0 {
		return
	}
	time.Sleep(250 * time.Millisecond)
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
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
