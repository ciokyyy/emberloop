package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Seam: pure builders plus the Daemon HTTP API with the fake Backend. Prompt
// content is observed through built prompts and live Session transcripts,
// never through internals.

// trimRunSkill drops optional HTML blocks and (optional) markdown sections.
func TestTrimRunSkillDropsOptional(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		keep []string
		drop []string
	}{
		{
			name: "html block",
			in:   "core\n<!-- optional:start -->\noptional body\n<!-- optional:end -->\nmore core",
			keep: []string{"core", "more core"},
			drop: []string{"optional body"},
		},
		{
			name: "heading section",
			in:   "# Skill\nCore line.\n\n## Advanced (optional)\nDrop this.\nStill drop.\n\n## Core again\nKeep this.",
			keep: []string{"Core line.", "Core again", "Keep this."},
			drop: []string{"Drop this."},
		},
		{
			name: "unclosed heading drops to end",
			in:   "Keep.\n\n## Extra (Optional)\nDrop to end.",
			keep: []string{"Keep."},
			drop: []string{"Drop to end."},
		},
		{
			name: "unclosed html drops to end",
			in:   "Keep.\n<!-- optional:start -->\nDrop.",
			keep: []string{"Keep."},
			drop: []string{"Drop."},
		},
		{
			name: "uppercase markers with non-ascii kept intact",
			in:   "keep İ\n<!-- OPTIONAL:START -->\ndrop\n<!-- OPTIONAL:END -->\nmore",
			keep: []string{"keep İ", "more"},
			drop: []string{"drop"},
		},
		{
			name: "hash comment in fence is not a heading",
			in:   "Head.\n\n```sh\n# run (optional)\necho hi\n```\n\n## Real (optional)\nDrop me.\n\n## Back\nKeep me.",
			keep: []string{"Head.", "# run (optional)", "echo hi", "Back", "Keep me."},
			drop: []string{"Drop me."},
		},
		{
			name: "sublevel stays dropped",
			in:   "# S\nKeep.\n\n## Opt (optional)\nDrop.\n\n### Sub\nStill drop.\n\n## Back\nKeep back.",
			keep: []string{"Keep.", "Back", "Keep back."},
			drop: []string{"Still drop."},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := trimRunSkill(tc.in)
			for _, want := range tc.keep {
				if !strings.Contains(got, want) {
					t.Errorf("trimmed %q lacks %q", got, want)
				}
			}
			for _, bad := range tc.drop {
				if strings.Contains(got, bad) {
					t.Errorf("trimmed %q still holds %q", got, bad)
				}
			}
		})
	}
}

// The built-in briefs are the first skill entries per role.
func TestRunSkillsFirstEntriesAreBriefs(t *testing.T) {
	t.Setenv("EL_SETTINGS", filepath.Join(tempDir(t), "missing.json"))
	if got := runSkillsFor(reviewerSpecType); len(got) == 0 || got[0] != specBrief {
		t.Fatalf("spec skills[0] = %.60q, want the spec brief", strings.Join(got, "\n"))
	}
	if got := runSkillsFor(reviewerStandardsType); len(got) == 0 || got[0] != standardsBrief {
		t.Fatalf("standards skills[0] = %.60q, want the standards brief", strings.Join(got, "\n"))
	}
	worker := runSkillsFor("worker")
	if len(worker) < 2 || worker[0] != specBrief || worker[1] != standardsBrief {
		t.Fatalf("worker skills = %d entries, want both briefs first", len(worker))
	}
	// The reviewerPrompt brief is the same first entry, not a second copy:
	// the injected first prompt carries it once via the skills list.
	if !strings.Contains(reviewerPrompt("01-a", "spec.md", reviewerSpecType, "b", "abc"), specBrief) {
		t.Fatal("reviewerPrompt lost its spec brief")
	}
	if !strings.Contains(reviewerPrompt("01-a", "spec.md", reviewerStandardsType, "b", "abc"), standardsBrief) {
		t.Fatal("reviewerPrompt lost its standards brief")
	}
}

