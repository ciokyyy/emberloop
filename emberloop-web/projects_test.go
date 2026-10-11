package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func apiJSON(t *testing.T, method, url string, body any) (int, []byte) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(res.Body)
	return res.StatusCode, buf.Bytes()
}

// Projects persist across Daemon restarts, dedupe by path and start Sessions
// whose cwd is the Project path; worktree Sessions resolve to their Project.
func TestProjectsPersistAndStartSessions(t *testing.T) {
	state := tempDir(t)
	repo := tempDir(t)
	d := NewDaemon(state, NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	code, b := apiJSON(t, "POST", srv.URL+"/api/projects", map[string]string{"path": repo})
	if code != http.StatusCreated {
		t.Fatalf("add = %d %s", code, b)
	}
	var p Project
	_ = json.Unmarshal(b, &p)
	if code, _ := apiJSON(t, "POST", srv.URL+"/api/projects", map[string]string{"path": repo}); code != http.StatusOK {
		t.Fatalf("duplicate add = %d, want 200", code)
	}
	if code, _ := apiJSON(t, "POST", srv.URL+"/api/projects", map[string]string{"path": filepath.Join(repo, "nope")}); code != http.StatusBadRequest {
		t.Fatalf("missing dir = %d, want 400", code)
	}

	code, b = apiJSON(t, "POST", srv.URL+"/api/sessions", map[string]string{"harness": "fake", "model": "fake-model", "projectId": p.ID})
	if code != http.StatusCreated {
		t.Fatalf("create = %d %s", code, b)
	}
	var m SessionMeta
	_ = json.Unmarshal(b, &m)
	if m.WorkDir != p.Path || m.ProjectID != p.ID {
		t.Fatalf("session = %+v, want workDir and projectId of the Project", m)
	}
	wt := filepath.Join(p.Path, ".emberloop-worktrees", "t1")
	if got := d.projectIDForPath(wt); got != p.ID {
		t.Fatalf("worktree resolves to %q, want %q", got, p.ID)
	}

	d2 := NewDaemon(state, NewFakeBackend())
	if len(d2.projects) != 1 || d2.projects[0].ID != p.ID {
		t.Fatalf("reloaded projects = %+v", d2.projects)
	}
	if code, _ := apiJSON(t, "DELETE", srv.URL+"/api/projects/"+p.ID, nil); code != http.StatusOK {
		t.Fatalf("delete = %d", code)
	}
}

// The git view reports branch, changes and log, and diffs one path.
func TestProjectGitView(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := tempDir(t)
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-b", "main")
	_ = os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n"), 0o644)
	run("add", ".")
	run("commit", "-m", "first")
	_ = os.WriteFile(filepath.Join(repo, "a.txt"), []byte("two\n"), 0o644)
	_ = os.WriteFile(filepath.Join(repo, "b.txt"), []byte("new\n"), 0o644)

	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	_, b := apiJSON(t, "POST", srv.URL+"/api/projects", map[string]string{"path": repo})
	var p Project
	_ = json.Unmarshal(b, &p)

	_, b = apiJSON(t, "GET", srv.URL+"/api/projects/"+p.ID+"/git", nil)
	var g GitInfo
	_ = json.Unmarshal(b, &g)
	if !g.IsRepo || g.Branch != "main" || len(g.Files) != 2 || len(g.Log) != 1 || g.Log[0].Subject != "first" {
		t.Fatalf("git = %+v", g)
	}
	_, b = apiJSON(t, "GET", srv.URL+"/api/projects/"+p.ID+"/git/diff?path=a.txt", nil)
	var dd struct{ Diff string }
	_ = json.Unmarshal(b, &dd)
	if !bytes.Contains([]byte(dd.Diff), []byte("+two")) {
		t.Fatalf("diff = %q", dd.Diff)
	}
	_, b = apiJSON(t, "GET", srv.URL+"/api/projects/"+p.ID+"/git/diff?path=b.txt", nil)
	_ = json.Unmarshal(b, &dd)
	if !bytes.Contains([]byte(dd.Diff), []byte("+new")) {
		t.Fatalf("untracked diff = %q", dd.Diff)
	}
	if code, _ := apiJSON(t, "GET", srv.URL+"/api/projects/"+p.ID+"/git/diff?path=../x", nil); code != http.StatusBadRequest {
		t.Fatalf("escape = %d, want 400", code)
	}
}
