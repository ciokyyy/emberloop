package web

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Project is a recorded working directory. Sessions started in a Project run
// with the Project path as their cwd. Projects persist in
// stateDir/enderbloom/projects.json, so the sidebar survives restarts.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

func (d *Daemon) projectsFile() string {
	return filepath.Join(d.stateDir, "enderbloom", "projects.json")
}

// loadProjects reads projects.json; a missing or broken file means no Projects.
func (d *Daemon) loadProjects() {
	d.projMu.Lock()
	defer d.projMu.Unlock()
	d.projects = nil
	b, err := os.ReadFile(d.projectsFile())
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, &d.projects)
}

// saveProjectsLocked writes projects.json atomically; callers hold d.projMu.
func (d *Daemon) saveProjectsLocked() error {
	if err := os.MkdirAll(filepath.Dir(d.projectsFile()), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(d.projects, "", "  ")
	if err != nil {
		return err
	}
	tmp := d.projectsFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, d.projectsFile())
}

// projectByID returns a copy of the Project, or false.
func (d *Daemon) projectByID(id string) (Project, bool) {
	d.projMu.Lock()
	defer d.projMu.Unlock()
	for _, p := range d.projects {
		if p.ID == id {
			return p, true
		}
	}
	return Project{}, false
}

func (d *Daemon) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		d.projMu.Lock()
		list := append([]Project{}, d.projects...)
		d.projMu.Unlock()
		writeJSON(w, http.StatusOK, list)
	case "POST":
		var req struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Path) == "" {
			http.Error(w, "path is required", http.StatusBadRequest)
			return
		}
		path := absWorkDir(req.Path)
		if st, err := os.Stat(path); err != nil || !st.IsDir() {
			http.Error(w, "path is not a directory", http.StatusBadRequest)
			return
		}
		d.projMu.Lock()
		defer d.projMu.Unlock()
		for _, p := range d.projects {
			if p.Path == path {
				writeJSON(w, http.StatusOK, p)
				return
			}
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = filepath.Base(path)
		}
		p := Project{ID: newID(), Name: name, Path: path, CreatedAt: time.Now().UTC()}
		d.projects = append(d.projects, p)
		if err := d.saveProjectsLocked(); err != nil {
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, p)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProjectSub routes /api/projects/{id}, /git and /git/diff.
func (d *Daemon) handleProjectSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	id, sub, _ := strings.Cut(rest, "/")
	p, ok := d.projectByID(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch {
	case sub == "" && r.Method == "PATCH":
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		d.projMu.Lock()
		for i := range d.projects {
			if d.projects[i].ID == id {
				d.projects[i].Name = strings.TrimSpace(req.Name)
				p = d.projects[i]
			}
		}
		_ = d.saveProjectsLocked()
		d.projMu.Unlock()
		writeJSON(w, http.StatusOK, p)
	case sub == "" && r.Method == "DELETE":
		d.projMu.Lock()
		kept := d.projects[:0:0]
		for _, q := range d.projects {
			if q.ID != id {
				kept = append(kept, q)
			}
		}
		d.projects = kept
		_ = d.saveProjectsLocked()
		d.projMu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case sub == "git" && r.Method == "GET":
		writeJSON(w, http.StatusOK, gitSummary(p.Path))
	case sub == "git/diff" && r.Method == "GET":
		out, err := gitDiff(p.Path, r.URL.Query().Get("path"), r.URL.Query().Get("staged") == "1")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"diff": out})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- git view ---

// GitFile is one changed path from `git status --porcelain`. Index and Work
// are the two status letters (space when unchanged).
type GitFile struct {
	Path  string `json:"path"`
	Index string `json:"index"`
	Work  string `json:"work"`
}

// GitCommit is one recent commit.
type GitCommit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

// GitInfo is the read-only git view of a Project.
type GitInfo struct {
	IsRepo   bool        `json:"isRepo"`
	Branch   string      `json:"branch,omitempty"`
	Upstream string      `json:"upstream,omitempty"`
	Ahead    int         `json:"ahead"`
	Behind   int         `json:"behind"`
	Files    []GitFile   `json:"files"`
	Log      []GitCommit `json:"log"`
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "core.quotepath=off"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	out, err := cmd.Output()
	return string(out), err
}

