package emberloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellArgQuotesOnlyWhenNeeded(t *testing.T) {
	if got := shellArg(""); got != `""` {
		t.Errorf("an empty argument must survive the shell, got %q", got)
	}
	if got := shellArg("--no-skills"); got != "--no-skills" {
		t.Errorf("a plain flag must not be quoted, got %q", got)
	}
	if got := shellArg("Read /a b/c.md and follow it."); !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Errorf("an argument with spaces must be quoted, got %q", got)
	}
	if got := shellArg(`say "hi"`); strings.Count(got, `"`) != 4 {
		t.Errorf("embedded quotes must be escaped, got %q", got)
	}
}

func TestWriteBriefPointsAtSkillsWithoutCopyingThem(t *testing.T) {
	t.Setenv("EMBERLOOP_STATE", filepath.Join(t.TempDir(), "workers.json"))

	path, err := writeBrief("01-probe", "/repo/.scratch/p/issues/01-probe.md", "/repo/.scratch/p/spec.md", "spec/p", "pig")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)

	for _, want := range []string{"emberloop://matt-pocock/implement/SKILL.md", "emberloop://matt-pocock/tdd/SKILL.md"} {
		if !strings.Contains(text, want) {
			t.Errorf("brief must name %s:\n%s", want, text)
		}
	}
	implement := strings.Index(text, "emberloop://matt-pocock/implement/SKILL.md")
	tdd := strings.Index(text, "emberloop://matt-pocock/tdd/SKILL.md")
	if !(implement < tdd) {
		t.Errorf("order must be implement -> tdd, got %d/%d", implement, tdd)
	}
	if strings.Contains(text, "/skills/matt-pocock") {
		t.Errorf("brief must not name an absolute library path:\n%s", text)
	}
	if strings.Contains(text, "Commit your work to the current branch") {
		t.Errorf("brief copied skill content instead of referencing it:\n%s", text)
	}
}

// The brief carries what the worker needs beyond the skills: the spec its
// ticket belongs to, a spawn of both reviewer types in place of the
// code-review skill's subagents, a rebase onto the target before the last
// review, how to decline a finding it cannot fix, and that ticket files stay
// untouched while completed checkboxes ride the submit's completed list.
func TestBriefNamesSpecReviewRebaseAndFollowUps(t *testing.T) {
	t.Setenv("EMBERLOOP_STATE", filepath.Join(t.TempDir(), "workers.json"))
	path, err := writeBrief("01-probe", "/repo/.scratch/p/issues/01-probe.md", "/repo/.scratch/p/spec.md", "spec/p", "pig")
	if err != nil {
		t.Fatal(err)
	}
	text := readFile(t, path)
	for _, want := range []string{"/repo/.scratch/p/spec.md", "reviewer-standards", "reviewer-spec", "spawn(agents=", "git rebase spec/p", "Follow-ups", "Never edit ticket files", "completed"} {
		if !strings.Contains(text, want) {
			t.Errorf("brief must carry %q:\n%s", want, text)
		}
	}
	rebase := strings.Index(text, "git rebase")
	if rebase < 0 || !strings.Contains(text[:rebase], "final review") {
		t.Errorf("the rebase must come before the final review:\n%s", text)
	}

	bare, err := writeBrief("02-probe", "/repo/t.md", "", "", "pig")
	if err != nil {
		t.Fatal(err)
	}
	if text := readFile(t, bare); strings.Contains(text, "spec at") || strings.Contains(text, "git rebase") {
		t.Errorf("no spec and no target means neither is mentioned:\n%s", text)
	}
}

// Every harness reads skills through the read tool under one emberloop:// address:
// no session loads the library itself. The skill references are the same for
// every harness; only the root-doc line differs (ticket 15 probe).
func TestBriefIsTheSameForEveryHarness(t *testing.T) {
	t.Setenv("EMBERLOOP_STATE", filepath.Join(t.TempDir(), "workers.json"))
	pig, err := writeBrief("01-probe", "/repo/.scratch/p/issues/01-probe.md", "", "", "pig")
	if err != nil {
		t.Fatal(err)
	}
	claude, err := writeBrief("02-probe", "/repo/.scratch/p/issues/01-probe.md", "", "", "claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{pig, claude} {
		text := readFile(t, path)
		if !strings.Contains(text, "emberloop://matt-pocock/implement/SKILL.md") {
			t.Errorf("brief must name the implement skill by its emberloop:// address:\n%s", text)
		}
		if strings.Contains(text, "/skills/matt-pocock") {
			t.Errorf("brief must not name an absolute library path:\n%s", text)
		}
	}
}

