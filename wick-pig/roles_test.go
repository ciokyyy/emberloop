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
	want := []string{"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "spawn", "execute"}
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
	want := []string{"read", "edit", "write", "find", "websearch", "ctx_search", "spawn", "finish", "tell"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("implement tools = %v, want %v", got, want)
	}
}

// ctx_search rides with read: every role that can read serves it, so stored
// output is searchable wherever it is readable.
func TestEveryRoleThatCanReadServesCtxSearch(t *testing.T) {
	hermeticSettings(t)
	for _, role := range []string{"implement", "review", "scout", "work", "plan", "full"} {
		served, err := RoleTools(role)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, tl := range served {
			if tl.Name() == "ctx_search" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s role must serve ctx_search", role)
		}
	}
}

// full stays reachable, but only by asking for it: it is every tool, including
// the orchestration tools no planning session should hold. Tools whose default
// placement is execute (grep, git, check) serve behind execute, not in main.
func TestFullServesEveryTool(t *testing.T) {
	hermeticSettings(t)
	served, err := RoleTools("full")
	if err != nil {
		t.Fatal(err)
	}
	_, execute, err := ResolvedTools("full")
	if err != nil {
		t.Fatal(err)
	}
	if len(served)+len(execute) != len(AllTools()) {
		t.Errorf("full served %d in main plus %d behind execute, want all %d", len(served), len(execute), len(AllTools()))
	}
	for _, want := range []string{"grep", "git", "check"} {
		if containsName(toolNames(served), want) {
			t.Errorf("full must keep %q behind execute, not in main", want)
		}
		if !containsName(toolNames(execute), want) {
			t.Errorf("full must offer %q behind execute", want)
		}
	}
	names := map[string]bool{}
	for _, tool := range served {
		names[tool.Name()] = true
	}
	for _, want := range []string{"spawn", "execute"} {
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

// check lives behind execute by default for review, work, plan and full,
// and is offered to no scout list at all.
func TestCheckLivesBehindExecuteForItsRoles(t *testing.T) {
	hermeticSettings(t)
	for _, role := range []string{"review", "work", "plan", "full"} {
		main, execute, err := ResolvedTools(role)
		if err != nil {
			t.Fatal(err)
		}
		if containsName(toolNames(main), "check") {
			t.Errorf("%s must not serve check in main, want it behind execute", role)
		}
		if !containsName(toolNames(execute), "check") {
			t.Errorf("%s must offer check behind execute: main=%v execute=%v", role, toolNames(main), toolNames(execute))
		}
	}
	main, execute, err := ResolvedTools("scout")
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range append(toolNames(main), toolNames(execute)...) {
		if got == "check" {
			t.Errorf("scout must not reach check: main=%v execute=%v", toolNames(main), toolNames(execute))
		}
	}
}

// Settings can promote check to the main list for a role that offers it.
func TestCheckPlacementMainServesIt(t *testing.T) {
	writeSettingsFile(t, `{"tools":{"check":{"placement":"main"}}}`)
	served, err := RoleTools("work")
	if err != nil {
		t.Fatal(err)
	}
	if !containsName(toolNames(served), "check") {
		t.Fatalf("execute-placed check must serve in main when placed there: %v", toolNames(served))
	}
	served, err = RoleTools("scout")
	if err != nil {
		t.Fatal(err)
	}
	if containsName(toolNames(served), "check") {
		t.Fatalf("settings must not widen scout with check: %v", toolNames(served))
	}
}

// Every role that can read serves websearch: researchers in every seat get
// the same engine, and settings can still narrow it per role.
func TestEveryReadingRoleServesWebsearch(t *testing.T) {
	hermeticSettings(t)
	for _, role := range []string{"implement", "plan", "work", "review", "scout", "full"} {
		served, err := RoleTools(role)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, tool := range served {
			if tool.Name() == "websearch" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s can read but does not serve websearch", role)
		}
	}
}
