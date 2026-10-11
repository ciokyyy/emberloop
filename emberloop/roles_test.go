package el

import (
	"reflect"
	"strings"
	"testing"

	"example.com/emberloop/tools"
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

// RoleFromEnv is the launcher's channel: el and the harnesses always
// set EL_ROLE, so an empty value means a el-mcp started by hand with no
// role. That is refused at startup: there is no silent fallback.
func TestRoleFromEnvHonoursTheLauncher(t *testing.T) {
	t.Setenv("EL_ROLE", "plan")
	if got := RoleFromEnv(); got != "plan" {
		t.Errorf("RoleFromEnv() = %q, want the env role plan", got)
	}
	t.Setenv("EL_ROLE", "")
	if got := RoleFromEnv(); got != "" {
		t.Errorf("a hand-started el-mcp = %q, want empty (refused at startup)", got)
	}
}

// A server started with no role refuses with a message naming every valid
// role, so a hand-started server can never silently serve spawn, finish or
// tell. An explicit full role still serves everything. The want list is a
// literal, not RoleNames(), so the test can catch a role missing from either
// side.
func TestMissingRoleNamesEveryRole(t *testing.T) {
	want := []string{"agent", "full", "implement", "plan", "review", "run-worker", "scout", "web", "web-worker", "work"}
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

// Ticket 18: one test per role asserting its served tool list (the main list
// the MCP server serves) and its execute-reachable tools, in serve order,
// under default settings. review and scout read, find and execute (with
// websearch and ctx_search riding with read); work, plan and full add
// execute to their file tools; implement never gets execute.
func TestRoleToolsetsPerRole(t *testing.T) {
	hermeticSettings(t)
	cases := []struct {
		role          string
		main, execute []string
	}{
		{"implement", []string{"read", "edit", "write", "find", "websearch", "ctx_search", "spawn", "finish", "tell"}, []string{}},
		{"review", []string{"read", "find", "websearch", "ctx_search", "execute"}, []string{"check", "grep", "git"}},
		{"scout", []string{"read", "find", "websearch", "ctx_search", "execute"}, []string{"grep", "git"}},
		{"work", []string{"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "execute"}, []string{"check", "grep", "git"}},
		{"plan", []string{"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "spawn", "execute"}, []string{"check", "grep", "git"}},
		{"full", []string{"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "spawn", "finish", "tell", "execute"}, []string{"grep", "git", "check"}},
	}
	for _, tc := range cases {
		main, execute, err := ResolvedTools(tc.role)
		if err != nil {
			t.Fatal(err)
		}
		if got := toolNames(main); !equalNames(got, tc.main) {
			t.Errorf("%s main = %v, want %v", tc.role, got, tc.main)
		}
		if got := toolNames(execute); !equalNames(got, tc.execute) {
			t.Errorf("%s execute = %v, want %v", tc.role, got, tc.execute)
		}
	}
}

// The web Daemon serves one el MCP server per Session, bound through the
// environment: the role decides which Daemon tools ride along. A top-level
// parent manages its children but never submits, a spawned worker spawns its
// own (nesting kept) but never steers, peeks at or stops, and a scout only
// reads: submit stays registered for web-worker and scout but is unoffered
// while WebSubmitEnabled is false. Children never hold the parent's full
// set, and without the binding every role resolves as for herdr.
func TestWebRoleMatrixUnderDaemon(t *testing.T) {
	hermeticSettings(t)
	t.Setenv("EL_WEB_PARENT", "sess-parent")
	t.Setenv("EL_WEB_DAEMON_URL", "http://127.0.0.1:9")
	cases := []struct {
		role          string
		main, execute []string
		proxies       []string
	}{
		{"web", []string{"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "spawn", "steer", "check", "cancel", "execute"}, []string{"grep", "git"}, []string{"spawn", "steer", "check", "cancel"}},
		{"web-worker", []string{"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "spawn", "execute"}, []string{"check", "grep", "git"}, []string{"spawn"}},
		{"scout", []string{"read", "find", "websearch", "ctx_search", "execute"}, []string{"grep", "git"}, []string{}},
		// Run workers keep submit with the flag off: the Run route needs it.
		{"run-worker", []string{"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "spawn", "submit", "execute"}, []string{"check", "grep", "git"}, []string{"spawn", "submit"}},
	}
	// No test in this package runs parallel and no other test enables the
	// flag, so the default-off read here needs no save/restore.
	if WebSubmitEnabled {
		t.Fatal("WebSubmitEnabled must default false")
	}
	for _, tc := range cases {
		main, execute, err := ResolvedTools(tc.role)
		if err != nil {
			t.Fatal(err)
		}
		if got := toolNames(main); !equalNames(got, tc.main) {
			t.Errorf("%s main = %v, want %v", tc.role, got, tc.main)
		}
		if got := toolNames(execute); !equalNames(got, tc.execute) {
			t.Errorf("%s execute = %v, want %v", tc.role, got, tc.execute)
		}
		byName := map[string]tools.Tool{}
		for _, tl := range append(append([]tools.Tool(nil), main...), execute...) {
			byName[tl.Name()] = tl
		}
		for _, name := range tc.proxies {
			tl, ok := byName[name]
			if !ok {
				t.Errorf("%s must serve Daemon tool %q", tc.role, name)
				continue
			}
			if !tools.IsDaemonProxy(tl) {
				t.Errorf("%s tool %q must be the Daemon proxy, not a core tool", tc.role, name)
			}
		}
	}
	// The worker keeps the repo checks behind execute; only the parent's
	// check is the child peek.
	_, workerExecute, err := ResolvedTools("web-worker")
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range workerExecute {
		if tl.Name() == "check" && tools.IsDaemonProxy(tl) {
			t.Errorf("web-worker check behind execute must stay the repo checks, not a proxy")
		}
	}
	// While the flag is off no web role serves submit anywhere.
	for _, role := range []string{"web", "web-worker", "scout"} {
		main, execute, err := ResolvedTools(role)
		if err != nil {
			t.Fatal(err)
		}
		for _, tl := range append(main, execute...) {
			if tl.Name() == "submit" {
				t.Errorf("%s must not serve submit while WebSubmitEnabled is false", role)
			}
		}
	}
	// Full never takes proxies, even bound: it is the herdr tool set.
	bound, boundExecute, err := ResolvedTools("full")
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range append(bound, boundExecute...) {
		if tools.IsDaemonProxy(tl) {
			t.Errorf("full must not serve Daemon proxy %q", tl.Name())
		}
	}
}

// With the flag on the web matrix is the old one: web-worker settles
// through submit and scouts settle only. The flag is the test hook for the
// Daemon gate tests too; it is restored so the default-off tests keep it.
func TestWebRoleMatrixWithSubmitEnabled(t *testing.T) {
	hermeticSettings(t)
	t.Setenv("EL_WEB_PARENT", "sess-parent")
	t.Setenv("EL_WEB_DAEMON_URL", "http://127.0.0.1:9")
	WebSubmitEnabled = true
	t.Cleanup(func() { WebSubmitEnabled = false })
	cases := []struct {
		role          string
		main, execute []string
		proxies       []string
	}{
		{"web", []string{"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "spawn", "steer", "check", "cancel", "execute"}, []string{"grep", "git"}, []string{"spawn", "steer", "check", "cancel"}},
		{"web-worker", []string{"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "spawn", "submit", "execute"}, []string{"check", "grep", "git"}, []string{"spawn", "submit"}},
		{"scout", []string{"read", "find", "websearch", "ctx_search", "submit", "execute"}, []string{"grep", "git"}, []string{"submit"}},
	}
	for _, tc := range cases {
		main, execute, err := ResolvedTools(tc.role)
		if err != nil {
			t.Fatal(err)
		}
		if got := toolNames(main); !equalNames(got, tc.main) {
			t.Errorf("%s main = %v, want %v", tc.role, got, tc.main)
		}
		if got := toolNames(execute); !equalNames(got, tc.execute) {
			t.Errorf("%s execute = %v, want %v", tc.role, got, tc.execute)
		}
		byName := map[string]tools.Tool{}
		for _, tl := range append(append([]tools.Tool(nil), main...), execute...) {
			byName[tl.Name()] = tl
		}
		for _, name := range tc.proxies {
			tl, ok := byName[name]
			if !ok {
				t.Errorf("%s must serve Daemon tool %q", tc.role, name)
				continue
			}
			if !tools.IsDaemonProxy(tl) {
				t.Errorf("%s tool %q must be the Daemon proxy, not a core tool", tc.role, name)
			}
		}
	}
}

// Without the Daemon binding the web roles resolve as for herdr: no Daemon
// tool anywhere, the repo checks behind execute.
func TestWebRolesUnboundServeNoProxies(t *testing.T) {
	hermeticSettings(t)
	for _, role := range []string{"web", "web-worker", "scout"} {
		main, execute, err := ResolvedTools(role)
		if err != nil {
			t.Fatal(err)
		}
		for _, tl := range append(main, execute...) {
			if tools.IsDaemonProxy(tl) {
				t.Errorf("unbound %s must not serve Daemon proxy %q", role, tl.Name())
			}
		}
	}
	main, execute, err := ResolvedTools("web")
	if err != nil {
		t.Fatal(err)
	}
	if got := toolNames(main); !equalNames(got, []string{"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "execute"}) {
		t.Errorf("unbound web main = %v", got)
	}
	if got := toolNames(execute); !equalNames(got, []string{"check", "grep", "git"}) {
		t.Errorf("unbound web execute = %v", got)
	}
}
