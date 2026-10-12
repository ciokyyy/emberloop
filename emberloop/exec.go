package el

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Exec is the seam every operation in this package runs through.
//
// The SDK exposes process execution only as a host call, which is unavailable
// outside a running pig session. Routing every command through this one
// function type is what lets the tests drive the real herdr and git binaries
// from a plain `go test`.
type Exec func(cmd string, args []string, opts ExecOpts) (Result, error)

type ExecOpts struct {
	Cwd     string
	Timeout time.Duration
	Env     []string // added to the parent environment
	Stdin   string   // written to the process, which then reads EOF
}
type Result struct {
	Stdout string
	Stderr string
	Code   int
}

// osExec is the real implementation: plain os/exec, exit code in Result.
// A timeout kills the whole process group (not just the direct child), or a
// `sh -c` wrapper's children hold the output pipes and Run waits out their
// full run. Own process group, killed as a group; WaitDelay bounds the pipe
// wait past the kill. Test seam: none (timeout comes from ExecOpts).
func osExec(cmd string, args []string, opts ExecOpts) (Result, error) {
	ctx := context.Background()
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = opts.Cwd
	if len(opts.Env) > 0 {
		c.Env = append(os.Environ(), opts.Env...)
	}
	if opts.Stdin != "" {
		c.Stdin = strings.NewReader(opts.Stdin)
	}
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error { return syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	c.WaitDelay = 2 * time.Second

	var stdout, stderr strings.Builder
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()

	// Read the builders only after Run has filled them.
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return res, nil
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return res, err
	}
	res.Code = exitErr.ExitCode()
	return res, nil
}

// OSExec is the real Exec, for callers outside the package.
var OSExec Exec = osExec
