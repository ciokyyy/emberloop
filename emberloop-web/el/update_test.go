package el

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// updateTestState extends the install fixture with the web module: the UI
// source the update hashes and builds, empty docs and skills dirs so the
// docs refresh touches nothing real, and a passing health probe.
func updateTestState(t *testing.T) (dir, src, webDir string) {
	t.Helper()
	dir = installTestState(t)
	src = os.Getenv("EL_SOURCE_DIR")
	if err := os.MkdirAll(filepath.Join(src, "..", "emberloop-web"), 0o755); err != nil {
		t.Fatal(err)
	}
	webDir = filepath.Join(src, "..", "emberloop-web", "web")
	fixture := map[string]string{
		"src/main.js":    "console.log('app');\n",
		"src/lib/api.js": "export const x = 1;\n",
		"public/sw.js":   "/* worker */\n",
		"package.json":   "{\"name\": \"el-web-ui\"}\n",
		"vite.config.js": "export default {};\n",
		"index.html":     "<html></html>\n",
	}
	for rel, body := range fixture {
		p := filepath.Join(webDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("EL_DOCS_DIR", t.TempDir())
	t.Setenv("EL_SKILLS_DIR", t.TempDir())
	// installTestState already stubs the health probe to pass.
	return dir, src, webDir
}

// updateFake plays the update's commands: herdr pane list, git stamps, go
// builds (marker where -o points), and npx vite builds. It records go
// invocations and vite runs for the assertions.
type updateFake struct {
	panes      []string
	describe   string
	rev        string
	npxCode    int
	goArgs     [][]string
	npxCalls   int
	goCalls    int
	failNpxMsg string
}

func (f *updateFake) exec(t *testing.T, webDir string) Exec {
	t.Helper()
	return func(cmd string, args []string, o ExecOpts) (Result, error) {
		switch cmd {
		case "go":
			f.goCalls++
			f.goArgs = append(f.goArgs, append([]string{}, args...))
			out := ""
			for i, a := range args {
				if a == "-o" && i+1 < len(args) {
					out = args[i+1]
				}
			}
			if out == "" {
				return Result{Code: 1, Stderr: "no -o"}, nil
			}
			pkg := args[len(args)-1]
			if err := os.WriteFile(out, []byte("built "+pkg), 0o755); err != nil {
				return Result{}, err
			}
			return Result{Code: 0}, nil
		case "git":
			if len(args) > 0 && args[0] == "describe" {
				if f.describe == "" {
					return Result{Code: 1, Stderr: "no tag"}, nil
				}
				return Result{Code: 0, Stdout: f.describe + "\n"}, nil
			}
			if f.rev == "" {
				return Result{Code: 1, Stderr: "no rev"}, nil
			}
			return Result{Code: 0, Stdout: f.rev + "\n"}, nil
		case "npx":
			f.npxCalls++
			if f.npxCode != 0 {
				return Result{Code: f.npxCode, Stderr: f.failNpxMsg}, nil
			}
			if err := os.MkdirAll(filepath.Join(webDir, "dist"), 0o755); err != nil {
				return Result{}, err
			}
			if err := os.WriteFile(filepath.Join(webDir, "dist", "index.html"), []byte("<html/>"), 0o644); err != nil {
				return Result{}, err
			}
			return Result{Code: 0}, nil
		}
		if len(args) >= 2 && args[0] == "pane" && args[1] == "list" {
			var b strings.Builder
			b.WriteString(`{"result":{"panes":[`)
			for i, p := range f.panes {
				if i > 0 {
					b.WriteString(",")
				}
				b.WriteString(`{"pane_id":"` + p + `"}`)
			}
			b.WriteString(`]}}`)
			return Result{Code: 0, Stdout: b.String()}, nil
		}
		return Result{Code: 1, Stderr: "no such command"}, nil
	}
}

// The web source hash is stable and short, changes with the source, and
// refuses an empty tree.
func TestHashWebSourceStableShortAndSensitive(t *testing.T) {
	_, _, webDir := updateTestState(t)
	first, err := hashWebSource(webDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 12 {
		t.Errorf("hash = %q, want 12 hex chars", first)
	}
	second, err := hashWebSource(webDir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("hash unstable: %q vs %q", first, second)
	}
	if err := os.WriteFile(filepath.Join(webDir, "src", "main.js"), []byte("console.log('changed');\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := hashWebSource(webDir)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Error("hash must change when the source changes")
	}
	if _, err := hashWebSource(t.TempDir()); err == nil {
		t.Error("an empty tree must not hash")
	}
}

// An unchanged UI skips vite: no npx call, but all three binaries build.
func TestUpdateSkipsUnchangedUI(t *testing.T) {
	dir, _, webDir := updateTestState(t)
	h, err := hashWebSource(webDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(uiHashPath(dir), []byte(h+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(webDir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "dist", "index.html"), []byte("<html/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &updateFake{rev: "abc1234"}
	res, err := UpdateBinaries(fake.exec(t, webDir), UpdateOptions{})
	if err != nil {
		t.Fatalf("update with unchanged UI must succeed: %v", err)
	}
	if res.UIRebuilt {
		t.Error("an unchanged UI must skip the vite build")
	}
	if fake.npxCalls != 0 {
		t.Errorf("an unchanged UI must never run vite, got %d calls", fake.npxCalls)
	}
	for _, name := range []string{"el", "el-mcp", "el-web"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("binary %s must be installed: %v", name, err)
		}
	}
}

// A changed UI runs vite once and records the new hash, so the next update
// skips again.
func TestUpdateRebuildsChangedUIAndRecordsHash(t *testing.T) {
	dir, _, webDir := updateTestState(t)
	h, err := hashWebSource(webDir)
	if err != nil {
		t.Fatal(err)
	}
	fake := &updateFake{rev: "abc1234"}
	res, err := UpdateBinaries(fake.exec(t, webDir), UpdateOptions{})
	if err != nil {
		t.Fatalf("update with changed UI must succeed: %v", err)
	}
	if !res.UIRebuilt {
		t.Error("a changed UI must run the vite build")
	}
	if fake.npxCalls != 1 {
		t.Errorf("vite must run exactly once, got %d calls", fake.npxCalls)
	}
	raw, err := os.ReadFile(uiHashPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != h {
		t.Errorf("recorded hash = %q, want %q", strings.TrimSpace(string(raw)), h)
	}
}

// Every binary builds with the el stamps; the Daemon build carries the UI
// hash too.
func TestUpdateStampsEveryBinary(t *testing.T) {
	_, _, webDir := updateTestState(t)
	h, err := hashWebSource(webDir)
	if err != nil {
		t.Fatal(err)
	}
	fake := &updateFake{rev: "abc1234"}
	res, err := UpdateBinaries(fake.exec(t, webDir), UpdateOptions{})
	if err != nil {
		t.Fatalf("update must succeed: %v", err)
	}
	if res.Version != "dev" || res.Commit != "abc1234" || res.UIHash != h {
		t.Errorf("stamps = %+v, want dev/abc1234/%s", res, h)
	}
	joined := func() string {
		var b strings.Builder
		for _, args := range fake.goArgs {
			b.WriteString(strings.Join(args, " "))
			b.WriteString("\n")
		}
		return b.String()
	}()
	for _, want := range []string{
		"-X example.com/emberloop.Version=dev",
		"-X example.com/emberloop.Commit=abc1234",
		"-X example.com/emberloop-web.UIHash=" + h,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("a build must carry %q, got:\n%s", want, joined)
		}
	}
	if n := strings.Count(joined, "example.com/emberloop-web.UIHash"); n != 1 {
		t.Errorf("only the el-web build must carry the UI hash, got %d carriers", n)
	}
}

// A new el-web that never answers /api/health restores the previous set:
// every binary reads as before, and the update fails naming the rollback.
func TestUpdateRollsBackWhenHealthFails(t *testing.T) {
	dir, _, webDir := updateTestState(t)
	for _, name := range updateNames() {
		writeInstallSentinel(t, dir, name, "old-"+name)
	}
	oldProbe := probeWebHealth
	probeWebHealth = func(string) error { return errors.New("probe down") }
	t.Cleanup(func() { probeWebHealth = oldProbe })
	fake := &updateFake{rev: "abc1234"}
	_, err := UpdateBinaries(fake.exec(t, webDir), UpdateOptions{})
	if err == nil {
		t.Fatal("an update whose el-web fails health must fail")
	}
	if !strings.Contains(err.Error(), "health check") || !strings.Contains(err.Error(), "restored the previous binaries") {
		t.Errorf("the failure must name the health check and the rollback, got: %v", err)
	}
	for _, name := range updateNames() {
		if got := readInstallFile(t, dir, name); got != "old-"+name {
			t.Errorf("rollback must restore %s, got %q", name, got)
		}
	}
}

// A healthy update keeps one previous generation beside the new set.
func TestUpdateKeepsPreviousOnSuccess(t *testing.T) {
	dir, _, webDir := updateTestState(t)
	writeInstallSentinel(t, dir, "el-web", "old-el-web")
	fake := &updateFake{rev: "abc1234"}
	if _, err := UpdateBinaries(fake.exec(t, webDir), UpdateOptions{}); err != nil {
		t.Fatalf("update must succeed: %v", err)
	}
	if got := readInstallFile(t, filepath.Join(dir, prevDirName), "el-web"); got != "old-el-web" {
		t.Errorf("the previous el-web must be kept, got %q", got)
	}
	if got := readInstallFile(t, dir, "el-web"); got != "built ./cmd/emberloop-web" {
		t.Errorf("the new el-web must be installed, got %q", got)
	}
}

// Rollback removes binaries the update newly installed when there is no
// previous generation to restore.
func TestRestorePreviousRemovesNewBinaries(t *testing.T) {
	dir := t.TempDir()
	for _, name := range updateNames() {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("new"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := restorePrevious(dir, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range updateNames() {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("a new binary with no previous generation must be removed: %s", name)
		}
	}
}

// Unknown flags fail with usage; --help prints it; el install is the same
// command under its old name.
func TestRunUpdateFlagsAndInstallAlias(t *testing.T) {
	updateTestState(t)
	_, _, webDir := updateTestState(t)
	var out strings.Builder
	if err := RunUpdate([]string{"--bogus"}, &out); err == nil {
		t.Error("an unknown update flag must fail")
	} else if !strings.Contains(err.Error(), "usage") {
		t.Errorf("the failure must print usage, got: %v", err)
	}
	if err := RunUpdate([]string{"--help"}, &out); err != nil {
		t.Errorf("--help must succeed, got: %v", err)
	} else if !strings.Contains(out.String(), "usage: el update") {
		t.Errorf("--help must print update usage, got %q", out.String())
	}

	oldDeps := runInstallDeps
	runInstallDeps = func(out io.Writer) error { return nil }
	t.Cleanup(func() { runInstallDeps = oldDeps })
	oldExec := installExec
	installExec = (&updateFake{rev: "abc1234"}).exec(t, webDir)
	t.Cleanup(func() { installExec = oldExec })
	out.Reset()
	if err := RunInstall([]string{}, &out); err != nil {
		t.Fatalf("el install must alias el update: %v", err)
	}
	if !strings.Contains(out.String(), "installed el, el-mcp and el-web into") {
		t.Errorf("the alias must print the update report, got %q", out.String())
	}
}

// waitHealthy answers nil once the endpoint reports ok, and an error naming
// what it saw when the endpoint never does.
func TestWaitHealthy(t *testing.T) {
	oldPoll := waitHealthyPollInterval
	waitHealthyPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { waitHealthyPollInterval = oldPoll })
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ok.Close()
	if err := waitHealthy(ok.URL, "", 5*time.Second); err != nil {
		t.Errorf("a healthy endpoint must pass, got: %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer bad.Close()
	if err := waitHealthy(bad.URL, "", 150*time.Millisecond); err == nil {
		t.Error("a 404 endpoint must fail the wait")
	} else if !strings.Contains(err.Error(), "404") {
		t.Errorf("the failure must name what it saw, got: %v", err)
	}

	if err := waitHealthy("http://127.0.0.1:1/nope", "", 150*time.Millisecond); err == nil {
		t.Error("an unreachable endpoint must fail the wait")
	}
}

// The probe presents the machine token of the state dir it boots.
func TestWaitHealthyPresentsToken(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	if err := waitHealthy(srv.URL, "probe-token", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer probe-token" {
		t.Fatalf("Authorization = %q, want the probe token", auth)
	}
}
