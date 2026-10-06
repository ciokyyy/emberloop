// Package tui is the wick dashboard: a bubbletea terminal UI in its own Go
// module so the core stays dependency-free. The deep module is the Snapshot
// read model — repos holding their specs, each with run status and ticket
// progress — which later tickets extend with per-ticket status, agents, cost
// and memory. The bubbletea model polls Snapshot every second.
package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Herdr is the liveness seam Snapshot reads through: the set of herdr panes
// still alive. Tests pass a fake; the binary passes one that runs
// `herdr pane list`. A pane missing from the set is gone, never running.
type Herdr func() (map[string]bool, error)

// Snap is the Runs tab's read model: every known repo with its specs.
// Known repos are the run records' repos plus the current repo, current first;
// every other repo and every spec inside one sorts by path and slug. Specs
// with no run yet are discovered from the repos' spec directories on disk, so
// waiting work is visible; a recorded run whose spec file is gone stays listed
// under its repo.
type Snap struct {
	Repos []RepoSnap
}

// RepoSnap is one repo's specs.
type RepoSnap struct {
	Path  string
	Specs []SpecSnap
}

// SpecSnap is one spec's run status and ticket progress. HasRun says a run
// record exists; Running says its orchestrator pane is still alive in herdr.
// Merged/Total is merged tickets over ticket files.
type SpecSnap struct {
	Slug    string
	Spec    string
	Branch  string
	HasRun  bool
	Running bool
	Merged  int
	Total   int
}

