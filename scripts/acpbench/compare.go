package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// startHTTPStub launches "self mcp-stub --http 127.0.0.1:0 --stub-log PATH"
// for --mcp-transport http|sse agents and returns its /mcp URL.
func startHTTPStub(selfExe, stubLog string) (string, *os.Process, error) {
	cmd := exec.Command(selfExe, "mcp-stub", "--http", "127.0.0.1:0", "--stub-log", stubLog)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", nil, err
	}
	urlCh := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "MCP_URL ") {
				urlCh <- strings.TrimSpace(strings.TrimPrefix(line, "MCP_URL "))
				return
			}
		}
		urlCh <- ""
	}()
	select {
	case url := <-urlCh:
		if url == "" {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return "", nil, fmt.Errorf("http stub printed no MCP_URL")
		}
		return url, cmd.Process, nil
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", nil, fmt.Errorf("http stub did not print MCP_URL in time")
	}
}

func stopHTTPStub(p *os.Process) {
	if p == nil {
		return
	}
	_ = p.Kill()
}

// runCompare implements "acpbench compare a.json b.json ...": a markdown
// table of agents x scenarios x RAM on stdout.
func runCompare(files []string) int {
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "acpbench compare: need at least one report JSON")
		return 2
	}
	type rep struct {
		label string
		rep   BenchReport
	}
	var reps []rep
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "acpbench compare: read", f, err)
			return 1
		}
		var r BenchReport
		if err := json.Unmarshal(raw, &r); err != nil {
			fmt.Fprintln(os.Stderr, "acpbench compare: parse", f, err)
			return 1
		}
		if r.SchemaVersion != 0 && r.SchemaVersion != schemaVersion {
			fmt.Fprintf(os.Stderr, "acpbench compare: %s schemaVersion %d (want %d)\n", f, r.SchemaVersion, schemaVersion)
			return 1
		}
		reps = append(reps, rep{label: agentLabel(f, r), rep: r})
	}
	// Union of scenario names in registry order, then any extras.
	seen := map[string]bool{}
	var names []string
	for _, s := range scenarioRegistry {
		for _, r := range reps {
			if _, ok := scenByName(r.rep, s.name); ok && !seen[s.name] {
				seen[s.name] = true
				names = append(names, s.name)
			}
		}
	}
	for _, r := range reps {
		for _, s := range r.rep.Scenarios {
			if !seen[s.Name] {
				seen[s.Name] = true
				names = append(names, s.Name)
			}
		}
	}
	var sb strings.Builder
	sb.WriteString("| scenario |")
	for _, r := range reps {
		sb.WriteString(" " + mdEscape(r.label) + " |")
	}
	sb.WriteString("\n| --- |")
	for range reps {
		sb.WriteString(" --- |")
	}
	sb.WriteString("\n")
	for _, n := range names {
		sb.WriteString("| " + n + " |")
		for _, r := range reps {
			sb.WriteString(" " + cellFor(r.rep, n) + " |")
		}
		sb.WriteString("\n")
	}
	// RAM rows: peak RSS of the prompt-turn/concurrency phases + marginal.
	sb.WriteString("| ram: prompt-turn peak |")
	for _, r := range reps {
		sb.WriteString(" " + ramCell(r.rep, "prompt-turn") + " |")
	}
	sb.WriteString("\n| ram: concurrency peak |")
	for _, r := range reps {
		sb.WriteString(" " + ramCell(r.rep, "concurrency-multi") + " |")
	}
	sb.WriteString("\n| ram: per-session marginal |")
	for _, r := range reps {
		sb.WriteString(" " + marginalCell(r.rep) + " |")
	}
	sb.WriteString("\n")
	fmt.Print(sb.String())
	return 0
}

func agentLabel(file string, r BenchReport) string {
	if cmd, ok := r.Agent["cmd"].([]any); ok && len(cmd) > 0 {
		base := fmt.Sprint(cmd[0])
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		return fmt.Sprintf("%s (%s)", base, file)
	}
	return file
}

func scenByName(r BenchReport, name string) (ScenarioResult, bool) {
	for _, s := range r.Scenarios {
		if s.Name == name {
			return s, true
		}
	}
	return ScenarioResult{}, false
}

func cellFor(r BenchReport, name string) string {
	s, ok := scenByName(r, name)
	if !ok {
		return "—"
	}
	return fmt.Sprintf("%s (%dms)", s.Verdict, s.LatencyMs)
}

func ramCell(r BenchReport, phase string) string {
	st, ok := r.Memory.Phases[phase]
	if !ok || st.MaxRSS == 0 {
		return "—"
	}
	return fmt.Sprintf("%dMB", st.MaxRSS/1024/1024)
}

func marginalCell(r BenchReport) string {
	if r.Memory.PerSessionMarginalBytes == nil {
		return "—"
	}
	return fmt.Sprintf("%dMB", *r.Memory.PerSessionMarginalBytes/1024/1024)
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
