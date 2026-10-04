package wick

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
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

// sdkExec adapts the SDK's host-backed executor onto the same seam.
func sdkExec(ctx sdk.Context) Exec {
	return func(cmd string, args []string, opts ExecOpts) (Result, error) {
		res, err := ctx.ExecWithOptions(cmd, args, sdk.ExecOptions{
			Cwd:     opts.Cwd,
			Timeout: float64(opts.Timeout) / float64(time.Millisecond),
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Stdout: res.Stdout, Stderr: res.Stderr, Code: res.ExitCode}, nil
	}
}
