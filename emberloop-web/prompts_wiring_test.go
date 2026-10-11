package web

import (
	"strings"
	"testing"
)

// claudeSystemPromptFor carries the role extra ahead of the project context:
// a web Session names remember, a worker and a reviewer never do.
func TestClaudeSystemPromptForCarriesRoleExtra(t *testing.T) {
	work := tempDir(t)
	web := claudeSystemPromptFor(SessionMeta{Owner: "u"}, work, tempDir(t))
	if !strings.Contains(strings.ToLower(web), "remember") {
		t.Errorf("web system prompt lacks remember: %q", web)
	}
	worker := claudeSystemPromptFor(SessionMeta{Owner: "u", GroupID: "g"}, work, tempDir(t))
	if strings.Contains(strings.ToLower(worker), "remember") {
		t.Errorf("worker system prompt names remember: %q", worker)
	}
	if !strings.Contains(worker, "AGENTS.md") {
		t.Errorf("worker system prompt lacks AGENTS.md: %q", worker)
	}
	reviewer := claudeSystemPromptFor(SessionMeta{Owner: "u", ParentID: "run", Reviewer: reviewerSpecType}, work, tempDir(t))
	if strings.Contains(strings.ToLower(reviewer), "remember") {
		t.Errorf("reviewer system prompt names remember: %q", reviewer)
	}
	scout := claudeSystemPromptFor(SessionMeta{Owner: "u", GroupID: "g", Scout: true}, work, tempDir(t))
	if strings.Contains(strings.ToLower(scout), "remember") {
		t.Errorf("scout system prompt names remember: %q", scout)
	}
}

// sessionMetaFor carries the role extra into the ACP session params: web
// names remember, reviewers never do even though their el role is web.
func TestSessionMetaForCarriesRoleExtra(t *testing.T) {
	b := newStubClaudeBackend(nil)
	webMeta := b.sessionMetaFor(SessionMeta{Owner: "u", Model: "sonnet"}, ownerDirs{})
	web, _ := webMeta["systemPrompt"].(string)
	if !strings.Contains(strings.ToLower(web), "remember") {
		t.Errorf("web session meta lacks remember: %q", web)
	}
	reviewerMeta := b.sessionMetaFor(SessionMeta{Owner: "u", Model: "sonnet", ParentID: "run", Reviewer: reviewerStandardsType}, ownerDirs{})
	reviewer, _ := reviewerMeta["systemPrompt"].(string)
	if strings.Contains(strings.ToLower(reviewer), "remember") {
		t.Errorf("reviewer session meta names remember: %q", reviewer)
	}
	if !strings.Contains(reviewer, "AGENTS.md") {
		t.Errorf("reviewer session meta lacks AGENTS.md: %q", reviewer)
	}
}

// workerTaskPrompt is the Run worker's first prompt: the ticket task with no
// memory/DOX text of its own (the system prompt already carries the worker
// extra, so repeating it would inject it twice) and no remember instruction.
// A seeded worktree adds one commit line for the seeded root AGENTS.md, so an
// untracked seed never trips the merge's dirty-worktree refusal.
func TestWorkerTaskPromptCarriesDox(t *testing.T) {
	p := workerTaskPrompt("27-x", "spec", "tickets/27-x.md", "implement/27-x", false)
	if !strings.Contains(p, "implement/27-x") || !strings.Contains(p, "tickets/27-x.md") {
		t.Errorf("worker task lacks branch or ticket: %q", p)
	}
	if strings.Contains(p, workerRolePrompt()) {
		t.Errorf("worker task repeats the system-prompt extra: %q", p)
	}
	if strings.Contains(p, "commit it with your work") {
		t.Errorf("unseeded task carries the seed line: %q", p)
	}
	if strings.Contains(strings.ToLower(p), "remember") {
		t.Errorf("worker task names remember: %q", p)
	}
	seeded := workerTaskPrompt("27-x", "spec", "tickets/27-x.md", "implement/27-x", true)
	if !strings.Contains(seeded, "commit it with your work") {
		t.Errorf("seeded task lacks the commit line: %q", seeded)
	}
	if strings.Contains(seeded, workerRolePrompt()) {
		t.Errorf("seeded task repeats the system-prompt extra: %q", seeded)
	}
	if strings.Contains(strings.ToLower(seeded), "remember") {
		t.Errorf("seeded task names remember: %q", seeded)
	}
}

// reviewerPrompt carries no memory/DOX text of its own: the Reviewer's system
// prompt already carries the reviewer extra, so repeating it would inject it
// twice. It keeps the VERDICT format both reviewer types parse.
func TestReviewerPromptCarriesNoExtra(t *testing.T) {
	for _, typ := range []string{reviewerSpecType, reviewerStandardsType} {
		p := reviewerPrompt("27-x", "spec", typ, "branch", "tip")
		if strings.Contains(p, reviewerRolePrompt()) {
			t.Errorf("%s prompt repeats the system-prompt extra: %q", typ, p)
		}
		if !strings.Contains(p, "VERDICT: pass") {
			t.Errorf("%s prompt lost the verdict format: %q", typ, p)
		}
	}
}

// Pig agent Sessions carry the role extra as an appended system-prompt
// section; one-off calls append nothing.
func TestPigResourcesCarryRoleExtra(t *testing.T) {
	t.Setenv("PIG_TEST_FAUX", "1")
	svcs, _, err := buildPigServices(tempDir(t), nil, SessionMeta{Model: "test-faux/faux-1", WorkDir: tempDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer svcs.Close()
	worker, err := ownerResourcesExtra(svcs, workerRolePrompt())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, text := range worker.GetAppendSystemPrompt() {
		if strings.Contains(text, "AGENTS.md") {
			found = true
		}
		if strings.Contains(strings.ToLower(text), "remember") {
			t.Errorf("worker appended prompt names remember: %q", text)
		}
	}
	if !found {
		t.Errorf("worker appended prompt lacks AGENTS.md: %q", worker.GetAppendSystemPrompt())
	}
	plain, err := ownerResources(svcs)
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.GetAppendSystemPrompt()) != 0 {
		t.Errorf("plain resources append %q, want nothing", plain.GetAppendSystemPrompt())
	}
}