// runRecord is the slice of runs.json Snapshot needs. Unknown fields are
// ignored, so the core can grow the record without breaking this read.
type runRecord struct {
	Slug   string `json:"slug"`
	Spec   string `json:"spec"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Pane   string `json:"pane"`
}

// historyEntry is the slice of a run's ticket history Snapshot needs.
type historyEntry struct {
	Ticket  string `json:"ticket"`
	Outcome string `json:"outcome"`
}

// Snapshot reads the state dir's run records and ticket histories plus the
// repos' spec directories, deciding running from live herdr panes. A herdr
// failure is returned — the caller keeps its last snapshot and retries —
// while a missing or unreadable state file reads as no runs.
//
// currentRepo is a third parameter beside the spec's Snapshot(stateDir,
// herdr) seam: the pure read model needs the current checkout to order it
// first and to discover specs with no run yet, and herdr liveness carries
// no repo notion. Callers resolve it with CurrentRepo.
func Snapshot(stateDir, currentRepo string, herdr Herdr) (Snap, error) {
	live, err := herdr()
	if err != nil {
		return Snap{}, err
	}
	current := normalizeRepo(currentRepo)
	runs := loadRunRecords(stateDir)
	mergedBySlug := loadMergedBySlug(stateDir, runs)

	byRepo := map[string]map[string]*SpecSnap{}
	ensure := func(repo, slug string) *SpecSnap {
		if byRepo[repo] == nil {
			byRepo[repo] = map[string]*SpecSnap{}
		}
		s := byRepo[repo][slug]
		if s == nil {
			s = &SpecSnap{Slug: slug}
			byRepo[repo][slug] = s
		}
		return s
	}
	// Candidate roots are every normalized record repo plus the current
	// checkout. A run homes to the deepest root containing its spec file,
	// so a record launched from a subdirectory still groups with its
	// checkout no matter which repo is current.
	roots := map[string]bool{}
	for _, r := range runs {
		if nr := normalizeRepo(r.Repo); nr != "" {
			roots[nr] = true
		}
	}
	if current != "" {
		roots[current] = true
	}
	repoOf := func(r runRecord) string {
		best := ""
		for root := range roots {
			if specUnderRepo(r.Spec, root) && len(root) > len(best) {
				best = root
			}
		}
		if best != "" {
			return best
		}
		// No candidate root contains the spec: derive it from the standard
		// layout itself (<root>/.scratch/<dir>/spec.md), so a repo whose
		// runs were all recorded from subdirectories still groups. The
		// .scratch check keeps a stale spec path from homing to a phantom
		// root; those runs fall through to the recorded repo below.
		if root := specRepoRoot(r.Spec); root != "" {
			if fi, err := os.Stat(filepath.Join(root, ".scratch")); err == nil && fi.IsDir() {
				return normalizeRepo(root)
			}
		}
		if nr := normalizeRepo(r.Repo); nr != "" {
			return nr
		}
		return current
	}
	for _, r := range runs {
		s := ensure(repoOf(r), r.Slug)
		s.Spec, s.Branch = r.Spec, r.Branch
		s.HasRun = true
		s.Running = r.Pane != "" && live[r.Pane]
	}
	for repo := range byRepo {
		discoverSpecs(repo, byRepo[repo])
	}
	if current != "" {
		if byRepo[current] == nil {
			byRepo[current] = map[string]*SpecSnap{}
		}
		discoverSpecs(current, byRepo[current])
	}
	for _, specs := range byRepo {
		for slug, s := range specs {
			merged, total := specProgress(s, slug, mergedBySlug)
			s.Merged, s.Total = merged, total
		}
	}

	repos := make([]RepoSnap, 0, len(byRepo))
	for repo, specs := range byRepo {
		list := make([]SpecSnap, 0, len(specs))
		for _, s := range specs {
			list = append(list, *s)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Slug < list[j].Slug })
		repos = append(repos, RepoSnap{Path: repo, Specs: list})
	}
	sort.Slice(repos, func(i, j int) bool {
		if repos[i].Path == current {
			return true
		}
		if repos[j].Path == current {
			return false
		}
		return repos[i].Path < repos[j].Path
	})
	return Snap{Repos: repos}, nil
}

// normalizeRepo cleans a repo path for grouping: symlinks resolved
// best-effort (a record may point at a moved checkout), lexical clean
// always. Empty stays empty, so callers can tell "no repo" apart.
func normalizeRepo(p string) string {
	if p == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Clean(p)
}

// specRepoRoot derives the repo root from a spec path in the standard
// layout: <root>/.scratch/<dir>/spec.md. Anything else returns "", and
// callers check the root is real before trusting it.
func specRepoRoot(spec string) string {
	if filepath.Base(spec) != "spec.md" {
		return ""
	}
	scratch := filepath.Dir(filepath.Dir(spec))
	if filepath.Base(scratch) != ".scratch" {
		return ""
	}
	return filepath.Dir(scratch)
}

// specUnderRepo reports whether a spec file sits under a repo root, so a
// run recorded from a subdirectory still groups with its checkout.
func specUnderRepo(spec, repo string) bool {
	if spec == "" || repo == "" {
		return false
	}
	rel, err := filepath.Rel(repo, normalizeRepo(spec))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// specDirSlug keys a discovered spec directory the way the core's specSlug
// keys a run record, so the two meet on one key. Keep in sync with the
// core: lowercase, anything outside [a-z0-9-_.] becomes '-', trimmed.
func specDirSlug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// loadRunRecords reads runs.json keyed by spec slug. A missing file is no
// runs; a corrupt one reads as no runs too — the TUI shows waiting specs
// rather than failing the whole dashboard over one bad write.
func loadRunRecords(stateDir string) map[string]runRecord {
	raw, err := os.ReadFile(filepath.Join(stateDir, "runs.json"))
	if err != nil {
		return nil
	}
	var store map[string]runRecord
	if err := json.Unmarshal(raw, &store); err != nil {
		return nil
	}
	return store
}

// loadMergedBySlug collects, per run slug, the basenames of tickets whose
// history outcome is merged. Ticket paths are matched by basename because the
// record may hold an absolute path, a repo-relative one, or just the file.
func loadMergedBySlug(stateDir string, runs map[string]runRecord) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for slug := range runs {
		raw, err := os.ReadFile(filepath.Join(stateDir, "history", slug+".json"))
		if err != nil {
			continue
		}
		var entries []historyEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			continue
		}
		for _, e := range entries {
			if e.Outcome != "merged" || e.Ticket == "" {
				continue
			}
			if out[slug] == nil {
				out[slug] = map[string]bool{}
			}
			out[slug][filepath.Base(e.Ticket)] = true
		}
	}
	return out
}

// discoverSpecs adds every spec directory on disk under repo to specs, keyed
// by slug. A spec directory is a directory holding spec.md directly under the
// repo's .scratch tree. Entries a run record already named keep the record's
// spec path and branch; only the missing pieces fill in.
func discoverSpecs(repo string, specs map[string]*SpecSnap) {
	root := filepath.Join(repo, ".scratch")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		spec := filepath.Join(root, e.Name(), "spec.md")
		if fi, err := os.Stat(spec); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		slug := specDirSlug(e.Name())
		s := specs[slug]
		if s == nil {
			s = &SpecSnap{Slug: slug}
			specs[slug] = s
		}
		if s.Spec == "" {
			s.Spec = spec
		}
		if s.Branch == "" {
			s.Branch = "spec/" + slug
		}
	}
}

// specProgress counts a spec's ticket files and how many are merged: on-disk
// files whose status reads done, or whose basename has a merged history
// entry. Counting only on-disk files keeps merged <= total even when a file
// was deleted after its merge.
func specProgress(s *SpecSnap, slug string, mergedBySlug map[string]map[string]bool) (merged, total int) {
	files := ticketFiles(s)
	mergedSet := mergedBySlug[slug]
	for _, f := range files {
		total++
		base := filepath.Base(f)
		if mergedSet[base] {
			merged++
			continue
		}
		if raw, err := os.ReadFile(f); err == nil && ticketDone(string(raw)) {
			merged++
		}
	}
	return merged, total
}

// ticketFiles lists a spec's ticket Markdown: the .md files in its issues
// directory when there is one, else the .md files beside the spec file other
// than the spec file itself.
func ticketFiles(s *SpecSnap) []string {
	if s.Spec == "" {
		return nil
	}
	dir := filepath.Dir(s.Spec)
	if fi, err := os.Stat(filepath.Join(dir, "issues")); err == nil && fi.IsDir() {
		dir = filepath.Join(dir, "issues")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if p == s.Spec {
			continue
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// ticketDone reports whether a ticket file's status header reads done. Only
// the header field counts, the way finish completes it; a "Status:" mentioned
// in the body is still skipped because only a line that starts with the field
// counts.
func ticketDone(raw string) bool {
	done := false
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Status:") && !strings.HasPrefix(trimmed, "**Status:") {
			continue
		}
		i := strings.Index(line, "Status:")
		rest := strings.Trim(strings.TrimLeft(line[i+len("Status:"):], "* \t"), " \t")
		done = strings.EqualFold(rest, "done")
	}
	return done
}
