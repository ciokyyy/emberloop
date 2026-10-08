package web

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"

	emberloop "example.com/emberloop"
)

// specEntry is one discovered spec: its spec file, title and ticket count.
type specEntry struct {
	Spec    string `json:"spec"`
	Title   string `json:"title"`
	Tickets int    `json:"tickets"`
}

// specScanDepth is how deep the spec scan looks under a repo directory:
// deep enough for a spec area like .scratch/<name>/spec.md, shallow enough
// to stay out of vendored trees.
const specScanDepth = 4

// handleSpecs lists the specs under a repo directory.
func (d *Daemon) handleSpecs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo := r.URL.Query().Get("repo")
	if repo == "" {
		http.Error(w, "repo is required", http.StatusBadRequest)
		return
	}
	entries, err := discoverSpecs(repo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"specs": entries})
}

// discoverSpecs scans a repo directory for spec files (files named spec.md)
// and resolves each through core spec resolution, which also finds the
// issues directory beside it. A directory with no spec returns an empty list.
func discoverSpecs(repo string) ([]specEntry, error) {
	info, err := os.Stat(repo)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, os.ErrNotExist
	}
	var candidates []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if name == ".git" || name == "node_modules" {
				continue
			}
			if e.IsDir() {
				if depth < specScanDepth {
					walk(filepath.Join(dir, name), depth+1)
				}
				continue
			}
			if name == "spec.md" && depth+1 <= specScanDepth {
				candidates = append(candidates, filepath.Join(dir, name))
			}
		}
	}
	walk(repo, 0)
	entries := []specEntry{}
	seen := map[string]bool{}
	for _, c := range candidates {
		run, err := emberloop.ResolveSpec(c)
		if err != nil {
			continue
		}
		if seen[run.Spec] {
			continue
		}
		seen[run.Spec] = true
		files, err := emberloop.ReadDriverFiles(run)
		if err != nil {
			continue
		}
		count := len(emberloop.PreviewTickets(run, files))
		entries = append(entries, specEntry{Spec: run.Spec, Title: emberloop.SpecTitle(run.Spec), Tickets: count})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Spec < entries[j].Spec })
	return entries, nil
}

// handleSpecPreview returns the ticket preview for one spec, with blockers
// and checkbox progress per ticket. One bad ticket fails only its own row.
func (d *Daemon) handleSpecPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	arg := r.URL.Query().Get("spec")
	if arg == "" {
		http.Error(w, "spec is required", http.StatusBadRequest)
		return
	}
	run, err := emberloop.ResolveSpec(arg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	files, err := emberloop.ReadDriverFiles(run)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	previews := emberloop.PreviewTickets(run, files)
	if previews == nil {
		previews = []emberloop.TicketPreview{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"spec":    run.Spec,
		"title":   emberloop.SpecTitle(run.Spec),
		"tickets": previews,
	})
}
