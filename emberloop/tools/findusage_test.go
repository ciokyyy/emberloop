package tools

import (
	"context"
	"os"
	"strings"
	"testing"

	"example.com/emberloop/statedb"
	"example.com/emberloop/tools/systemone"
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

// readFindLines returns the agent's find rows from the state database, in row
// order: find logs rows there now, not JSONL lines.
func readFindLines(t *testing.T, agent string) []statedb.UsageRow {
	t.Helper()
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		t.Fatalf("open state database: %v", err)
	}
	defer db.Close()
	rows, err := statedb.UsageRowsForAgent(db, agent)
	if err != nil {
		t.Fatalf("read usage rows: %v", err)
	}
	for _, r := range rows {
		if r.Kind != statedb.KindFind {
			t.Fatalf("row kind = %q, want find", r.Kind)
		}
	}
	return rows
}

func TestFindLogsDecisionCallsAgainstTheCallingAgent(t *testing.T) {
	dir := project(t)
	t.Setenv("EMBERLOOP_SETTINGS", dir+"/none.json")
	t.Setenv("EMBERLOOP_STATE_DIR", t.TempDir())
	t.Setenv("EMBERLOOP_AGENT_ID", "agent-find")
	// The model assertion needs the default: an ambient override would log
	// whatever the machine prefers instead of the pin.
	t.Setenv("EMBERLOOP_DECISION_MODEL", "")

	out, err := run(t, findTool{provider: usageJudge{}}, dir, findArgs())
	if err != nil || !strings.Contains(out, "auth.go") {
		t.Fatalf("out = %q, err = %v", out, err)
	}

	lines := readFindLines(t, "agent-find")
	if len(lines) == 0 {
		t.Fatal("no find rows logged")
	}
	callID := lines[0].FindCallID
	if callID == "" {
		t.Fatalf("rows carry no find-call id: %+v", lines[0])
	}
	for _, line := range lines {
		if line.Agent != "agent-find" || line.Kind != statedb.KindFind {
			t.Errorf("row identity = %+v", line)
		}
		if line.Model != systemone.JevModelPinned {
			t.Errorf("row model = %v, want %v", line.Model, systemone.JevModelPinned)
		}
		if line.FindCallID != callID {
			t.Errorf("row groups under %v, want %v", line.FindCallID, callID)
		}
		if line.FindQuestions < 1 {
			t.Errorf("row questions = %v, want >= 1", line.FindQuestions)
		}
		if line.Input != 278 || line.ReportedCost == nil || *line.ReportedCost != 0.000011676 {
			t.Errorf("row usage = %d/%v, want 278/0.000011676", line.Input, line.ReportedCost)
		}
	}
}

func TestFindWithoutUsageStillLogsCallCounts(t *testing.T) {
	dir := project(t)
	t.Setenv("EMBERLOOP_SETTINGS", dir+"/none.json")
	t.Setenv("EMBERLOOP_STATE_DIR", t.TempDir())
	t.Setenv("EMBERLOOP_AGENT_ID", "agent-bare")
	t.Setenv("EMBERLOOP_DECISION_MODEL", "")

	if _, err := run(t, findTool{provider: bareJudge{fixedJudge{0.85}}}, dir, findArgs()); err != nil {
		t.Fatal(err)
	}
	lines := readFindLines(t, "agent-bare")
	if len(lines) == 0 {
		t.Fatal("no find rows logged")
	}
	for _, line := range lines {
		if line.ReportedCost != nil {
			t.Errorf("row carries cost = %v, want none", *line.ReportedCost)
		}
		if line.Agent != "agent-bare" || line.Kind != statedb.KindFind {
			t.Errorf("row = %+v", line)
		}
		// The call count is the rows themselves; each row names how many
		// decisions its call carried.
		if line.FindQuestions < 1 {
			t.Errorf("row questions = %v, want >= 1", line.FindQuestions)
		}
		if line.FindCallID == "" {
			t.Errorf("row carries no find-call id: %+v", line)
		}
	}
}

func TestFindCallIDGroupsOneSearchAndResetsOnReuse(t *testing.T) {
	dir := project(t)
	t.Setenv("EMBERLOOP_SETTINGS", dir+"/none.json")
	t.Setenv("EMBERLOOP_STATE_DIR", t.TempDir())
	t.Setenv("EMBERLOOP_DECISION_MODEL", "")
	searcher := NewCascadeSearcher(usageJudge{}, systemone.JevModelPinned, 0.2, dir)
	ctx := context.Background()

	t.Setenv("EMBERLOOP_AGENT_ID", "agent-reuse")
	if _, err := searcher.Search(ctx, "how are tokens verified", []string{"VerifyToken"}, ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMBERLOOP_AGENT_ID", "agent-next")
	if _, err := searcher.Search(ctx, "how are tokens verified", []string{"VerifyToken"}, ""); err != nil {
		t.Fatal(err)
	}

	first := readFindLines(t, "agent-reuse")
	second := readFindLines(t, "agent-next")
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("logged %d then %d rows, want both non-empty", len(first), len(second))
	}
	if first[0].FindCallID == second[0].FindCallID {
		t.Errorf("both searches share find-call id %v", first[0].FindCallID)
	}
	for _, line := range second {
		if line.Agent != "agent-next" {
			t.Errorf("reused searcher attributed to %v", line.Agent)
		}
	}
}

func TestFindWithoutAgentLogsNothing(t *testing.T) {
	state := t.TempDir()
	t.Setenv("EMBERLOOP_SETTINGS", state+"/none.json")
	t.Setenv("EMBERLOOP_STATE_DIR", state)
	t.Setenv("EMBERLOOP_AGENT_ID", "")
	t.Setenv("EMBERLOOP_DECISION_MODEL", "")

	if _, err := run(t, findTool{provider: usageJudge{}}, project(t), findArgs()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(state + "/usage"); !os.IsNotExist(err) {
		t.Errorf("usage log written with no agent id")
	}
}
