// Bubblewrap sandbox runner for read-only roles (ticket 14).
//
// RunSandbox starts a command under bwrap with the root filesystem
// bind-mounted read-only, no network, a PID namespace, --die-with-parent,
// a private tmpfs /tmp and a minimal environment. The only writable bind
// is the caller-supplied bridge directory, where ticket 16 puts its socket
// and generated module. A one-time self-test per process proves the
// sandbox blocks writes and network before anything runs; when bwrap is
// missing or the self-test fails, every call returns a
// SandboxUnavailableError naming `el deps` and never runs the
// command unsandboxed. This package does not know about Python: callers
// pass any argv, ticket 16 passes python3.
//
// Test hooks: EL_BIN_DIR overrides the el bin directory,
// EL_BWRAP overrides the bwrap path outright, and
// EL_SANDBOX_FORCE_FAIL=1 forces the self-test to fail.
package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultSandboxTimeout = 120 * time.Second
	maxSandboxTimeout     = 600 * time.Second
	sandboxProbeTimeout   = 30 * time.Second
)

// SandboxUnavailableError reports that a el dependency cannot be used:
// bwrap is missing or the sandbox failed its self-test. The message names
// `el deps`, the command that installs and verifies it. Binary names
// the missing binary, and is empty when the sandbox itself failed, so the
// same type serves the other tools that share ResolveBinary.
type SandboxUnavailableError struct {
	Binary string
	Reason string
}

func (e *SandboxUnavailableError) Error() string {
	what := "sandbox"
	if e.Binary != "" {
		what = e.Binary
	}
	return fmt.Sprintf("%s unavailable (%s); run el deps to install it", what, e.Reason)
}

// IsSandboxUnavailable reports whether err is a *SandboxUnavailableError.
func IsSandboxUnavailable(err error) bool {
	var t *SandboxUnavailableError
	return errors.As(err, &t)
}

// BinDir is where el looks for its own binaries first, before PATH.
// EL_BIN_DIR overrides it for tests. Ticket 19's installer and the
// grep, execute and graph tools share this resolver.
func BinDir() string {
	if d := os.Getenv("EL_BIN_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".emberloop", "bin")
}

// ResolveBinary finds name in the el bin directory first, then on PATH.
// A miss is a *SandboxUnavailableError naming `el deps`.
func ResolveBinary(name string) (string, error) {
	if p := filepath.Join(BinDir(), name); isExecutable(p) {
		return p, nil
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", &SandboxUnavailableError{Binary: name, Reason: fmt.Sprintf("not found in %s or PATH", BinDir())}
}

// resolveBwrap is ResolveBinary("bwrap") with the EL_BWRAP test
// override: when set it wins outright, and a path that is not executable
// is an unavailable sandbox, never a fallthrough to PATH.
func resolveBwrap() (string, error) {
	if p := os.Getenv("EL_BWRAP"); p != "" {
		if isExecutable(p) {
			return p, nil
		}
		return "", &SandboxUnavailableError{Binary: "bwrap", Reason: fmt.Sprintf("EL_BWRAP=%q is not executable", p)}
	}
	return ResolveBinary("bwrap")
}

// SandboxConfig tunes one sandboxed run.
type SandboxConfig struct {
	// Worktree is the reviewed tree, visible read-only. Required.
	Worktree string
	// BridgeDir is the one read-write bind, holding the caller's socket
	// and generated files. Empty means no writable bind. It must not be
	// the worktree, inside it, or above it.
	BridgeDir string
	// Timeout caps the run; zero is the 120s default, capped at 600s.
	Timeout time.Duration
	// Env holds extra KEY=value pairs added to the minimal base
	// (PATH, plus HOME when set).
	Env []string
}

// SandboxResult is the sandboxed command's captured output and exit code.
// A non-zero exit is a result, not an error: err is nil for it.
type SandboxResult struct {
	Stdout string
	Stderr string
	Code   int
}

// sandboxBaseEnv is the minimal environment: PATH, HOME when the host
// has one, plus the caller's extras.
func sandboxBaseEnv(extra []string) ([]string, error) {
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/bin:/usr/bin:/bin"
	}
	env := []string{"PATH=" + path}
	if home := os.Getenv("HOME"); home != "" {
		env = append(env, "HOME="+home)
	}
	for _, kv := range extra {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("bad env entry %q: want KEY=value", kv)
		}
		env = append(env, k+"="+v)
	}
	return env, nil
}

