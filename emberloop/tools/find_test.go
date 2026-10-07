package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/emberloop/tools/systemone"
)

type fixedJudge struct{ score float64 }

func (j fixedJudge) Evaluate(_ context.Context, req systemone.Request) (systemone.Response, error) {
	answers := map[string]systemone.Answer{}
	for id := range req.Questions {
		answers[id] = systemone.Answer{Type: systemone.TypeNoul, Noul: &j.score}
	}
	return systemone.Response{Answers: answers}, nil
}

func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"auth.go":         "package auth\n\nfunc VerifyToken() bool {\n\treturn true\n}\n",
		"other.go":        "package other\n\nfunc Unrelated() {}\n",
		"go.sum":          "VerifyToken noise\n",
		"vendor/lib/x.go": "package lib\n\nfunc VerifyToken() {}\n",
	}
	for name, body := range files {
		full := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(body), 0o644)
	}
	return dir
}

func findArgs() map[string]any {
	return map[string]any{"query": "how are tokens verified", "grep_keywords": []string{"VerifyToken"}}
}

func TestFindLexicalFallbackLocatesTheFile(t *testing.T) {
	dir := project(t)
	t.Setenv("EMBERLOOP_SETTINGS", filepath.Join(t.TempDir(), "none.json"))
	out, err := run(t, findTool{lexicalOnly: true}, dir, findArgs())
	if err != nil || !strings.Contains(out, "auth.go") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	for _, noise := range []string{"go.sum", "vendor", "other.go"} {
		if strings.Contains(out, noise) {
			t.Errorf("%s leaked into %q", noise, out)
		}
	}
}

func TestFindUsesTheDecisionModelScores(t *testing.T) {
	dir := project(t)
	t.Setenv("EMBERLOOP_SETTINGS", filepath.Join(t.TempDir(), "none.json"))
	out, err := run(t, findTool{provider: fixedJudge{0.85}}, dir, findArgs())
	if err != nil || !strings.Contains(out, "auth.go") || !strings.Contains(out, "0.85") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
}

func TestFindRefusesAPathOutsideTheWorkspace(t *testing.T) {
	a := findArgs()
	a["path"] = "../.."
	if _, err := run(t, findTool{lexicalOnly: true}, project(t), a); err == nil {
		t.Fatal("searched outside the workspace")
	}
}

func TestMergePassageRangesJoinsOverlaps(t *testing.T) {
	got := MergePassageRanges([]PassageRange{{StartLine: 1, EndLine: 10, Score: 0.5}, {StartLine: 8, EndLine: 20, Score: 0.9}, {StartLine: 40, EndLine: 50, Score: 0.3}})
	if len(got) != 2 || got[0].StartLine != 1 || got[0].EndLine != 20 {
		t.Fatalf("got %+v", got)
	}
}

func TestSettingsRoundTripAndEnvWins(t *testing.T) {
	t.Setenv("EMBERLOOP_SETTINGS", filepath.Join(t.TempDir(), "s.json"))
	if err := SaveSettings(Settings{DecisionModel: "typesafe/jev-1.14", Threshold: 0.3}); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(); s.DecisionModel != "typesafe/jev-1.14" || s.Threshold != 0.3 {
		t.Fatalf("got %+v", s)
	}
	t.Setenv("EMBERLOOP_DECISION_MODEL", "respan/span-01")
	if s := LoadSettings(); s.DecisionModel != "respan/span-01" {
		t.Fatalf("env did not win: %+v", s)
	}
}
