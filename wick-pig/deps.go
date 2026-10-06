package wick

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"example.com/wick/tools"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// `wick-pig deps` installs and verifies every external dependency.
//
// Required: python3 (3.9+), rg, git, bwrap. Optional: the code graph binary
// (codebase-memory-mcp), unless graph.enabled is false in settings. Missing
// system packages install with the detected package manager from one table;
// the exact command is printed and run through sudo only when attached to a
// terminal, otherwise printed and the command exits non-zero. The graph
// binary downloads from its GitHub release tarball for the platform,
// verified against the release checksums.txt SHA-256, into the wick bin
// directory (WICK_BIN_DIR overrides). Its own install.sh never runs and no
// agent client config is touched. Every dependency is proved after install and
// printed one line with version and status; a failing required dependency
// fails the command while the graph only warns. A second run changes nothing
// and says so.
//
// Test seams: DepsConfig overrides the bin directory, terminal detection,
// release server, platform and settings, so tests use a fake PATH with fake
// binaries, a fake package manager and a fake release server. No test runs a
// real package install or download.

// GraphBinaryName is the code graph binary ticket 19 installs and ticket 20
// serves: one string, owned by the tools package that resolves it. Its own
// `install` command never runs.
const GraphBinaryName = tools.GraphBinaryName

// DefaultGraphVersion pins the graph release deps installs. Bump with the
// upstream release; tests override it with DepsConfig.GraphVersion.
const DefaultGraphVersion = "v0.11.0"

// DefaultGraphReleaseBase is the release download base: version and asset
// append as "<base>/<version>/<asset>". Tests point it at an httptest server.
const DefaultGraphReleaseBase = "https://github.com/DeusData/codebase-memory-mcp/releases/download"

// DepsConfig tunes InstallDeps. Zero values mean the real environment.
type DepsConfig struct {
	// BinDir overrides the wick bin directory; empty means tools.BinDir().
	BinDir string
	// IsTerminal reports whether sudo may prompt; nil checks stdin.
	IsTerminal func() bool
	// ReleaseBaseURL overrides the graph release base for tests.
	ReleaseBaseURL string
	// GraphVersion overrides DefaultGraphVersion.
	GraphVersion string
	// GOOS and GOARCH override runtime for tests.
	GOOS   string
	GOARCH string
	// Worktree is the tree the bubblewrap self-test proves; empty means cwd.
	Worktree string
	// Settings overrides the loaded settings (graph.enabled, graph.binary).
	// Nil loads the effective settings for the worktree.
	Settings *tools.Settings
	// HTTPClient does the graph download; nil uses a 60s client.
	HTTPClient *http.Client
}

func (c DepsConfig) binDir() string {
	if c.BinDir != "" {
		return c.BinDir
	}
	return tools.BinDir()
}

func (c DepsConfig) terminal() bool {
	if c.IsTerminal != nil {
		return c.IsTerminal()
	}
	return stdinIsTerminal()
}

func (c DepsConfig) releaseBase() string {
	if c.ReleaseBaseURL != "" {
		return strings.TrimSuffix(c.ReleaseBaseURL, "/")
	}
	return DefaultGraphReleaseBase
}

func (c DepsConfig) graphVersion() string {
	if c.GraphVersion != "" {
		return c.GraphVersion
	}
	return DefaultGraphVersion
}

func (c DepsConfig) goos() string {
	if c.GOOS != "" {
		return c.GOOS
	}
	return runtime.GOOS
}

func (c DepsConfig) goarch() string {
	if c.GOARCH != "" {
		return c.GOARCH
	}
	return runtime.GOARCH
}

func (c DepsConfig) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (c DepsConfig) settings() tools.Settings {
	if c.Settings != nil {
		return *c.Settings
	}
	root := c.Worktree
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	return tools.EffectiveSettings(root)
}

