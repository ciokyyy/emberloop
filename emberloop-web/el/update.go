package el

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// `el update` (alias `el install`) is the only path that replaces the
// installed el binaries. It rebuilds the web UI unless its source is
// unchanged, rebuilds el, el-mcp and el-web with version stamps,
// swaps them atomically, keeps the previous set, and rolls back to it when
// the new el-web fails its health check.

// UpdateOptions tunes UpdateBinaries. It has no fields today.
type UpdateOptions struct{}

// UpdateResult is where UpdateBinaries put the new binaries. UIRebuilt
// tells whether the web bundle was rebuilt or skipped as unchanged; the
// stamps name the new builds for the report line. Warning is a
// notice to print.
type UpdateResult struct {
	Dir       string
	Warning   string
	UIRebuilt bool
	UIHash    string
	Version   string
	Commit    string
}

// updateTargets are the binaries one update replaces, each built from its
// package in the one module under the source checkout: dir is the module's
// directory relative to the main module ("" for the main module itself),
// pkg the package within it. All three build in the main module now.
// web marks the Daemon: it alone carries the UI hash stamp, naming the UI
// build embedded in it.
var updateTargets = []struct {
	name, pkg, dir string
	web            bool
}{
	{"el", "./cmd/el", "", false},
	{"el-mcp", "./cmd/el-mcp", "", false},
	{"el-web", "./cmd/emberloop-web", "", true},
}

// updateNames lists every binary the update owns, previous copies included.
func updateNames() []string {
	names := make([]string, 0, len(updateTargets))
	for _, target := range updateTargets {
		names = append(names, target.name)
	}
	return names
}

// buildStamps names the new builds: the version is the release tag on HEAD
// ("dev" on untagged checkouts), the commit HEAD's short sha, and the UI
// hash the web source hash the Daemon stamps and the UI bundle carries.
func buildStamps(run Exec, src string) (version, commit, uiHash string) {
	version = "dev"
	if res, err := run("git", []string{"describe", "--tags", "--exact-match"}, ExecOpts{Cwd: src, Timeout: 30 * time.Second}); err == nil && res.Code == 0 {
		if v := strings.TrimSpace(firstLine(res.Stdout, "")); v != "" {
			version = v
		}
	}
	commit = "unknown"
	if res, err := run("git", []string{"rev-parse", "--short", "HEAD"}, ExecOpts{Cwd: src, Timeout: 30 * time.Second}); err == nil && res.Code == 0 {
		if c := strings.TrimSpace(firstLine(res.Stdout, "")); c != "" {
			commit = c
		}
	}
	if h, err := hashWebSource(webSourceDir()); err == nil {
		uiHash = h
	}
	return version, commit, uiHash
}

// ldflagsFor stamps one target's build: every binary carries the el
// Version and Commit, and the Daemon carries the UI hash too, so
// /api/version names the UI build it embeds.
func ldflagsFor(target struct {
	name, pkg, dir string
	web            bool
}, version, commit, uiHash string) string {
	flags := "-X example.com/emberloop-web/el.Version=" + version +
		" -X example.com/emberloop-web/el.Commit=" + commit
	if target.web && uiHash != "" {
		flags += " -X example.com/emberloop-web.UIHash=" + uiHash
	}
	return flags
}

// UpdateBinaries rebuilds the web UI unless its source is unchanged, then
// rebuilds el, el-mcp and el-web with version stamps and installs
// them into installDir. After the swap the new el-web must answer /api/health on a
// throwaway port; when it never does, the previous set is restored and the
// update fails, so a broken Daemon build never strands prod.
func UpdateBinaries(run Exec, opts UpdateOptions) (UpdateResult, error) {
	dir, src := installDir(), installSourceDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return UpdateResult{}, err
	}
	version, commit, uiHash := buildStamps(run, src)
	res := UpdateResult{Dir: dir, UIHash: uiHash, Version: version, Commit: commit}
	if _, err := os.Stat(webSourceDir()); err == nil {
		rebuilt, err := updateWebUI(run, webSourceDir(), uiHashPath(dir), uiHash)
		if err != nil {
			return UpdateResult{}, err
		}
		res.UIRebuilt = rebuilt
	}
	targets := make([]buildTarget, 0, len(updateTargets))
	for _, target := range updateTargets {
		targets = append(targets, buildTarget{name: target.name, pkg: target.pkg, dir: target.dir})
	}
	flags := func(target buildTarget) string {
		for _, t := range updateTargets {
			if t.name == target.name {
				return ldflagsFor(t, version, commit, uiHash)
			}
		}
		return ""
	}
	done, err := buildTargets(run, src, dir, targets, flags)
	if err != nil {
		return UpdateResult{}, err
	}
	discardStaged := func() {
		for _, s := range done {
			_ = os.Remove(s.tmp)
		}
	}
	prev, err := stashPrevious(dir)
	if err != nil {
		discardStaged()
		return UpdateResult{}, err
	}
	if err := swapInstalled(done); err != nil {
		return UpdateResult{}, err
	}
	if err := probeWebHealth(filepath.Join(dir, "el-web")); err != nil {
		if rerr := restorePrevious(dir, prev); rerr != nil {
			return UpdateResult{}, fmt.Errorf("new el-web failed its health check (%v) and restoring the previous binaries failed too: %v", err, rerr)
		}
		return UpdateResult{}, fmt.Errorf("new el-web failed its health check, restored the previous binaries: %v", err)
	}
	warn := func(w string) {
		if w == "" {
			return
		}
		if res.Warning != "" {
			res.Warning += "\n"
		}
		res.Warning += w
	}
	if _, err := SyncAgentDocs(); err != nil {
		warn("warning: could not refresh el://docs: " + err.Error())
	}
	return res, nil
}

