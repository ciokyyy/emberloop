// Read-only git inspection over the workspace (ticket 12).
//
// The tool takes `args` (a list of strings) and runs
// `git -C <root> --no-pager ...`, so agents can check history and the
// exact change without being able to alter the repo. Only the listing
// and inspection subcommands are allowed; everything else is refused
// with the allowed list. Flags that would write, configure or execute
// (`-c`, `--output`, `--ext-diff`, `--textconv`, `--exec`,
// `--upload-pack`, plus grep's `-O`/`--open-files-in-pager`, cat-file's
// `--filters` and `--show-signature`) are refused in `--flag value`,
// `--flag=value` and unambiguous-abbreviation (`--out`, `--del`) form:
// git accepts long-option prefixes, and the abbreviation a git version
// rejects today may be accepted by the next, so matching is by prefix
// and fails closed. `--no-ext-diff` rides along on the subcommands
// that accept it (diff, log, show, blame) and `--no-textconv` on
// those plus grep; the remaining allowed subcommands reject those
// options outright (verified: appending them breaks status, ls-files,
// branch, merge-base and cat-file), and the cleared config environment
// plus the refused enabling flags keep a hostile repository's drivers
// from executing. Output goes through the bash-style sanitising and
// timeout.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// gitTool is read-only git inspection: diff, log, show, blame, status,
// ls-files, grep, rev-parse, merge-base, cat-file, plus branch and tag
// in list mode only.
type gitTool struct{}

func (gitTool) Name() string   { return "git" }
func (gitTool) ReadOnly() bool { return true }
func (gitTool) Description() string {
	return "Read-only git inspection: diff, log, show, blame, status, ls-files, grep, rev-parse, merge-base, cat-file, branch and tag listing. Takes args (a list of strings) run as git -C <worktree> --no-pager. Anything else is refused; executing, writing and configuring flags (-c, -O, --output, --ext-diff, --textconv, --exec, --upload-pack, --open-files-in-pager, --filters, --show-signature and their abbreviations) are never allowed."
}
func (gitTool) Schema() map[string]any {
	return obj(map[string]any{
		"args":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Git arguments, starting with the subcommand, e.g. [\"log\", \"--oneline\", \"-5\"]. Branch and tag need a list flag (--list, -a/--all, -r/--remotes for branch; --list/-l for tag)."},
		"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default 120, maximum 600)."},
	}, "args")
}

// gitAllowed names every subcommand the tool runs, in the refusal message's
// order. Branch and tag are list-only (see allowsBranchTag).
var gitAllowed = []string{"blame", "branch", "cat-file", "diff", "grep", "log", "ls-files", "merge-base", "rev-parse", "show", "status", "tag"}

// gitAlwaysAllowed is the subcommands that need no further check beyond the
// refused flags: every invocation is an inspection.
var gitAlwaysAllowed = map[string]bool{
	"diff": true, "log": true, "show": true, "blame": true,
	"status": true, "ls-files": true, "grep": true,
	"rev-parse": true, "merge-base": true, "cat-file": true,
}

// gitNoExtDiff are the subcommands that accept --no-ext-diff, and
// gitNoTextconv the ones that accept --no-textconv. The other allowed
// subcommands reject those options outright (verified: appending them
// breaks status, ls-files, branch, merge-base and cat-file), and they
// produce no external-diff output; textconv there is off by default
// and its enabling flag is refused, so a hostile repo's drivers stay
// off everywhere.
var gitNoExtDiff = map[string]bool{
	"diff": true, "log": true, "show": true, "blame": true,
}

var gitNoTextconv = map[string]bool{
	"diff": true, "log": true, "show": true, "blame": true, "grep": true,
}

// gitRefusedLongs are long flags that would write, configure or execute,
// without the leading dashes. Matching is by prefix because git accepts
// unambiguous long-option abbreviations: `--out`, `--ext-d`, `--text`
// and `--upl` reach the same code as the full flag (verified against
// branch --del deleting a branch and grep --textc enabling textconv),
// and an abbreviation this git rejects may be accepted by the next.
var gitRefusedLongs = []string{
	"output", "ext-diff", "textconv", "exec", "upload-pack",
	// grep's pager runs an agent-chosen command; cat-file's filters run
	// the repo's drivers; --show-signature runs the repo's gpg.program.
	"open-files-in-pager", "filters", "show-signature",
}