func (c DepsConfig) worktree() string {
	if c.Worktree != "" {
		return c.Worktree
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// depPackageTable maps a dependency to its package per manager. One table,
// so every install command names the same packages.
var depPackageTable = map[string]map[string]string{
	"python3": {
		"pacman": "python",
		"apt":    "python3",
		"dnf":    "python3",
		"zypper": "python3",
		"brew":   "python3",
	},
	"rg": {
		"pacman": "ripgrep",
		"apt":    "ripgrep",
		"dnf":    "ripgrep",
		"zypper": "ripgrep",
		"brew":   "ripgrep",
	},
	"git": {
		"pacman": "git",
		"apt":    "git",
		"dnf":    "git",
		"zypper": "git",
		"brew":   "git",
	},
	"bwrap": {
		"pacman": "bubblewrap",
		"apt":    "bubblewrap",
		"dnf":    "bubblewrap",
		"zypper": "bubblewrap",
		"brew":   "bubblewrap",
	},
}

// requiredDeps are the system dependencies in proof order.
var requiredDeps = []string{"python3", "rg", "git", "bwrap"}

// depPackage names the package for dep under manager.
func depPackage(dep, manager string) (string, bool) {
	pkgs, ok := depPackageTable[dep]
	if !ok {
		return "", false
	}
	pkg, ok := pkgs[manager]
	return pkg, ok
}

// packageManagers is the detection order.
var packageManagers = []string{"pacman", "apt", "dnf", "zypper", "brew"}

// managerProbe maps a manager to the binary its detection looks for.
var managerProbe = map[string]string{
	"pacman": "pacman",
	"apt":    "apt-get",
	"dnf":    "dnf",
	"zypper": "zypper",
	"brew":   "brew",
}

// detectPackageManager names the first supported manager on PATH.
func detectPackageManager() string {
	for _, m := range packageManagers {
		if _, err := exec.LookPath(managerProbe[m]); err == nil {
			return m
		}
	}
	return ""
}

// installCommandDisplay is the exact command printed and run for pkgs.
func installCommandDisplay(manager string, pkgs []string) string {
	switch manager {
	case "pacman":
		return "sudo pacman -S --needed " + strings.Join(pkgs, " ")
	case "apt":
		return "sudo apt-get install -y " + strings.Join(pkgs, " ")
	case "dnf":
		return "sudo dnf install -y " + strings.Join(pkgs, " ")
	case "zypper":
		return "sudo zypper install -y " + strings.Join(pkgs, " ")
	case "brew":
		return "brew install " + strings.Join(pkgs, " ")
	default:
		return strings.Join(pkgs, " ")
	}
}

// installCommandArgv is the argv run for the install. Brew needs no sudo;
// the others run through sudo, which prompts, so the caller checks for a
// terminal first.
func installCommandArgv(manager string, pkgs []string) []string {
	switch manager {
	case "pacman":
		return append([]string{"sudo", "pacman", "-S", "--needed"}, pkgs...)
	case "apt":
		return append([]string{"sudo", "apt-get", "install", "-y"}, pkgs...)
	case "dnf":
		return append([]string{"sudo", "dnf", "install", "-y"}, pkgs...)
	case "zypper":
		return append([]string{"sudo", "zypper", "install", "-y"}, pkgs...)
	case "brew":
		return append([]string{"brew", "install"}, pkgs...)
	default:
		return pkgs
	}
}

// runCmd runs name with args, returning trimmed stdout.
func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%s timed out after %v", name, timeout)
		}
		out := strings.TrimSpace(stdout.String())
		if out == "" {
			out = strings.TrimSpace(stderr.String())
		}
		if out != "" {
			return "", fmt.Errorf("%s failed: %v (%s)", name, err, depsFirstLine(out))
		}
		return "", fmt.Errorf("%s failed: %w", name, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func depsFirstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// resolveDep finds dep in the wick bin directory first, then PATH, through
// the shared tools.ResolveBinary order so the installer and the tools agree
// on what "installed" means. A DepsConfig.BinDir override is checked first
// for tests; otherwise resolution is exactly the shared helper.
func (c DepsConfig) resolveDep(name string) (string, error) {
	if dir := c.BinDir; dir != "" {
		if p := filepath.Join(dir, name); isDepsExecutable(p) {
			return p, nil
		}
	}
	return tools.ResolveBinary(name)
}

func isDepsExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

var pythonVersionRe = regexp.MustCompile(`Python\s+(\d+)\.(\d+)(?:\.(\d+))?`)

// parsePythonVersion reads "Python 3.11.2" into its numbers.
func parsePythonVersion(out string) (major, minor, patch int, ok bool) {
	m := pythonVersionRe.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	if m[3] != "" {
		patch, _ = strconv.Atoi(m[3])
	}
	return major, minor, patch, true
}

// probePython proves python3: it runs under -I and is 3.9 or newer.
func (c DepsConfig) probePython() (version string, err error) {
	bin, err := c.resolveDep("python3")
	if err != nil {
		return "", err
	}
	out, err := runCmd(30*time.Second, bin, "--version")
	if err != nil {
		return "", err
	}
	major, minor, patch, ok := parsePythonVersion(out)
	if !ok {
		return "", fmt.Errorf("python3 --version gave %q", depsFirstLine(out))
	}
	version = fmt.Sprintf("%d.%d.%d", major, minor, patch)
	if major < 3 || (major == 3 && minor < 9) {
		return version, fmt.Errorf("python3 is %s, need 3.9 or newer", version)
	}
	if _, err := runCmd(30*time.Second, bin, "-I", "-c", "print('ok')"); err != nil {
		return version, fmt.Errorf("python3 -I failed: %v", err)
	}
	return version, nil
}

// probeVersion runs bin --version and returns its first line.
func (c DepsConfig) probeVersion(bin, dep string) (string, error) {
	path, err := c.resolveDep(bin)
	if err != nil {
		return "", err
	}
	out, err := runCmd(30*time.Second, path, "--version")
	if err != nil {
		return "", err
	}
	line := depsFirstLine(out)
	if line == "" {
		return "", fmt.Errorf("%s --version gave no output", dep)
	}
	return line, nil
}

// probeBwrap proves bubblewrap with ticket 14's self-test: it must block
// writes and network, or review and scout Python never runs unsandboxed.
func (c DepsConfig) probeBwrap() (string, error) {
	if _, err := c.resolveDep("bwrap"); err != nil {
		return "", err
	}
	if err := tools.EnsureHealthy(c.worktree()); err != nil {
		return "", err
	}
	out, err := c.probeVersion("bwrap", "bwrap")
	if err != nil {
		// bwrap has no --version on some builds; the self-test is the proof.
		return "self-test passed", nil
	}
	return out + " (self-test passed)", nil
}

// graphAssetName names the upstream release tarball for the platform, e.g.
// codebase-memory-mcp-linux-amd64.tar.gz. Upstream ships tarballs (and .mcpb
// bundles); the checksums live in one checksums.txt beside them.
func graphAssetName(goos, goarch string) (string, error) {
	switch goos {
	case "linux", "darwin":
	default:
		return "", fmt.Errorf("unsupported platform %s/%s", goos, goarch)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported platform %s/%s", goos, goarch)
	}
	return fmt.Sprintf("%s-%s-%s.tar.gz", GraphBinaryName, goos, goarch), nil
}

// resolveGraph finds the graph binary in the bin dir, on PATH, or at
// graph.binary in settings. Empty means absent, never an error.
func (c DepsConfig) resolveGraph(s tools.Settings) string {
	if s.Graph.Binary != "" {
		if isDepsExecutable(s.Graph.Binary) {
			return s.Graph.Binary
		}
		if p, err := exec.LookPath(s.Graph.Binary); err == nil {
			return p
		}
		// A configured path that does not resolve counts as absent: the
		// download still lands in the bin directory.
	}
	if dir := c.binDir(); dir != "" {
		if p := filepath.Join(dir, GraphBinaryName); isDepsExecutable(p) {
			return p
		}
	}
	if p, err := exec.LookPath(GraphBinaryName); err == nil {
		return p
	}
	return ""
}

// downloadGraph fetches the release tarball for the platform, verifies it
// against the release checksums.txt SHA-256, extracts the graph binary and
// places it in the bin directory with the execute bit. A wrong checksum is
// refused. The tarball's install.sh never runs and no agent client config is
// touched: only the binary is extracted.
func (c DepsConfig) downloadGraph() (string, error) {
	asset, err := graphAssetName(c.goos(), c.goarch())
	if err != nil {
		return "", err
	}
	base := c.releaseBase()
	version := c.graphVersion()
	assetURL := base + "/" + version + "/" + asset
	sumsURL := base + "/" + version + "/checksums.txt"
	client := c.httpClient()
	sumsRaw, err := fetchURL(client, sumsURL)
	if err != nil {
		return "", fmt.Errorf("could not fetch %s: %v", sumsURL, err)
	}
	want, err := graphChecksum(sumsRaw, asset)
	if err != nil {
		return "", fmt.Errorf("%s: %v", sumsURL, err)
	}
	data, err := fetchURL(client, assetURL)
	if err != nil {
		return "", fmt.Errorf("could not fetch %s: %v", assetURL, err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return "", fmt.Errorf("checksum mismatch for %s: want %s, got %s; refusing it", asset, want, got)
	}
	bin, err := extractGraphBinary(data)
	if err != nil {
		return "", err
	}
	dir := c.binDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, GraphBinaryName+".*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(bin); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", err
	}
	_ = tmp.Close()
	if err := os.Chmod(tmpName, 0o755); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	dst := filepath.Join(dir, GraphBinaryName)
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	return dst, nil
}

// graphChecksum finds the SHA-256 for asset in a checksums.txt body. An
// empty or malformed file is an error, never a panic, so the caller warns
// instead of crashing.
func graphChecksum(sums []byte, asset string) (string, error) {
	var want string
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		if fields[1] == asset {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		if len(strings.TrimSpace(string(sums))) == 0 {
			return "", fmt.Errorf("checksum file is empty")
		}
		return "", fmt.Errorf("no checksum for %s", asset)
	}
	if _, err := hex.DecodeString(want); err != nil || len(want) != 64 {
		return "", fmt.Errorf("checksum for %s holds %q, not a SHA-256", asset, want)
	}
	return want, nil
}

// extractGraphBinary pulls the graph binary out of its release tarball. Only
// the binary is read; install.sh and the docs stay in the tarball.
func extractGraphBinary(tarball []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return nil, fmt.Errorf("release tarball is not gzip: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("release tarball is corrupt: %v", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != GraphBinaryName {
			continue
		}
		bin, err := io.ReadAll(io.LimitReader(tr, 200<<20))
		if err != nil {
			return nil, fmt.Errorf("release tarball is corrupt: %v", err)
		}
		if len(bin) == 0 {
			return nil, fmt.Errorf("release tarball holds an empty %s", GraphBinaryName)
		}
		return bin, nil
	}
	return nil, fmt.Errorf("release tarball holds no %s binary", GraphBinaryName)
}

func fetchURL(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 100<<20))
}

