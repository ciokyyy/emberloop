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
		if d.previewReserved[port] != "" {
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
	defer d.previewMu.Unlock()
	port, ok := d.previewPorts[sessionID]
	return port, ok && port != 0
}

func (d *Daemon) stopPreview(sessionID string, workDir string, design bool) {
	d.previewMu.Lock()
	port := d.previewPorts[sessionID]
	delete(d.previewPorts, sessionID)
	delete(d.previewReserved, port)
	d.previewMu.Unlock()
	if design {
		stopPreviewProcesses(workDir)
	}
}

// stopPreviewProcesses uses a cgroup-free cwd scan to stop detached
// processes rooted in the Session worktree without touching other work.
func stopPreviewProcesses(workDir string) {
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
