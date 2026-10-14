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
	t.Parallel()
	state := tempDir(t)
	repo := tempDir(t)
	d := NewDaemon(state, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
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
	t.Parallel()
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
	srv := httptest.NewServer(authedHandler(t, d))
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

func TestProjectGistSourcesAreCachedAndRederived(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		file        string
		content     string
		updated     string
		want        string
		wantUpdated string
		otherDocs   string
	}{
		{
			name:        "agents takes precedence",
			file:        "AGENTS.md",
			content:     "# Example\n\n## Purpose\n\nThis project builds an **agent loop** for Go teams.\n\n## Tests\n\nIgnore this section.\n",
			updated:     "# Example\n\n## Purpose\n\nUpdated project purpose for Go teams.\n",
			want:        "This project builds an agent loop for Go teams.",
			wantUpdated: "Updated project purpose for Go teams.",
			otherDocs:   "# README\n\nREADME gist should lose to AGENTS.\n",
		},
		{
			name:        "readme fallback",
			file:        "README.md",
			content:     "# Example\n\nThe README describes the application and its main purpose.\n\nMore details follow.\n",
			updated:     "# Example\n\nThe README now describes its updated application purpose.\n",
			want:        "The README describes the application and its main purpose.",
			wantUpdated: "The README now describes its updated application purpose.",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			state, projectPath := tempDir(t), tempDir(t)
			write := func(name, content string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(projectPath, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write(tt.file, tt.content)
			if tt.otherDocs != "" {
				write("README.md", tt.otherDocs)
			}
			d := NewDaemon(state, NewFakeBackend())
			srv := httptest.NewServer(authedHandler(t, d))
			defer srv.Close()
			code, body := apiJSON(t, "POST", srv.URL+"/api/projects", map[string]string{"path": projectPath})
			if code != http.StatusCreated {
				t.Fatalf("create project = %d %s", code, body)
			}
			var created Project
			if err := json.Unmarshal(body, &created); err != nil {
				t.Fatal(err)
			}
			if created.Gist != tt.want || created.GistEdited {
				t.Fatalf("created gist = %+v, want %q and unedited", created, tt.want)
			}

			stored, err := os.ReadFile(d.projectsFile())
			if err != nil {
				t.Fatal(err)
			}
			var cached []Project
			if err := json.Unmarshal(stored, &cached); err != nil || len(cached) != 1 || cached[0].Gist != tt.want {
				t.Fatalf("cached projects = %+v, err %v", cached, err)
			}

			write(tt.file, tt.updated)
			d2 := NewDaemon(state, NewFakeBackend())
			if len(d2.projects) != 1 || d2.projects[0].Gist != tt.wantUpdated {
				t.Fatalf("rederived projects = %+v, want gist %q", d2.projects, tt.wantUpdated)
			}
		})
	}
}

func TestEditedProjectGistWinsOverRederivation(t *testing.T) {
	t.Parallel()
	state, projectPath := tempDir(t), tempDir(t)
	if err := os.WriteFile(filepath.Join(projectPath, "AGENTS.md"), []byte("## Purpose\n\nDerived gist.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := NewDaemon(state, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	code, body := apiJSON(t, "POST", srv.URL+"/api/projects", map[string]string{"path": projectPath})
	if code != http.StatusCreated {
		t.Fatalf("create project = %d %s", code, body)
	}
	var project Project
	if err := json.Unmarshal(body, &project); err != nil {
		t.Fatal(err)
	}

	code, body = apiJSON(t, "PATCH", srv.URL+"/api/projects/"+project.ID, map[string]string{"gist": "Edited gist.\nStill one line."})
	if code != http.StatusOK {
		t.Fatalf("edit gist = %d %s", code, body)
	}
	var edited Project
	if err := json.Unmarshal(body, &edited); err != nil {
		t.Fatal(err)
	}
	if edited.Gist != "Edited gist. Still one line." || !edited.GistEdited {
		t.Fatalf("edited project = %+v", edited)
	}

	if err := os.WriteFile(filepath.Join(projectPath, "AGENTS.md"), []byte("## Purpose\n\nChanged derived gist.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d2 := NewDaemon(state, NewFakeBackend())
	if len(d2.projects) != 1 || d2.projects[0].Gist != edited.Gist || !d2.projects[0].GistEdited {
		t.Fatalf("edited gist was overwritten on reload: %+v", d2.projects)
	}
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	code, body = apiJSON(t, "GET", srv2.URL+"/api/projects", nil)
	var listed []Project
	if err := json.Unmarshal(body, &listed); err != nil || code != http.StatusOK || len(listed) != 1 || listed[0].Gist != edited.Gist || !listed[0].GistEdited {
		t.Fatalf("listed projects = %+v, code %d, err %v", listed, code, err)
	}
}
