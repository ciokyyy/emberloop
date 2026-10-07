package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Choices are the optional parts the user picks on the first screen.
type Choices struct {
	Pig    bool
	Claude bool
	Graph  bool
}

// Env is everything a step needs: where the repo is, where files go, and
// whether to touch the machine at all.
type Env struct {
	Repo    string
	Home    string
	BinDir  string
	DryRun  bool
	Choices Choices
}

// warning marks a step that failed without blocking the install.
type warning struct{ msg string }

func (w warning) Error() string { return w.msg }

// Step is one unit of install work. Run returns a short detail line.
type Step struct {
	Name string
	Run  func(*Env) (string, error)
}

// binaries are the three programs built from the repo.
var binaries = []struct{ name, dir, pkg string }{
	{"emberloop", "emberloop", "./cmd/emberloop"},
	{"emberloop-mcp", "emberloop", "./cmd/emberloop-mcp"},
	{"emberloop-tui", "emberloop-tui", "./cmd/emberloop-tui"},
}

// Steps lists the install steps for the given choices, in run order.
func Steps(c Choices) []Step {
	steps := []Step{
		{"Check build tools", checkTools},
		{"Install herdr", installHerdr},
		{"Build emberloop binaries", buildBinaries},
		{"Link herdr plugin", linkPlugin},
		{"Add herdr compress key", addKey},
		{"Migrate old wick-pig data", migrateOld},
		{"Write emberloop settings", writeSettings},
	}
	if c.Pig {
		steps = append(steps,
			Step{"Check pig", checkPig},
			Step{"Link pig extensions", linkPigExtensions},
			Step{"Register emberloop MCP with pig", registerPigMCP},
		)
	}
	if c.Claude {
		steps = append(steps, Step{"Check claude", checkClaude})
	}
	steps = append(steps, Step{"Check bun", installBun})
	return steps
}

func (e *Env) sh(dir string, name string, args ...string) (string, error) {
	if e.DryRun {
		return "dry run: " + name + " " + strings.Join(args, " "), nil
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+e.pathWithBin())
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, lastLine(text))
	}
	return text, nil
}

func (e *Env) pathWithBin() string {
	return e.BinDir + string(os.PathListSeparator) + os.Getenv("PATH")
}

func (e *Env) has(name string) bool {
	_, err := lookIn(e.pathWithBin(), name)
	return err == nil
}

func lookIn(path, name string) (string, error) {
	for _, dir := range filepath.SplitList(path) {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", errors.New("not found")
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func checkTools(e *Env) (string, error) {
	var missing []string
	for _, t := range []string{"git", "go", "curl"} {
		if !e.has(t) {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing %s: install them and rerun", strings.Join(missing, ", "))
	}
	return "git, go, curl found", nil
}

func installHerdr(e *Env) (string, error) {
	if e.has("herdr") {
		return "already installed", nil
	}
	if _, err := e.sh("", "sh", "-c", "curl -fsSL https://herdr.dev/install.sh | sh"); err != nil {
		return "", err
	}
	if !e.DryRun && !e.has("herdr") {
		return "", warning{"herdr installed but not on PATH: add its bin directory to PATH"}
	}
	return "installed with the official script", nil
}

func buildBinaries(e *Env) (string, error) {
	if !e.DryRun {
		if err := os.MkdirAll(e.BinDir, 0o755); err != nil {
			return "", err
		}
	}
	for _, b := range binaries {
		tmp := filepath.Join(e.BinDir, "."+b.name+".new")
		if _, err := e.sh(filepath.Join(e.Repo, b.dir), "go", "build", "-buildvcs=false", "-o", tmp, b.pkg); err != nil {
			return "", err
		}
		if !e.DryRun {
			if err := os.Rename(tmp, filepath.Join(e.BinDir, b.name)); err != nil {
				return "", err
			}
		}
	}
	return "built into " + e.BinDir, nil
}

func linkPlugin(e *Env) (string, error) {
	if !e.has("herdr") {
		return "", warning{"herdr missing: run `herdr plugin link " + filepath.Join(e.Repo, "herdr-emberloop") + "` later"}
	}
	dir := filepath.Join(e.Repo, "herdr-emberloop")
	if _, err := e.sh("", "herdr", "plugin", "link", dir); err != nil {
		return "", warning{err.Error()}
	}
	return "linked " + dir, nil
}

func herdrConfigPath(e *Env) string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "herdr", "config.toml")
	}
	return filepath.Join(e.Home, ".config", "herdr", "config.toml")
}