func TestDesignerBrowserSkillsAndCodeMention(t *testing.T) {
	t.Setenv("EL_SETTINGS", filepath.Join(tempDir(t), "missing.json"))
	designer := runSkillsFor("designer")
	designReviewer := runSkillsFor(reviewerDesignType)
	for name, skills := range map[string][]string{"designer": designer, "design reviewer": designReviewer} {
		if len(skills) == 0 || !strings.Contains(skills[len(skills)-1], "Browser skill (local UI only)") {
			t.Errorf("%s lacks the default browser skill: %#v", name, skills)
		}
	}
	for _, role := range []string{"worker", reviewerSpecType, reviewerStandardsType} {
		skills := runSkillsFor(role)
		count := 0
		for _, skill := range skills {
			if skill == browserMention {
				count++
			}
			if strings.Contains(skill, "Browser skill (local UI only)") {
				t.Errorf("%s got design browser section", role)
			}
		}
		if count != 1 {
			t.Errorf("%s mention count = %d, want one", role, count)
		}
	}
	if got := runSkillsFor("designer"); !strings.Contains(strings.Join(got, "\n"), "Snapshot first") || !strings.Contains(strings.Join(got, "\n"), "localhost only") {
		t.Fatalf("designer skill lacks core rules: %q", got)
	}
}

// Custom skills append after the briefs, trimmed, with alias keys merged.
func TestRunSkillsCustomAppendAndTrim(t *testing.T) {
	settings := filepath.Join(tempDir(t), "settings.json")
	raw, _ := json.Marshal(map[string]any{
		"run_skills": map[string]any{
			"worker":             []string{"worker core\n## Extra (optional)\ndrop me"},
			"spec-review":        []string{"spec custom <!-- optional:start -->drop<!-- optional:end --> keep"},
			"reviewer-standards": []string{"std custom"},
		},
	})
	if err := os.WriteFile(settings, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)

	worker := runSkillsFor("worker")
	if len(worker) != 4 || !strings.Contains(worker[3], "worker core") || strings.Contains(worker[3], "drop me") {
		t.Fatalf("worker custom = %q, want trimmed core without the optional section", worker)
	}
	spec := runSkillsFor(reviewerSpecType)
	if len(spec) != 3 || !strings.Contains(spec[2], "keep") || strings.Contains(spec[2], "drop") {
		t.Fatalf("spec custom = %q, want trimmed alias skill", spec)
	}
	// The canonical key wins the same way: both keys merge.
	std := runSkillsFor(reviewerStandardsType)
	if len(std) != 3 || !strings.Contains(std[2], "std custom") {
		t.Fatalf("standards custom = %q, want custom after default skills", std)
	}

}

// run_skills round-trips through the settings API and validates shapes.
func TestRunSkillsSettingsRoundTrip(t *testing.T) {
	srv, settings := settingsServer(t, `{"roles":{}}`)

	out, code := putSettingsBody(t, srv.URL, `{"run_skills":{"worker":["w1"],"reviewer-spec":["s1"],"reviewer-standards":["t1"]}}`)
	if code != http.StatusOK {
		t.Fatalf("put skills status = %d, want 200", code)
	}
	_ = out
	resp, err := http.Get(srv.URL + "/api/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var view struct {
		RunSkills map[string][]string `json:"run_skills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.RunSkills["worker"][0] != "w1" || view.RunSkills["reviewer-spec"][0] != "s1" {
		t.Fatalf("view run_skills = %+v, want the written skills", view.RunSkills)
	}
	raw, _ := os.ReadFile(settings)
	if !strings.Contains(string(raw), `"run_skills"`) {
		t.Fatalf("settings file lacks run_skills: %s", raw)
	}

	// A malformed shape is refused and keeps the file.
	if _, code := putSettingsBody(t, srv.URL, `{"run_skills":{"worker":"not-an-array"}}`); code != http.StatusBadRequest {
		t.Fatalf("bad shape status = %d, want 400", code)
	}
	rawAfter, _ := os.ReadFile(settings)
	if string(raw) != string(rawAfter) {
		t.Fatalf("file moved on a refused write: %s vs %s", raw, rawAfter)
	}

	// An explicit empty list clears one role; other roles stay.
	if _, code := putSettingsBody(t, srv.URL, `{"run_skills":{"worker":[]}}`); code != http.StatusOK {
		t.Fatalf("clear status = %d, want 200", code)
	}
	resp2, _ := http.Get(srv.URL + "/api/settings")
	var view2 struct {
		RunSkills map[string][]string `json:"run_skills"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&view2)
	resp2.Body.Close()
	if _, ok := view2.RunSkills["worker"]; ok {
		t.Fatalf("worker skills = %+v, want cleared", view2.RunSkills)
	}
	if view2.RunSkills["reviewer-spec"][0] != "s1" {
		t.Fatalf("spec skills after clear = %+v, want kept", view2.RunSkills)
	}
}

