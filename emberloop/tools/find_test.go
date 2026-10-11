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
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "none.json"))
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
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "none.json"))
	out, err := run(t, findTool{provider: fixedJudge{0.85}}, dir, findArgs())
	if err != nil || !strings.Contains(out, "auth.go") || !strings.Contains(out, "0.85") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
}
func TestFindHitsCarrySourceTags(t *testing.T) {
	dir := project(t)
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "none.json"))
	graph := func(_ context.Context, _ string, _ []string, _ string) ([]SearchHit, error) {
		return []SearchHit{{
			Path:   "graph.go",
			Score:  0.8,
			Source: SourceGraph,
			Symbol: "VerifyToken",
			Ranges: []PassageRange{{StartLine: 1, EndLine: 5, Score: 0.8, Snippet: "func VerifyToken()"}},
		}}, nil
	}
	out, err := run(t, findTool{lexicalOnly: true, graph: graph}, dir, findArgs())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[grep]", "[graph]"} {
		if !strings.Contains(out, want) {
			t.Errorf("out misses source tag %q: %q", want, out)
		}
	}
	out, err = run(t, findTool{provider: fixedJudge{0.85}, graph: graph}, dir, findArgs())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[jev]", "[graph]"} {
		if !strings.Contains(out, want) {
			t.Errorf("out misses source tag %q: %q", want, out)
		}
	}
}

func TestFindGraphExactSymbolRanksFirst(t *testing.T) {
	dir := project(t)
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "none.json"))
	graph := func(_ context.Context, _ string, _ []string, _ string) ([]SearchHit, error) {
		return []SearchHit{
			{
				Path:   "unrelated.go",
				Score:  0.9,
				Source: SourceGraph,
				Symbol: "SomethingElse",
				Ranges: []PassageRange{{StartLine: 1, EndLine: 3, Score: 0.9, Snippet: "func SomethingElse()"}},
			},
			{
				Path:   "exact.go",
				Score:  0.3,
				Source: SourceGraph,
				Symbol: "VerifyToken",
				Ranges: []PassageRange{{StartLine: 10, EndLine: 14, Score: 0.3, Snippet: "func VerifyToken()"}},
			},
		}, nil
	}
	out, err := run(t, findTool{lexicalOnly: true, graph: graph}, dir, findArgs())
	if err != nil {
		t.Fatal(err)
	}
	exact := strings.Index(out, "exact.go")
	other := strings.Index(out, "unrelated.go")
	if exact < 0 || other < 0 {
		t.Fatalf("both graph hits must appear: %q", out)
	}
	if exact > other {
		t.Errorf("exact symbol hit sorts after a higher-scored graph hit:\n%s", out)
	}
}

func TestFindGraphFailureLeavesBehaviorUnchanged(t *testing.T) {
	dir := project(t)
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "none.json"))
	plain, err := run(t, findTool{lexicalOnly: true}, dir, findArgs())
	if err != nil {
		t.Fatal(err)
	}
	failing := GraphSource(func(_ context.Context, _ string, _ []string, _ string) ([]SearchHit, error) {
		return nil, context.DeadlineExceeded
	})
	got, err := run(t, findTool{lexicalOnly: true, graph: failing}, dir, findArgs())
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Errorf("failing graph changed output:\nplain %q\ngot   %q", plain, got)
	}
	empty := GraphSource(func(_ context.Context, _ string, _ []string, _ string) ([]SearchHit, error) {
		return nil, nil
	})
	got, err = run(t, findTool{lexicalOnly: true, graph: empty}, dir, findArgs())
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Errorf("empty graph changed output:\nplain %q\ngot   %q", plain, got)
	}
}

func TestFindGraphAnswersWhenCascadeHasNoCandidates(t *testing.T) {
	dir := project(t)
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "none.json"))
	args := map[string]any{"query": "zzzqqq nomatch terms here", "grep_keywords": []string{"ZzzNoSuchSymbol"}}
	plain, err := run(t, findTool{lexicalOnly: true}, dir, args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "[graph]") {
		t.Fatalf("plain search must not carry graph hits: %q", plain)
	}
	graph := func(_ context.Context, _ string, _ []string, _ string) ([]SearchHit, error) {
		return []SearchHit{{
			Path:   "graph-only.go",
			Score:  0.8,
			Source: SourceGraph,
			Symbol: "ZzzNoSuchSymbol",
			Ranges: []PassageRange{{StartLine: 1, EndLine: 2, Score: 0.8, Snippet: "func ZzzNoSuchSymbol()"}},
		}}, nil
	}
	out, err := run(t, findTool{lexicalOnly: true, graph: graph}, dir, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "graph-only.go") || !strings.Contains(out, "[graph]") {
		t.Errorf("graph hit missing when cascade has none: %q", out)
	}
}

func TestParseGraphResultsJSONAndText(t *testing.T) {
	dir := project(t)
	body := `{"results":[{"file":"auth.go","symbol":"VerifyToken","line":3,"snippet":"func VerifyToken()","score":0.9}]}`
	hits := parseGraphResults(body, dir, dir, []string{"VerifyToken"}, "tokens")
	if len(hits) != 1 || hits[0].Source != SourceGraph || hits[0].Path != "auth.go" || hits[0].Symbol != "VerifyToken" {
		t.Fatalf("json parse = %+v", hits)
	}
	hits = parseGraphResults("see auth.go:12 and other.go", dir, dir, nil, "x")
	found := map[string]bool{}
	for _, h := range hits {
		found[h.Path] = true
		if h.Source != SourceGraph {
			t.Errorf("text hit source = %q, want graph", h.Source)
		}
	}
	if !found["auth.go"] {
		t.Errorf("text parse missed auth.go: %+v", hits)
	}
	if got := parseGraphResults("nothing here", dir, dir, nil, "x"); len(got) != 0 {
		t.Errorf("garbage text parsed as %d hits", len(got))
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
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "s.json"))
	if err := SaveSettings(Settings{DecisionModel: "typesafe/jev-1.14", Threshold: 0.3}); err != nil {
		t.Fatal(err)
	}
	if s := LoadSettings(); s.DecisionModel != "typesafe/jev-1.14" || s.Threshold != 0.3 {
		t.Fatalf("got %+v", s)
	}
	t.Setenv("EL_DECISION_MODEL", "respan/span-01")
	if s := LoadSettings(); s.DecisionModel != "respan/span-01" {
		t.Fatalf("env did not win: %+v", s)
	}
}
