package wick

import (
	"strings"
	"testing"
)

// Seam (pre-agreed by the ticket): spawn → review → finish through the
// existing fake herdr, asserting the records on disk. reviewRepo records the
// worker, reviewSpawn runs both reviewers through the fake, and Finish merges
// or discards through it — the same public boundary the review tests use.
func TestFinishWritesHistoryThatSurvivesWorkerDeletion(t *testing.T) {
	repo, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	tip := gitOut(t, tree, "rev-parse", "HEAD")
	// The worker ran on a harness with a model and effort: history must carry
	// all three, the way stats will group by them.
	store, err := load()
	if err != nil {
		t.Fatal(err)
	}
	w := store["07-x"]
	w.Harness, w.Model, w.Effort = "pig", "m1", "high"
	store["07-x"] = w
	if err := save(store); err != nil {
		t.Fatal(err)
	}

	res, err := Finish(f.run, "07-x", true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !res.Merged {
		t.Fatalf("merged = false, detail = %q", res.Detail)
	}
	// The worker entry is gone with the worktree.
	if _, err := recall("07-x"); err == nil {
		t.Fatal("the merged worker is still on record")
	}

	// The run lists by its slug, the key FinishRun takes.
	got, err := ListTicketHistory("t")
	if err != nil {
		t.Fatalf("ListTicketHistory: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("history = %+v, want one entry", got)
	}
	h := got[0]
	if h.Ticket != "/t/07-x.md" || h.Spec != "/t/spec.md" {
		t.Errorf("history must name the ticket and spec: %+v", h)
	}
	if h.Outcome != "merged" || h.SHA != res.SHA || h.SHA == "" {
		t.Errorf("merged history must carry the merge commit: %+v vs %+v", h, res)
	}
	if h.Branch != "implement/07-x" {
		t.Errorf("history must carry the branch: %+v", h)
	}
	if h.Harness != "pig" || h.Model != "m1" || h.Effort != "high" {
		t.Errorf("history must carry harness/model/effort: %+v", h)
	}
	if h.Started != "2026-10-05T00:00:00Z" {
		t.Errorf("history must carry the worker's start time: %+v", h)
	}
	if h.Started == "" || h.Ended == "" {
		t.Errorf("history must carry start and end times: %+v", h)
	}
	if h.Worker != "07-x" {
		t.Errorf("history must carry the worker agent id: %+v", h)
	}
	if len(h.Reviews) != 2 {
		t.Fatalf("history must carry both reviewer verdicts: %+v", h)
	}
	for _, r := range h.Reviews {
		if r.Verdict != verdictPass || r.VerdictAt != tip {
			t.Errorf("%s verdict = %q at %q, want pass at %q", r.Type, r.Verdict, r.VerdictAt, tip)
		}
		if r.Findings == "" {
			t.Errorf("%s verdict must carry its findings: %+v", r.Type, r)
		}
		if r.AgentID == "" {
			t.Errorf("%s verdict must carry its reviewer agent id: %+v", r.Type, r)
		}
	}
	if gitOut(t, repo, "rev-parse", "main") != h.SHA {
		t.Error("the merge commit must be what main now points at")
	}
}

// A discarded ticket leaves history too, with no merge commit.
func TestDiscardWritesHistory(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("changes", "changes"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	res, err := Finish(f.run, "07-x", false)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if res.Merged || res.Kept {
		t.Fatalf("discard: %+v", res)
	}
	got, err := ListTicketHistory("t")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Outcome != "discarded" || got[0].SHA != "" {
		t.Fatalf("discard history = %+v, want one discarded entry with no sha", got)
	}
	if len(got[0].Reviews) != 2 {
		t.Errorf("discard history must still carry the verdicts: %+v", got[0])
	}
}

// A refused merge keeps the worker, but still leaves a kept entry with the
// verdicts as they stand — captured before any reviewer is forgotten (here
// none are: the worker stays, and so do they).
func TestKeptFinishWritesHistoryAndKeepsTheWorker(t *testing.T) {
	_, tree := reviewRepo(t)
	_ = tree
	res, err := Finish(osExec, "07-x", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged || !res.Kept {
		t.Fatalf("an unreviewed worker must be kept: %+v", res)
	}
	if _, err := recall("07-x"); err != nil {
		t.Fatalf("a kept worker must stay on record: %v", err)
	}
	got, err := ListTicketHistory("t")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Outcome != "kept" {
		t.Fatalf("kept history = %+v, want one kept entry", got)
	}
	if !strings.Contains(got[0].Detail, "no review yet") {
		t.Errorf("kept history must carry why it was kept: %+v", got[0])
	}
}

// A ticket refused and then merged leaves both entries in order: the kept
// refusal first, the merge after. The two finishes land in the same second,
// so this also pins the stable oldest-first order.
func TestKeptThenMergedAppendsInOrder(t *testing.T) {
	_, tree := reviewRepo(t)
	kept, err := Finish(osExec, "07-x", true)
	if err != nil {
		t.Fatal(err)
	}
	if kept.Merged || !kept.Kept {
		t.Fatalf("an unreviewed worker must be kept: %+v", kept)
	}
	f := newFakeReviewers(answers("pass", "pass"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	merged, err := Finish(f.run, "07-x", true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !merged.Merged {
		t.Fatalf("merged = false, detail = %q", merged.Detail)
	}
	got, err := ListTicketHistory("t")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Outcome != "kept" || got[1].Outcome != "merged" {
		t.Fatalf("history = %+v, want kept then merged", got)
	}
	if got[1].SHA != merged.SHA {
		t.Errorf("merged entry sha = %q, want %q", got[1].SHA, merged.SHA)
	}
	if len(got[1].Reviews) != 2 {
		t.Errorf("the merged entry must carry both verdicts: %+v", got[1])
	}
}
