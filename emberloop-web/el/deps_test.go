package el

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"example.com/emberloop-web/el/tools"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Seam (ticket 19): installer — the install command chosen per package
// manager comes from one table. Expected values come from each manager's own
// install usage, not from the implementation.
func TestDepsInstallCommandPerManager(t *testing.T) {
	cases := []struct {
		manager string
		pkgs    []string
		want    string
	}{
		{"pacman", []string{"python", "ripgrep"}, "sudo pacman -S --needed python ripgrep"},
		{"apt", []string{"python3", "ripgrep"}, "sudo apt-get install -y python3 ripgrep"},
		{"dnf", []string{"python3", "ripgrep"}, "sudo dnf install -y python3 ripgrep"},
		{"zypper", []string{"python3", "ripgrep"}, "sudo zypper install -y python3 ripgrep"},
		{"brew", []string{"python3", "ripgrep"}, "brew install python3 ripgrep"},
	}
	for _, c := range cases {
		got := installCommandDisplay(c.manager, c.pkgs)
		if got != c.want {
			t.Errorf("installCommandDisplay(%q, %v) = %q, want %q", c.manager, c.pkgs, got, c.want)
		}
	}
}

// Seam (ticket 19): installer — the package table names the right package
// per manager for each dependency.
func TestDepsPackageTable(t *testing.T) {
	cases := []struct {
		dep     string
		manager string
		want    string
	}{
		{"python3", "pacman", "python"},
		{"python3", "apt", "python3"},
		{"python3", "dnf", "python3"},
		{"python3", "zypper", "python3"},
		{"python3", "brew", "python3"},
		{"rg", "pacman", "ripgrep"},
		{"rg", "apt", "ripgrep"},
		{"rg", "brew", "ripgrep"},
		{"git", "pacman", "git"},
		{"git", "apt", "git"},
		{"bwrap", "pacman", "bubblewrap"},
		{"bwrap", "apt", "bubblewrap"},
		{"bwrap", "dnf", "bubblewrap"},
	}
	for _, c := range cases {
		got, ok := depPackage(c.dep, c.manager)
		if !ok {
			t.Errorf("depPackage(%q, %q): no entry, want %q", c.dep, c.manager, c.want)
			continue
		}
		if got != c.want {
			t.Errorf("depPackage(%q, %q) = %q, want %q", c.dep, c.manager, got, c.want)
		}
	}
}

// Seam (ticket 19): installer — without a terminal the command is printed
// and exits non-zero instead of calling sudo.
func TestDepsWithoutTerminalPrintsAndFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EL_BIN_DIR", dir)
	// An empty PATH with no managers and no deps: detection finds nothing,
	// and with no terminal nothing may run.
	t.Setenv("PATH", t.TempDir())
	var out strings.Builder
	cfg := DepsConfig{BinDir: dir, IsTerminal: func() bool { return false }}
	err := InstallDeps(&out, cfg)
	if err == nil {
		t.Fatalf("without a terminal and missing deps, InstallDeps must fail, output:\n%s", out.String())
	}
	got := out.String()
	if !strings.Contains(got, "python3") {
		t.Errorf("output must name the missing dependency, got:\n%s", got)
	}
}

// Seam (ticket 19): installer — graph download is checksum-verified; a wrong
// checksum is refused and nothing lands in the bin directory.
func TestDepsGraphDownloadVerifiesChecksum(t *testing.T) {
	binDir := t.TempDir()
	payload := []byte("#!/bin/sh\nread line\necho '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2024-11-05\"}}'\n")
	srv := depsFakeRelease(t, payload, false)
	cfg := DepsConfig{BinDir: binDir, ReleaseBaseURL: srv, GraphVersion: "v9.9.9", GOOS: "linux", GOARCH: "amd64"}
	got, err := cfg.downloadGraph()
	if err != nil {
		t.Fatalf("downloadGraph with a good checksum must succeed: %v", err)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("downloaded binary missing: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("downloaded binary %s must have the execute bit, mode %v", got, info.Mode())
	}
	if dir := filepath.Dir(got); dir != binDir {
		t.Errorf("binary landed in %s, want the bin directory %s", dir, binDir)
	}
}

func TestDepsGraphDownloadRefusesWrongChecksum(t *testing.T) {
	binDir := t.TempDir()
	payload := []byte("#!/bin/sh\necho hi\n")
	srv := depsFakeRelease(t, payload, true)
	cfg := DepsConfig{BinDir: binDir, ReleaseBaseURL: srv, GraphVersion: "v9.9.9", GOOS: "linux", GOARCH: "amd64"}
	if _, err := cfg.downloadGraph(); err == nil {
		t.Fatal("a wrong checksum must be refused")
	} else if !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("the refusal must name the checksum, got: %v", err)
	}
	entries, _ := os.ReadDir(binDir)
	if len(entries) != 0 {
		t.Errorf("a refused download must leave the bin directory empty, holds %d files", len(entries))
	}
}