// Probe finding (ticket 15): the pig harness loads the root AGENTS.md as a
// context file, so its workers need no extra instruction; the stripped
// claude harness disables setting sources and CLAUDE.md loading, so its
// workers must be told to read the root AGENTS.md first.
func TestBriefCarriesRootDocLineOnlyWhereNeeded(t *testing.T) {
	t.Setenv("EMBERLOOP_STATE", filepath.Join(t.TempDir(), "workers.json"))
	pig, err := writeBrief("01-probe", "/repo/.scratch/p/issues/01-probe.md", "", "", "pig")
	if err != nil {
		t.Fatal(err)
	}
	if text := readFile(t, pig); strings.Contains(text, "root AGENTS.md") {
		t.Errorf("pig already loads the root AGENTS.md, its brief must not repeat it:\n%s", text)
	}
	claude, err := writeBrief("02-probe", "/repo/.scratch/p/issues/02-probe.md", "", "", "claude")
	if err != nil {
		t.Fatal(err)
	}
	text := readFile(t, claude)
	if !strings.Contains(text, "root AGENTS.md") {
		t.Errorf("claude loads neither context file, its brief must tell the worker to read the root AGENTS.md first:\n%s", text)
	}
	if first, second := strings.Index(text, "AGENTS.md"), strings.Index(text, "Implement the ticket"); first < 0 || second < 0 || !(first < second) {
		t.Errorf("the root-doc line must come before the ticket instructions so it is read first:\n%s", text)
	}
}

func TestBackgroundRuleReachesBrief(t *testing.T) {
	t.Setenv("EMBERLOOP_STATE", filepath.Join(t.TempDir(), "workers.json"))

	path, err := writeBrief("01-probe", "/repo/.scratch/p/issues/01-probe.md", "", "", "pig")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "background") {
		t.Errorf("a worker brief must carry the background rule, it only lives in the system prompt:\n%s", body)
	}
}

var testRun = SpecRun{Spec: "/repo/.scratch/p/spec.md", Tickets: "/repo/.scratch/p/issues", Branch: "spec/p"}

// The merger never reviews: each worker already spawned its own reviewers on
// its work, and a second review per worker only spends the merger's tokens.
func TestImplementPromptDoesNotReview(t *testing.T) {
	got := implementPrompt(testRun)
	if strings.Contains(got, "review_worker") || strings.Contains(strings.ToLower(got), "verdict") {
		t.Errorf("the merger prompt must not ask for a review:\n%s", got)
	}
	if !strings.Contains(got, "Skip the skill's final code-review step") {
		t.Errorf("the prompt must drop implement-spec's final review:\n%s", got)
	}
}

// The implement session runs the upstream skill, adapted: it is pointed at the
// skill by address, and every subagent the skill names maps onto a emberloop tool.
func TestImplementPromptAdaptsImplementSpec(t *testing.T) {
	got := implementPrompt(testRun)
	for _, want := range []string{
		"emberloop://matt-pocock/implement-spec/SKILL.md",
		"/repo/.scratch/p/spec.md", "/repo/.scratch/p/issues", "spec/p",
		"worker", "scout", "finish", "tell", "end your turn",
		"carry on", "follow-ups",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("implement prompt missing %q:\n%s", want, got)
		}
	}
}

// A worker that did not merge is sent back, not handed to a human: the
// worker still has its context and its worktree.
func TestRenderFinishSendsAKeptWorkerBack(t *testing.T) {
	got := renderFinish(&FinishResult{ID: "01-x", Kept: true, Detail: "rebase onto spec/p conflicted in a.go"})
	if !strings.Contains(got, "tell") || !strings.Contains(got, "a.go") || strings.Contains(got, "human") {
		t.Errorf("got %q", got)
	}
}

func TestRenderFinishIsProse(t *testing.T) {
	if got := renderFinish(&FinishResult{ID: "01-a", Merged: true, SHA: "abc123"}); !strings.Contains(got, "abc123") {
		t.Errorf("a merge must report the sha, got %q", got)
	}
	kept := renderFinish(&FinishResult{ID: "01-a", Detail: "rebase failed", Kept: true})
	if !strings.Contains(kept, "NOT merge") || !strings.Contains(kept, "rebase failed") {
		t.Errorf("a conflict must be reported and escalated, got %q", kept)
	}
}

func TestReportExtensionPrefersTheEnvVar(t *testing.T) {
	t.Setenv("EMBERLOOP_HERDR_REPORT", "/exts/from-env")
	if got := reportExtension("/opt/emberloop/bin/pig", "/home/cioky"); got != "/exts/from-env" {
		t.Errorf("a set env var must win over any directory on disk, got %q", got)
	}
}

