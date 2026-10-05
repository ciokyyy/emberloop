package wick

import (
	"reflect"
	"testing"
)

// plan is a planning session: file tools plus spawn, so it can start scouts. It
// must not carry code_review or anything that spawns, merges or reports
// workers, so the same tool set that plans cannot run an implementation loop.
func TestPlanServesFileToolsAndSpawnOnly(t *testing.T) {
	served, err := RoleTools("plan")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(served))
	for i, tool := range served {
		got[i] = tool.Name()
	}
	want := []string{"read", "edit", "write", "bash", "find", "background", "spawn"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("plan tools = %v, want %v", got, want)
	}
}

// implement delegates and explores; it never edits, runs code or reviews.
func TestImplementDelegatesAndSpawns(t *testing.T) {
	served, err := RoleTools("implement")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(served))
	for i, tool := range served {
		got[i] = tool.Name()
	}
	want := []string{"read", "find", "spawn", "finish", "tell"}
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
	for _, want := range []string{"spawn", "code_review"} {
		if !names[want] {
			t.Errorf("full must still reach %q", want)
		}
	}
	for _, gone := range []string{"spawn_worker", "scout"} {
		if names[gone] {
			t.Errorf("the %s tool is gone and must not be served", gone)
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

// No role serves a wait tool: the parent learns a result arrived instead of
// blocking on it. tell and finish are what a delegating role gets instead.
func TestNoRoleServesAWaitTool(t *testing.T) {
	removed := []string{"wait_workers", "tell_worker", "finish_worker", "wait"}
	for _, role := range []string{"implement", "plan", "work", "review", "scout", "full"} {
		served, err := RoleTools(role)
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		for _, tool := range served {
			names[tool.Name()] = true
		}
		for _, gone := range removed {
			if names[gone] {
				t.Errorf("%s role still serves %s", role, gone)
			}
		}
	}
	for _, role := range []string{"implement", "full"} {
		served, _ := RoleTools(role)
		names := map[string]bool{}
		for _, tool := range served {
			names[tool.Name()] = true
		}
		for _, want := range []string{"tell", "finish"} {
			if !names[want] {
				t.Errorf("%s role must serve %s", role, want)
			}
		}
	}
}
