package emberloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Seam 1: harness command — every session emberloop launches carries EMBERLOOP_AGENT_ID
// in its role environment, so its emberloop-mcp knows who it serves. The id is the
// session id, or the worker's ticket label when there is no session id yet.
func TestEverySessionCarriesAnAgentID(t *testing.T) {
	isolateState(t)
	cases := map[string]struct {
		sess Session
		want string
	}{
		"top-level": {Session{Role: "plan", ID: "top-1", Prompt: "hi"}, "top-1"},
		"reviewer":  {readOnlySession("rev-1", "/tmp", "", ""), "rev-1"},
		"scout":     {Session{Role: "agent", ID: "scout-1", AgentType: "scout", Prompt: "q"}, "scout-1"},
		"worker":    {Session{Role: "work", Label: "01-x", Prompt: "do it"}, "01-x"},
	}
	for name, c := range cases {
		for _, harness := range HarnessNames() {
			cmd, err := BuildCommand(harness, c.sess)
			if err != nil {
				t.Fatalf("%s/%s: %v", name, harness, err)
			}
			found := ""
			for _, e := range cmd.Env {
				if v, ok := strings.CutPrefix(e, "EMBERLOOP_AGENT_ID="); ok {
					found = v
				}
			}
			if found != c.want {
				t.Errorf("%s/%s: EMBERLOOP_AGENT_ID = %q, want %q in %q", name, harness, found, c.want, cmd.Env)
			}
		}
	}
	// An explicit agent id wins over the session id: a worker keeps its ticket
	// id while the harness session id names its transcript.
	cmd, err := BuildCommand("pig", Session{Role: "agent", ID: "sess-9", AgentID: "01-x", AgentType: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	got := ""
	for _, e := range cmd.Env {
		if v, ok := strings.CutPrefix(e, "EMBERLOOP_AGENT_ID="); ok {
			got = v
		}
	}
	if got != "01-x" {
		t.Errorf("explicit agent id = %q, want 01-x", got)
	}
}

// Seam 2: run record store — implement writes one record per spec slug,
// re-running updates it, and readers list it. Temporary state dir plus fake
// Exec, the way the worker tests do it.
func TestImplementRunRecordIsWrittenListedAndUpdated(t *testing.T) {
	isolateState(t)
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	t.Setenv("HERDR_TAB_ID", "t1")
	// No herdr call is needed when the tab id is already in the environment.
	failExec := Exec(func(cmd string, args []string, o ExecOpts) (Result, error) {
		t.Fatalf("no exec needed, got %s %v", cmd, args)
		return Result{}, nil
	})
	root := t.TempDir()
	dir := filepath.Join(root, "dogfood-run")
	if err := os.MkdirAll(filepath.Join(dir, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "spec.md"), "# spec\n")
	spec, err := ResolveSpec(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := RecordRun(failExec, spec, root, "orch-1", "pig", "m1", "high")
	if err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	if first.Slug == "" || first.Spec != spec.Spec || first.Branch != spec.Branch {
		t.Errorf("record must key on the spec: %+v want spec %q branch %q", first, spec.Spec, spec.Branch)
	}
	if first.AgentID != "orch-1" || first.Pane != "w1:p1" || first.Tab != "t1" {
		t.Errorf("record must carry the orchestrator: %+v", first)
	}
	if first.Harness != "pig" || first.Model != "m1" || first.Effort != "high" || first.Started == "" {
		t.Errorf("record must carry harness/model/effort/start: %+v", first)
	}
	if first.Ended != "" {
		t.Errorf("a fresh run has no end time: %+v", first)
	}
	// The launch serves the recorded orchestrator id to its emberloop-mcp: the
	// record and the session agree on who the orchestrator is.
	launch, err := PrepareLaunch("pig", Session{ID: "orch-1", AgentID: "orch-1", Role: "implement", Spec: spec.Spec, Prompt: "x"})
	if err != nil {
		t.Fatalf("PrepareLaunch: %v", err)
	}
	file, ok := execFileFromLine(launch.Line)
	if !ok {
		t.Fatalf("no launch file in %q", launch.Line)
	}
	cmd, err := readLaunch(file)
	if err != nil {
		t.Fatal(err)
	}
	launchID := ""
	for _, e := range cmd.Env {
		if v, ok := strings.CutPrefix(e, "EMBERLOOP_AGENT_ID="); ok {
			launchID = v
		}
	}
	if launchID != first.AgentID {
		t.Errorf("launch agent id = %q, want the recorded orchestrator %q", launchID, first.AgentID)
	}
	got, err := ListRunRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Slug != first.Slug {
		t.Fatalf("list = %+v, want one record %q", got, first.Slug)
	}
	// Re-running the same spec updates the record rather than duplicating it.
	second, err := RecordRun(failExec, spec, root, "orch-2", "claude", "m2", "low")
	if err != nil {
		t.Fatalf("re-record: %v", err)
	}
	if second.Slug != first.Slug {
		t.Fatalf("re-run changed the key: %q vs %q", second.Slug, first.Slug)
	}
	got, err = ListRunRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("re-run duplicated the record: %+v", got)
	}
	if got[0].AgentID != "orch-2" || got[0].Harness != "claude" {
		t.Errorf("re-run must update the record: %+v", got[0])
	}
	// A normal exit stamps the end time.
	if err := FinishRun(first.Slug); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	got, err = ListRunRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Ended == "" {
		t.Errorf("finished run must carry an end time: %+v", got)
	}
}
