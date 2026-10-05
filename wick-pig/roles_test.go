package wick

import (
	"reflect"
	"testing"
)

// plan is a planning session: file tools plus scout. It must not carry
// code_review or anything that spawns, merges or reports workers, so the same
// tool set that plans cannot run an implementation loop.
func TestPlanServesFileToolsAndScoutOnly(t *testing.T) {
	served, err := RoleTools("plan")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(served))
	for i, tool := range served {
		got[i] = tool.Name()
	}
	want := []string{"read", "edit", "write", "bash", "find", "background", "scout"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("plan tools = %v, want %v", got, want)
	}
}

// implement delegates and explores; it never edits, runs code or reviews.
func TestImplementDelegatesAndScouts(t *testing.T) {
	served, err := RoleTools("implement")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(served))
	for i, tool := range served {
		got[i] = tool.Name()
	}
	want := []string{"read", "find", "spawn_worker", "wait_workers", "finish_worker", "tell_worker", "scout"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("implement tools = %v, want %v", got, want)
	}
}

// full stays reachable, but only by asking for it: it is every tool, including
// the orchestration and review tools no planning session should hold.
func TestFullServesEveryTool(t *testing.T) {
	served, err := RoleTools("full")
	if err != nil {
		t.Fatal(err)
	}
	if len(served) != len(AllTools()) {
		t.Errorf("full served %d tools, want all %d", len(served), len(AllTools()))
	}
	names := map[string]bool{}
	for _, tool := range served {
		names[tool.Name()] = true
	}
	for _, want := range []string{"spawn_worker", "code_review"} {
		if !names[want] {
			t.Errorf("full must still reach %q", want)
		}
	}
}

func TestUnknownRoleIsAnError(t *testing.T) {
	if _, err := RoleTools("nope"); err == nil {
		t.Error("an unknown role must be refused")
	}
}

// wick-pig names every role it launches, so RoleFromEnv is the launcher's
// channel. An unset env means wick-mcp was started by hand and keeps full.
func TestRoleFromEnvHonoursTheLauncher(t *testing.T) {
	t.Setenv("WICK_ROLE", "plan")
	if got := RoleFromEnv(); got != "plan" {
		t.Errorf("RoleFromEnv() = %q, want the env role plan", got)
	}
	t.Setenv("WICK_ROLE", "")
	if got := RoleFromEnv(); got != "full" {
		t.Errorf("a hand-started wick-mcp = %q, want full", got)
	}
}
