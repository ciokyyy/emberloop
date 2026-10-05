package wick

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// `wick-pig install` is the only path that replaces the installed wick
// binaries (wick-pig and wick-mcp). Nothing else in this package writes to
// the install directory, so a running session never changes behaviour under
// itself except through this gate: while any agent of a run is live, the
// install is refused, naming the agents; --force installs anyway and warns
// that running sessions keep the old binary until they restart.

// InstallOptions tunes InstallBinaries. Force installs even while agents
// are live, for the rare case; the result then carries the stale-session
// warning to print.
type InstallOptions struct {
	Force bool
}

// InstallResult is where InstallBinaries put the new binaries. Warning is
// the stale-session notice to print, set only when --force ran past live
// agents.
type InstallResult struct {
	Dir     string
	Warning string
}

// installExec runs the install's herdr and build commands, so tests drive it
// with a fake multiplexer and builder.
var installExec Exec = OSExec

// installDir is where the wick binaries live. The install updates the
// directory the running binary came from, so it follows a relocated setup;
// WICK_INSTALL_DIR points it at a temp dir in tests.
func installDir() string {
	if dir := os.Getenv("WICK_INSTALL_DIR"); dir != "" {
		return dir
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		return filepath.Dir(exe)
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "bin")
}

// installSourceDir is the checkout the install builds from: the current
// directory — run `wick-pig install` from the wick source — or
// WICK_SOURCE_DIR when set, which is what tests use.
func installSourceDir() string {
	if dir := os.Getenv("WICK_SOURCE_DIR"); dir != "" {
		return dir
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

// liveInstallAgents names every live agent of a run: worker, reviewer and
// scout panes known to the stores whose pane the multiplexer still lists.
// Dead panes — records whose agent is gone — are not live and do not block.
func liveInstallAgents(run Exec) ([]string, error) {
	live, err := livePanes(run)
	if err != nil {
		return nil, err
	}
	var names []string
	seen := map[string]bool{}
	attach := func(pane, name string) {
		if pane == "" || !live[pane] || seen[pane] {
			return
		}
		seen[pane] = true
		names = append(names, name)
	}
	if store, err := load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		ids := make([]string, 0, len(store))
		for id := range store {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			w := store[id]
			attach(w.Pane, fmt.Sprintf("worker %s (pane %s)", w.ID, w.Pane))
		}
	}
	if all, err := loadReviewers(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		var trees []string
		for tree := range all {
			trees = append(trees, tree)
		}
		sort.Strings(trees)
		for _, tree := range trees {
			var types []string
			for typ := range all[tree] {
				types = append(types, typ)
			}
			sort.Strings(types)
			for _, typ := range types {
				r := all[tree][typ]
				attach(r.Pane, fmt.Sprintf("%s for %s (pane %s)", typ, tree, r.Pane))
			}
		}
	}
	// Agent states hold the scouts, which have no other store. Workers and
	// reviewers are already named above, so panes seen there are skipped.
	if entries, err := os.ReadDir(agentStateDir()); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		var ids []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				ids = append(ids, strings.TrimSuffix(e.Name(), ".json"))
			}
		}
		sort.Strings(ids)
		for _, id := range ids {
			s, ok, err := ReadAgentState(id)
			if err != nil || !ok {
				continue
			}
			kind := s.Type
			if kind == "" {
				kind = "agent"
			}
			attach(s.Pane, fmt.Sprintf("%s %s (pane %s)", kind, id, s.Pane))
		}
	}
	sort.Strings(names)
	return names, nil
}

// installTargets are the binaries one install replaces, each built from its
// package in the source checkout. Both go, or neither does: every target is
// built to a temp file first and renamed into place only when all built, so
// a failed build replaces nothing.
var installTargets = []struct{ name, pkg string }{
	{"wick-pig", "./cmd/wick-pig"},
	{"wick-mcp", "./cmd/wick-mcp"},
}

// InstallBinaries rebuilds wick-pig and wick-mcp from the source checkout
// and installs them into installDir. While any agent of a run is live it
// refuses instead, naming the agents, and changes nothing: the live check
// runs before any build, so a refused install never even builds.
func InstallBinaries(run Exec, opts InstallOptions) (InstallResult, error) {
	live, checkErr := liveInstallAgents(run)
	if checkErr != nil && !opts.Force {
		return InstallResult{}, fmt.Errorf("cannot check which agents are live: %v; refusing the install (or rerun with --force)", checkErr)
	}
	if len(live) > 0 && !opts.Force {
		return InstallResult{}, fmt.Errorf("refusing to install: %d live %s: %s; stop them or rerun with --force",
			len(live), plural(len(live), "agent", "agents"), strings.Join(live, ", "))
	}
	dir, src := installDir(), installSourceDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, err
	}
	type staged struct{ tmp, dst string }
	var done []staged
	discard := func() {
		for _, s := range done {
			_ = os.Remove(s.tmp)
		}
	}
	for _, target := range installTargets {
		f, err := os.CreateTemp(dir, target.name+".*")
		if err != nil {
			discard()
			return InstallResult{}, err
		}
		tmp := f.Name()
		_ = f.Close()
		_ = os.Remove(tmp)
		res, err := run("go", []string{"build", "-o", tmp, target.pkg},
			ExecOpts{Cwd: src, Timeout: 5 * time.Minute})
		if err != nil {
			_ = os.Remove(tmp)
			discard()
			return InstallResult{}, err
		}
		if res.Code != 0 {
			_ = os.Remove(tmp)
			discard()
			return InstallResult{}, fmt.Errorf("go build %s failed: %s", target.pkg, firstLine(res.Stderr, res.Stdout))
		}
		done = append(done, staged{tmp: tmp, dst: filepath.Join(dir, target.name)})
	}
	for _, s := range done {
		if err := os.Chmod(s.tmp, 0o755); err != nil {
			discard()
			return InstallResult{}, err
		}
		if err := os.Rename(s.tmp, s.dst); err != nil {
			discard()
			return InstallResult{}, err
		}
	}
	res := InstallResult{Dir: dir}
	if len(live) > 0 || checkErr != nil {
		res.Warning = "warning: running sessions keep the old binary until they restart"
	}
	return res, nil
}

// installUsage documents the one command that replaces the installed wick
// binaries, and the gate around it.
const installUsage = `usage: wick-pig install [--force]

  Rebuild wick-pig and wick-mcp from this source checkout and install them.
  Refuses while any agent of a run is live (worker, reviewer or scout),
  naming the agents, and changes nothing. --force installs anyway: running
  sessions keep the old binary until they restart.

  This is the only way to replace the installed wick binaries.`

// RunInstall serves `wick-pig install`. It rebuilds both binaries through
// the gate: live agents refuse with their names, --force installs anyway
// and prints the stale-session warning.
func RunInstall(argv []string, out io.Writer) error {
	var opts InstallOptions
	for _, a := range argv {
		switch a {
		case "--force", "-force", "-f":
			opts.Force = true
		case "--help", "-h", "help":
			fmt.Fprintln(out, installUsage)
			return nil
		default:
			return fmt.Errorf("unknown flag %q\n%s", a, installUsage)
		}
	}
	res, err := InstallBinaries(installExec, opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "installed wick-pig and wick-mcp into %s\n", res.Dir)
	if res.Warning != "" {
		fmt.Fprintln(out, res.Warning)
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
