package memory

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitEnv isolates git from the developer's config the way runs_driver_test
// does: fixed author, no system config, throwaway HOME.
func gitEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "memory-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "memory-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// initGitRepo creates a main-branch repo with the given files committed.
func initGitRepo(t *testing.T, files map[string]string) (dir, head string) {
	t.Helper()
	gitEnv(t)
	dir = t.TempDir()
	runGitTest(t, dir, "init", "-b", "main", ".")
	for name, content := range files {
		writeRepoFile(t, dir, name, content)
	}
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "base")
	return dir, runGitTest(t, dir, "rev-parse", "HEAD")
}

func TestVerifyUnchangedInjectsClean(t *testing.T) {
	dir, head := initGitRepo(t, map[string]string{"app.go": "package app\n"})
	rule := Rule{Text: "keep app simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1", Anchors: []string{"app.go"}, ObservedCommit: head}
	res := VerifyRule(context.Background(), dir, rule)
	if res.Status != VerifyClean {
		t.Fatalf("unchanged status = %q (%s), want clean; changed=%q deleted=%q", res.Status, res.Reason, res.Changed, res.Deleted)
	}
	if res.NeedsFlag() || res.FlagNote() != "" {
		t.Fatalf("clean rule flags: needs=%v note=%q", res.NeedsFlag(), res.FlagNote())
	}
}

func TestVerifyChangedInjectsWithFlag(t *testing.T) {
	dir, base := initGitRepo(t, map[string]string{"app.go": "package app\n"})
	writeRepoFile(t, dir, "app.go", "package app\n\n// new comment\n")
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "change app")
	rule := Rule{Text: "keep app simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1", Anchors: []string{"app.go"}, ObservedCommit: base}
	res := VerifyRule(context.Background(), dir, rule)
	if res.Status != VerifyChanged {
		t.Fatalf("changed status = %q (%s), want changed", res.Status, res.Reason)
	}
	if len(res.Changed) != 1 || res.Changed[0] != "app.go" {
		t.Fatalf("changed = %q, want [app.go]", res.Changed)
	}
	if !res.NeedsFlag() {
		t.Fatal("changed rule needs no flag")
	}
	if res.CommitDate.IsZero() {
		t.Fatal("changed result has no commit date for the flag")
	}
	note := res.FlagNote()
	if !strings.HasPrefix(note, "code changed since ") || !strings.HasSuffix(note, ", verify") {
		t.Fatalf("flag note = %q, want \"code changed since <date>, verify\"", note)
	}
}

func TestVerifyDeletedGoesDormant(t *testing.T) {
	b, _, _ := openTestBank(t)
	ctx := context.Background()
	dir, base := initGitRepo(t, map[string]string{"app.go": "package app\n", "other.go": "package app\n"})
	rule, err := b.AddRule(ctx, Rule{
		Text: "keep app simple", Kind: KindFact, ApprovedBy: "u1",
		Anchors: []string{"app.go"}, ObservedCommit: base,
	})
	if err != nil {
		t.Fatal(err)
	}
	runGitTest(t, dir, "rm", "app.go")
	runGitTest(t, dir, "commit", "-m", "drop app")
	res := VerifyRule(ctx, dir, rule)
	if res.Status != VerifyDormant {
		t.Fatalf("deleted status = %q (%s), want dormant; changed=%q deleted=%q", res.Status, res.Reason, res.Changed, res.Deleted)
	}
	if len(res.Deleted) != 1 || res.Deleted[0] != "app.go" {
		t.Fatalf("deleted = %q, want [app.go]", res.Deleted)
	}
	// The Daemon marks the Rule dormant and stops injecting it.
	if err := b.SetRuleStatus(ctx, rule.RuleID, StatusDormant); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Recall(ctx, "keep app simple", 10); len(got) != 0 {
		t.Fatalf("dormant rule still recalled: %d hits", len(got))
	}
}