// webSourceDir is the web UI source the update builds: web/ inside the
// module the update builds from.
func webSourceDir() string {
	return filepath.Join(installSourceDir(), "web")
}

// uiHashPath is where the update records the web source hash of the last
// UI build, beside the binaries it belongs to.
func uiHashPath(dir string) string {
	return filepath.Join(dir, ".el-ui-build-hash")
}

// updateWebUI rebuilds the web bundle unless the source hashes to the
// recorded hash of the last build and its dist still stands. It reports
// whether vite ran; a failed vite build fails the update before any Go
// build, so a broken UI never ships under a fresh Daemon stamp.
func updateWebUI(run Exec, webDir, hashPath, uiHash string) (bool, error) {
	if uiHash != "" {
		if raw, err := os.ReadFile(hashPath); err == nil && strings.TrimSpace(string(raw)) == uiHash {
			if _, err := os.Stat(filepath.Join(webDir, "dist", "index.html")); err == nil {
				return false, nil
			}
		}
	}
	res, err := run("npx", []string{"vite", "build"}, ExecOpts{Cwd: webDir, Timeout: 10 * time.Minute})
	if err != nil {
		return false, err
	}
	if res.Code != 0 {
		return false, fmt.Errorf("vite build failed: %s", firstLine(res.Stderr, res.Stdout))
	}
	if uiHash != "" {
		if err := os.WriteFile(hashPath, []byte(uiHash+"\n"), 0o644); err != nil {
			return false, err
		}
	}
	return true, nil
}