// sandboxArgs builds the bwrap argv. Binds come after the /tmp tmpfs so a
// bridge under /tmp lands on the fresh tmpfs instead of being hidden by
// it. The command is resolved to an absolute path on the host, so the
// sandbox's minimal PATH never changes what runs.
func sandboxArgs(cfg SandboxConfig, env []string, command string, args []string) ([]string, error) {
	wt, err := filepath.Abs(cfg.Worktree)
	if err != nil {
		return nil, err
	}
	// The /tmp tmpfs comes before the binds, so a worktree or bridge under
	// /tmp lands on the fresh tmpfs instead of being hidden by it.
	argv := []string{
		"--ro-bind", "/", "/",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--ro-bind", wt, wt,
	}
	if cfg.BridgeDir != "" {
		bridge, err := filepath.Abs(cfg.BridgeDir)
		if err != nil {
			return nil, err
		}
		sep := string(filepath.Separator)
		if bridge == wt || strings.HasPrefix(bridge, wt+sep) || strings.HasPrefix(wt, bridge+sep) {
			return nil, fmt.Errorf("sandbox bridge %q must not overlap the worktree", cfg.BridgeDir)
		}
		// A missing bridge fails bwrap setup with a plain exit 1, which
		// reads as the command's own failure. Catch it here instead, so
		// a caller typo is an error, not a result.
		if info, err := os.Stat(bridge); err != nil {
			return nil, fmt.Errorf("sandbox bridge %q: %w", cfg.BridgeDir, err)
		} else if !info.IsDir() {
			return nil, fmt.Errorf("sandbox bridge %q is not a directory", cfg.BridgeDir)
		}
		argv = append(argv, "--bind", bridge, bridge)
	}
	argv = append(argv, "--unshare-net", "--unshare-pid", "--die-with-parent", "--clearenv")
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		argv = append(argv, "--setenv", k, v)
	}
	argv = append(argv, "--", command)
	argv = append(argv, args...)
	return argv, nil
}

// startSandboxed runs bwrap with argv. A timeout kills the whole process
// group, including children still holding the pipes.
func startSandboxed(ctx context.Context, bwrap string, argv []string, timeout time.Duration) (SandboxResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bwrap, argv...)
	// A timeout must reach the children too, or Run waits on a pipe they
	// still hold. Own process group, killed as a group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := SandboxResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return res, nil
	}
	if ctx.Err() == context.DeadlineExceeded {
		return res, fmt.Errorf("sandboxed command timed out after %v", timeout)
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		res.Code = exit.ExitCode()
		return res, nil
	}
	return res, err
}

var (
	sandboxHealthMu sync.Mutex
	sandboxHealthy  = map[string]bool{}
)