func gitSummary(dir string) GitInfo {
	info := GitInfo{Files: []GitFile{}, Log: []GitCommit{}}
	if out, err := gitOut(dir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return info
	}
	info.IsRepo = true
	if out, err := gitOut(dir, "status", "--porcelain=v2", "--branch"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			switch {
			case strings.HasPrefix(line, "# branch.head "):
				info.Branch = strings.TrimPrefix(line, "# branch.head ")
			case strings.HasPrefix(line, "# branch.upstream "):
				info.Upstream = strings.TrimPrefix(line, "# branch.upstream ")
			case strings.HasPrefix(line, "# branch.ab "):
				f := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
				if len(f) == 2 {
					info.Ahead, _ = strconv.Atoi(strings.TrimPrefix(f[0], "+"))
					info.Behind, _ = strconv.Atoi(strings.TrimPrefix(f[1], "-"))
				}
			case strings.HasPrefix(line, "1 "):
				f := strings.SplitN(line, " ", 9)
				if len(f) == 9 {
					info.Files = append(info.Files, gitFile(f[1], f[8]))
				}
			case strings.HasPrefix(line, "2 "):
				f := strings.SplitN(line, " ", 10)
				if len(f) == 10 {
					path, _, _ := strings.Cut(f[9], "\t")
					info.Files = append(info.Files, gitFile(f[1], path))
				}
			case strings.HasPrefix(line, "u "):
				f := strings.SplitN(line, " ", 11)
				if len(f) == 11 {
					info.Files = append(info.Files, gitFile(f[1], f[10]))
				}
			case strings.HasPrefix(line, "? "):
				info.Files = append(info.Files, GitFile{Path: strings.TrimPrefix(line, "? "), Index: "?", Work: "?"})
			}
		}
		sort.SliceStable(info.Files, func(i, j int) bool { return info.Files[i].Path < info.Files[j].Path })
	}
	if out, err := gitOut(dir, "log", "-20", "--format=%h%x1f%s%x1f%an%x1f%cI"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			f := strings.Split(line, "\x1f")
			if len(f) == 4 {
				info.Log = append(info.Log, GitCommit{Hash: f[0], Subject: f[1], Author: f[2], Date: f[3]})
			}
		}
	}
	return info
}

// gitFile converts the porcelain v2 XY code ('.' means unchanged).
func gitFile(xy, path string) GitFile {
	f := GitFile{Path: path, Index: " ", Work: " "}
	if len(xy) == 2 {
		if xy[0] != '.' {
			f.Index = string(xy[0])
		}
		if xy[1] != '.' {
			f.Work = string(xy[1])
		}
	}
	return f
}

// gitDiff returns the diff for one path (or everything when path is empty).
// The path must stay inside the Project directory.
func gitDiff(dir, path string, staged bool) (string, error) {
	args := []string{"diff", "--no-color"}
	if staged {
		args = append(args, "--cached")
	}
	if path != "" {
		if filepath.IsAbs(path) || strings.HasPrefix(filepath.Clean(path), "..") {
			return "", os.ErrInvalid
		}
		args = append(args, "--", path)
	}
	out, err := gitOut(dir, args...)
	if err != nil {
		return "", err
	}
	if out == "" && path != "" && !staged {
		// Untracked file: show it as an all-added diff.
		out, _ = gitOutAllowDiff(dir, "diff", "--no-color", "--no-index", "--", os.DevNull, path)
	}
	const limit = 200_000
	if len(out) > limit {
		out = out[:limit] + "\n… diff truncated"
	}
	return out, nil
}

// runGitAllowDiff tolerates exit status 1, which --no-index uses for "differs".
func gitOutAllowDiff(dir string, args ...string) (string, error) {
	out, err := gitOut(dir, args...)
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return out, nil
	}
	return out, err
}

// projectIDForPath returns the Project whose directory contains path (the
// deepest match), or "". Worktrees under a Project resolve to that Project.
func (d *Daemon) projectIDForPath(path string) string {
	if path == "" {
		return ""
	}
	d.projMu.Lock()
	defer d.projMu.Unlock()
	best, bestLen := "", 0
	for _, p := range d.projects {
		if (path == p.Path || strings.HasPrefix(path, p.Path+string(filepath.Separator))) && len(p.Path) > bestLen {
			best, bestLen = p.ID, len(p.Path)
		}
	}
	return best
}
