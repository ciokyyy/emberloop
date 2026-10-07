package herdrreport

import (
	"strings"
	"testing"
)

func TestReportArgsOmitSessionFlagsUntilThereIsASession(t *testing.T) {
	// A session file does not exist until the first turn has run, so the early
	// reports must not carry flags built from an empty string.
	early := reportArgs("w1:p1", "pig", "idle", 5, "", "")
	if idx := strings.Index(strings.Join(early, " "), "--agent-session-id"); idx != -1 {
		t.Errorf("report with no session id must not pass --agent-session-id: %v", early)
	}
	if !hasPair(early, "--seq", "5") {
		t.Errorf("seq must be present and ordered: %v", early)
	}

	late := reportArgs("w1:p1", "pig", "working", 6, "sess-1", "/tmp/s.jsonl")
	if !hasPair(late, "--agent-session-id", "sess-1") {
		t.Errorf("session id missing: %v", late)
	}
	if !hasPair(late, "--agent-session-path", "/tmp/s.jsonl") {
		t.Errorf("session path missing: %v", late)
	}
}

func TestReleaseArgsMatchTheSourceThatReported(t *testing.T) {
	// herdr drops the agent only if the release names the same source as the
	// report; a mismatch leaves a stale entry in agent list.
	rep := reportArgs("w1:p1", "pig", "working", 9, "s", "/p")
	rel := releaseArgs("w1:p1", "pig", 10)
	repSource, relSource := flagValue(rep, "--source"), flagValue(rel, "--source")
	if repSource == "" || repSource != relSource {
		t.Errorf("release source %q must equal report source %q", relSource, repSource)
	}
	if flagValue(rel, "--agent") != "pig" {
		t.Errorf("release must name the same agent: %v", rel)
	}
	// A release must never carry session flags, or herdr treats it as a
	// different agent's handover.
	if strings.Contains(strings.Join(rel, " "), "--agent-session") {
		t.Errorf("release must not carry session flags: %v", rel)
	}
}

func TestPaneIDRequiresHerdrEnv(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	t.Setenv("HERDR_ENV", "")
	if _, ok := paneID(); ok {
		t.Error("a pane outside herdr has no pane id and must report no agent")
	}
	t.Setenv("HERDR_ENV", "1")
	pane, ok := paneID()
	if !ok || pane != "w1:p1" {
		t.Errorf("paneID() = %q, %v; want w1:p1, true", pane, ok)
	}
	t.Setenv("HERDR_PANE_ID", "")
	if _, ok := paneID(); ok {
		t.Error("HERDR_ENV=1 with no pane id must not report an agent")
	}
}

func TestLabelFallsBackWhenUnset(t *testing.T) {
	t.Setenv("EMBERLOOP_AGENT_LABEL", "")
	if got := label(); got != defaultLabel {
		t.Errorf("label() = %q, want the default %q", got, defaultLabel)
	}
	t.Setenv("EMBERLOOP_AGENT_LABEL", "w01-probe")
	if got := label(); got != "w01-probe" {
		t.Errorf("label() = %q, want w01-probe", got)
	}
}

func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func hasPair(args []string, flag, want string) bool {
	return flagValue(args, flag) == want
}
