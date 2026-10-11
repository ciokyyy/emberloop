package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// Slice 1: a directory with no spec returns an empty list.
func TestListSpecsEmptyDir(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/specs?repo=" + url.QueryEscape(tempDir(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Specs []struct {
			Spec    string `json:"spec"`
			Title   string `json:"title"`
			Tickets int    `json:"tickets"`
		} `json:"specs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Specs == nil {
		t.Fatalf("specs = nil, want empty list")
	}
	if len(out.Specs) != 0 {
		t.Fatalf("specs = %+v, want empty", out.Specs)
	}
}

// Slice 2: the list returns title and ticket count for each spec found.
func TestListSpecsFindsSpecWithTitleAndCount(t *testing.T) {
	repo := tempDir(t)
	writeFile(t, repo, ".scratch/demo/spec.md", "# Demo spec\n")
	writeFile(t, repo, ".scratch/demo/issues/01-first.md", "# 01: First\n\n**Blocked by:** None\n\n- [ ] a box\n")
	writeFile(t, repo, ".scratch/demo/issues/02-second.md", "# 02: Second\n\n**Blocked by:** 01\n\n- [ ] a box\n")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/specs?repo=" + url.QueryEscape(repo))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Specs []struct {
			Spec    string `json:"spec"`
			Title   string `json:"title"`
			Tickets int    `json:"tickets"`
		} `json:"specs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Specs) != 1 {
		t.Fatalf("specs = %+v, want one", out.Specs)
	}
	got := out.Specs[0]
	if got.Title != "Demo spec" {
		t.Fatalf("title = %q, want %q", got.Title, "Demo spec")
	}
	if got.Tickets != 2 {
		t.Fatalf("tickets = %d, want 2", got.Tickets)
	}
}

// Specs under a directory named archive are never listed.
func TestListSpecsSkipsArchive(t *testing.T) {
	repo := tempDir(t)
	writeFile(t, repo, ".scratch/live/spec.md", "# Live spec\n")
	writeFile(t, repo, ".scratch/archive/old/spec.md", "# Old spec\n")
	got, err := discoverSpecs(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Live spec" {
		t.Fatalf("specs = %+v, want only the live spec", got)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := root + "/" + name
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Slice 3: the preview returns number, title, blockers and checkbox progress.
func TestSpecPreviewBlockersAndProgress(t *testing.T) {
	repo := tempDir(t)
	writeFile(t, repo, ".scratch/demo/spec.md", "# Demo spec\n")
	writeFile(t, repo, ".scratch/demo/issues/01-first.md", "# 01: First\n\n**Blocked by:** None\n\n- [x] done box\n- [ ] todo box\n")
	writeFile(t, repo, ".scratch/demo/issues/02-second.md", "# 02: Second\n\n**Blocked by:** 01\n\n- [ ] a box\n")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	spec := filepath.Join(repo, ".scratch", "demo", "spec.md")
	resp, err := http.Get(srv.URL + "/api/specs/preview?spec=" + url.QueryEscape(spec))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Spec    string `json:"spec"`
		Title   string `json:"title"`
		Tickets []struct {
			ID       string   `json:"id"`
			Num      int      `json:"num"`
			Title    string   `json:"title"`
			Blockers []string `json:"blockers"`
			Done     int      `json:"done"`
			Total    int      `json:"total"`
			Error    string   `json:"error"`
		} `json:"tickets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tickets) != 2 {
		t.Fatalf("tickets = %+v, want two", out.Tickets)
	}
	first, second := out.Tickets[0], out.Tickets[1]
	if first.ID != "01-first" || first.Num != 1 || first.Title != "First" {
		t.Fatalf("first = %+v, want 01-first/1/First", first)
	}
	if len(first.Blockers) != 0 || first.Done != 1 || first.Total != 2 {
		t.Fatalf("first = %+v, want no blockers and 1/2 boxes", first)
	}
	if second.ID != "02-second" || len(second.Blockers) != 1 || second.Blockers[0] != "01-first" {
		t.Fatalf("second = %+v, want blocker 01-first", second)
	}
}

// Slice 4: an unparsable Blocked by line surfaces as an error on that
// ticket and blocks nothing else in the list.
func TestSpecPreviewBadBlockedByIsRowError(t *testing.T) {
	repo := tempDir(t)
	writeFile(t, repo, ".scratch/demo/spec.md", "# Demo spec\n")
	writeFile(t, repo, ".scratch/demo/issues/01-first.md", "# 01: First\n\n**Blocked by:** None\n\n- [ ] a box\n")
	writeFile(t, repo, ".scratch/demo/issues/02-bad.md", "# 02: Bad\n\n**Blocked by** none\n\n- [ ] a box\n")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	spec := filepath.Join(repo, ".scratch", "demo", "spec.md")
	resp, err := http.Get(srv.URL + "/api/specs/preview?spec=" + url.QueryEscape(spec))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Tickets []struct {
			ID    string `json:"id"`
			Error string `json:"error"`
		} `json:"tickets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tickets) != 2 {
		t.Fatalf("tickets = %+v, want two", out.Tickets)
	}
	if out.Tickets[0].Error != "" {
		t.Fatalf("first error = %q, want none", out.Tickets[0].Error)
	}
	if out.Tickets[1].ID != "02-bad" || out.Tickets[1].Error == "" {
		t.Fatalf("bad ticket = %+v, want a clear error", out.Tickets[1])
	}
}