func addKey(e *Env) (string, error) {
	path := herdrConfigPath(e)
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	out, status := addHerdrKeybinding(string(raw))
	switch status {
	case "present":
		return "key already bound", nil
	case "conflict":
		return "", warning{"prefix+shift+c is taken in " + path + ": bind command emberloop.compress to another key"}
	}
	if e.DryRun {
		return "dry run: would append the key to " + path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if len(raw) > 0 {
		if err := os.WriteFile(path+".bak", raw, 0o644); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return "", err
	}
	if e.has("herdr") {
		_, _ = e.sh("", "herdr", "server", "reload-config")
	}
	return "bound prefix+shift+c in " + path, nil
}

func writeSettings(e *Env) (string, error) {
	path := os.Getenv("EMBERLOOP_SETTINGS")
	if path == "" {
		path = filepath.Join(e.Home, ".emberloop", "settings.json")
	}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	out, err := mergeGraphEnabled(raw, e.Choices.Graph)
	if err != nil {
		return "", err
	}
	if e.DryRun {
		return fmt.Sprintf("dry run: would set graph.enabled=%v in %s", e.Choices.Graph, path), nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("graph.enabled=%v in %s", e.Choices.Graph, path), nil
}

func checkPig(e *Env) (string, error) {
	if e.has("pig") {
		return "pig found", nil
	}
	return "", warning{"pig not found: install PiG from https://github.com/MichaelKinsy/PiG, then rerun this installer"}
}

func linkPigExtensions(e *Env) (string, error) {
	extDir := filepath.Join(e.Home, ".pig", "agent", "extensions")
	sdk := filepath.Join(e.Home, ".pig", "state", "pigsdk", "sdk")
	if !e.DryRun {
		if err := os.MkdirAll(extDir, 0o755); err != nil {
			return "", err
		}
	}
	for _, name := range []string{"herdr-report", "pig-antigravity-auth"} {
		src := filepath.Join(e.Repo, name)
		dst := filepath.Join(extDir, name)
		if e.DryRun {
			continue
		}
		if target, err := os.Readlink(dst); err != nil || target != src {
			_ = os.Remove(dst)
			if err := os.Symlink(src, dst); err != nil {
				return "", err
			}
		}
		work := filepath.Join(src, "go.work")
		if raw, err := os.ReadFile(work); err == nil {
			if out := rewriteGoWorkSDK(string(raw), sdk); out != string(raw) {
				if err := os.WriteFile(work, []byte(out), 0o644); err != nil {
					return "", err
				}
			}
		}
	}
	return "linked into " + extDir, nil
}

func registerPigMCP(e *Env) (string, error) {
	path := filepath.Join(e.Home, ".pig", "agent", "mcp.json")
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	out, err := mergeMCPServer(raw, filepath.Join(e.BinDir, "emberloop-mcp"))
	if err != nil {
		return "", err
	}
	if e.DryRun {
		return "dry run: would write emberloop to " + path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", err
	}
	return "wrote emberloop entry to " + path, nil
}

func checkClaude(e *Env) (string, error) {
	if e.has("claude") {
		return "claude found: emberloop passes its MCP config and hooks per run", nil
	}
	if _, err := e.sh("", "sh", "-c", "curl -fsSL https://claude.ai/install.sh | bash"); err != nil {
		return "", warning{"claude install failed: " + err.Error()}
	}
	return "installed with the official script", nil
}

func installBun(e *Env) (string, error) {
	if e.has("bunx") {
		return "bun found (skill library needs it)", nil
	}
	if !e.has("unzip") {
		return "", warning{"unzip is missing: install it, then rerun to get bun (the skill library needs it)"}
	}
	if _, err := e.sh("", "sh", "-c", "curl -fsSL https://bun.sh/install | bash"); err != nil {
		return "", warning{"bun install failed: skills install will not work: " + err.Error()}
	}
	return "installed with the official script", nil
}

// FinalCommands run after the TUI exits, on the plain terminal, because they
// may ask for a sudo password.
func FinalCommands(e *Env) [][]string {
	bin := filepath.Join(e.BinDir, "emberloop")
	return [][]string{
		{bin, "deps"},
		{bin, "skills", "install"},
	}
}

// copyTree copies src to dst when dst does not exist yet. It never overwrites
// and never touches src.
func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil
		}
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
		raw, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, raw, info.Mode().Perm())
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, en := range entries {
		if err := copyTree(filepath.Join(src, en.Name()), filepath.Join(dst, en.Name())); err != nil {
			return err
		}
	}
	return nil
}

// migrateOld copies the settings, skill library and state of the project's
// former name into the emberloop locations. The old files stay in place.
func migrateOld(e *Env) (string, error) {
	pairs := [][2]string{
		{filepath.Join(e.Home, ".wick-pig"), filepath.Join(e.Home, ".emberloop")},
		{filepath.Join(e.Home, ".pig", "state", "wick"), filepath.Join(e.Home, ".pig", "state", "emberloop")},
	}
	n := 0
	for _, p := range pairs {
		if _, err := os.Stat(p[0]); err != nil {
			continue
		}
		n++
		if e.DryRun {
			continue
		}
		if err := copyTree(p[0], p[1]); err != nil {
			return "", err
		}
	}
	if n == 0 {
		return "nothing to migrate", nil
	}
	return "copied old data (originals kept)", nil
}
