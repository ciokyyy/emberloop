package main

import (
	"os/exec"
	"syscall"
)

// setProcessGroup makes the agent a group leader so --kill-group can signal
// the whole tree (qwen-code leaves orphaned grandchildren otherwise).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcess kills one process, or the whole group with --kill-group.
func killProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if killGroup {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return
	}
	_ = cmd.Process.Kill()
}