// hashWebSource hashes the web UI source into a short id: every file under
// src/ and public/, plus package.json, vite.config.js and index.html. The
// file set is fixed and shared with web/vite.config.js, which stamps the
// same digest into the bundle at build time: equal hashes mean the bundle
// and the Daemon's stamp were built from the same source.
func hashWebSource(webDir string) (string, error) {
	var rels []string
	add := func(rel string) {
		rels = append(rels, rel)
	}
	for _, top := range []string{"src", "public"} {
		root := filepath.Join(webDir, top)
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if info.Name() == "node_modules" || info.Name() == "dist" {
					return filepath.SkipDir
				}
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(webDir, path)
			if err != nil {
				return err
			}
			add(filepath.ToSlash(rel))
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	for _, fixed := range []string{"package.json", "vite.config.js", "index.html"} {
		if info, err := os.Stat(filepath.Join(webDir, fixed)); err == nil && info.Mode().IsRegular() {
			add(fixed)
		}
	}
	if len(rels) == 0 {
		return "", fmt.Errorf("no web source files under %s", webDir)
	}
	sort.Strings(rels)
	h := sha256.New()
	for _, rel := range rels {
		raw, err := os.ReadFile(filepath.Join(webDir, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		h.Write([]byte(rel + "\n"))
		h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

// prevDirName holds the previous binary set beside the installed one: one
// generation only, overwritten by every update that swaps.
const prevDirName = ".el-prev"

// stashPrevious copies the installed binaries aside before the swap, so a
// failed health check restores them. It reports which names had a previous
// binary; names with none are removed again on rollback instead.
func stashPrevious(dir string) (map[string]bool, error) {
	prev := map[string]bool{}
	prevDir := filepath.Join(dir, prevDirName)
	if err := os.MkdirAll(prevDir, 0o755); err != nil {
		return nil, err
	}
	for _, name := range updateNames() {
		dst := filepath.Join(dir, name)
		info, err := os.Stat(dst)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		raw, err := os.ReadFile(dst)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(prevDir, name), raw, 0o755); err != nil {
			return nil, err
		}
		prev[name] = true
	}
	return prev, nil
}

// restorePrevious puts the stashed set back after a failed health check:
// names that had a previous binary get it back, names the update newly
// installed are removed, returning the install dir to its pre-update set.
func restorePrevious(dir string, prev map[string]bool) error {
	prevDir := filepath.Join(dir, prevDirName)
	var errs []string
	for _, name := range updateNames() {
		dst := filepath.Join(dir, name)
		if !prev[name] {
			_ = os.Remove(dst)
			continue
		}
		raw, err := os.ReadFile(filepath.Join(prevDir, name))
		if err != nil {
			errs = append(errs, name+": "+err.Error())
			continue
		}
		if err := os.WriteFile(dst, raw, 0o755); err != nil {
			errs = append(errs, name+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("could not restore: %s", strings.Join(errs, ", "))
	}
	return nil
}

// probeWebHealth boots a freshly built el-web on a throwaway loopback port
// with a throwaway state dir and waits for its /api/health. A var so tests
// stub the boot and drive the rollback without a toolchain.
var probeWebHealth = func(bin string) error {
	stateDir, err := os.MkdirTemp("", "el-update-probe")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stateDir)
	cmd := exec.Command(bin, "-addr", "127.0.0.1:0", "-restart-check", "0", "-state-dir", stateDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	url, err := awaitDaemonURL(stdout)
	if err != nil {
		return err
	}
	return waitHealthy(url+"/api/health", readTokenFile(filepath.Join(stateDir, "enderbloom", "daemon.token")), 20*time.Second)
}

// awaitDaemonURL reads the "el-web on <url>" line the Daemon prints once it
// listens. A binary that never prints it never served.
func awaitDaemonURL(stdout io.Reader) (string, error) {
	lines := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if url, ok := strings.CutPrefix(sc.Text(), "el-web on "); ok {
				lines <- strings.TrimSpace(url)
				return
			}
		}
		lines <- ""
	}()
	select {
	case url := <-lines:
		if url == "" {
			return "", fmt.Errorf("the new el-web exited before it listened")
		}
		return url, nil
	case <-time.After(15 * time.Second):
		return "", fmt.Errorf("the new el-web never listened")
	}
}

// waitHealthyPollInterval spaces the /api/health polls inside waitHealthy. A
// var so tests can shrink it; production keeps the 200ms pace.
var waitHealthyPollInterval = 200 * time.Millisecond

// waitHealthy polls a health endpoint until it answers ok or the timeout
// passes. A wait that only fails is a false alarm about a slow disk away
// from stranding prod, so every error reads as what it saw last.
func waitHealthy(url, token string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	last := "no response yet"
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(withDaemonToken(req, token))
		if err != nil {
			last = err.Error()
		} else {
			func() {
				defer resp.Body.Close()
				var body struct {
					OK bool `json:"ok"`
				}
				if resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&body) == nil && body.OK {
					last = ""
					return
				}
				last = fmt.Sprintf("status %d", resp.StatusCode)
			}()
			if last == "" {
				return nil
			}
		}
		time.Sleep(waitHealthyPollInterval)
	}
	return fmt.Errorf("/api/health never answered ok: %s", last)
}

// updateUsage documents the one command that rebuilds the UI and replaces
// the installed el binaries, and the gate around it.
const updateUsage = `usage: el update

  Rebuild the web UI unless its source is unchanged, then rebuild el,
  el-mcp and el-web with version stamps and install them into the
  directory of the running el binary (normally ~/.local/bin). Keeps the
  previous set and restores it when the new el-web fails its health check.
  Running sessions keep the old binary until they restart. Ends with the same step
  as el deps, proving every external dependency. el install is an alias.

  This is the only way to replace the installed el binaries.`

// RunUpdate serves `el update` (and its alias `el install`). It rebuilds
// the UI unless unchanged and every binary; a new el-web that fails its
// health check rolls back to the previous set.
func RunUpdate(argv []string, out io.Writer) error {
	var opts UpdateOptions
	for _, a := range argv {
		switch a {
		case "--help", "-h", "help":
			fmt.Fprintln(out, updateUsage)
			return nil
		default:
			return fmt.Errorf("unknown flag %q\n%s", a, updateUsage)
		}
	}
	res, err := UpdateBinaries(installExec, opts)
	if err != nil {
		return err
	}
	switch {
	case res.UIHash == "":
		fmt.Fprintln(out, "ui: no web source found, skipping vite build")
	case res.UIRebuilt:
		fmt.Fprintf(out, "ui: rebuilt the web bundle (%s)\n", res.UIHash)
	default:
		fmt.Fprintf(out, "ui: unchanged (%s), skipping vite build\n", res.UIHash)
	}
	fmt.Fprintf(out, "installed el, el-mcp and el-web into %s\n", res.Dir)
	fmt.Fprintf(out, "version %s, commit %s, ui %s\n", res.Version, res.Commit, res.UIHash)
	if res.Warning != "" {
		fmt.Fprintln(out, res.Warning)
	}
	if err := runInstallDeps(out); err != nil {
		return err
	}
	return nil
}

// RunInstall serves `el install`, the alias of `el update`.
func RunInstall(argv []string, out io.Writer) error {
	return RunUpdate(argv, out)
}
