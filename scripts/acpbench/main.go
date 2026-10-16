package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// scenarioFn is one probe. The registry order is the "all" run order.
var scenarioRegistry = []struct {
	name string
	fn   func(*benchCfg) ScenarioResult
}{
	{"initialize", scInitialize},
	{"new_session", scNewSession},
	{"prompt_text", scPromptText},
	{"cancel", scCancel},
	{"steer_followup", scSteer},
	{"load_resume", scLoadResume},
	{"set_model_config", scSetModel},
	{"images", scImages},
	{"permission", scPermission},
	{"crash_recovery", scCrash},
	{"concurrency", scConcurrency},
	{"soak", scSoak},
	{"provider_fault", scProviderFault},
}

func scenarioNames() []string {
	out := make([]string, 0, len(scenarioRegistry))
	for _, s := range scenarioRegistry {
		out = append(out, s.name)
	}
	return out
}

type envList []string

func (e *envList) String() string     { return strings.Join(*e, ",") }
func (e *envList) Set(v string) error { *e = append(*e, v); return nil }

func usage() {
	fmt.Fprintf(os.Stderr, `acpbench %s: black-box ACP client probe/benchmark.

 Usage:
   acpbench --cmd '<argv>' [--env K=V ...] [--cwd DIR] [--mcp-stub]
            [--mcp-transport stdio|http|sse] [--stub-log FILE]
            [--scenario all|<name>] [--iterations K] [--sessions N]
            [--report out.json] [--fake-llm-url URL] [--cooperate]
            [--echo-directive TEXT] [--auth-method ID|auto] [--meta-json FILE]
            [--pre-prompt-wait-mcp] [--init-timeout D] [--turn-timeout D]
            [--kill-group] [--cancel-mix N] [--fault-rate R] [--fault CLASS]
   acpbench mcp-stub [--stub-log FILE] [--http ADDR]  # hidden MCP server
   acpbench fake-agent [flags...]    # in-repo fake ACP agent for self-tests
   acpbench compare a.json b.json ...  # markdown table agents x scenarios x RAM

 Scenarios:
   %s

 No real model APIs are ever called and no credentials are read: only --env
 pairs are passed through, and {{FAKE_LLM_URL}} in their values is replaced
 with --fake-llm-url.
 `, version, strings.Join(scenarioNames(), ", "))
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "mcp-stub" {
		os.Exit(runMCPStub())
	}
	if len(os.Args) > 1 && os.Args[1] == "fake-agent" {
		os.Exit(runFakeAgent(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "compare" {
		os.Exit(runCompare(os.Args[2:]))
	}
	os.Exit(runBench(os.Args[1:]))
}

// splitArgv splits a command line on whitespace honoring single and double
// quotes and backslash escapes.
func splitArgv(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	var quote rune
	esc := false
	inTok := false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\' && quote != '\'':
			esc = true
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '\'' || r == '"'):
			quote = r
			inTok = true
		case quote == 0 && (r == ' ' || r == '\t' || r == '\n'):
			if inTok {
				out = append(out, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			cur.WriteRune(r)
			inTok = true
		}
	}
	if esc {
		return nil, fmt.Errorf("trailing backslash in --cmd")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in --cmd")
	}
	if inTok {
		out = append(out, cur.String())
	}
	return out, nil
}

func runBench(args []string) int {
	applyTimeoutEnv()
	fs := flag.NewFlagSet("acpbench", flag.ContinueOnError)
	cmdStr := fs.String("cmd", "", "agent argv to spawn (quoted, e.g. --cmd 'npx -y pkg')")
	var envs envList
	fs.Var(&envs, "env", "extra env K=V (repeatable)")
	cwd := fs.String("cwd", "", "working directory for the agent")
	mcpStub := fs.Bool("mcp-stub", false, "offer the built-in stub MCP server (echo, slow)")
	mcpTransport := fs.String("mcp-transport", "stdio", "stub transport: stdio|http|sse")
	stubLog := fs.String("stub-log", "", "stub server-side invocation log (default: temp file)")
	scenario := fs.String("scenario", "all", "all or one scenario name")
	iterations := fs.Int("iterations", 5, "soak iterations")
	sessions := fs.Int("sessions", 3, "concurrency sessions")
	report := fs.String("report", "", "write JSON report here (default stdout)")
	fakeURL := fs.String("fake-llm-url", "", "replaces {{FAKE_LLM_URL}} in --env values")
	cooperate := fs.Bool("cooperate", false, "use cooperative prompts (English + TOOL directives)")
	echoDirective := fs.String("echo-directive", "", "extra directive text appended to scripted echo prompts")
	authMethod := fs.String("auth-method", "", "authenticate after initialize: explicit id or 'auto'")
	metaJSON := fs.String("meta-json", "", "JSON file merged into session/new _meta")
	prePromptWaitMCP := fs.Bool("pre-prompt-wait-mcp", false, "wait for MCP-ready before each session's first prompt")
	initTimeoutFlag := fs.String("init-timeout", "", "initialize timeout (Go duration, overrides ACPBENCH_INIT_TIMEOUT)")
	turnTimeoutFlag := fs.String("turn-timeout", "", "per-turn prompt timeout (Go duration, overrides ACPBENCH_PROMPT_TIMEOUT)")
	killGroupFlag := fs.Bool("kill-group", false, "kill the whole agent process group on teardown")
	cancelMix := fs.Int("cancel-mix", 0, "soak: every Nth iteration cancels mid-stream (0 disables)")
	faultRate := fs.String("fault-rate", "", "recorded fault rate hint; also exported as ACPBENCH_FAULT_RATE for fakellm --faults")
	fault := fs.String("fault", "", "provider_fault class (http429|http500|http529|midstream-error|stall-5s|stall-forever|truncate|reset|empty)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if d, err := time.ParseDuration(*initTimeoutFlag); err == nil && *initTimeoutFlag != "" && d > 0 {
		initTimeout = d
	} else if *initTimeoutFlag != "" {
		fmt.Fprintln(os.Stderr, "acpbench: bad --init-timeout:", *initTimeoutFlag)
		return 2
	}
	if d, err := time.ParseDuration(*turnTimeoutFlag); err == nil && *turnTimeoutFlag != "" && d > 0 {
		promptTimeout = d
	} else if *turnTimeoutFlag != "" {
		fmt.Fprintln(os.Stderr, "acpbench: bad --turn-timeout:", *turnTimeoutFlag)
		return 2
	}
	killGroup = *killGroupFlag
	switch *mcpTransport {
	case "stdio", "http", "sse":
	default:
		fmt.Fprintln(os.Stderr, "acpbench: bad --mcp-transport (want stdio|http|sse):", *mcpTransport)
		return 2
	}
	argv, err := splitArgv(*cmdStr)
	if err != nil || len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "acpbench: bad --cmd:", err)
		return 2
	}
	if *cwd == "" {
		*cwd, _ = os.Getwd()
	}
	// Build the agent environment: host env plus --env overrides, with the
	// fake-LLM substitution. Only keys named on the command line are added;
	// report records key names, never values.
	envMap := map[string]string{}
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			envMap[kv[:i]] = kv[i+1:]
		}
	}
	var envKeys []string
	for _, kv := range envs {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			fmt.Fprintln(os.Stderr, "acpbench: bad --env (want K=V):", kv)
			return 2
		}
		v := strings.ReplaceAll(kv[i+1:], fakeLLMTokenName, *fakeURL)
		envMap[kv[:i]] = v
		envKeys = append(envKeys, kv[:i])
	}
	// --fault-rate is a passthrough hint for fakellm --faults configs: the
	// agent (backed by fakellm) reads ACPBENCH_FAULT_RATE. Documented only;
	// the probe records it and soaks normally.
	if *faultRate != "" {
		envMap["ACPBENCH_FAULT_RATE"] = *faultRate
	}
	fullEnv := make([]string, 0, len(envMap))
	for k, v := range envMap {
		fullEnv = append(fullEnv, k+"="+v)
	}
	selfExe, _ := os.Executable()
	if selfExe == "" {
		selfExe = os.Args[0]
	}
	// --meta-json merges a file into session/new _meta (e.g. Daemon
	// claudeCode options replication).
	var metaExtra map[string]any
	if *metaJSON != "" {
		raw, err := os.ReadFile(*metaJSON)
		if err != nil {
			fmt.Fprintln(os.Stderr, "acpbench: read --meta-json:", err)
			return 2
		}
		if err := json.Unmarshal(raw, &metaExtra); err != nil {
			fmt.Fprintln(os.Stderr, "acpbench: parse --meta-json:", err)
			return 2
		}
	}
	// Stub-side log: default temp file so verification always has proof.
	stubLogPath := *stubLog
	if *mcpStub && stubLogPath == "" {
		tf, err := os.CreateTemp("", "acpbench-stub-*.log")
		if err != nil {
			fmt.Fprintln(os.Stderr, "acpbench: stub log temp:", err)
			return 2
		}
		tf.Close()
		stubLogPath = tf.Name()
	}

	var wanted []string
	if *scenario == "all" {
		wanted = scenarioNames()
	} else {
		for _, part := range strings.Split(*scenario, ",") {
			wanted = append(wanted, strings.TrimSpace(part))
		}
	}
	fns := map[string]func(*benchCfg) ScenarioResult{}
	for _, s := range scenarioRegistry {
		fns[s.name] = s.fn
	}
	for _, w := range wanted {
		if _, ok := fns[w]; !ok {
			fmt.Fprintln(os.Stderr, "acpbench: unknown scenario:", w)
			return 2
		}
	}

	mem := newMemWatcher(250 * time.Millisecond)
	stopMem := make(chan struct{})
	go mem.run(stopMem)
	cfg := &benchCfg{
		argv: argv, env: fullEnv, cwd: *cwd, mcpStub: *mcpStub,
		mcpTransport: *mcpTransport, stubLog: stubLogPath,
		selfExe: selfExe, iterations: *iterations, sessions: *sessions,
		cooperate: *cooperate, fakeLLMURL: *fakeURL, echoDirective: *echoDirective,
		authMethod: *authMethod, metaJSON: metaExtra, prePromptWaitMCP: *prePromptWaitMCP,
		cancelMix: *cancelMix, faultRate: *faultRate, fault: *fault,
		mem: mem, phases: map[string][]memWindow{},
	}
	// http/sse stub: serve it once for the whole run, pass the URL.
	if *mcpStub && (*mcpTransport == "http" || *mcpTransport == "sse") {
		url, proc, err := startHTTPStub(selfExe, stubLogPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "acpbench: start http stub:", err)
			return 2
		}
		cfg.httpStubURL = url
		cfg.httpStubLog = stubLogPath
		defer stopHTTPStub(proc)
	}
	started := time.Now()
	var results []ScenarioResult
	for _, w := range wanted {
		r := fns[w](cfg)
		results = append(results, r)
		fmt.Fprintf(os.Stderr, "[acpbench] %-16s %s (%dms)\n", w, r.Verdict, r.LatencyMs)
	}
	close(stopMem)

	phaseStats := map[string]phaseStat{}
	for name := range cfg.phases {
		phaseStats[name] = cfg.statPhase(name)
	}
	var marginal *int64
	cfg.mu.Lock()
	marginal = cfg.marginal
	cfg.mu.Unlock()
	rep := BenchReport{
		Tool: versionTool(), Version: version, SchemaVersion: schemaVersion, StartedAt: started,
		DurationMs: ms(time.Since(started)),
		Agent: map[string]any{
			"cmd": argv, "cwd": *cwd, "mcpStub": *mcpStub, "mcpTransport": *mcpTransport,
			"envKeys": envKeys, "authMethod": *authMethod, "fault": *fault,
			"cancelMix": *cancelMix, "faultRate": *faultRate,
		},
		Scenarios: results,
		Memory: MemoryReport{
			SampleIntervalMs: 250, Samples: mem.sampleCount(),
			Phases: phaseStats, PerSessionMarginalBytes: marginal,
			Note: "RSS summed over each agent process tree via /proc task/children walk; phases are merged windows (last=most recent, max=peak).",
		},
		Summary: summarize(results),
	}
	if marginal != nil {
		rep.Memory.PerSessionMarginalBytes = marginal
	}
	b, _ := json.MarshalIndent(rep, "", "  ")
	if *report == "" {
		fmt.Println(string(b))
	} else {
		if dir := filepath.Dir(*report); dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
		if err := os.WriteFile(*report, append(b, '\n'), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "acpbench: write report:", err)
			return 1
		}
		fmt.Fprintln(os.Stderr, "[acpbench] report:", *report)
	}
	for _, r := range results {
		if r.Verdict != VerdictOK && r.Verdict != VerdictSkipped {
			return 1
		}
	}
	return 0
}

func versionTool() string { return "acpbench" }
