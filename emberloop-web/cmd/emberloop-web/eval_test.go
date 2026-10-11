package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/emberloop-web/memory"
	"example.com/emberloop-web/testsupport"
)

func writeEvalSession(t *testing.T, stateDir, id, project, createdAt, body string) {
	t.Helper()
	dir := filepath.Join(stateDir, "enderbloom", "sessions", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := `{"id":"` + id + `","projectId":"` + project + `","createdAt":"` + createdAt + `","status":"ended","harness":"fake","model":"m","workDir":"/tmp","owner":"u1"}`
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEvalReplayPrintsRecallAndRepeatRate(t *testing.T) {
	dir := testsupport.TempDir(t)
	writeEvalSession(t, dir, "s1", "", "2026-10-11T09:00:00Z",
		"{\"kind\":\"prompt\",\"sender\":\"user\",\"user\":\"u1\",\"seq\":1,\"text\":\"remember: always return wrapped errors\"}\n")
	writeEvalSession(t, dir, "s2", "", "2026-10-11T10:00:00Z",
		"{\"kind\":\"prompt\",\"sender\":\"user\",\"user\":\"u1\",\"seq\":1,\"text\":\"remember: always return wrapped errors in handlers\"}\n")
	var out, errOut bytes.Buffer
	if code := runEvalCommand([]string{"replay", "-state-dir", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errOut.String())
	}
	got := out.String()
	for _, want := range []string{"recall@5:", "repeat-correction rate:", "cases: 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
	}
}

func TestEvalReplayWithEmbedderScoresPrintsGap(t *testing.T) {
	dir := testsupport.TempDir(t)
	writeEvalSession(t, dir, "s1", "", "2026-10-11T09:00:00Z",
		"{\"kind\":\"prompt\",\"sender\":\"user\",\"user\":\"u1\",\"seq\":1,\"text\":\"remember: always return wrapped errors\"}\n")
	writeEvalSession(t, dir, "s2", "", "2026-10-11T10:00:00Z",
		"{\"kind\":\"prompt\",\"sender\":\"user\",\"user\":\"u1\",\"seq\":1,\"text\":\"remember: always return wrapped errors in handlers\"}\n")
	scores := `{"s2:1":{"remember: always return wrapped errors":1.0}}`
	scoresPath := filepath.Join(dir, "scores.json")
	if err := os.WriteFile(scoresPath, []byte(scores), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runEvalCommand([]string{"replay", "-state-dir", dir, "-embedder-scores", scoresPath}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errOut.String())
	}
	got := out.String()
	for _, want := range []string{"fts recall@5:", "embedder recall@5:", "gap (embedder-fts):"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
	}
}

func TestEvalDecidersPrintsPrecisionPerDecider(t *testing.T) {
	dir := testsupport.TempDir(t)
	labels := "{\"id\":\"a\",\"text\":\"remember: always wrap errors\",\"durable\":true}\n" +
		"{\"id\":\"b\",\"text\":\"What time is lunch?\",\"durable\":false}\n"
	labelsPath := filepath.Join(dir, "labels.jsonl")
	if err := os.WriteFile(labelsPath, []byte(labels), 0o644); err != nil {
		t.Fatal(err)
	}
	scoresPath := filepath.Join(dir, "scores.json")
	if err := os.WriteFile(scoresPath, []byte(`{"llm":{"a":0.9,"b":0.1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runEvalCommand([]string{"deciders", "-labels", labelsPath, "-scores", scoresPath}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errOut.String())
	}
	got := out.String()
	for _, want := range []string{"heuristic:", "llm:", "jev:", "laya:", "precision"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
	}
}

func TestEvalCommandUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"nope"}, {"replay", "extra"}, {"deciders"}, {"sample"}, {"sample", "-n", "0", "-labels-out", "x"}} {
		var out, errOut bytes.Buffer
		if code := runEvalCommand(args, &out, &errOut); code != 2 {
			t.Errorf("args %q exit = %d, want 2", args, code)
		}
	}
}

func TestEvalReplayPeriodBoundsSessions(t *testing.T) {
	dir := testsupport.TempDir(t)
	writeEvalSession(t, dir, "s1", "", "2026-10-11T09:00:00Z",
		"{\"kind\":\"prompt\",\"sender\":\"user\",\"user\":\"u1\",\"seq\":1,\"text\":\"remember: always return wrapped errors\"}\n")
	writeEvalSession(t, dir, "s2", "", "2026-10-11T10:00:00Z",
		"{\"kind\":\"prompt\",\"sender\":\"user\",\"user\":\"u1\",\"seq\":1,\"text\":\"remember: always return wrapped errors in handlers\"}\n")
	var out, errOut bytes.Buffer
	if code := runEvalCommand([]string{"replay", "-state-dir", dir, "-since", "2026-10-11T09:30:00Z"}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "sessions: 1") || !strings.Contains(got, "cases: 1") {
		t.Fatalf("stdout = %q, want the period to keep only s2", got)
	}
	out.Reset()
	if code := runEvalCommand([]string{"replay", "-state-dir", dir, "-since", "nope"}, &out, &errOut); code != 2 {
		t.Fatalf("bad since exit = %d, want 2", code)
	}
}

func TestEvalDecidersMergesQueueLabels(t *testing.T) {
	dir := testsupport.TempDir(t)
	labelsPath := filepath.Join(dir, "labels.jsonl")
	if err := os.WriteFile(labelsPath, []byte("{\"id\":\"hand-1\",\"text\":\"remember: always wrap errors\",\"durable\":true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eng, err := memory.Open(filepath.Join(dir, "enderbloom", "memory"), memory.Options{})
	if err != nil {
		t.Fatal(err)
	}
	bank, err := eng.Bank("u1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	approved, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: `{"text":"never wrap errors"}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := bank.Decide(ctx, approved, memory.QueueRejected, "human"); err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runEvalCommand([]string{"deciders", "-labels", labelsPath, "-state-dir", dir, "-queue-user", "u1"}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "labels: 2 (hand 1, queue 1)") {
		t.Fatalf("stdout = %q, want the merged label mix", got)
	}
	errOut.Reset()
	if code := runEvalCommand([]string{"deciders", "-labels", labelsPath, "-state-dir", dir, "-queue-user", "nobody"}, &out, &errOut); code != 1 {
		t.Fatalf("missing bank exit = %d, want 1", code)
	}
}

func TestEvalDecidersWriteScoresGrowsTheFile(t *testing.T) {
	dir := testsupport.TempDir(t)
	labelsPath := filepath.Join(dir, "labels.jsonl")
	if err := os.WriteFile(labelsPath, []byte("{\"id\":\"a\",\"text\":\"remember: always wrap errors\",\"durable\":true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scoresPath := filepath.Join(dir, "scores.json")
	var out, errOut bytes.Buffer
	if code := runEvalCommand([]string{"deciders", "-labels", labelsPath, "-write-scores", scoresPath}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errOut.String())
	}
	raw, err := os.ReadFile(scoresPath)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]map[string]float64
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["heuristic"]["a"] <= 0 {
		t.Fatalf("scores = %s, want the live heuristic baseline", raw)
	}
}

func TestEvalSampleWritesHitsFirst(t *testing.T) {
	dir := testsupport.TempDir(t)
	writeEvalSession(t, dir, "s1", "", "2026-10-11T09:00:00Z",
		"{\"kind\":\"prompt\",\"sender\":\"user\",\"user\":\"u1\",\"seq\":1,\"text\":\"remember: always return wrapped errors\"}\n"+
			"{\"kind\":\"prompt\",\"sender\":\"user\",\"user\":\"u1\",\"seq\":2,\"text\":\"What time is lunch?\"}\n")
	labelsOut := filepath.Join(dir, "sample.jsonl")
	var out, errOut bytes.Buffer
	if code := runEvalCommand([]string{"sample", "-state-dir", dir, "-n", "2", "-labels-out", labelsOut}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errOut.String())
	}
	raw, err := os.ReadFile(labelsOut)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("rows = %d, want 2", len(lines))
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	// Hits oversample: the hit comes first with its hint, durable unset for
	// the human.
	if first["id"] != "s1:1" || first["durable"] != false || first["heuristic"].(float64) <= 0 {
		t.Fatalf("first = %s, want the hit with a hint and durable false", lines[0])
	}
	if got := out.String(); !strings.Contains(got, "set durable by hand") {
		t.Fatalf("stdout = %q, want the hand-labelling reminder", got)
	}
}