func depsFakeRelease(t *testing.T, payload []byte, wrong bool) string {
	t.Helper()
	tarball := depsMakeTarball(t, payload)
	sum := sha256.Sum256(tarball)
	good := hex.EncodeToString(sum[:])
	if wrong {
		good = strings.Repeat("0", 64)
	}
	var sums strings.Builder
	for _, asset := range []string{
		GraphBinaryName + "-linux-amd64-portable.tar.gz",
		GraphBinaryName + "-linux-arm64-portable.tar.gz",
		GraphBinaryName + "-darwin-amd64.tar.gz",
		GraphBinaryName + "-darwin-arm64.tar.gz",
	} {
		fmt.Fprintf(&sums, "%s  %s\n", good, asset)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			_, _ = io.WriteString(w, sums.String())
			return
		}
		if strings.HasSuffix(r.URL.Path, ".tar.gz") {
			_, _ = w.Write(tarball)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// depsMakeTarball packs payload as the graph binary plus a decoy install.sh,
// mirroring the upstream tarball layout. Only the binary may be extracted.
func depsMakeTarball(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		body []byte
		mode int64
	}{
		{GraphBinaryName, payload, 0o755},
		{"install.sh", []byte("#!/bin/sh\nexit 1\n"), 0o755},
		{"LICENSE", []byte("fake license\n"), 0o644},
	} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func depsWriteFake(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func depsFakePath(t *testing.T, pythonVersion string) string {
	t.Helper()
	dir := t.TempDir()
	if pythonVersion == "" {
		pythonVersion = "Python 3.11.2"
	}
	depsWriteFake(t, dir, "python3", "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \""+pythonVersion+"\"; else exit 0; fi\n")
	depsWriteFake(t, dir, "rg", "#!/bin/sh\necho \"ripgrep 15.0.0\"\n")
	depsWriteFake(t, dir, "git", "#!/bin/sh\necho \"git version 2.44.0\"\n")
	depsWriteFake(t, dir, "bwrap", "#!/bin/sh\nexit 0\n")
	return dir
}

func depsDisabledSettings() *tools.Settings {
	s := tools.Settings{}
	off := false
	s.Graph.Enabled = &off
	s.Browser.Enabled = &off
	return &s
}
func depsEnabledSettings() *tools.Settings {
	s := tools.Settings{}
	return &s
}

func depsHermeticPath(t *testing.T, fake string) {
	t.Helper()
	t.Setenv("PATH", fake+":/usr/bin:/bin")
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "settings.json"))
	t.Setenv("EL_PROJECT_SETTINGS", filepath.Join(t.TempDir(), "project-settings.json"))
}

// Seam (ticket 19): installer — each dependency gets a proof line with its
// version; a failing required dependency fails the command.
func TestDepsProofLinesAndRequiredFailure(t *testing.T) {
	fake := depsFakePath(t, "")
	depsHermeticPath(t, fake)
	binDir := t.TempDir()
	t.Setenv("EL_BIN_DIR", binDir)
	var out strings.Builder
	cfg := DepsConfig{BinDir: binDir, Settings: depsDisabledSettings(), IsTerminal: func() bool { return true }, Worktree: t.TempDir()}
	if err := InstallDeps(&out, cfg); err != nil {
		t.Fatalf("all fakes present must succeed: %v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{"python3", "3.11.2", "rg", "ripgrep", "git", "2.44.0", "bwrap", "ok"} {
		if !strings.Contains(got, want) {
			t.Errorf("proof output must contain %q, got:\n%s", want, got)
		}
	}
	old := depsFakePath(t, "Python 3.8.0")
	depsHermeticPath(t, old)
	t.Setenv("EL_BIN_DIR", t.TempDir())
	var out2 strings.Builder
	cfg2 := DepsConfig{BinDir: t.TempDir(), Settings: depsDisabledSettings(), IsTerminal: func() bool { return true }, Worktree: t.TempDir()}
	// The old fake must win: prepend it and drop the good one by using only it plus system path.
	_ = fake
	if err := InstallDeps(&out2, cfg2); err == nil {
		t.Fatalf("python 3.8 must fail the command, output:\n%s", out2.String())
	}
	if !strings.Contains(out2.String(), "FAIL") || !strings.Contains(out2.String(), "python3") {
		t.Errorf("failure output must mark python3 FAIL, got:\n%s", out2.String())
	}
}

// Seam (ticket 19): installer — the graph binary only warns.
func TestDepsGraphOnlyWarns(t *testing.T) {
	fake := depsFakePath(t, "")
	depsHermeticPath(t, fake)
	binDir := t.TempDir()
	t.Setenv("EL_BIN_DIR", binDir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	var out strings.Builder
	cfg := DepsConfig{BinDir: binDir, ReleaseBaseURL: srv.URL, GraphVersion: "v9.9.9", GOOS: "linux", GOARCH: "amd64", Settings: depsEnabledSettings(), IsTerminal: func() bool { return true }, Worktree: t.TempDir()}
	if err := InstallDeps(&out, cfg); err != nil {
		t.Fatalf("a failing graph must only warn, got error %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "warn") || !strings.Contains(out.String(), GraphBinaryName) {
		t.Errorf("graph failure must print a warn line, got:\n%s", out.String())
	}
}

// Seam (ticket 19): installer — graph.enabled=false skips the graph.
func TestDepsGraphDisabledSkipsDownload(t *testing.T) {
	fake := depsFakePath(t, "")
	depsHermeticPath(t, fake)
	binDir := t.TempDir()
	t.Setenv("EL_BIN_DIR", binDir)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	var out strings.Builder
	cfg := DepsConfig{BinDir: binDir, ReleaseBaseURL: srv.URL, Settings: depsDisabledSettings(), IsTerminal: func() bool { return true }, Worktree: t.TempDir()}
	if err := InstallDeps(&out, cfg); err != nil {
		t.Fatalf("disabled graph must succeed: %v\n%s", err, out.String())
	}
	if hits != 0 {
		t.Errorf("disabled graph must not hit the release server, got %d hits", hits)
	}
	if !strings.Contains(out.String(), "disabled") {
		t.Errorf("disabled graph must print disabled, got:\n%s", out.String())
	}
}

// Seam (ticket 19): installer — manager detection comes from the probe binary
// on PATH.
func TestDepsDetectsManager(t *testing.T) {
	cases := []struct{ manager, probe string }{
		{"pacman", "pacman"},
		{"apt", "apt-get"},
		{"dnf", "dnf"},
		{"zypper", "zypper"},
		{"brew", "brew"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		depsWriteFake(t, dir, c.probe, "#!/bin/sh\nexit 0\n")
		t.Setenv("PATH", dir)
		if got := detectPackageManager(); got != c.manager {
			t.Errorf("detectPackageManager with %s = %q, want %q", c.probe, got, c.manager)
		}
	}
}

// Seam (ticket 19): installer — with a terminal the exact command runs
// through sudo; a fake sudo and fake manager record the call and provide
// nothing, so the re-check still fails.
func TestDepsTerminalRunsInstallThroughSudo(t *testing.T) {
	dir := t.TempDir()
	sudoRecord := filepath.Join(t.TempDir(), "sudo-args")
	aptRecord := filepath.Join(t.TempDir(), "apt-args")
	depsWriteFake(t, dir, "sudo", "#!/bin/sh\necho \"$@\" > \""+sudoRecord+"\"\nexec \"$@\"\n")
	depsWriteFake(t, dir, "apt-get", "#!/bin/sh\necho \"$@\" > \""+aptRecord+"\"\nexit 0\n")
	t.Setenv("PATH", dir)
	t.Setenv("EL_BIN_DIR", t.TempDir())
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "settings.json"))
	t.Setenv("EL_PROJECT_SETTINGS", filepath.Join(t.TempDir(), "proj.json"))
	var out strings.Builder
	cfg := DepsConfig{BinDir: t.TempDir(), Settings: depsEnabledSettings(), IsTerminal: func() bool { return true }, Worktree: t.TempDir()}
	if err := InstallDeps(&out, cfg); err == nil {
		t.Fatalf("a fake install that provides nothing must still fail, output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "sudo apt-get install -y") {
		t.Errorf("output must print the exact command, got:\n%s", out.String())
	}
	raw, _ := os.ReadFile(sudoRecord)
	if !strings.Contains(string(raw), "apt-get") {
		t.Errorf("sudo must have run apt-get, sudo saw %q", string(raw))
	}
	if _, err := os.Stat(aptRecord); err != nil {
		t.Errorf("the fake package manager must have run: %v", err)
	}
}

// Seam (ticket 19): installer — a second run changes nothing and says so.
func TestDepsSecondRunChangesNothing(t *testing.T) {
	fake := depsFakePath(t, "")
	depsHermeticPath(t, fake)
	binDir := t.TempDir()
	depsWriteFake(t, binDir, GraphBinaryName, "#!/bin/sh\nread line\necho '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2024-11-05\"}}'\n")
	depsWriteFake(t, binDir, BrowserBinaryName, "#!/bin/sh\necho \"agent-browser 0.39.1\"\n")
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	run := func() string {
		var out strings.Builder
		cfg := DepsConfig{BinDir: binDir, ReleaseBaseURL: srv.URL, GraphVersion: "v9.9.9", GOOS: "linux", GOARCH: "amd64", Settings: depsEnabledSettings(), IsTerminal: func() bool { return true }, Worktree: t.TempDir()}
		if err := InstallDeps(&out, cfg); err != nil {
			t.Fatalf("idempotent run must succeed: %v\n%s", err, out.String())
		}
		return out.String()
	}
	first := run()
	before, _ := os.Stat(filepath.Join(binDir, GraphBinaryName))
	second := run()
	after, _ := os.Stat(filepath.Join(binDir, GraphBinaryName))
	if !strings.Contains(second, "nothing to do") {
		t.Errorf("second run must say nothing to do, got:\n%s", second)
	}
	if !strings.Contains(first, "nothing to do") {
		t.Errorf("first run with everything present must say nothing to do, got:\n%s", first)
	}
	if hits != 0 {
		t.Errorf("no download must happen when the binary is present, got %d hits", hits)
	}
	if !os.SameFile(before, after) {
		// SameFile compares identity; fall back to modtime and size.
		if before.ModTime() != after.ModTime() || before.Size() != after.Size() {
			t.Errorf("second run must not touch the binary")
		}
	}
}

// Seam (ticket 19): installer deps CLI usage.
func TestRunDepsFlags(t *testing.T) {
	var out strings.Builder
	if err := RunDeps([]string{"--bogus"}, &out); err == nil {
		t.Error("deps --bogus must fail")
	}
	out.Reset()
	if err := RunDeps([]string{"--help"}, &out); err != nil {
		t.Errorf("deps --help must succeed, got %v", err)
	}
}

// Seam (ticket 19): installer — an empty checksums.txt is a refused
// download, never a panic; the graph only warns.
func TestDepsGraphEmptyChecksumIsRefused(t *testing.T) {
	binDir := t.TempDir()
	tarball := depsMakeTarball(t, []byte("#!/bin/sh\necho hi\n"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			_, _ = io.WriteString(w, "   \n")
			return
		}
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)
	cfg := DepsConfig{BinDir: binDir, ReleaseBaseURL: srv.URL, GraphVersion: "v9.9.9", GOOS: "linux", GOARCH: "amd64"}
	if _, err := cfg.downloadGraph(); err == nil {
		t.Fatal("an empty checksum file must be refused")
	} else if !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("the refusal must name the checksum, got: %v", err)
	}
	if entries, _ := os.ReadDir(binDir); len(entries) != 0 {
		t.Errorf("a refused download must leave the bin directory empty")
	}
}

// Seam (ticket 19): installer — without a terminal and with a manager
// present, the exact command is printed and exits non-zero without calling
// sudo.
func TestDepsWithoutTerminalWithManagerPrintsCommand(t *testing.T) {
	dir := t.TempDir()
	sudoRecord := filepath.Join(t.TempDir(), "sudo-args")
	aptRecord := filepath.Join(t.TempDir(), "apt-args")
	depsWriteFake(t, dir, "sudo", "#!/bin/sh\necho ran > \""+sudoRecord+"\"\nexit 0\n")
	depsWriteFake(t, dir, "apt-get", "#!/bin/sh\necho ran > \""+aptRecord+"\"\nexit 0\n")
	t.Setenv("PATH", dir)
	t.Setenv("EL_BIN_DIR", t.TempDir())
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "settings.json"))
	t.Setenv("EL_PROJECT_SETTINGS", filepath.Join(t.TempDir(), "proj.json"))
	var out strings.Builder
	cfg := DepsConfig{BinDir: t.TempDir(), Settings: depsEnabledSettings(), IsTerminal: func() bool { return false }, Worktree: t.TempDir()}
	if err := InstallDeps(&out, cfg); err == nil {
		t.Fatalf("without a terminal, InstallDeps must fail, output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "sudo apt-get install -y") {
		t.Errorf("output must print the exact install command, got:\n%s", out.String())
	}
	if _, err := os.Stat(sudoRecord); err == nil {
		t.Error("sudo must not run without a terminal")
	}
	if _, err := os.Stat(aptRecord); err == nil {
		t.Error("the package manager must not run without a terminal")
	}
}