// EnsureHealthy runs the one-time bubblewrap self-test for worktree: a
// script that tries to write inside and outside the worktree and to open
// a network socket, and must fail at all three. Success is cached per
// process, keyed by bwrap path and worktree; failures retry on the next
// call, so installing bwrap needs no restart. A forced failure
// (EL_SANDBOX_FORCE_FAIL) always fails, even when cached healthy.
// Ticket 19 reuses this as the bubblewrap proof in `el deps`.
func EnsureHealthy(worktree string) error {
	bwrap, err := resolveBwrap()
	if err != nil {
		return err
	}
	if os.Getenv("EL_SANDBOX_FORCE_FAIL") != "" {
		return &SandboxUnavailableError{Reason: "self-test forced to fail (EL_SANDBOX_FORCE_FAIL is set)"}
	}
	wt, err := filepath.Abs(worktree)
	if err != nil {
		return err
	}
	key := bwrap + "\x00" + wt
	sandboxHealthMu.Lock()
	ok := sandboxHealthy[key]
	sandboxHealthMu.Unlock()
	if ok {
		return nil
	}
	if err := runSelfTest(bwrap, wt); err != nil {
		return err
	}
	sandboxHealthMu.Lock()
	sandboxHealthy[key] = true
	sandboxHealthMu.Unlock()
	return nil
}

// runSelfTest executes the probe inside bwrap directly, bypassing
// EnsureHealthy: it is the check, not a caller of it.
func runSelfTest(bwrap, worktree string) error {
	// The outside probe targets the host home directory: unlike /etc it
	// is host-writable, so the probe proves the read-only bind instead
	// of passing vacuously. It falls back to /etc only when HOME is
	// unset. Either way a leaked probe is removed on the success branch,
	// and the name is unique per run.
	outside := os.Getenv("HOME")
	if outside == "" {
		outside = "/etc"
	}
	script := `set -u
in="$1/.emberloop-sandbox-selftest.$$-$RANDOM"
if touch "$in" 2>/dev/null; then rm -f "$in"; exit 10; fi
out="$2/el-sandbox-selftest.$$-$RANDOM"
if touch "$out" 2>/dev/null; then rm -f "$out"; exit 11; fi
if (exec 3<>/dev/tcp/127.0.0.1/9) 2>/dev/null; then exit 12; fi
exit 0`
	bash, err := exec.LookPath("bash")
	if err != nil {
		return &SandboxUnavailableError{Reason: fmt.Sprintf("self-test needs bash: %v", err)}
	}
	env, err := sandboxBaseEnv(nil)
	if err != nil {
		return err
	}
	argv, err := sandboxArgs(SandboxConfig{Worktree: worktree}, env, bash,
		[]string{"-c", script, "selftest", worktree, outside})
	if err != nil {
		return err
	}
	res, err := startSandboxed(context.Background(), bwrap, argv, sandboxProbeTimeout)
	if err != nil {
		return &SandboxUnavailableError{Reason: fmt.Sprintf("self-test could not run: %v", err)}
	}
	if res.Code != 0 {
		return &SandboxUnavailableError{
			Reason: fmt.Sprintf("self-test failed with exit %d (sandbox lets writes or network through)", res.Code),
		}
	}
	return nil
}

// RunSandbox runs name with args under the bubblewrap sandbox. It first
// proves the sandbox healthy and refuses with a
// *SandboxUnavailableError when it cannot: the command never runs
// unsandboxed.
func RunSandbox(ctx context.Context, cfg SandboxConfig, name string, args ...string) (SandboxResult, error) {
	if strings.TrimSpace(cfg.Worktree) == "" {
		return SandboxResult{}, errors.New("SandboxConfig.Worktree is required")
	}
	if err := EnsureHealthy(cfg.Worktree); err != nil {
		return SandboxResult{}, err
	}
	bwrap, err := resolveBwrap()
	if err != nil {
		return SandboxResult{}, err
	}
	abs, err := exec.LookPath(name)
	if err != nil {
		return SandboxResult{}, fmt.Errorf("sandbox: %s not found on PATH: %w", name, err)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultSandboxTimeout
	}
	timeout = min(timeout, maxSandboxTimeout)
	env, err := sandboxBaseEnv(cfg.Env)
	if err != nil {
		return SandboxResult{}, err
	}
	argv, err := sandboxArgs(cfg, env, abs, args)
	if err != nil {
		return SandboxResult{}, err
	}
	return startSandboxed(ctx, bwrap, argv, timeout)
}