// run_skills is host-global prompt text injected into every Run, so like
// signup, decider and jev it writes admin-only.
func TestRunSkillsAreAdminOnly(t *testing.T) {
	settings := filepath.Join(tempDir(t), "settings.json")
	if err := os.WriteFile(settings, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "alice")
	bob := signedIn(t, h, "bob")
	put := func(cookies ...*http.Cookie) int {
		return authDo(t, h, http.MethodPut, "/api/settings", `{"run_skills":{"worker":["w1"]}}`, cookies...).Code
	}
	if code := put(bob); code != http.StatusForbidden {
		t.Fatalf("plain User writes run_skills = %d, want 403", code)
	}
	if code := put(); code != http.StatusUnauthorized {
		t.Fatalf("anonymous writes run_skills = %d, want 401", code)
	}
	if code := put(admin); code != http.StatusOK {
		t.Fatalf("admin writes run_skills = %d, want 200", code)
	}
}

// Worker and Reviewer first prompts carry the Ticket, spec, AGENTS.md chain
// and per-role skills.
func TestFirstPromptsCarryInjectionPerRole(t *testing.T) {
	t.Setenv("EL_SETTINGS", filepath.Join(tempDir(t), "missing.json"))
	repo := tempDir(t)
	writeFile(t, repo, "AGENTS.md", "# AGENTS.md — demo root\n")
	writeFile(t, repo, "sub/AGENTS.md", "# AGENTS.md — sub chain\n")
	ticketPath := filepath.Join(repo, "ticket.md")
	specPath := filepath.Join(repo, "spec.md")
	if err := os.WriteFile(ticketPath, []byte("# 01: Ticket\n\nTicket body alpha.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("# Spec\n\nSpec body beta.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(repo, "wt")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "AGENTS.md"), []byte("# AGENTS.md — worktree\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	worker := buildWorkerFirstPrompt("01-x", specPath, ticketPath, "implement/01-x", false, worktree, "worker")
	for _, want := range []string{"Ticket body alpha", "Spec body beta", "AGENTS.md", "worker", "Spec axis.", "Standards axis."} {
		if !strings.Contains(worker, want) {
			t.Errorf("worker prompt lacks %q:\n%s", want, worker)
		}
	}
	if !strings.Contains(worker, ticketPath) || !strings.Contains(worker, specPath) {
		t.Errorf("worker prompt lacks the ticket or spec path:\n%s", worker)
	}
	if strings.Count(worker, "agent-browser skills get core") != 1 || strings.Contains(worker, "Browser skill (local UI only)") {
		t.Errorf("code Worker has wrong browser guidance: %q", worker)
	}
	designerPrompt := buildWorkerFirstPrompt("01-x", specPath, ticketPath, "implement/01-x", false, worktree, "worker-design")
	if !strings.Contains(designerPrompt, "Snapshot first") || !strings.Contains(designerPrompt, "localhost only") || strings.Contains(designerPrompt, "agent-browser skills get core") {
		t.Errorf("Designer prompt has wrong browser skill: %q", designerPrompt)
	}
	designReviewerPrompt := buildReviewerFirstPrompt("01-x", specPath, ticketPath, reviewerDesignType, "b", "tip", worktree, repo, "")
	if !strings.Contains(designReviewerPrompt, "Browser skill (local UI only)") || !strings.Contains(designReviewerPrompt, "a11y") {
		t.Errorf("design Reviewer prompt lacks browser skill: %q", designReviewerPrompt)
	}


	spec := buildReviewerFirstPrompt("01-x", specPath, ticketPath, reviewerSpecType, "b", "tip", worktree, repo, "")
	for _, want := range []string{"Ticket body alpha", "Spec body beta", "AGENTS.md", "Spec axis."} {
		if !strings.Contains(spec, want) {
			t.Errorf("spec reviewer prompt lacks %q:\n%s", want, spec)
		}
	}
	if strings.Contains(spec, "Smell baseline") {
		t.Errorf("spec reviewer prompt carries the standards brief:\n%s", spec)
	}

	std := buildReviewerFirstPrompt("01-x", specPath, ticketPath, reviewerStandardsType, "b", "tip", worktree, repo, "")
	for _, want := range []string{"Ticket body alpha", "Spec body beta", "AGENTS.md", "Standards axis.", "Smell baseline"} {
		if !strings.Contains(std, want) {
			t.Errorf("standards reviewer prompt lacks %q:\n%s", want, std)
		}
	}
	if strings.Contains(std, "Spec axis.") {
		t.Errorf("standards reviewer prompt carries the spec brief:\n%s", std)
	}
}

// The reviewer chain follows the changed files; the worker chain is the root
// chain when no diff exists yet.
func TestAgentsChainFollowsChangedPaths(t *testing.T) {
	t.Parallel()
	repo := tempDir(t)
	testGit(t, repo, "init", "-b", "main", ".")
	writeFile(t, repo, "AGENTS.md", "# AGENTS.md — root\n")
	writeFile(t, repo, "sub/AGENTS.md", "# AGENTS.md — sub\n")
	writeFile(t, repo, "sub/file.txt", "v1\n")
	testGit(t, repo, "add", "-A")
	testGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))
	writeFile(t, repo, "sub/file.txt", "v2\n")
	testGit(t, repo, "commit", "-am", "change sub")
	tip := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))

	worktree := filepath.Join(repo, "wt")
	testGit(t, repo, "worktree", "add", "--detach", worktree, tip)
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", worktree) })

	changed := reviewChangedFiles(repo, base, tip)
	if len(changed) != 1 || changed[0] != "sub/file.txt" {
		t.Fatalf("changed = %q, want [sub/file.txt]", changed)
	}
	chain := agentsChainForReviewer(worktree, repo, base, tip)
	joined := ""
	for _, cf := range chain {
		joined += cf.Path + "\n" + cf.Content + "\n"
	}
	if !strings.Contains(joined, "sub") {
		t.Fatalf("reviewer chain misses the sub chain:\n%s", joined)
	}
	rootChain := agentsChainForWorker(worktree)
	if len(rootChain) == 0 {
		t.Fatal("worker root chain empty, want the worktree chain")
	}
}