// probeGraphMCP proves the graph binary answers an MCP initialize.
func probeGraphMCP(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	defer func() { _ = cmd.Process.Kill() }()
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "wick", "version": "deps"},
		},
	}
	raw, _ := json.Marshal(req)
	raw = append(raw, '\n')
	if _, err := stdin.Write(raw); err != nil {
		return "", fmt.Errorf("graph initialize write failed: %v", err)
	}
	type resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	dec := json.NewDecoder(stdout)
	var r resp
	if err := dec.Decode(&r); err != nil {
		return "", fmt.Errorf("graph did not answer initialize: %v (%s)", err, strings.TrimSpace(stderr.String()))
	}
	if r.Error != nil {
		return "", fmt.Errorf("graph initialize failed: %s", r.Error.Message)
	}
	if len(r.Result) == 0 {
		return "", fmt.Errorf("graph gave an empty initialize result")
	}
	_ = cmd.Wait()
	return "initialize ok", nil
}

// depLine is one proved dependency for the output.
type depLine struct {
	name    string
	version string
	state   string
	note    string
}

func (d depLine) String() string {
	if d.note != "" {
		return fmt.Sprintf("%s %s %s (%s)", d.name, d.version, d.state, d.note)
	}
	if d.version != "" {
		return fmt.Sprintf("%s %s %s", d.name, d.version, d.state)
	}
	return fmt.Sprintf("%s %s", d.name, d.state)
}

