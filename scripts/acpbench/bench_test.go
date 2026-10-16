package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Integration: the bench binary drives the in-repo fake agent. No live
// sessions, no network, no credentials.

var testBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "acpbench-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "acpbench")
	cmd := exec.Command("go", "build", "-o", out, ".")
	if b, err := cmd.CombinedOutput(); err != nil {
		panic("build: " + err.Error() + ": " + string(b))
	}
	testBin = out
	os.Exit(m.Run())
}

func fastEnv() []string {
	return []string{
		"ACPBENCH_PROMPT_TIMEOUT=8s",
		"ACPBENCH_CTL_TIMEOUT=5s",
		"ACPBENCH_INIT_TIMEOUT=5s",
		"ACPBENCH_CANCEL_WAIT=5s",
		"ACPBENCH_STEER_WAIT=5s",
		"ACPBENCH_CRASH_WAIT=5s",
		"ACPBENCH_MCP_WAIT=3s",
		"ACPBENCH_RACE_WAIT=300ms",
		"ACPBENCH_SETTLE_WAIT=100ms",
	}
}

// runBenchReport runs the binary with --report to a temp file and decodes it.
func runBenchReport(t *testing.T, cmdArgs []string, extraEnv []string) (BenchReport, int) {
	t.Helper()
	report := filepath.Join(t.TempDir(), "report.json")
	args := append([]string{"--cmd", cmdArgs[0], "--report", report}, cmdArgs[1:]...)
	cmd := exec.Command(testBin, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run: %v\n%s", err, out)
		}
	}
	raw, rerr := os.ReadFile(report)
	if rerr != nil {
		t.Fatalf("no report (exit %d): %v\n%s", code, rerr, out)
	}
	var rep BenchReport
	if jerr := json.Unmarshal(raw, &rep); jerr != nil {
		t.Fatalf("bad report: %v", jerr)
	}
	return rep, code
}

func fakeCmd(t *testing.T, extra ...string) string {
	t.Helper()
	quoted := "\"" + testBin + "\" fake-agent"
	for _, e := range extra {
		quoted += " " + e
	}
	return quoted
}

func verdictOf(rep BenchReport, name string) ScenarioResult {
	for _, s := range rep.Scenarios {
		if s.Name == name {
			return s
		}
	}
	return ScenarioResult{Name: name, Verdict: "missing"}
}

