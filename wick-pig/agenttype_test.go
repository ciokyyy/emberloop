package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAgentFile(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, agentTypeDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The built-in scout ships in wick: a none-checkout, blocking, read-only type
// that answers with status "answered".
func TestBuiltinScoutTypeLoads(t *testing.T) {
	types, err := LoadAgentTypes(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	scout, ok := types["scout"]
	if !ok {
		t.Fatal("the built-in scout did not load")
	}
	if scout.Checkout != "none" || !scout.Blocking {
		t.Errorf("scout checkout=%q blocking=%v", scout.Checkout, scout.Blocking)
	}
	if got := strings.Join(scout.Statuses, ","); got != "answered" {
		t.Errorf("scout statuses = %q", got)
	}
	for _, tool := range scout.Tools {
		if tool != "read" && tool != "find" {
			t.Errorf("a none type may only read and find, scout has %q", tool)
		}
	}
	if scout.Description == "" || scout.Body == "" {
		t.Error("scout must carry a description and a body")
	}
}

// The built-in worker ships in wick: a worktree, non-blocking type that
// reports done or blocked and can edit and run code.
func TestBuiltinWorkerTypeLoads(t *testing.T) {
	types, err := LoadAgentTypes(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	worker, ok := types["worker"]
	if !ok {
		t.Fatal("the built-in worker did not load")
	}
	if worker.Checkout != "worktree" || worker.Blocking {
		t.Errorf("worker checkout=%q blocking=%v, want worktree and non-blocking", worker.Checkout, worker.Blocking)
	}
	if got := strings.Join(worker.Statuses, ","); got != "done,blocked" {
		t.Errorf("worker statuses = %q, want done,blocked", got)
	}
	for _, want := range []string{"read", "edit", "write", "bash", "find", "background"} {
		if !contains(worker.Tools, want) {
			t.Errorf("a worker must be able to %s, tools = %v", want, worker.Tools)
		}
	}
}

// The built-in worker names the reviewer types it spawns and the ones whose
// pass opens its finish gate.
func TestBuiltinWorkerNamesItsReviewers(t *testing.T) {
	types, err := LoadAgentTypes(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	worker := types["worker"]
	if got := strings.Join(worker.Spawns, ","); got != "reviewer-standards,reviewer-spec" {
		t.Errorf("worker spawns = %q, want both reviewer types", got)
	}
	if got := strings.Join(worker.RequiresReview, ","); got != "reviewer-standards,reviewer-spec" {
		t.Errorf("worker requires_review = %q, want both reviewer types", got)
	}
}

// Both built-in reviewer types ship in wick: parent checkout, blocking,
// persistent, read-only, with pass and changes statuses.
func TestBuiltinReviewerTypesLoad(t *testing.T) {
	types, err := LoadAgentTypes(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{reviewerStandards, reviewerSpec} {
		r, ok := types[name]
		if !ok {
			t.Fatalf("the built-in %s did not load", name)
		}
		if r.Checkout != "parent" || !r.Blocking || !r.Persistent {
			t.Errorf("%s checkout=%q blocking=%v persistent=%v, want parent/blocking/persistent",
				name, r.Checkout, r.Blocking, r.Persistent)
		}
		if got := strings.Join(r.Statuses, ","); got != "pass,changes" {
			t.Errorf("%s statuses = %q, want pass,changes", name, got)
		}
		for _, tool := range r.Tools {
			if tool != "read" && tool != "find" {
				t.Errorf("%s is read-only, so it may only read and find, has %q", name, tool)
			}
		}
		if r.Description == "" || r.Body == "" {
			t.Errorf("%s must carry a description and a body", name)
		}
	}
}

// A repo file adds a type the built-ins do not have.
func TestRepoAddsAnAgentType(t *testing.T) {
	root := t.TempDir()
	writeAgentFile(t, root, "librarian.md", `---
name: librarian
description: Find the right document.
tools: [read, find]
checkout: none
blocking: true
statuses: [answered]
---
Find it.
`)
	types, err := LoadAgentTypes(root)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := types["librarian"]
	if !ok {
		t.Fatal("the repo type did not load")
	}
	if got.Description != "Find the right document." || got.Body != "Find it." {
		t.Errorf("repo type = %+v", got)
	}
}

// A type may name its own harness, model and effort; these are the type layer
// the resolver falls back to when a spawn entry says nothing.
func TestRepoTypeCarriesHarnessModelAndEffort(t *testing.T) {
	root := t.TempDir()
	writeAgentFile(t, root, "tuned.md", `---
name: tuned
description: A tuned implementer.
tools: [read, edit]
checkout: worktree
blocking: false
statuses: [done]
harness: claude
model: opus
effort: high
---
Do the work.
`)
	types, err := LoadAgentTypes(root)
	if err != nil {
		t.Fatal(err)
	}
	got := types["tuned"]
	if got.Harness != "claude" || got.Model != "opus" || got.Effort != "high" {
		t.Errorf("frontmatter = harness %q model %q effort %q, want claude/opus/high", got.Harness, got.Model, got.Effort)
	}
}

// A repo file named like a built-in overrides it by name, fields and body.
func TestRepoFileOverridesABuiltin(t *testing.T) {
	root := t.TempDir()
	writeAgentFile(t, root, "scout.md", `---
name: scout
description: A tuned scout for this project.
tools: [read]
checkout: none
blocking: true
statuses: [done]
---
Project rules.
`)
	types, err := LoadAgentTypes(root)
	if err != nil {
		t.Fatal(err)
	}
	scout := types["scout"]
	if scout.Description != "A tuned scout for this project." || scout.Body != "Project rules." {
		t.Errorf("the override did not win: %+v", scout)
	}
	if got := strings.Join(scout.Statuses, ","); got != "done" {
		t.Errorf("overridden statuses = %q", got)
	}
}

// A required field left out fails loading, and the error names the file and
// the field.
func TestMissingFieldNamesTheFileAndField(t *testing.T) {
	root := t.TempDir()
	writeAgentFile(t, root, "broken.md", `---
description: no name here
tools: [read, find]
checkout: none
blocking: true
statuses: [answered]
---
`)
	_, err := LoadAgentTypes(root)
	if err == nil {
		t.Fatal("a file with no name must fail loading")
	}
	if !strings.Contains(err.Error(), "broken.md") || !strings.Contains(err.Error(), "name") {
		t.Errorf("the error must name the file and the field, got %v", err)
	}
}

// An invalid field value is refused the same way.
func TestInvalidFieldNamesTheFileAndField(t *testing.T) {
	root := t.TempDir()
	writeAgentFile(t, root, "broken.md", `---
name: broken
description: a bad checkout
tools: [read, find]
checkout: elsewhere
blocking: true
statuses: [answered]
---
`)
	_, err := LoadAgentTypes(root)
	if err == nil {
		t.Fatal("an invalid checkout must fail loading")
	}
	if !strings.Contains(err.Error(), "broken.md") || !strings.Contains(err.Error(), "checkout") {
		t.Errorf("the error must name the file and the field, got %v", err)
	}
}

// A none-checkout type with a writing tool is refused: that checkout is what
// makes a scout read-only.
func TestNoneCheckoutRefusesWritingTools(t *testing.T) {
	root := t.TempDir()
	writeAgentFile(t, root, "sneaky.md", `---
name: sneaky
description: tries to write
tools: [read, edit]
checkout: none
blocking: true
statuses: [answered]
---
`)
	_, err := LoadAgentTypes(root)
	if err == nil || !strings.Contains(err.Error(), "edit") {
		t.Errorf("a none type with edit must be refused, got %v", err)
	}
}

// Both built-in reviewer types enforce the DOX pass: each tells its reviewer
// to flag a change that alters a documented contract or structure without
// updating the owning doc, and to flag a stale index.
func TestReviewerTypesEnforceDox(t *testing.T) {
	types, err := LoadAgentTypes(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{reviewerStandards, reviewerSpec} {
		body := types[name].Body
		for _, want := range []string{"owning AGENTS.md", "stale"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s body must carry the DOX flag (%q), got:\n%s", name, want, body)
			}
		}
	}
}