func TestReportExtensionFallsBackToTheFirstDirectoryThatExists(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "opt", "emberloop", "bin")
	mk := func(p string) string {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Setenv("EMBERLOOP_HERDR_REPORT", "")
	exe := filepath.Join(bin, "pig")

	if got := reportExtension(exe, home); got != "" {
		t.Errorf("with nothing on disk the lookup must return empty, got %q", got)
	}

	homeExt := mk(filepath.Join(home, ".pig", "agent", "extensions", "herdr-report"))
	if got := reportExtension(exe, home); got != homeExt {
		t.Errorf("with only the home extension present, got %q want %q", got, homeExt)
	}

	beside := mk(filepath.Join(bin, "herdr-report"))
	if got := reportExtension(exe, home); got != beside {
		t.Errorf("the extension beside the executable must beat the home one, got %q", got)
	}

	above := mk(filepath.Join(root, "opt", "emberloop", "herdr-report"))
	if got := reportExtension(exe, home); got != above {
		t.Errorf("../herdr-report must be the first candidate, got %q", got)
	}
}

// reportExtensionWarning is the spawn-side counterpart to reportExtension: it
// must warn only for the harness that needs the extension, and only when the
// lookup really comes up empty.
func TestReportExtensionWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("EMBERLOOP_HERDR_REPORT", "")

	if got := reportExtensionWarning("pig"); !strings.Contains(got, "unknown") {
		t.Errorf("a pig spawn with no extension must warn, got %q", got)
	}
	if got := reportExtensionWarning("claude"); got != "" {
		t.Errorf("only pig registers through the extension, got %q", got)
	}

	ext := filepath.Join(home, ".pig", "agent", "extensions", "herdr-report")
	if err := os.MkdirAll(ext, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := reportExtensionWarning("pig"); got != "" {
		t.Errorf("a found extension must not warn, got %q", got)
	}
}

// Ticket completion moved into finish: the orchestrator prompt no longer asks
// the orchestrator to set a merged ticket's status or tick its boxes. Its
// allowed-paths ability stays, for specs and notes.
func TestImplementPromptLeavesTicketsToFinish(t *testing.T) {
	got := implementPrompt(testRun)
	for _, want := range []string{"orchestrator_paths"} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt must keep the orchestrator's allowed-paths ability, missing %q:\n%s", want, got)
		}
	}
	for _, stale := range []string{"tick its checkboxes", "set its ticket's Status", "Status line to done"} {
		if strings.Contains(got, stale) {
			t.Errorf("the prompt must not ask the orchestrator to complete tickets, found %q:\n%s", stale, got)
		}
	}
}

// The brief carries the DOX pass: read the root-to-target AGENTS.md chain
// before editing, and update the owning doc after a contract-changing edit.
// Review enforces it; the brief only tells the worker to do it.
func TestBriefCarriesDoxReadAndUpdateRules(t *testing.T) {
	t.Setenv("EMBERLOOP_STATE", filepath.Join(t.TempDir(), "workers.json"))
	path, err := writeBrief("01-probe", "/repo/.scratch/p/issues/01-probe.md", "/repo/.scratch/p/spec.md", "spec/p", "pig")
	if err != nil {
		t.Fatal(err)
	}
	text := readFile(t, path)
	for _, want := range []string{"AGENTS.md", "Child DOX Index", "owning AGENTS.md"} {
		if !strings.Contains(text, want) {
			t.Errorf("brief must carry the DOX pass (%q):\n%s", want, text)
		}
	}
	before := strings.Index(text, "Before editing")
	index := strings.Index(text, "Child DOX Index")
	if before < 0 || index < 0 || !(before < index) {
		t.Errorf("the chain must be read before editing:\n%s", text)
	}
}

// Ticket 18: the worker brief carries one positive execute direction: search
// and process content with execute.
func TestBriefDirectsExecute(t *testing.T) {
	t.Setenv("EMBERLOOP_STATE", filepath.Join(t.TempDir(), "workers.json"))
	path, err := writeBrief("01-probe", "/repo/.scratch/p/issues/01-probe.md", "/repo/.scratch/p/spec.md", "spec/p", "pig")
	if err != nil {
		t.Fatal(err)
	}
	text := readFile(t, path)
	const line = "Search and process content with `execute`"
	if got := strings.Count(text, line); got != 1 {
		t.Errorf("brief must carry the execute direction exactly once, found %d:\n%s", got, text)
	}
}
