package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Slice 1: a directory with no spec returns an empty list.
func TestListSpecsEmptyDir(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// Local spec-*.md plans under docs/plans are listed with their title, and
// sibling specs never count as tickets of each other.
func TestListSpecsFindsLocalPlansSpecs(t *testing.T) {
	t.Parallel()
	repo := tempDir(t)
	writeFile(t, repo, "docs/plans/spec-05-demo.md", "# Spec 05: Demo\n")
	writeFile(t, repo, "docs/plans/spec-06-other.md", "# Spec 06: Other\n")
	got, err := discoverSpecs(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("specs = %+v, want two local plans", got)
	}
	if got[0].Title != "Spec 05: Demo" || got[1].Title != "Spec 06: Other" {
		t.Fatalf("specs = %+v, want both plans with titles", got)
	}
	for _, e := range got {
		if e.Tickets != 0 {
			t.Errorf("%s tickets = %d, want 0: sibling specs are not tickets", e.Spec, e.Tickets)
		}
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
	t.Parallel()
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
	t.Parallel()
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
func TestRunListReturnsSummariesAndDetailReturnsTicketBodies(t *testing.T) {
	t.Parallel()
	const acceptance = "acceptance evidence belongs to the detail"
	ticketFiles := map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n- [ ] " + acceptance + "\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** 01-first\n\n- [ ] second acceptance\n",
	}
	repo, spec := initRunRepo(t, ticketFiles)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.projects = []Project{{ID: "project-1", Name: "Demo", Path: repo}}
	now := time.Date(2026, time.October, 14, 10, 0, 0, 0, time.UTC)
	run := &runState{
		ID: "run-summary", Spec: spec, Repo: repo, Status: runStatusNeedsYou,
		Owner: "owner-1", CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
		SpecBranch: "main",
		Tickets: []staticTicket{
			{ID: "01-first", Num: 1, Title: "First", Path: filepath.Join(repo, ".scratch", "demo", "issues", "01-first.md")},
			{ID: "02-second", Num: 2, Title: "Second", Path: filepath.Join(repo, ".scratch", "demo", "issues", "02-second.md")},
		},
		Rows: map[string]savedRow{
			"01-first":  {State: "running"},
			"02-second": {State: runStatusNeedsYou},
		},
		Merged:         map[string]string{"01-first": "merged-sha"},
		NeedsYouReason: map[string]string{"02-second": "waiting for input"},
	}
	d.runs = map[string]*runState{run.ID: run}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/api/runs")
	if err != nil {
		t.Fatal(err)
	}
	rawList, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", resp.StatusCode, rawList)
	}
	if strings.Contains(string(rawList), `"tickets"`) || strings.Contains(string(rawList), acceptance) {
		t.Fatalf("Run list contains Ticket detail: %s", rawList)
	}
	var list struct {
		Runs []runSummary `json:"runs"`
	}
	if err := json.Unmarshal(rawList, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Runs) != 1 {
		t.Fatalf("runs = %+v, want one summary", list.Runs)
	}
	summary := list.Runs[0]
	if summary.ID != run.ID || summary.Spec != spec || summary.Status != runStatusNeedsYou || summary.Repo != repo || summary.ProjectID != "project-1" || summary.Owner != "owner-1" {
		t.Fatalf("summary = %+v, want identity, status, repo, Project and owner", summary)
	}
	if summary.Counts != (runSummaryCounts{Total: 2, Merged: 1, NeedsYou: 1}) {
		t.Fatalf("counts = %+v, want 2 total, 1 merged, 1 needs-you", summary.Counts)
	}
	if !summary.UpdatedAt.Equal(now) {
		t.Fatalf("updatedAt = %s, want %s", summary.UpdatedAt, now)
	}

	resp, err = srv.Client().Get(srv.URL + "/api/runs/" + run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("detail status = %d, want 200: %s", resp.StatusCode, raw)
	}
	var detail struct {
		Run struct {
			Tickets []struct {
				ID    string `json:"id"`
				Body  string `json:"body"`
				Boxes []struct {
					Text    string `json:"text"`
					Checked bool   `json:"checked"`
				} `json:"boxes"`
			} `json:"tickets"`
		} `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Run.Tickets) != 2 || !strings.Contains(detail.Run.Tickets[0].Body, acceptance) {
		t.Fatalf("detail tickets = %+v, want the Ticket body and acceptance text", detail.Run.Tickets)
	}
	if len(detail.Run.Tickets[0].Boxes) != 1 || detail.Run.Tickets[0].Boxes[0].Text != acceptance {
		t.Fatalf("detail boxes = %+v, want parsed acceptance text", detail.Run.Tickets[0].Boxes)
	}
}

func TestRunListIsBoundedWithManyRuns(t *testing.T) {
	const runCount, ticketCount = 40, 8
	ticketFiles := make(map[string]string, ticketCount)
	for i := 1; i <= ticketCount; i++ {
		name := fmt.Sprintf("%02d-item.md", i)
		ticketFiles[name] = fmt.Sprintf("# %02d: Item %d\n\n**Blocked by:** None\n\n- [ ] acceptance item %d\n", i, i, i)
	}
	repo, spec := initRunRepo(t, ticketFiles)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.projects = []Project{{ID: "project-1", Name: "Demo", Path: repo}}
	d.runs = make(map[string]*runState, runCount)
	now := time.Now().UTC()
	for i := 0; i < runCount; i++ {
		id := fmt.Sprintf("run-%03d", i)
		run := &runState{
			ID: id, Spec: spec, Repo: repo, Status: runStatusDone, Owner: "owner-1",
			CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(time.Duration(i) * time.Second), SpecBranch: "main",
		}
		for j := 1; j <= ticketCount; j++ {
			name := fmt.Sprintf("%02d-item.md", j)
			run.Tickets = append(run.Tickets, staticTicket{
				ID: fmt.Sprintf("%02d-item", j), Num: j, Title: fmt.Sprintf("Item %d", j),
				Path: filepath.Join(repo, ".scratch", "demo", "issues", name),
			})
		}
		d.runs[id] = run
	}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	var samples []time.Duration
	for i := 0; i < 7; i++ {
		start := time.Now()
		resp, err := client.Get(srv.URL + "/api/runs")
		if err != nil {
			t.Fatalf("GET /api/runs failed within 2s: %v", err)
		}
		var out struct {
			Runs []runSummary `json:"runs"`
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("list status = %d, want 200", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		elapsed := time.Since(start)
		if elapsed > 750*time.Millisecond {
			t.Fatalf("GET /api/runs for %d Runs × %d Tickets took %s, want under 750ms", runCount, ticketCount, elapsed)
		}
		if len(out.Runs) != runCount {
			t.Fatalf("runs = %d, want %d", len(out.Runs), runCount)
		}
		for idx, summary := range out.Runs {
			if want := fmt.Sprintf("run-%03d", idx); summary.ID != want {
				t.Fatalf("runs[%d].id = %q, want %q", idx, summary.ID, want)
			}
		}
		if i >= 2 {
			samples = append(samples, elapsed)
		}
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	t.Logf("GET /api/runs: runs=%d tickets=%d p50=%s p95=%s", runCount, runCount*ticketCount, samples[len(samples)/2], samples[len(samples)-1])
}