func TestPromptTextOK(t *testing.T) {
	rep, code := runBenchReport(t, []string{fakeCmd(t), "--mcp-stub", "--scenario", "prompt_text"}, fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	if r := verdictOf(rep, "prompt_text"); r.Verdict != VerdictOK {
		t.Fatalf("%+v", r)
	}
}

func TestFaultMatrix(t *testing.T) {
	for _, tc := range []struct {
		flags   []string
		scen    string
		want    string
		timeout string
	}{
		{[]string{"--hang-prompt"}, "prompt_text", VerdictTimeoutHang, "3s"},
		{[]string{"--crash-prompt"}, "prompt_text", VerdictCrash, ""},
		{[]string{"--empty"}, "prompt_text", VerdictSilentEmptyTurn, ""},
		{[]string{"--wrong"}, "prompt_text", VerdictWrongResult, ""},
		{[]string{"--image-behavior", "error"}, "images", VerdictProtocolError, ""},
		{[]string{"--image-behavior", "drop"}, "images", VerdictWrongResult, ""},
	} {
		env := fastEnv()
		if tc.timeout != "" {
			env = append(env, "ACPBENCH_PROMPT_TIMEOUT="+tc.timeout)
		}
		rep, _ := runBenchReport(t, []string{fakeCmd(t, tc.flags...), "--scenario", tc.scen}, env)
		if r := verdictOf(rep, tc.scen); r.Verdict != tc.want {
			t.Errorf("flags %v: got %+v want %s", tc.flags, r, tc.want)
		}
	}
}

func TestCancelSteerCrashSlow(t *testing.T) {
	cmd := fakeCmd(t, "--slow-ms", "15000", "--slow-match", "slow tool")
	for _, scen := range []string{"cancel", "steer_followup", "crash_recovery"} {
		rep, code := runBenchReport(t, []string{cmd, "--scenario", scen}, fastEnv())
		if code != 0 {
			t.Errorf("%s: exit %d: %+v", scen, code, rep.Scenarios)
		}
		if r := verdictOf(rep, scen); r.Verdict != VerdictOK {
			t.Errorf("%s: %+v", scen, r)
		}
	}
}

func TestSlowCancelWaits(t *testing.T) {
	// slow-cancel 2s must still resolve inside the cancel wait.
	cmd := fakeCmd(t, "--slow-ms", "15000", "--slow-match", "slow tool", "--slow-cancel-ms", "2000")
	rep, code := runBenchReport(t, []string{cmd, "--scenario", "cancel"}, fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	if r := verdictOf(rep, "cancel"); r.Verdict != VerdictOK {
		t.Fatalf("%+v", r)
	}
}

func TestLoadResume(t *testing.T) {
	state := t.TempDir()
	cmd := fakeCmd(t, "--state-dir", state)
	rep, code := runBenchReport(t, []string{cmd, "--scenario", "load_resume"}, fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	r := verdictOf(rep, "load_resume")
	if r.Verdict != VerdictOK {
		t.Fatalf("%+v", r)
	}
	if h, _ := r.Details["history"].(string); !strings.Contains(h, "history:") {
		t.Fatalf("history not preserved across respawn: %+v", r.Details)
	}
}

func TestPermissionApproveDeny(t *testing.T) {
	rep, code := runBenchReport(t, []string{fakeCmd(t, "--ask-permission"), "--scenario", "permission"}, fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	if r := verdictOf(rep, "permission"); r.Verdict != VerdictOK {
		t.Fatalf("%+v", r)
	}
}

func TestSetModelImagesSoakConcurrency(t *testing.T) {
	rep, code := runBenchReport(t,
		[]string{fakeCmd(t), "--scenario", "set_model_config,images,soak,concurrency", "--iterations", "3", "--sessions", "2"},
		fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	for _, name := range []string{"set_model_config", "images", "soak", "concurrency"} {
		if r := verdictOf(rep, name); r.Verdict != VerdictOK {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if r := verdictOf(rep, "set_model_config"); !strings.Contains(jsonString(r.Details), "applied-and-verified") {
		t.Errorf("model switch not applied-and-verified: %+v", r.Details)
	}
	if r := verdictOf(rep, "soak"); r.Details["successRate"] != 1.0 {
		t.Errorf("soak rate: %+v", r.Details)
	}
	if r := verdictOf(rep, "soak"); r.Details["iters"] == nil {
		t.Errorf("soak missing per-iteration records: %+v", r.Details)
	}
}

func TestAllDefaultSkipsHonestly(t *testing.T) {
	// Default fake answers fast: cancel/steer/crash race, permission unasked.
	// --state-dir lets load_resume survive the respawn.
	state := t.TempDir()
	rep, code := runBenchReport(t, []string{fakeCmd(t, "--state-dir", state), "--mcp-stub", "--scenario", "all", "--iterations", "2", "--sessions", "2"}, fastEnv())
	if code != 0 {
		t.Fatalf("exit %d summary %+v", code, rep.Summary)
	}
	for _, name := range []string{"cancel", "steer_followup", "crash_recovery", "permission"} {
		if r := verdictOf(rep, name); r.Verdict != VerdictSkipped {
			t.Errorf("%s: got %q want skipped (%+v)", name, r.Verdict, r)
		}
	}
	for _, name := range []string{"initialize", "new_session", "prompt_text", "load_resume", "set_model_config", "images", "concurrency", "soak"} {
		if r := verdictOf(rep, name); r.Verdict != VerdictOK {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if rep.Memory.Samples == 0 || len(rep.Memory.Phases) == 0 {
		t.Errorf("empty memory section: %+v", rep.Memory)
	}
}

func TestEnvPassthroughAndFakeURL(t *testing.T) {
	rep, _ := runBenchReport(t,
		[]string{fakeCmd(t), "--scenario", "initialize", "--env", "FOO={{FAKE_LLM_URL}}/v1", "--fake-llm-url", "http://127.0.0.1:9"},
		fastEnv())
	raw, _ := json.Marshal(rep)
	if strings.Contains(string(raw), "127.0.0.1") {
		t.Fatal("report leaks env values")
	}
	keys, _ := rep.Agent["envKeys"].([]any)
	if len(keys) != 1 || keys[0] != "FOO" {
		t.Fatalf("envKeys: %v", rep.Agent["envKeys"])
	}
}

func TestMCPStubTools(t *testing.T) {
	cmd := exec.Command(testBin, "mcp-stub")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close(); _ = cmd.Wait() }()
	call := func(id int, method string, params any) map[string]any {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		stdin.Write(append(b, '\n'))
		rd := bufio.NewReader(stdout)
		for {
			line, err := rd.ReadBytes('\n')
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if json.Unmarshal(line, &m) != nil {
				continue
			}
			if m["id"] == float64(id) {
				return m
			}
		}
	}
	call(1, "initialize", map[string]any{"protocolVersion": "2024-11-05"})
	list := call(2, "tools/list", map[string]any{})
	res, _ := list["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	if !names["echo"] || !names["slow"] {
		t.Fatalf("tools: %v", names)
	}
	echo := call(3, "tools/call", map[string]any{"name": "echo", "arguments": map[string]any{"text": "hi"}})
	if !strings.Contains(jsonString(echo), "echo:hi") {
		t.Fatalf("echo: %v", echo)
	}
	bad := call(4, "tools/call", map[string]any{"name": "nope"})
	if _, ok := bad["error"]; !ok {
		t.Fatalf("unknown tool should error: %v", bad)
	}
	_ = time.Now()
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestAuthAutoAgainstRequireAuth(t *testing.T) {
	// Fake demands authenticate: auto picks the env/api-key method and the
	// session/new retry succeeds.
	rep, code := runBenchReport(t,
		[]string{fakeCmd(t, "--require-auth"), "--auth-method", "auto", "--scenario", "initialize,new_session,prompt_text"},
		fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	if r := verdictOf(rep, "initialize"); r.Verdict != VerdictOK || r.Details["authenticate"] != "ok" {
		t.Fatalf("auth not reported ok: %+v", r)
	}
	if r := verdictOf(rep, "prompt_text"); r.Verdict != VerdictOK {
		t.Fatalf("prompt after auth: %+v", r)
	}
}

func TestAuthRequiredWithoutFlagFailsNewSession(t *testing.T) {
	rep, _ := runBenchReport(t,
		[]string{fakeCmd(t, "--require-auth"), "--scenario", "new_session"},
		fastEnv())
	if r := verdictOf(rep, "new_session"); r.Verdict == VerdictOK {
		t.Fatalf("AUTH_REQUIRED should fail without --auth-method: %+v", r)
	}
}

func TestSetModelNoBogusID(t *testing.T) {
	// Fake advertises nothing: the probe must skip, never probe a bogus id.
	rep, _ := runBenchReport(t,
		[]string{fakeCmd(t, "--no-models", "--no-config-options"), "--scenario", "set_model_config"},
		fastEnv())
	if r := verdictOf(rep, "set_model_config"); r.Verdict != VerdictSkipped {
		t.Fatalf("want skipped without discoverable ids, got %+v", r)
	}
}

func TestToolPrefixNamespacing(t *testing.T) {
	rep, code := runBenchReport(t,
		[]string{fakeCmd(t, "--tool-prefix", "acpbench-stub__"), "--mcp-stub", "--scenario", "prompt_text"},
		fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	r := verdictOf(rep, "prompt_text")
	if r.Verdict != VerdictOK || r.Details["stubVerified"] != true {
		t.Fatalf("namespaced tool not derived/verified: %+v", r)
	}
}

func TestNewSessionMCPColumns(t *testing.T) {
	rep, code := runBenchReport(t,
		[]string{fakeCmd(t), "--mcp-stub", "--scenario", "new_session"},
		fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	r := verdictOf(rep, "new_session")
	if r.Details["notificationSeen"] != true || r.Details["toolListSeen"] != true {
		t.Fatalf("mcp columns missing: %+v", r.Details)
	}
}

func TestProviderFaultMatrix(t *testing.T) {
	for _, tc := range []struct {
		fault   string
		want    string
		timeout string
	}{
		{"http500", VerdictOK, ""},
		{"http429", VerdictOK, ""},
		{"midstream-error", VerdictOK, ""},
		{"reset", VerdictOK, ""},
		{"empty", VerdictSilentEmptyTurn, ""},
		{"truncate", VerdictWrongResult, ""},
		{"stall-forever", VerdictTimeoutHang, "2s"},
	} {
		env := fastEnv()
		if tc.timeout != "" {
			env = append(env, "ACPBENCH_PROMPT_TIMEOUT="+tc.timeout)
		}
		rep, _ := runBenchReport(t,
			[]string{fakeCmd(t), "--scenario", "provider_fault", "--fault", tc.fault}, env)
		if r := verdictOf(rep, "provider_fault"); r.Verdict != tc.want {
			t.Errorf("fault %s: got %+v want %s", tc.fault, r, tc.want)
		}
	}
}

func TestProviderFaultNeedsFlag(t *testing.T) {
	rep, code := runBenchReport(t, []string{fakeCmd(t), "--scenario", "provider_fault"}, fastEnv())
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if r := verdictOf(rep, "provider_fault"); r.Verdict != VerdictSkipped {
		t.Fatalf("want skipped without --fault: %+v", r)
	}
}

func TestSoakCancelMixAndRAM(t *testing.T) {
	cmd := fakeCmd(t, "--slow-ms", "15000", "--slow-match", "slow tool")
	rep, code := runBenchReport(t,
		[]string{cmd, "--scenario", "soak", "--iterations", "4", "--cancel-mix", "2"},
		fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	r := verdictOf(rep, "soak")
	if r.Verdict != VerdictOK {
		t.Fatalf("%+v", r)
	}
	iters, _ := r.Details["iters"].([]any)
	if len(iters) != 4 {
		t.Fatalf("want 4 per-iteration records, got %+v", r.Details)
	}
	if r.Details["cancelled"] != 2.0 && r.Details["cancelled"] != 2 {
		t.Fatalf("want 2 cancelled iters: %+v", r.Details)
	}
	if len(rep.Memory.Phases) == 0 {
		t.Fatalf("soak-only run has no RAM phases: %+v", rep.Memory)
	}
}

func TestLoadResumeLegs(t *testing.T) {
	state := t.TempDir()
	rep, code := runBenchReport(t,
		[]string{fakeCmd(t, "--state-dir", state), "--scenario", "load_resume"}, fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	r := verdictOf(rep, "load_resume")
	if r.Verdict != VerdictOK {
		t.Fatalf("%+v", r)
	}
	for _, leg := range []string{"resume", "loadSameProcess", "loadCrossProcess"} {
		if _, ok := r.Details[leg]; !ok {
			t.Errorf("missing leg %s: %+v", leg, r.Details)
		}
	}
}

func TestMCPTransportHTTP(t *testing.T) {
	rep, code := runBenchReport(t,
		[]string{fakeCmd(t), "--mcp-stub", "--mcp-transport", "http", "--scenario", "prompt_text,new_session"},
		fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	if r := verdictOf(rep, "prompt_text"); r.Verdict != VerdictOK || r.Details["stubVerified"] != true {
		t.Fatalf("http stub not verified: %+v", r)
	}
}

func TestStubLogFile(t *testing.T) {
	log := filepath.Join(t.TempDir(), "stub.log")
	rep, code := runBenchReport(t,
		[]string{fakeCmd(t), "--mcp-stub", "--stub-log", log, "--scenario", "prompt_text"},
		fastEnv())
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, rep.Scenarios)
	}
	raw, err := os.ReadFile(log)
	if err != nil || !strings.Contains(string(raw), "tools/call") {
		t.Fatalf("stub log missing tools/call: %v %q", err, string(raw))
	}
}

func TestCompareCommand(t *testing.T) {
	mk := func() string {
		rep, _ := runBenchReport(t, []string{fakeCmd(t), "--scenario", "prompt_text,soak", "--iterations", "2"}, fastEnv())
		f := filepath.Join(t.TempDir(), "r.json")
		b, _ := json.Marshal(rep)
		if err := os.WriteFile(f, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return f
	}
	f1, f2 := mk(), mk()
	cmd := exec.Command(testBin, "compare", f1, f2)
	cmd.Env = append(os.Environ(), fastEnv()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compare: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "prompt_text") || !strings.Contains(string(out), "soak") || !strings.Contains(string(out), "ram:") {
		t.Fatalf("compare table incomplete:\n%s", out)
	}
}

func TestSchemaVersion(t *testing.T) {
	rep, _ := runBenchReport(t, []string{fakeCmd(t), "--scenario", "initialize"}, fastEnv())
	if rep.SchemaVersion != schemaVersion {
		t.Fatalf("schemaVersion %d want %d", rep.SchemaVersion, schemaVersion)
	}
	if rep.Scenarios[0].LatencyMs < 0 {
		t.Fatalf("no per-scenario duration: %+v", rep.Scenarios[0])
	}
}
