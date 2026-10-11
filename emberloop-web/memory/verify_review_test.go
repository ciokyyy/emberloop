package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyBareFilenameInSubdir(t *testing.T) {
	dir, base := initGitRepo(t, map[string]string{"memory/bank.go": "package memory\n"})
	rule := Rule{Text: "keep banks simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{"bank.go"}, ObservedCommit: base}
	writeRepoFile(t, dir, "memory/bank.go", "package memory\n// changed\n")
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "change bank")
	if res := VerifyRule(context.Background(), dir, rule); res.Status != VerifyChanged {
		t.Fatalf("bare changed status = %q (%s), want changed; changed=%q", res.Status, res.Reason, res.Changed)
	}

	dir2, base2 := initGitRepo(t, map[string]string{"memory/bank.go": "package memory\n"})
	rule2 := Rule{Text: "keep banks simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{"bank.go"}, ObservedCommit: base2}
	runGitTest(t, dir2, "rm", "memory/bank.go")
	runGitTest(t, dir2, "commit", "-m", "drop bank")
	if res := VerifyRule(context.Background(), dir2, rule2); res.Status != VerifyDormant {
		t.Fatalf("bare deleted status = %q (%s), want dormant; deleted=%q", res.Status, res.Reason, res.Deleted)
	}
}

func TestVerifyDirectoryAnchorSemantics(t *testing.T) {
	dir, base := initGitRepo(t, map[string]string{
		"pkg/a.go": "package pkg\n",
		"pkg/b.go": "package pkg\n",
	})
	rule := Rule{Text: "keep pkg simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{"pkg"}, ObservedCommit: base}
	// One file gone, the directory anchor itself still exists: changed, not dormant.
	runGitTest(t, dir, "rm", "pkg/a.go")
	runGitTest(t, dir, "commit", "-m", "drop a")
	if res := VerifyRule(context.Background(), dir, rule); res.Status != VerifyChanged {
		t.Fatalf("partial dir status = %q (%s), want changed; changed=%q deleted=%q", res.Status, res.Reason, res.Changed, res.Deleted)
	}
	// The whole directory gone: dormant.
	runGitTest(t, dir, "rm", "pkg/b.go")
	runGitTest(t, dir, "commit", "-m", "drop b")
	if res := VerifyRule(context.Background(), dir, rule); res.Status != VerifyDormant {
		t.Fatalf("gone dir status = %q (%s), want dormant; deleted=%q", res.Status, res.Reason, res.Deleted)
	}
}

func TestVerifyModuleAnchorsPrecise(t *testing.T) {
	goMod := "module example.com/x\n\ngo 1.26.0\n\nrequire (\n\tmodernc.org/sqlite v1.59.0\n\tgithub.com/MichaelKinsy/PiG v0.4.0\n)\n"
	dir, base := initGitRepo(t, map[string]string{"go.mod": goMod, "app.go": "package app\n"})
	anchor := "modernc.org/sqlite@v1.59.0"
	rule := Rule{Text: "use sqlite", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{anchor}, ObservedCommit: base}
	// Unrelated require bump leaves this anchor clean.
	writeRepoFile(t, dir, "go.mod", strings.Replace(goMod, "github.com/MichaelKinsy/PiG v0.4.0", "github.com/MichaelKinsy/PiG v0.5.0", 1))
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "bump other")
	if res := VerifyRule(context.Background(), dir, rule); res.Status != VerifyClean {
		t.Fatalf("unrelated go.mod status = %q (%s), want clean", res.Status, res.Reason)
	}
	// Removing the require dormants the anchor.
	writeRepoFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.26.0\n\nrequire github.com/MichaelKinsy/PiG v0.5.0\n")
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "drop sqlite")
	if res := VerifyRule(context.Background(), dir, rule); res.Status != VerifyDormant {
		t.Fatalf("removed require status = %q (%s), want dormant", res.Status, res.Reason)
	}
}

func TestVerifyOutsideWorktreeIsStale(t *testing.T) {
	dir, base := initGitRepo(t, map[string]string{"app.go": "package app\n"})
	outside := filepath.Join(t.TempDir(), "outside.go")
	rule := Rule{Text: "keep app simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{outside}, ObservedCommit: base}
	res := VerifyRule(context.Background(), dir, rule)
	if res.Status != VerifyStale {
		t.Fatalf("outside status = %q (%s), want stale", res.Status, res.Reason)
	}
	if res.Reason != "anchor outside worktree" {
		t.Fatalf("reason = %q, want anchor outside worktree", res.Reason)
	}
}

func TestVerifyGitFailureIsStale(t *testing.T) {
	dir, base := initGitRepo(t, map[string]string{"app.go": "package app\n"})
	rule := Rule{Text: "keep app simple", Kind: KindFact, Status: StatusActive, ApprovedBy: "u1",
		Anchors: []string{"app.go"}, ObservedCommit: base}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if res := VerifyRule(ctx, dir, rule); res.Status != VerifyStale {
		t.Fatalf("cancelled status = %q (%s), want stale, never clean", res.Status, res.Reason)
	}
}

func TestApplyVerificationMarksDormant(t *testing.T) {
	b, _, _ := openTestBank(t)
	ctx := context.Background()
	dir, base := initGitRepo(t, map[string]string{"app.go": "package app\n"})
	rule, err := b.AddRule(ctx, Rule{
		Text: "keep app simple", Kind: KindFact, ApprovedBy: "u1",
		Anchors: []string{"app.go"}, ObservedCommit: base,
	})
	if err != nil {
		t.Fatal(err)
	}
	runGitTest(t, dir, "rm", "app.go")
	runGitTest(t, dir, "commit", "-m", "drop app")
	verified, err := ApplyVerification(ctx, b, dir, []Rule{rule})
	if err != nil {
		t.Fatal(err)
	}
	if len(verified) != 1 || verified[0].Result.Status != VerifyDormant {
		t.Fatalf("apply status = %v, want dormant", verified)
	}
	versions, err := b.RuleVersions(ctx, rule.RuleID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Status != StatusDormant {
		t.Fatalf("after apply versions = %+v, want dormant", versions)
	}
}