func TestVerifyMissingCommitFlagsInsteadOfFailing(t *testing.T) {
	dir, _ := initGitRepo(t, map[string]string{"app.go": "package app\n"})
	rule := Rule{Text: "keep app simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{"app.go"}, ObservedCommit: "0123456789abcdef0123456789abcdef01234567"}
	res := VerifyRule(context.Background(), dir, rule)
	if res.Status != VerifyStale {
		t.Fatalf("missing commit status = %q, want stale", res.Status)
	}
	if res.Reason != "missing commit" {
		t.Fatalf("reason = %q, want missing commit", res.Reason)
	}
	if !res.NeedsFlag() {
		t.Fatal("stale rule needs the flag")
	}
}

func TestVerifyForeignBranchFlagged(t *testing.T) {
	dir, _ := initGitRepo(t, map[string]string{"app.go": "package app\n"})
	runGitTest(t, dir, "checkout", "-b", "other")
	writeRepoFile(t, dir, "other.go", "package app\n")
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "other branch")
	other := runGitTest(t, dir, "rev-parse", "HEAD")
	runGitTest(t, dir, "checkout", "main")
	rule := Rule{Text: "keep other simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{"other.go"}, ObservedCommit: other}
	res := VerifyRule(context.Background(), dir, rule)
	if res.Status != VerifyStale {
		t.Fatalf("foreign branch status = %q (%s), want stale", res.Status, res.Reason)
	}
	if res.Reason != "not an ancestor of HEAD" {
		t.Fatalf("reason = %q, want not an ancestor of HEAD", res.Reason)
	}
}

func TestVerifyPreferenceAndAnchorlessAreClean(t *testing.T) {
	dir, base := initGitRepo(t, map[string]string{"app.go": "package app\n"})
	writeRepoFile(t, dir, "app.go", "package app\n// changed\n")
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "change")
	ctx := context.Background()
	pref := Rule{Text: "prefer tabs", Kind: KindPreference, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{"app.go"}, ObservedCommit: base}
	if res := VerifyRule(ctx, dir, pref); res.Status != VerifyClean {
		t.Fatalf("preference status = %q, want clean", res.Status)
	}
	bare := Rule{Text: "keep app simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{"app.go"}, ObservedCommit: ""}
	if res := VerifyRule(ctx, dir, bare); res.Status != VerifyClean {
		t.Fatalf("empty commit status = %q, want clean", res.Status)
	}
	plain := t.TempDir()
	outside := Rule{Text: "keep app simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{"app.go"}, ObservedCommit: base}
	if res := VerifyRule(ctx, plain, outside); res.Status != VerifyStale {
		t.Fatalf("non-repo status = %q, want stale", res.Status)
	}
}

func TestVerifyModuleAnchorFlagsGoModChange(t *testing.T) {
	goMod := "module example.com/x\n\ngo 1.26.0\n\nrequire modernc.org/sqlite v1.59.0\n"
	dir, base := initGitRepo(t, map[string]string{"go.mod": goMod, "app.go": "package app\n"})
	writeRepoFile(t, dir, "go.mod", strings.Replace(goMod, "v1.59.0", "v1.60.0", 1))
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "bump sqlite")
	rule := Rule{Text: "use sqlite for banks", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{"modernc.org/sqlite@v1.59.0"}, ObservedCommit: base}
	res := VerifyRule(context.Background(), dir, rule)
	if res.Status != VerifyChanged {
		t.Fatalf("module status = %q (%s), want changed", res.Status, res.Reason)
	}
}

func TestVerifyRulesKeepsOrder(t *testing.T) {
	dir, base := initGitRepo(t, map[string]string{"app.go": "package app\n"})
	writeRepoFile(t, dir, "app.go", "package app\n// changed\n")
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "change")
	rules := []Rule{
		{RuleID: "r1", Text: "keep app simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1", Anchors: []string{"app.go"}, ObservedCommit: base},
		{RuleID: "r2", Text: "prefer tabs", Kind: KindPreference, Status: StatusActive, ApprovedBy: "u1"},
	}
	got := VerifyRules(context.Background(), dir, rules)
	if len(got) != 2 || got[0].Rule.RuleID != "r1" || got[1].Rule.RuleID != "r2" {
		t.Fatalf("VerifyRules order = %v", got)
	}
	if got[0].Result.Status != VerifyChanged || got[1].Result.Status != VerifyClean {
		t.Fatalf("VerifyRules = %q, %q; want changed, clean", got[0].Result.Status, got[1].Result.Status)
	}
}

func TestAnchorsRoundTripThroughBank(t *testing.T) {
	b, _, _ := openTestBank(t)
	ctx := context.Background()
	anchors := ExtractAnchors("fix emberloop-web/memory/bank.go", []string{"edit bank.go"}, nil)
	anchors = NormalizeAnchors(KindFact, anchors)
	if len(anchors) == 0 {
		t.Fatal("no anchors extracted for round trip")
	}
	rule, err := b.AddRule(ctx, Rule{
		Text: "keep banks simple", Kind: KindFact, ApprovedBy: "u1",
		Anchors: anchors, ObservedCommit: "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := b.RuleVersions(ctx, rule.RuleID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || strings.Join(versions[0].Anchors, ",") != strings.Join(anchors, ",") {
		t.Fatalf("round trip anchors = %q, want %q", versions[0].Anchors, anchors)
	}
	if versions[0].ObservedCommit != "abc123" {
		t.Fatalf("observed commit = %q, want abc123", versions[0].ObservedCommit)
	}
}