// gitBranchMutatingShorts are branch short flags that change refs or
// config. Matching decomposes single-dash bundles, because git parses
// `-ad` as `-a -d` and delete wins over listing.
const gitBranchMutatingShorts = "cdDmMCfut"

// gitBranchMutatingLongs are the same for long flags, matched by prefix
// for the same abbreviation reason as gitRefusedLongs.
var gitBranchMutatingLongs = []string{
	"delete", "move", "copy", "edit-description", "create-reflog",
	"set-upstream-to", "unset-upstream", "track", "no-track", "force",
}

// gitTagMutatingShorts and gitTagMutatingLongs refuse tag modes that
// create, delete, annotate or verify: only listing stays.
const gitTagMutatingShorts = "dasmFuv"

var gitTagMutatingLongs = []string{
	"delete", "annotate", "sign", "local-user", "message", "file", "verify",
}

func (gitTool) Execute(ctx context.Context, root string, raw json.RawMessage) (string, error) {
	var p struct {
		Args    []string `json:"args"`
		Timeout int      `json:"timeout"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("invalid git arguments: %w", err)
	}
	if len(p.Args) == 0 {
		return "", fmt.Errorf("git args cannot be empty; allowed subcommands: %s", strings.Join(gitAllowed, ", "))
	}
	if refused := refusedGitFlag(p.Args); refused != "" {
		return "", fmt.Errorf("git flag %q is not allowed (refused flags: -c, -O, --output, --ext-diff, --textconv, --exec, --upload-pack, --open-files-in-pager, --filters, --show-signature, and their abbreviations)", refused)
	}
	sub := p.Args[0]
	rest := p.Args[1:]
	switch {
	case gitAlwaysAllowed[sub]:
		// Inspected below; every flag combination stays read-only once the
		// refused flags are out.
	case sub == "branch" || sub == "tag":
		if err := checkBranchTagList(sub, rest); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("git subcommand %q is not allowed; allowed subcommands: %s (branch and tag list only)", sub, strings.Join(gitAllowed, ", "))
	}
	timeout := defaultBashTimeout
	if p.Timeout > 0 {
		timeout = min(time.Duration(p.Timeout)*time.Second, maxBashTimeout)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin, err := ResolveBinary("git")
	if err != nil {
		return "", err
	}
	argv := []string{"-C", root, "--no-pager", sub}
	if gitNoExtDiff[sub] {
		argv = append(argv, "--no-ext-diff")
	}
	if gitNoTextconv[sub] {
		argv = append(argv, "--no-textconv")
	}
	argv = append(argv, rest...)
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Dir = root
	cmd.Env = gitEnv(os.Environ())
	// A timeout must reach the pager or driver children too, or
	// CombinedOutput waits on a pipe they still hold. Own process
	// group, killed as a group, like bash.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second

	out, err := cmd.CombinedOutput()
	text := sanitize(string(out))
	if err == nil {
		return text, nil
	}
	if ctx.Err() == context.DeadlineExceeded {
		return text + fmt.Sprintf("\n[timed out after %v]", timeout), nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if exit.ExitCode() == 1 && strings.TrimSpace(text) == "" {
			return "(no output, exit status 1)", nil
		}
		return text + fmt.Sprintf("\n[exit status %d]", exit.ExitCode()), nil
	}
	return "", err
}

// gitSafeExactLongs are legitimate long flags that prefix-match a refused
// one but resolve to themselves: git prefers the exact match, so `--text`
// (treat binary as text, on diff, log, show and grep) never reaches
// textconv. They skip the prefix refusal below.
var gitSafeExactLongs = map[string]bool{"text": true}

// refusedGitFlag reports the first refused flag in args, or "". `-c` is
// refused as its own argument (the `git -c key=value ...` form), and
// grep's `-O` pager in any single-dash bundle containing `O` (`-O`,
// `-Oless`, `-iOless`), since git parses bundles letter by letter. A
// double-dash flag is refused when its name (before any `=`) is a
// refused flag or a prefix of one (`--out`, `--ext-d`, `--upl`), because
// git parses unambiguous abbreviations as the full flag. A `--format`
// or `--pretty` value containing `%G` is refused: formatting a signature
// runs the repo's gpg.program even without `--show-signature`. Scanning
// stops at `--`: past it every argument is a path or pattern to git too.
func refusedGitFlag(args []string) string {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "-c" {
			return a
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			if strings.ContainsRune(a[1:], 'O') {
				return a
			}
			continue
		}
		if !strings.HasPrefix(a, "--") {
			continue
		}
		name, value := strings.TrimPrefix(a, "--"), ""
		if i := strings.IndexByte(name, '='); i >= 0 {
			name, value = name[:i], name[i+1:]
		}
		if name == "" {
			continue
		}
		if gitSafeExactLongs[name] {
			continue
		}
		for _, f := range gitRefusedLongs {
			if strings.HasPrefix(f, name) {
				return a
			}
		}
		if strings.Contains(value, "%G") && (strings.HasPrefix("format", name) || strings.HasPrefix("pretty", name)) {
			return a
		}
	}
	return ""
}

// checkBranchTagList allows branch and tag in list mode only: no args
// (bare `git branch` lists), or an explicit list flag with no mutating
// flag. Anything that creates, deletes, moves or annotates is refused,
// as is any non-flag operand without a list flag (a new branch or tag
// name).
func checkBranchTagList(sub string, rest []string) error {
	refusal := fmt.Errorf("git %s is allowed for listing only (branch: --list, -a/--all, -r/--remotes; tag: --list/-l); allowed subcommands: %s (branch and tag list only)", sub, strings.Join(gitAllowed, ", "))
	shorts, longs := gitBranchMutatingShorts, gitBranchMutatingLongs
	hasList := hasBranchListFlag(rest)
	if sub == "tag" {
		shorts, longs = gitTagMutatingShorts, gitTagMutatingLongs
		hasList = hasTagListFlag(rest)
	}
	for _, a := range rest {
		if a == "--" {
			continue
		}
		// Bundles decompose: git reads `-ad` as `-a -d`.
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			for _, ch := range a[1:] {
				if strings.ContainsRune(shorts, ch) {
					return refusal
				}
			}
			continue
		}
		// Long abbreviations parse as the full flag (`--del`
		// deleted a branch in testing), so matching is by prefix.
		if strings.HasPrefix(a, "--") {
			name := strings.TrimPrefix(a, "--")
			if i := strings.IndexByte(name, '='); i >= 0 {
				name = name[:i]
			}
			if name != "" {
				for _, m := range longs {
					if strings.HasPrefix(m, name) {
						return refusal
					}
				}
			}
			continue
		}
	}
	if len(rest) == 0 {
		return nil
	}
	if hasList {
		return nil
	}
	return refusal
}

// hasBranchListFlag reports whether rest asks branch to list: --list, -a,
// --all, -r, --remotes or --show-current. A short bundle containing a or
// r (e.g. -av) counts, so `-av` lists instead of refusing.
func hasBranchListFlag(rest []string) bool {
	for _, a := range rest {
		switch a {
		case "--list", "--all", "--remotes", "--show-current":
			return true
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			if strings.ContainsAny(a[1:], "ar") {
				return true
			}
		}
	}
	return false
}

// hasTagListFlag reports whether rest asks tag to list: --list or -l,
// with short bundles containing l (e.g. -ln) counting.
func hasTagListFlag(rest []string) bool {
	for _, a := range rest {
		if a == "--list" {
			return true
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			if strings.ContainsRune(a[1:], 'l') {
				return true
			}
		}
	}
	return false
}

// gitEnv clears the git config environment around the call: the global
// and system configs point at the null device, the external diff driver
// is unset, the pager is off, and inherited env-carried config
// (GIT_CONFIG_COUNT and friends) is stripped. Repo-local config still
// applies, but the refused enabling flags plus --no-ext-diff/--no-textconv
// where they are accepted keep a hostile repo's diff and textconv drivers
// from executing, and env config outranks files, so core.fsmonitor (which
// would otherwise run a repo command on status and friends) is forced
// off for the call.
func gitEnv(env []string) []string {
	out := make([]string, 0, len(env)+7)
	for _, kv := range env {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		switch {
		case key == "GIT_EXTERNAL_DIFF",
			key == "GIT_CONFIG_GLOBAL",
			key == "GIT_CONFIG_SYSTEM",
			key == "GIT_CONFIG_COUNT",
			key == "GIT_CONFIG_PARAMETERS",
			key == "GIT_PAGER",
			key == "PAGER":
			continue
		case strings.HasPrefix(key, "GIT_CONFIG_KEY_") || strings.HasPrefix(key, "GIT_CONFIG_VALUE_"):
			continue
		}
		out = append(out, kv)
	}
	out = append(out,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_PAGER=cat",
		"PAGER=cat",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.fsmonitor",
		"GIT_CONFIG_VALUE_0=false",
	)
	return out
}
