package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// projectGistLink removes Markdown link syntax while retaining its readable label.
var projectGistLink = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)

// agentsMDNames are the files that satisfy a root DOX contract, in the same
// priority the project-context loader reads them. A directory holding any of
// them already has a chain root.
var agentsMDNames = []string{"AGENTS.override.md", "AGENTS.md", "AGENTS.MD"}

// agentsMDTemplate seeds a root DOX contract. %s is the directory base name,
// twice: the title and the purpose line.
const agentsMDTemplate = "# AGENTS.md — %s\n\n## Purpose\n\nWorking contract for %s. Project knowledge lives here; keep it current so agents share one source of truth.\n\nBefore editing, read this file and any AGENTS.md nearer your target; after a contract-changing edit, update the owning file in the same task.\n\n## Child DOX Index\n\n(none yet — add `path/AGENTS.md — what it owns` lines as subcontracts appear)\n"

// ensureAgentsMD creates a root AGENTS.md in dir when the directory holds no
// AGENTS file. Run worker starts (runStartWorker) call this on the fresh
// worktree before the Worker starts, so a Run-launched project always has a
// DOX chain even when the repo root lacks one. (Seeding through `el init`
// belongs to `.scratch/project-init/issues/02`, which owns that command; this
// helper stays unhooked from project creation until that ticket lands.) An
// existing file is never touched. It reports whether it created the file.
func ensureAgentsMD(dir string) (bool, error) {
	for _, n := range agentsMDNames {
		if st, err := os.Stat(filepath.Join(dir, n)); err == nil && !st.IsDir() {
			return false, nil
		}
	}
	content := fmt.Sprintf(agentsMDTemplate, filepath.Base(dir), filepath.Base(dir))
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// Project is a recorded working directory. Sessions started in a Project run
// with the Project path as their cwd. Projects persist in
// stateDir/enderbloom/projects.json, so the sidebar survives restarts. Gist is
// derived from the Project docs and cached; GistEdited marks an explicit override.
type Project struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Gist       string    `json:"gist,omitempty"`
	GistEdited bool      `json:"gistEdited,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
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
	if err := json.Unmarshal(b, &d.projects); err != nil {
		d.projects = nil
		return
	}
	changed := false
	for i := range d.projects {
		if refreshProjectGist(&d.projects[i]) {
			changed = true
		}
	}
	if changed {
		_ = d.saveProjectsLocked()
	}
}

// refreshProjectGist derives and caches a Project's gist unless the user has
// explicitly edited it. It returns whether the cached value changed.
func refreshProjectGist(project *Project) bool {
	if project.GistEdited {
		return false
	}
	gist := deriveProjectGist(project.Path)
	if gist == "" || gist == project.Gist {
		return false
	}
	project.Gist = gist
	return true
}

// deriveProjectGist reads a root AGENTS file first, then README.md. Only local
// project documentation is read; its first Purpose paragraph (or first prose
// paragraph when no Purpose section exists) becomes the one-line gist.
func deriveProjectGist(projectPath string) string {
	for _, name := range agentsMDNames {
		b, err := os.ReadFile(filepath.Join(projectPath, name))
		if err != nil {
			continue
		}
		gist := projectGistParagraph(string(b))
		if gist != "" && !(strings.HasPrefix(gist, "Working contract for ") && strings.Contains(gist, "Project knowledge lives here;")) {
			return gist
		}
	}
	for _, name := range []string{"README", "README.md", "README.MD", "Readme.md", "readme.md"} {
		b, err := os.ReadFile(filepath.Join(projectPath, name))
		if err == nil {
			if gist := projectGistParagraph(string(b)); gist != "" {
				return gist
			}
		}
	}
	return ""
}

func projectGistParagraph(markdown string) string {
	lines := strings.Split(markdown, "\n")
	start := 0
	for i, line := range lines {
		level, title := projectMarkdownHeading(line)
		if level == 2 && strings.EqualFold(title, "Purpose") {
			start = i + 1
			break
		}
	}

	var paragraph []string
	inCode := false
	for _, raw := range lines[start:] {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
			continue
		}
		if inCode || strings.HasPrefix(line, "<!--") || strings.HasPrefix(line, "![") {
			continue
		}
		if line == "" {
			if len(paragraph) != 0 {
				break
			}
			continue
		}
		if level, _ := projectMarkdownHeading(line); level != 0 {
			if len(paragraph) != 0 {
				break
			}
			continue
		}
		paragraph = append(paragraph, line)
	}
	text := strings.Join(paragraph, " ")
	text = projectGistLink.ReplaceAllString(text, "$1")
	text = strings.NewReplacer("`", "", "**", "", "__", "", "*", "", "_", "", "~~", "").Replace(text)
	return strings.Join(strings.Fields(text), " ")
}

func projectMarkdownHeading(line string) (int, string) {
	line = strings.TrimSpace(line)
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || (level < len(line) && line[level] != ' ') {
		return 0, ""
	}
	return level, strings.TrimSpace(line[level:])
}

func oneLineGist(text string) string {
	return strings.Join(strings.Fields(text), " ")
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
		gist := deriveProjectGist(path)
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
		p := Project{ID: newID(), Name: name, Path: path, Gist: gist, CreatedAt: time.Now().UTC()}
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
			Name *string `json:"name"`
			Gist *string `json:"gist"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Name == nil && req.Gist == nil) || (req.Name != nil && strings.TrimSpace(*req.Name) == "") {
			http.Error(w, "name or gist is required", http.StatusBadRequest)
			return
		}
		d.projMu.Lock()
		found := false
		for i := range d.projects {
			if d.projects[i].ID == id {
				if req.Name != nil {
					d.projects[i].Name = strings.TrimSpace(*req.Name)
				}
				if req.Gist != nil {
					d.projects[i].Gist = oneLineGist(*req.Gist)
					d.projects[i].GistEdited = true
				}
				p = d.projects[i]
				found = true
			}
		}
		if !found {
			d.projMu.Unlock()
			http.NotFound(w, r)
			return
		}
		if err := d.saveProjectsLocked(); err != nil {
			d.projMu.Unlock()
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
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
