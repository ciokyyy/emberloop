package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The launch file must hold the full multi-line prompt verbatim: newlines,
// quotes and shell metacharacters survive the round trip, because pane run
// can no longer type them.
func TestLaunchFileHoldsTheFullPromptVerbatim(t *testing.T) {
	isolateState(t)
	prompt := "line one\nline two with \"double\" and 'single' quotes\n$dollar `backtick` \\backslash\\ |pipe| &amp; ;semi"
	cmd, err := BuildCommand("pig", Session{Role: "work", Label: "01-x", Prompt: prompt})
	if err != nil {
		t.Fatal(err)
	}
	path, err := writeLaunch(Session{ID: "launch-1", Role: "work"}, cmd)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readLaunch(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range got.Args {
		if a == prompt {
			found = true
		}
	}
	if !found {
		t.Errorf("launch file must hold the prompt verbatim, got args %q", got.Args)
	}
	if got.Bin == "" || len(got.Env) == 0 {
		t.Errorf("launch file must hold the binary and env, got %+v", got)
	}
}

// The typed pane line is one shell-agnostic line: no newline even for a
// multi-line prompt, no env prefix (fish has no VAR=val form), just
// `wick-pig exec <file>`.
func TestPrepareLaunchTypesASingleExecLine(t *testing.T) {
	isolateState(t)
	prompt := "first line\nsecond line with \"quotes\" and $dollar"
	launch, err := PrepareLaunch("pig", Session{Role: "work", Label: "01-x", Prompt: prompt})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(launch.Line, "\n") {
		t.Errorf("pane run types one line, got %q", launch.Line)
	}
	if !strings.HasPrefix(launch.Line, "wick-pig exec ") {
		t.Errorf("the typed line must be the exec line, got %q", launch.Line)
	}
	if strings.Contains(launch.Line, "WICK_ROLE=") {
		t.Errorf("the typed line must carry no env assignments (fish has no VAR=val form), got %q", launch.Line)
	}
	if strings.Contains(launch.Line, prompt) {
		t.Errorf("the multi-line prompt must live in the launch file, not the typed line, got %q", launch.Line)
	}
	path, ok := execFileFromLine(launch.Line)
	if !ok {
		t.Fatalf("the exec line must parse to a launch file, got %q", launch.Line)
	}
	got, err := readLaunch(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range got.Args {
		if a == prompt {
			found = true
		}
	}
	if !found {
		t.Errorf("the launch file must hold the prompt verbatim, got %q", got.Args)
	}
}

// `wick-pig exec` runs the binary with the exact argv and env from the file,
// preserving newlines and quotes.
func TestExecRunsTheLaunchWithExactArgvAndEnv(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "argv")
	bin := filepath.Join(dir, "fake.sh")
	script := "#!/bin/sh\n" +
		"i=0\n" +
		"for a in \"$@\"; do echo \"arg$i=$a\" >> " + out + "; i=$((i+1)); done\n" +
		"echo \"env=$WICK_PROMPT\" >> " + out + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	isolateState(t)
	prompt := "say \"hi\"\nsecond line $HOME `tick`"
	cmd := Command{Bin: bin, Args: []string{"--", prompt}, Env: []string{"WICK_PROMPT=" + prompt}}
	path, err := writeLaunch(Session{ID: "exec-1"}, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunExecFile(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "arg1="+prompt) {
		t.Errorf("argv must arrive intact, got:\n%s", text)
	}
	if !strings.Contains(text, "env="+prompt) {
		t.Errorf("env must arrive intact, got:\n%s", text)
	}
}

// Every agent wick launches gets its full brief or prompt as its first
// message, instead of "Read <file> and follow it exactly". The brief and
// prompt files are still written, for the archive, but the agent is not told
// to read them.
func TestWorkerGetsItsFullBriefAsTheFirstMessage(t *testing.T) {
	for _, harness := range []string{"pig", "claude"} {
		t.Run(harness, func(t *testing.T) {
			statePath = filepath.Join(t.TempDir(), "workers.json")
			t.Setenv("HERDR_PANE_ID", "")
			t.Setenv("HERDR_TAB_ID", "")
			t.Setenv("WICK_AGENT_ID", "")
			var launched string
			var late string
			base := spawnToolFake()
			spawnExec = func(cmd string, args []string, o ExecOpts) (Result, error) {
				if len(args) > 2 && args[0] == "agent" && args[1] == "prompt" {
					late = args[3]
				}
				if len(args) > 1 && args[0] == "pane" && args[1] == "run" {
					launched = args[3]
				}
				return base(cmd, args, o)
			}
			t.Cleanup(func() { spawnExec = osExec })
			spawnGap = 0
			resolverSettings(t, "")
			t.Setenv("HERDR_WORKSPACE_ID", "w1")
			t.Setenv("WICK_HERDR_REPORT", "/fake/herdr-report")
			repo := t.TempDir()
			ticket := filepath.Join(repo, "20-x.md")
			if err := os.WriteFile(ticket, []byte("# x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			spawn, _ := spawnToolForRole(t, "implement")
			if _, err := spawn(repo, map[string]any{"agents": []any{
				map[string]any{"type": "worker", "ticket": ticket, "harness": harness},
			}}); err != nil {
				t.Fatalf("spawn: %v", err)
			}
			if launched == "" {
				t.Fatal("the worker was never launched")
			}
			if !strings.HasPrefix(launched, "wick-pig exec ") {
				t.Fatalf("the typed line must be the exec line, got %q", launched)
			}
			file, ok := execFileFromLine(launched)
			if !ok {
				t.Fatalf("no launch file in %q", launched)
			}
			cmd, err := readLaunch(file)
			if err != nil {
				t.Fatal(err)
			}
			// The brief file stays for the archive.
			brief, err := os.ReadFile(briefPath("20-x"))
			if err != nil {
				t.Fatal(err)
			}
			if harness == "claude" {
				// claude takes its first message after startup: the launch
				// holds the command without it, and Late is the full brief.
				for _, a := range cmd.Args {
					if strings.Contains(a, "Read ") && strings.Contains(a, "follow it exactly") {
						t.Errorf("claude launch args must not tell the agent to read a file, got %q", cmd.Args)
					}
				}
				if late != string(brief) {
					t.Errorf("claude's first message must be the full brief, got %q want %q", late, string(brief))
				}
				if strings.Contains(late, "follow it exactly") && strings.Contains(late, "Read ") {
					t.Errorf("claude's first message must not be a read indirection, got %q", late)
				}
				// The brief is multi-line and travels as one herdr
				// agent-prompt argument (DeliverLate), not typed text, so its
				// newlines must survive intact.
				if !strings.Contains(late, "\n") {
					t.Errorf("claude's first message must keep its newlines, got %q", late)
				}
			} else {
				found := false
				for _, a := range cmd.Args {
					if a == string(brief) {
						found = true
					}
					if strings.Contains(a, "Read ") && strings.Contains(a, "follow it exactly") {
						t.Errorf("the first message must be the brief itself, not a read indirection, got %q", a)
					}
				}
				if !found {
					t.Errorf("the launch must carry the full brief verbatim, got %q", cmd.Args)
				}
			}
			if !strings.Contains(string(brief), "Implement the ticket") {
				t.Errorf("brief must hold the worker instructions, got:\n%s", brief)
			}
		})
	}
}

// A scout's first message is the full rendered prompt, not a file to read.
func TestScoutGetsItsFullPromptAsTheFirstMessage(t *testing.T) {
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	ex := &scoutExec{g: g, writeOn: 1}
	spawnExec = ex.run
	spawn, _ := spawnToolForRole(t, "implement")
	if _, err := spawn("/repo", map[string]any{"agents": []any{
		map[string]any{"type": "scout", "prompt": "where is the auth check?"},
	}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if len(ex.lines) != 1 {
		t.Fatalf("launched %d scouts, want 1", len(ex.lines))
	}
	line := ex.lines[0]
	if !strings.HasPrefix(line, "wick-pig exec ") {
		t.Fatalf("the typed line must be the exec line, got %q", line)
	}
	if strings.Contains(line, "follow it exactly") {
		t.Errorf("the typed line must not tell the agent to read a file, got %q", line)
	}
	// The launch file must still exist after the launch returns: the pane's
	// `wick-pig exec` reads it at startup, so launchPlacedAgent may only drop
	// the MCP config right away (cleanupMCP), never the launch itself.
	file, ok := execFileFromLine(line)
	if !ok {
		t.Fatalf("no launch file in %q", line)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("the launch file must survive the launch for exec to read: %v", err)
	}
	cmd := launchCmd(line)
	joined := strings.Join(cmd.Args, "\n")
	if !strings.Contains(joined, "where is the auth check?") {
		t.Errorf("the launch must carry the scout's question, got %q", cmd.Args)
	}
	for _, a := range cmd.Args {
		if strings.Contains(a, "follow it exactly") && strings.Contains(a, ".prompt") {
			t.Errorf("the first message must be the prompt itself, not a read indirection, got %q", a)
		}
	}
}

// a worker removes its launch file as well as its MCP config.
func TestFinishLeavesNoLaunchFileBehind(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	if err := save(map[string]Worker{"01-x": {
		ID: "01-x", Repo: "/repo", Base: "main", Branch: "implement/01-x", Worktree: "/repo/wt", SessionID: "sess-1",
	}}); err != nil {
		t.Fatal(err)
	}
	launch, err := PrepareLaunch("pig", Session{ID: "sess-1", Role: "work", Label: "01-x", Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	file, ok := execFileFromLine(launch.Line)
	if !ok {
		t.Fatalf("no launch file in %s", launch.Line)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("launch was not written: %v", err)
	}
	rec := &recorder{reply: func(string, []string) Result { return Result{Stdout: "{}"} }}
	if _, err := Finish(rec.run, "01-x", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("launch file %s outlived the worker (err=%v)", file, err)
	}
}

// A scout or reviewer whose pane never opens must not leave its launch file
// behind: placement runs after PrepareLaunch wrote it.
func TestFailedPlacementLeavesNoLaunchFileBehind(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	rec := &recorder{reply: func(string, []string) Result { return Result{Code: 1, Stderr: "no herdr\n"} }}
	sess := readOnlySession(newSessionID(), t.TempDir(), "", "")
	if _, err := launchPlacedAgent(rec.run, readOnlyPlace{}, "pig", sess, "do the thing"); err == nil {
		t.Fatal("want the placement error")
	}
	if _, err := os.Stat(launchPath(sess)); !os.IsNotExist(err) {
		t.Errorf("launch file outlived the failed placement (err=%v)", err)
	}
}

// A launch that fails after the MCP config was written must not leave the
// config behind: PrepareLaunch removes it when the launch file cannot be
// written.
func TestFailedLaunchLeavesNoMCPConfigBehind(t *testing.T) {
	dir := t.TempDir()
	statePath = filepath.Join(dir, "sub", "workers.json")
	// A regular file where the launches directory goes makes writeLaunch
	// fail, after BuildCommand already wrote the MCP config.
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "launches"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := Session{ID: "sess-9", Role: "work", Prompt: "do it"}
	if _, err := PrepareLaunch("claude", sess); err == nil {
		t.Fatal("want the launch-file error")
	}
	if _, err := os.Stat(mcpConfigPath(sess)); !os.IsNotExist(err) {
		t.Errorf("MCP config outlived the failed launch (err=%v)", err)
	}
}

// A pig session logs how many tools pig sent with each provider request, so a
// session that says a tool is missing can be checked against what its provider
// received. Other harnesses and an explicit opt-out leave no log.
func TestPigSessionLogsToolCountsPerRequest(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "pig")
	script := "#!/bin/sh\n" +
		"if [ \"$PIG_DEBUG_TOOLS\" = 1 ]; then echo 'openai.Stream: sending 8 tools (tools=8)' >&2; echo 'openai.Stream: sending 7 tools (tools=7)' >&2; fi\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	isolateState(t)
	run := func(bin, session string) string {
		cmd := Command{Bin: bin, Env: []string{"WICK_SESSION_ID=" + session}}
		path, err := writeLaunch(Session{ID: session}, cmd)
		if err != nil {
			t.Fatal(err)
		}
		if err := RunExecFile(path); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(filepath.Dir(statePath), "debug", session+".tools.log")
	}

	raw, err := os.ReadFile(run(fake, "tools-1"))
	if err != nil {
		t.Fatalf("a pig session must leave a tools log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "tools=8") || !strings.Contains(lines[1], "tools=7") {
		t.Fatalf("log must hold one line per request, got %q", raw)
	}
	if !strings.HasPrefix(lines[0], "20") || !strings.Contains(lines[0], "Z ") {
		t.Errorf("each line must lead with a timestamp, got %q", lines[0])
	}

	t.Setenv("WICK_PIG_TOOLS_LOG", "0")
	if _, err := os.Stat(run(fake, "tools-off")); err == nil {
		t.Error("WICK_PIG_TOOLS_LOG=0 must leave no log")
	}
	t.Setenv("WICK_PIG_TOOLS_LOG", "")
	other := filepath.Join(dir, "claude")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(run(other, "tools-claude")); err == nil {
		t.Error("only pig sessions log tool counts")
	}
}

// With the strip off, a pig session still logs its tool counts: the two
// switches stay independent.
func TestPigToolsLogWithoutStrip(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "pig")
	script := "#!/bin/sh\n" +
		"if [ \"$PIG_DEBUG_TOOLS\" = 1 ]; then echo 'openai.Stream: sending 8 tools (tools=8)' >&2; fi\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	isolateState(t)
	t.Setenv("WICK_HEADER_STRIP", "0")
	cmd := Command{Bin: fake, Env: []string{"WICK_SESSION_ID=nostrip"}}
	path, err := writeLaunch(Session{ID: "nostrip"}, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunExecFile(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(statePath), "debug", "nostrip.tools.log"))
	if err != nil {
		t.Fatalf("WICK_HEADER_STRIP=0 must keep the tools log: %v", err)
	}
	if !strings.Contains(string(raw), "tools=8") {
		t.Errorf("log must hold the child's line, got %q", raw)
	}
}

// The launcher is the agent's parent: when the child exits badly it records
// the exit in the agent's state, so a crash leaves a trace instead of a bare
// shell. Under the header strip the wrapper owns the terminal, so only the
// code is recorded there.
func TestExecRecordsChildExitInAgentState(t *testing.T) {
	isolateState(t)
	path, err := writeLaunch(Session{ID: "s1", AgentID: "dead1"}, Command{
		Bin:  "sh",
		Args: []string{"-c", "echo last words >&2; exit 3"},
		Env:  []string{"WICK_AGENT_ID=dead1", "WICK_SESSION_ID=s1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := RunExecFile(path); err == nil {
		t.Fatal("a child exiting 3 must fail the exec")
	}
	s, ok, err := ReadAgentState("dead1")
	if err != nil || !ok {
		t.Fatalf("the child's exit must be recorded: %v %v", s, err)
	}
	if s.ExitCode == nil || *s.ExitCode != 3 {
		t.Errorf("exit code = %v, want 3", s.ExitCode)
	}
}

// A pig session keeps the child's stderr on its own pipe under the strip and
// on the process's stderr without it, so either way its trace keeps the last
// lines too. The pig-named script takes the tools-log path without needing a
// real pig binary.
func TestExecRecordsChildStderrUnwrapped(t *testing.T) {
	isolateState(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "pig")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho last words >&2\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := writeLaunch(Session{ID: "s3", AgentID: "dead3"}, Command{
		Bin:  bin,
		Args: []string{},
		Env:  []string{"WICK_AGENT_ID=dead3", "WICK_SESSION_ID=s3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := RunExecFile(path); err == nil {
		t.Fatal("a child exiting 3 must fail the exec")
	}
	s, ok, err := ReadAgentState("dead3")
	if err != nil || !ok {
		t.Fatalf("the child's exit must be recorded: %v %v", s, err)
	}
	if s.ExitCode == nil || *s.ExitCode != 3 {
		t.Errorf("exit code = %v, want 3", s.ExitCode)
	}
	if !strings.Contains(s.ExitStderr, "last words") {
		t.Errorf("stderr tail = %q, want the child's last words", s.ExitStderr)
	}
}

// A child killed by a signal records the signal, not just a code.
func TestExecRecordsChildSignalInAgentState(t *testing.T) {
	isolateState(t)
	path, err := writeLaunch(Session{ID: "s2", AgentID: "dead2"}, Command{
		Bin:  "sh",
		Args: []string{"-c", "kill -TERM $$"},
		Env:  []string{"WICK_AGENT_ID=dead2", "WICK_SESSION_ID=s2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := RunExecFile(path); err == nil {
		t.Fatal("a child killed by TERM must fail the exec")
	}
	s, ok, err := ReadAgentState("dead2")
	if err != nil || !ok {
		t.Fatalf("the child's death must be recorded: %v %v", s, err)
	}
	if s.ExitSignal == "" {
		t.Errorf("no signal recorded: %+v", s)
	}
}