// InstallDeps checks, installs and proves every dependency, printing one
// line per dependency with its version and status. It exits non-zero when a
// required dependency fails; the graph binary only warns.
func InstallDeps(out io.Writer, cfg DepsConfig) error {
	settings := cfg.settings()
	// Find what is missing before printing anything about managers.
	var missing []string
	for _, dep := range requiredDeps {
		if _, err := cfg.resolveDep(dep); err != nil {
			missing = append(missing, dep)
		}
	}
	changed := false
	if len(missing) > 0 {
		manager := detectPackageManager()
		if manager == "" {
			fmt.Fprintf(out, "missing: %s\n", strings.Join(missing, ", "))
			fmt.Fprintln(out, "no supported package manager found (looked for pacman, apt-get, dnf, zypper, brew)")
			return fmt.Errorf("missing dependencies: %s", strings.Join(missing, ", "))
		}
		var pkgs []string
		for _, dep := range missing {
			pkg, ok := depPackage(dep, manager)
			if !ok {
				return fmt.Errorf("no %s package for %s", manager, dep)
			}
			pkgs = append(pkgs, pkg)
		}
		display := installCommandDisplay(manager, pkgs)
		fmt.Fprintf(out, "missing: %s\n", strings.Join(missing, ", "))
		fmt.Fprintf(out, "run: %s\n", display)
		if !cfg.terminal() {
			return fmt.Errorf("not attached to a terminal; run: %s", display)
		}
		argv := installCommandArgv(manager, pkgs)
		installCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(installCtx, argv[0], argv[1:]...)
		cmd.Stdout = out
		cmd.Stderr = out
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("install failed (%s): %v", display, err)
		}
		changed = true
		// Re-check: the install must have provided them.
		for _, dep := range missing {
			if _, err := cfg.resolveDep(dep); err != nil {
				return fmt.Errorf("%s still missing after %s", dep, display)
			}
		}
	}
	var lines []depLine
	failed := false
	var firstErr error
	prove := func(name string, version string, err error) {
		if err != nil {
			lines = append(lines, depLine{name: name, version: version, state: "FAIL", note: err.Error()})
			failed = true
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %v", name, err)
			}
			return
		}
		lines = append(lines, depLine{name: name, version: version, state: "ok"})
	}
	pyVer, pyErr := cfg.probePython()
	prove("python3", pyVer, pyErr)
	rgVer, rgErr := cfg.probeVersion("rg", "rg")
	prove("rg", rgVer, rgErr)
	gitVer, gitErr := cfg.probeVersion("git", "git")
	prove("git", gitVer, gitErr)
	bwVer, bwErr := cfg.probeBwrap()
	prove("bwrap", bwVer, bwErr)

	graphEnabled := settings.GraphEnabled()
	graphWarn := ""
	if !graphEnabled {
		lines = append(lines, depLine{name: GraphBinaryName, version: "", state: "disabled", note: "graph.enabled=false"})
	} else {
		gpath := cfg.resolveGraph(settings)
		if gpath == "" {
			got, err := cfg.downloadGraph()
			if err != nil {
				graphWarn = err.Error()
				lines = append(lines, depLine{name: GraphBinaryName, version: cfg.graphVersion(), state: "warn", note: err.Error()})
			} else {
				changed = true
				gpath = got
			}
		}
		if gpath != "" && graphWarn == "" {
			if _, err := probeGraphMCP(gpath); err != nil {
				graphWarn = err.Error()
				lines = append(lines, depLine{name: GraphBinaryName, version: cfg.graphVersion(), state: "warn", note: err.Error()})
			} else {
				lines = append(lines, depLine{name: GraphBinaryName, version: cfg.graphVersion(), state: "ok"})
			}
		}
	}
	for _, l := range lines {
		fmt.Fprintln(out, l.String())
	}
	if !changed {
		fmt.Fprintln(out, "all dependencies satisfied (nothing to do)")
	}
	if failed {
		return firstErr
	}
	return nil
}

// depsUsage documents the installer.
const depsUsage = `usage: wick-pig deps

  Check python3 (3.9+), rg, git, bwrap and the code graph binary, install
  what is missing with the system package manager, download the graph binary
  verified by SHA-256 into the wick bin directory, and prove each one.
  Prints one line per dependency with its version and status; exits non-zero
  when a required dependency fails (the graph only warns). A second run
  changes nothing.`

// RunDeps serves `wick-pig deps`.
func RunDeps(argv []string, out io.Writer) error {
	for _, a := range argv {
		switch a {
		case "--help", "-h", "help":
			fmt.Fprintln(out, depsUsage)
			return nil
		default:
			return fmt.Errorf("unknown flag %q\n%s", a, depsUsage)
		}
	}
	return InstallDeps(out, DepsConfig{})
}