// The Run seam: a live Worker and both Reviewers receive the injected first
// prompt over the Daemon HTTP API with the fake Backend.
func TestRunFirstPromptsCarryInjectionAtSeam(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	settings := filepath.Join(tempDir(t), "settings.json")
	raw, _ := json.Marshal(map[string]any{
		"run_skills": map[string]any{
			"worker": []string{"worker-skill-marker\n## Opt (optional)\ndrop"},
		},
	})
	if err := os.WriteFile(settings, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	// The Session exists before its first prompt lands; read the transcript
	// only once that prompt is in, or a loaded machine reads it empty.
	firstPromptWhere(t, srv.URL, worker, func(Event) bool { return true })
	wt := sessTranscript(getSession(t, srv.URL, worker))
	specRaw, _ := os.ReadFile(spec)
	for _, want := range []string{
		"alpha", string(specRaw), "AGENTS.md", "Spec axis.", "Standards axis.",
		"worker-skill-marker",
	} {
		if !strings.Contains(wt, want) {
			t.Fatalf("worker transcript lacks %q:\n%.2000s", want, wt)
		}
	}
	if strings.Contains(wt, "drop") && strings.Contains(wt, "Opt (optional)") {
		t.Fatalf("worker transcript carries the untrimmed optional section:\n%.2000s", wt)
	}

	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	for typ, sess := range revs {
		firstPromptWhere(t, srv.URL, sess, func(Event) bool { return true })
		tr := sessTranscript(getSession(t, srv.URL, sess))
		for _, want := range []string{"alpha", string(specRaw), "AGENTS.md"} {
			if !strings.Contains(tr, want) {
				t.Fatalf("%s transcript lacks %q:\n%.2000s", typ, want, tr)
			}
		}
		switch typ {
		case reviewerSpecType:
			if !strings.Contains(tr, "Spec axis.") || strings.Contains(tr, "Smell baseline") {
				t.Fatalf("spec reviewer transcript has the wrong brief:\n%.2000s", tr)
			}
		case reviewerStandardsType:
			if !strings.Contains(tr, "Standards axis.") || !strings.Contains(tr, "Smell baseline") {
				t.Fatalf("standards reviewer transcript has the wrong brief:\n%.2000s", tr)
			}
		}
	}
}
