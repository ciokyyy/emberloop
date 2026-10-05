package tools

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"example.com/wick/tools/systemone"
)

// usageJudge answers every question with usage, the way the live endpoint
// does (probe 2026-10-05, ticket 04).
type usageJudge struct{}

func (usageJudge) Evaluate(_ context.Context, req systemone.Request) (systemone.Response, error) {
	answers := map[string]systemone.Answer{}
	for id := range req.Questions {
		score := 0.85
		answers[id] = systemone.Answer{Type: systemone.TypeNoul, Noul: &score}
	}
	return systemone.Response{
		Answers:    answers,
		Usage:      &systemone.Usage{InputTokens: 278, OutputTokens: 21, Cost: 0.000011676},
		Generation: "gen-dec-test",
		Provider:   "TypeSafe",
		Model:      "typesafe/jev-1.13-20260917",
	}, nil
}

// bareJudge answers with no usage, for endpoints that return none.
type bareJudge struct{ fixedJudge }

func (b bareJudge) Evaluate(ctx context.Context, req systemone.Request) (systemone.Response, error) {
	resp, err := b.fixedJudge.Evaluate(ctx, req)
	resp.Usage = nil
	return resp, err
}

func readFindLines(t *testing.T, agent string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(UsagePath(agent))
	if err != nil {
		t.Fatalf("read usage log: %v", err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestFindLogsDecisionCallsAgainstTheCallingAgent(t *testing.T) {
	dir := project(t)
	t.Setenv("WICK_SETTINGS", dir+"/none.json")
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	t.Setenv("WICK_AGENT_ID", "agent-find")
	// The model assertion needs the default: an ambient override would log
	// whatever the machine prefers instead of the pin.
	t.Setenv("WICK_DECISION_MODEL", "")

	out, err := run(t, findTool{provider: usageJudge{}}, dir, findArgs())
	if err != nil || !strings.Contains(out, "auth.go") {
		t.Fatalf("out = %q, err = %v", out, err)
	}

	lines := readFindLines(t, "agent-find")
	if len(lines) == 0 {
		t.Fatal("no find lines logged")
	}
	callID, _ := lines[0]["find_call_id"].(string)
	if callID == "" {
		t.Fatalf("lines carry no find-call id: %v", lines[0])
	}
	for _, line := range lines {
		if line["agent"] != "agent-find" || line["kind"] != "find" {
			t.Errorf("line identity = %v", line)
		}
		if line["model"] != systemone.JevModelPinned {
			t.Errorf("line model = %v, want %v", line["model"], systemone.JevModelPinned)
		}
		if line["find_call_id"] != callID {
			t.Errorf("line groups under %v, want %v", line["find_call_id"], callID)
		}
		if q, _ := line["questions"].(float64); q < 1 {
			t.Errorf("line questions = %v", line["questions"])
		}
		usage, _ := line["usage"].(map[string]any)
		if usage["input_tokens"] != float64(278) || usage["cost"] != 0.000011676 {
			t.Errorf("line usage = %v", line["usage"])
		}
	}
}

func TestFindWithoutUsageStillLogsCallCounts(t *testing.T) {
	dir := project(t)
	t.Setenv("WICK_SETTINGS", dir+"/none.json")
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	t.Setenv("WICK_AGENT_ID", "agent-bare")
	t.Setenv("WICK_DECISION_MODEL", "")

	if _, err := run(t, findTool{provider: bareJudge{fixedJudge{0.85}}}, dir, findArgs()); err != nil {
		t.Fatal(err)
	}
	lines := readFindLines(t, "agent-bare")
	if len(lines) == 0 {
		t.Fatal("no find lines logged")
	}
	for _, line := range lines {
		if _, ok := line["usage"]; ok {
			t.Errorf("line carries usage = %v, want none", line["usage"])
		}
		if line["agent"] != "agent-bare" || line["kind"] != "find" {
			t.Errorf("line = %v", line)
		}
		// The call count is the lines themselves; each line names how many
		// decisions its call carried.
		if q, _ := line["questions"].(float64); q < 1 {
			t.Errorf("line questions = %v, want >= 1", line["questions"])
		}
		if line["find_call_id"] == nil || line["find_call_id"] == "" {
			t.Errorf("line carries no find-call id: %v", line)
		}
	}
}

func TestFindCallIDGroupsOneSearchAndResetsOnReuse(t *testing.T) {
	dir := project(t)
	t.Setenv("WICK_SETTINGS", dir+"/none.json")
	t.Setenv("WICK_STATE_DIR", t.TempDir())
	t.Setenv("WICK_DECISION_MODEL", "")
	searcher := NewCascadeSearcher(usageJudge{}, systemone.JevModelPinned, 0.2, dir)
	ctx := context.Background()

	t.Setenv("WICK_AGENT_ID", "agent-reuse")
	if _, err := searcher.Search(ctx, "how are tokens verified", []string{"VerifyToken"}, ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WICK_AGENT_ID", "agent-next")
	if _, err := searcher.Search(ctx, "how are tokens verified", []string{"VerifyToken"}, ""); err != nil {
		t.Fatal(err)
	}

	first := readFindLines(t, "agent-reuse")
	second := readFindLines(t, "agent-next")
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("logged %d then %d lines, want both non-empty", len(first), len(second))
	}
	if first[0]["find_call_id"] == second[0]["find_call_id"] {
		t.Errorf("both searches share find-call id %v", first[0]["find_call_id"])
	}
	for _, line := range second {
		if line["agent"] != "agent-next" {
			t.Errorf("reused searcher attributed to %v", line["agent"])
		}
	}
}

func TestFindWithoutAgentLogsNothing(t *testing.T) {
	state := t.TempDir()
	t.Setenv("WICK_SETTINGS", state+"/none.json")
	t.Setenv("WICK_STATE_DIR", state)
	t.Setenv("WICK_AGENT_ID", "")
	t.Setenv("WICK_DECISION_MODEL", "")

	if _, err := run(t, findTool{provider: usageJudge{}}, project(t), findArgs()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(state + "/usage"); !os.IsNotExist(err) {
		t.Errorf("usage log written with no agent id")
	}
}
