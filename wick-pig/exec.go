package wick

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
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
}

type Result struct {
	Stdout string
	Stderr string
	Code   int
}

// osExec is the real implementation: plain os/exec, exit code in Result.
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
