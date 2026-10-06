package wick

import (
	"reflect"
	"strings"
	"testing"
)

// plan is a planning session: file tools plus spawn, so it can start scouts. It
// must not carry the review tools or anything that spawns, merges or reports
// workers, so the same tool set that plans cannot run an implementation loop.
func TestPlanServesFileToolsAndSpawnOnly(t *testing.T) {
	hermeticSettings(t)
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

// implement delegates and explores; it never runs code or reviews, and its
// edit and write are limited to the project's listed paths.
func TestImplementDelegatesAndSpawns(t *testing.T) {
	hermeticSettings(t)
	served, err := RoleTools("implement")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(served))
	for i, tool := range served {
		got[i] = tool.Name()
	}
	want := []string{"read", "edit", "write", "find", "spawn", "finish", "tell"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("implement tools = %v, want %v", got, want)
	}
}

// full stays reachable, but only by asking for it: it is every tool, including
// the orchestration tools no planning session should hold.
func TestFullServesEveryTool(t *testing.T) {
	hermeticSettings(t)
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
	for _, want := range []string{"spawn"} {
		if !names[want] {
			t.Errorf("full must still reach %q", want)
		}
	}
	for _, gone := range []string{"spawn_worker", "scout", "code_review"} {
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

// RoleFromEnv is the launcher's channel: wick-pig and the harnesses always
// set WICK_ROLE, so an empty value means a wick-mcp started by hand with no
// role. That is refused at startup: there is no silent fallback.
func TestRoleFromEnvHonoursTheLauncher(t *testing.T) {
	t.Setenv("WICK_ROLE", "plan")
	if got := RoleFromEnv(); got != "plan" {
		t.Errorf("RoleFromEnv() = %q, want the env role plan", got)
	}
	t.Setenv("WICK_ROLE", "")
	if got := RoleFromEnv(); got != "" {
		t.Errorf("a hand-started wick-mcp = %q, want empty (refused at startup)", got)
	}
}

// A server started with no role refuses with a message naming every valid
// role, so a hand-started server can never silently serve spawn, finish or
// tell. An explicit full role still serves everything. The want list is a
// literal, not RoleNames(), so the test can catch a role missing from either
// side.
func TestMissingRoleNamesEveryRole(t *testing.T) {
	want := []string{"agent", "full", "implement", "plan", "review", "scout", "work"}
	if got := RoleNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("RoleNames() = %v, want %v", got, want)
	}
	for _, err := range []error{MissingRoleError(), mustRoleErr(t, "")} {
		msg := err.Error()
		for _, role := range want {
			if !strings.Contains(msg, role) {
				t.Errorf("missing-role error %q must name role %q", msg, role)
			}
		}
	}
}

func mustRoleErr(t *testing.T, role string) error {
	t.Helper()
	_, err := RoleTools(role)
	if err == nil {
		t.Fatalf("RoleTools(%q) must fail", role)
	}
	return err
}

// No role serves a wait tool: the parent learns a result arrived instead of
// blocking on it. tell and finish are what a delegating role gets instead.
func TestNoRoleServesAWaitTool(t *testing.T) {
	hermeticSettings(t)
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
