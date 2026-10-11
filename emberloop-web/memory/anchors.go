// Anchor extraction and git verification for memory Rules.
//
// Anchors pin a Rule to the code it came from: file paths, Go packages and
// module@version pairs from go.mod, plus the commit the Rule was observed at
// (Rule.ObservedCommit). Preferences carry no anchors; they get new versions,
// never staleness.
//
// Extraction is deterministic: the same quote, tool texts and go.mod always
// give the same sorted, deduplicated anchor list. Verification runs at
// injection time in the Session's worktree:
//
//	git diff --name-status <observed_commit>..HEAD -- <paths>
//
// --name-status carries the same change set as the spec's --stat but is
// machine-readable, so deletes (status D) can be told apart from edits.
// A Rule whose anchors changed injects with a "code changed since <date>,
// verify" flag. A Rule whose anchor path is gone at HEAD goes dormant and is
// not injected. A Rule whose commit is missing (rebased away), is not an
// ancestor of HEAD, or cannot be checked (not a repo, anchor outside the
// worktree, git failure) is flagged stale: it injects with the flag instead
// of failing the turn.
package memory

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// slashPathRe finds slash paths: repo files, import paths, dotted paths.
	slashPathRe = regexp.MustCompile(`[A-Za-z0-9_.\-]+(?:/[A-Za-z0-9_.\-]+)+`)
	// fileRe finds bare source files such as main.go and go.mod.
	fileRe = regexp.MustCompile(`\b[A-Za-z0-9_.\-]+\.(?:go|mod|sum|md|json|yaml|yml|toml)\b`)
	// modAtRe finds explicit module@version mentions.
	modAtRe = regexp.MustCompile(`\b[A-Za-z0-9_.\-/]+@v[A-Za-z0-9_.\-]+\b`)
)

const (
	// MaxAnchors caps the anchors one extraction returns. It keeps
	// anchors_json small; storage itself takes any anchor count.
	MaxAnchors = 20
	// MaxAnchorRunes caps one extracted anchor, counted in characters.
	// cleanAnchor drops anything longer; storage takes any length.
	MaxAnchorRunes = 200
)

// VerifyStatus is the injection decision for one Rule.
type VerifyStatus string

const (
	// VerifyClean injects the Rule as-is.
	VerifyClean VerifyStatus = "clean"
	// VerifyChanged injects the Rule with a verify flag.
	VerifyChanged VerifyStatus = "changed"
	// VerifyDormant skips the Rule and marks it dormant.
	VerifyDormant VerifyStatus = "dormant"
	// VerifyStale flags the Rule: its commit is missing, not an ancestor of
	// HEAD, or cannot be checked. It injects with the flag, never fails.
	VerifyStale VerifyStatus = "stale"
)

// VerifyResult is the outcome of checking one Rule against a worktree.
type VerifyResult struct {
	Status VerifyStatus
	// Changed holds the anchors that differ since the observed commit.
	Changed []string
	// Deleted holds the anchors gone at HEAD.
	Deleted []string
	// Reason is a short human cause: "code changed", "anchor deleted",
	// "missing commit", "not an ancestor of HEAD", "not a git repo",
	// "anchor outside worktree", "git diff failed".
	Reason string
	// CommitDate is the committer date of the observed commit when known.
	CommitDate time.Time
}

// NeedsFlag reports whether the Rule injects with the verify flag.
func (r VerifyResult) NeedsFlag() bool {
	return r.Status == VerifyChanged || r.Status == VerifyStale
}

// FlagNote renders the injection flag for ticket 24: "code changed since
// <date>, verify", or "code changed, verify" when the commit date is unknown.
func (r VerifyResult) FlagNote() string {
	if !r.NeedsFlag() {
		return ""
	}
	if r.CommitDate.IsZero() {
		return "code changed, verify"
	}
	return fmt.Sprintf("code changed since %s, verify", r.CommitDate.Format("2006-01-02"))
}

// VerifiedRule pairs a Rule with its verification outcome.
type VerifiedRule struct {
	Rule   Rule
	Result VerifyResult
}

// NormalizeAnchors enforces the storage invariant: preferences carry no
// anchors. Any other kind keeps the anchors deduplicated, sorted and capped.
func NormalizeAnchors(kind string, anchors []string) []string {
	if kind == KindPreference {
		return nil
	}
	if len(anchors) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(anchors))
	var out []string
	for _, a := range anchors {
		a = strings.TrimSpace(a)
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	sort.Strings(out)
	if len(out) > MaxAnchors {
		out = out[:MaxAnchors]
	}
	return out
}

// ExtractAnchors collects anchors from a verbatim user quote, the tool-call
// texts of its Session (each entry is one call's input plus output, e.g.
// ToolCall.Text()), and the Session worktree's go.mod. The result is sorted,
// deduplicated and capped at MaxAnchors, so it is deterministic for the same
// inputs.
//
// Paths are slash paths and bare source files (go.mod included). Packages are
// import-like paths. module@version pairs resolve the packages against the
// require lines of goMod; explicit module@version mentions in the text are
// kept as-is. A nil or empty goMod skips the resolution step. Preferences
// ignore anchors entirely; call NormalizeAnchors with the Rule kind before
// storing.
func ExtractAnchors(quote string, toolTexts []string, goMod []byte) []string {
	var blob strings.Builder
	blob.WriteString(quote)
	for _, t := range toolTexts {
		blob.WriteString("\n")
		blob.WriteString(t)
	}
	text := blob.String()

	seen := make(map[string]bool)
	var cands []string
	add := func(a string) {
		a = cleanAnchor(a)
		if a == "" || seen[a] {
			return
		}
		seen[a] = true
		cands = append(cands, a)
	}
	for _, m := range slashPathRe.FindAllString(text, -1) {
		add(m)
	}
	for _, m := range fileRe.FindAllString(text, -1) {
		add(m)
	}
	var explicitMods []string
	for _, m := range modAtRe.FindAllString(text, -1) {
		m = cleanAnchor(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		explicitMods = append(explicitMods, m)
	}

	requires := parseGoModRequires(goMod)
	var resolved []string
	if len(requires) > 0 {
		for _, c := range cands {
			if mod, ver, ok := resolveModule(c, requires); ok {
				at := mod + "@" + ver
				if !seen[at] {
					seen[at] = true
					resolved = append(resolved, at)
				}
			}
		}
	}

	out := append(append([]string{}, cands...), resolved...)
	out = append(out, explicitMods...)
	sort.Strings(out)
	if len(out) > MaxAnchors {
		out = out[:MaxAnchors]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cleanAnchor trims quoting and trailing punctuation and drops whitespace,
// empty and overlong anchors. It agrees with validRule: anything cleanAnchor
// drops, validRule refuses.
func cleanAnchor(a string) string {
	a = strings.TrimSpace(a)
	a = strings.Trim(a, "`\"'()[]{}<>")
	a = strings.TrimRight(a, ".,;:!?")
	a = strings.TrimSpace(a)
	if a == "" || a == "." || a == ".." {
		return ""
	}
	if strings.ContainsAny(a, " \t\r\n\f\v") {
		return ""
	}
	a = strings.TrimPrefix(a, "./")
	if a == "" {
		return ""
	}
	if utf8.RuneCountInString(a) > MaxAnchorRunes {
		return ""
	}
	return a
}

// resolveModule maps an extracted path or package to the longest require
// module that prefixes it. "modernc.org/sqlite" with a require on
// "modernc.org/sqlite v1.59.0" resolves to that pair; a repo-relative path
// such as "emberloop-web/memory" matches nothing.
func resolveModule(cand string, requires map[string]string) (string, string, bool) {
	best := ""
	for mod := range requires {
		if cand == mod || strings.HasPrefix(cand, mod+"/") {
			if len(mod) > len(best) {
				best = mod
			}
		}
	}
	if best == "" {
		return "", "", false
	}
	return best, requires[best], true
}

// parseGoModRequires reads module->version pairs from go.mod requires. It
// handles single-line requires and parenthesized blocks, strips // comments,
// and ignores replace, exclude and retract lines.
func parseGoModRequires(goMod []byte) map[string]string {
	out := make(map[string]string)
	inBlock := false
	for _, line := range strings.Split(string(goMod), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "require (") || line == "require (" {
			inBlock = true
			continue
		}
		if inBlock && line == ")" {
			inBlock = false
			continue
		}
		rest := line
		if strings.HasPrefix(rest, "require ") {
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "require "))
			if rest == "(" || strings.HasPrefix(rest, "(") {
				inBlock = true
				continue
			}
		} else if !inBlock {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 2 {
			continue
		}
		mod, ver := fields[0], fields[1]
		if !strings.Contains(mod, ".") || !strings.HasPrefix(ver, "v") {
			continue
		}
		if _, ok := out[mod]; !ok {
			out[mod] = ver
		}
	}
	return out
}

// VerifyRule checks one Rule against the git worktree at workDir. It never
// fails the turn: uncheckable Rules return VerifyStale with a reason.
// Preferences, anchorless Rules and Rules without an observed commit are
// VerifyClean without running git.
func VerifyRule(ctx context.Context, workDir string, rule Rule) VerifyResult {
	if rule.Kind == KindPreference {
		return VerifyResult{Status: VerifyClean}
	}
	anchors := nonEmptyAnchors(rule.Anchors)
	if len(anchors) == 0 || strings.TrimSpace(rule.ObservedCommit) == "" {
		return VerifyResult{Status: VerifyClean}
	}
	commit := strings.TrimSpace(rule.ObservedCommit)
	if strings.TrimSpace(workDir) == "" {
		return VerifyResult{Status: VerifyStale, Reason: "not a git repo"}
	}
	if _, err := runGit(ctx, workDir, "rev-parse", "--git-dir"); err != nil {
		return VerifyResult{Status: VerifyStale, Reason: "not a git repo"}
	}
	if err := runGitErr(ctx, workDir, "cat-file", "-e", commit); err != nil {
		return VerifyResult{Status: VerifyStale, Reason: "missing commit"}
	}
	commitDate := observedCommitDate(ctx, workDir, commit)
	if err := runGitErr(ctx, workDir, "merge-base", "--is-ancestor", commit, "HEAD"); err != nil {
		return VerifyResult{Status: VerifyStale, Reason: "not an ancestor of HEAD", CommitDate: commitDate}
	}

	paths, outside := pathAnchors(workDir, anchors)
	mods := moduleAnchors(anchors)
	if len(paths) == 0 && len(mods) == 0 {
		if outside {
			return VerifyResult{Status: VerifyStale, Reason: "anchor outside worktree", CommitDate: commitDate}
		}
		return VerifyResult{Status: VerifyClean, CommitDate: commitDate}
	}

	var modChanged, modDeleted []string
	if len(mods) > 0 {
		changed, deleted, stale := checkModuleAnchors(ctx, workDir, commit, mods)
		if stale {
			return VerifyResult{Status: VerifyStale, Reason: "git diff failed", CommitDate: commitDate}
		}
		modChanged, modDeleted = changed, deleted
	}

	var pathChanged, pathDeleted []string
	if len(paths) > 0 {
		changed, deleted, err := diffAnchors(ctx, workDir, commit, paths)
		if err != nil {
			return VerifyResult{Status: VerifyStale, Reason: "git diff failed", CommitDate: commitDate}
		}
		pathChanged, pathDeleted = changed, deleted
	}

	changed := append(append([]string{}, modChanged...), pathChanged...)
	deleted := append(append([]string{}, modDeleted...), pathDeleted...)
	sort.Strings(changed)
	sort.Strings(deleted)
	switch {
	case len(deleted) > 0:
		return VerifyResult{Status: VerifyDormant, Changed: changed, Deleted: deleted, Reason: "anchor deleted", CommitDate: commitDate}
	case outside:
		// A dropped absolute anchor outside the worktree cannot be checked.
		return VerifyResult{Status: VerifyStale, Reason: "anchor outside worktree", CommitDate: commitDate, Changed: changed}
	case len(changed) > 0:
		return VerifyResult{Status: VerifyChanged, Changed: changed, Reason: "code changed", CommitDate: commitDate}
	default:
		return VerifyResult{Status: VerifyClean, CommitDate: commitDate}
	}
}

// VerifyRules checks every Rule against workDir in order. It never returns an
// error: per-Rule failures are VerifyStale results.
func VerifyRules(ctx context.Context, workDir string, rules []Rule) []VerifiedRule {
	out := make([]VerifiedRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, VerifiedRule{Rule: r, Result: VerifyRule(ctx, workDir, r)})
	}
	return out
}

// ApplyVerification checks every Rule against workDir and marks dormant
// outcomes via SetRuleStatus, so the spec's "deleted anchors set the Rule
// dormant" side effect has one production path. Injection (ticket 24) and the
// nightly anchor check call this instead of VerifyRules when they want the
// mark applied. Mark failures join into the returned error; verification
// itself never fails.
func ApplyVerification(ctx context.Context, m Memory, workDir string, rules []Rule) ([]VerifiedRule, error) {
	verified := VerifyRules(ctx, workDir, rules)
	var errs []error
	for _, v := range verified {
		if v.Result.Status != VerifyDormant {
			continue
		}
		if v.Rule.RuleID == "" {
			continue
		}
		if err := m.SetRuleStatus(ctx, v.Rule.RuleID, StatusDormant); err != nil {
			errs = append(errs, err)
		}
	}
	return verified, errors.Join(errs...)
}

// nonEmptyAnchors drops blank anchors.
func nonEmptyAnchors(anchors []string) []string {
	var out []string
	for _, a := range anchors {
		if strings.TrimSpace(a) != "" {
			out = append(out, a)
		}
	}
	return out
}

// pathAnchors returns the anchors git can diff, relativized to workDir.
// Absolute anchors outside the worktree cannot be diffed: they are dropped
// and reported via outside. module@version anchors are not paths.
func pathAnchors(workDir string, anchors []string) (paths []string, outside bool) {
	for _, a := range anchors {
		if strings.Contains(a, "@") {
			continue
		}
		p := strings.TrimSpace(a)
		if p == "" {
			continue
		}
		if filepath.IsAbs(p) {
			rel, err := filepath.Rel(workDir, p)
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				outside = true
				continue
			}
			p = filepath.ToSlash(rel)
		}
		paths = append(paths, p)
	}
	return paths, outside
}

// moduleAnchors returns the module@version anchors.
func moduleAnchors(anchors []string) []string {
	var out []string
	for _, a := range anchors {
		if strings.Contains(a, "@") && strings.TrimSpace(a) != "" {
			out = append(out, a)
		}
	}
	return out
}

// diffPathspecs expands slash-less anchors so git sees subdirectory matches:
// "bank.go" becomes "bank.go" plus "**/bank.go" (root plus any directory),
// while "pkg" keeps its directory match and gains the same glob for a
// same-named file elsewhere. Slash anchors pass through unchanged.
func diffPathspecs(paths []string) []string {
	var out []string
	for _, p := range paths {
		out = append(out, p)
		if !strings.Contains(p, "/") {
			glob := "**/" + p
			if !slices.Contains(out, glob) {
				out = append(out, glob)
			}
		}
	}
	return out
}

// diffAnchors runs git diff --name-status commit..HEAD over the already
// normalized relative paths and maps the changed files back onto them.
// Exact matches take the diff status; a directory anchor matches any diff
// path under it as a change, and goes dormant only when the directory itself
// is gone at HEAD. A bare filename anchor (no slash) matches by basename in
// any subdirectory, and a slash-less directory anchor (no slash, no
// extension) matches its directory prefix too. A diff failure returns an
// error so the caller flags stale instead of passing clean.
func diffAnchors(ctx context.Context, workDir, commit string, paths []string) (changed, deleted []string, err error) {
	if len(paths) == 0 {
		return nil, nil, nil
	}
	args := append([]string{"diff", "--name-status", commit + "..HEAD", "--"}, diffPathspecs(paths)...)
	out, err := runGit(ctx, workDir, args...)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil, nil
	}
	type entry struct {
		path    string
		deleted bool
	}
	var entries []entry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		switch {
		case strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C"):
			// Rename/copy: both sides count as changed, never as deleted.
			if len(fields) >= 3 {
				entries = append(entries, entry{fields[1], false}, entry{fields[2], false})
			}
		case strings.HasPrefix(status, "D"):
			entries = append(entries, entry{fields[len(fields)-1], true})
		default:
			entries = append(entries, entry{fields[len(fields)-1], false})
		}
	}
	// Basename index for bare anchors; built lazily only when needed.
	var headBasenames map[string]bool
	basenamesNeeded := slices.ContainsFunc(paths, func(p string) bool { return !strings.Contains(p, "/") })
	if basenamesNeeded {
		// Only consulted when every match for a bare anchor is a delete;
		// a missing ls-tree means we cannot prove deletion, so treat as changed.
		if names, lserr := headFileBasenames(ctx, workDir); lserr == nil {
			headBasenames = names
		}
	}
	hitChanged := make(map[string]bool)
	hitDeleted := make(map[string]bool)
	for _, anchor := range paths {
		if strings.Contains(anchor, "/") {
			var exactDeleted, exactChanged bool
			var prefixChanged bool
			for _, e := range entries {
				switch {
				case e.path == anchor:
					if e.deleted {
						exactDeleted = true
					} else {
						exactChanged = true
					}
				case strings.HasPrefix(e.path, strings.TrimSuffix(anchor, "/")+"/"):
					prefixChanged = true
				}
			}
			switch {
			case exactDeleted:
				if !anchorExistsAtHEAD(ctx, workDir, anchor) {
					hitDeleted[anchor] = true
				} else {
					hitChanged[anchor] = true
				}
			case exactChanged || prefixChanged:
				// A file under a directory anchor changed or vanished: the
				// directory anchor itself is changed unless the directory is
				// gone at HEAD, in which case the anchor is deleted.
				if !anchorExistsAtHEAD(ctx, workDir, anchor) {
					hitDeleted[anchor] = true
				} else {
					hitChanged[anchor] = true
				}
			}
			continue
		}
		// Slash-less anchor: it may name a top-level directory (pkg matches
		// pkg/a.go) or a bare file (bank.go matches memory/bank.go). Match
		// either shape; directory matches are changes unless the directory is
		// gone at HEAD, bare-file deletes go dormant only when no namesake
		// remains at HEAD.
		var exactDeleted, exactChanged, prefixChanged bool
		var baseChanged, baseDeleted bool
		for _, e := range entries {
			switch {
			case e.path == anchor:
				if e.deleted {
					exactDeleted = true
				} else {
					exactChanged = true
				}
			case strings.HasPrefix(e.path, anchor+"/"):
				prefixChanged = true
			case strings.HasSuffix(e.path, "/"+anchor) || filepath.Base(e.path) == anchor:
				if e.deleted {
					baseDeleted = true
				} else {
					baseChanged = true
				}
			}
		}
		switch {
		case exactDeleted:
			if !anchorExistsAtHEAD(ctx, workDir, anchor) {
				hitDeleted[anchor] = true
			} else {
				hitChanged[anchor] = true
			}
		case exactChanged || prefixChanged:
			if !anchorExistsAtHEAD(ctx, workDir, anchor) {
				hitDeleted[anchor] = true
			} else {
				hitChanged[anchor] = true
			}
		case baseChanged:
			hitChanged[anchor] = true
		case baseDeleted:
			if headBasenames != nil && !headBasenames[anchor] {
				hitDeleted[anchor] = true
			} else {
				// Cannot prove every namesake is gone; flag as changed.
				hitChanged[anchor] = true
			}
		}
	}
	for a := range hitDeleted {
		deleted = append(deleted, a)
	}
	for a := range hitChanged {
		if !hitDeleted[a] {
			changed = append(changed, a)
		}
	}
	sort.Strings(changed)
	sort.Strings(deleted)
	return changed, deleted, nil
}

// checkModuleAnchors compares each module@version anchor against HEAD's
// go.mod. A module missing at HEAD is deleted (dormant); a module present at
// a different version is changed. Unrelated go.mod edits leave other anchors
// clean. It returns stale=true when HEAD's go.mod cannot be read and the
// outcome cannot be decided.
func checkModuleAnchors(ctx context.Context, workDir, commit string, mods []string) (changed, deleted []string, stale bool) {
	headMod, headErr := runGit(ctx, workDir, "show", "HEAD:go.mod")
	headRequires := parseGoModRequires([]byte(headMod))
	if headErr != nil {
		// go.mod gone at HEAD: dormant when it existed at the observed
		// commit, stale when it never existed (nothing to compare).
		if _, obserr := runGit(ctx, workDir, "show", commit+":go.mod"); obserr == nil {
			return nil, append([]string{}, mods...), false
		}
		return nil, nil, true
	}
	for _, m := range mods {
		mod, ver, ok := splitModuleVersion(m)
		if !ok {
			continue
		}
		headVer, present := headRequires[mod]
		switch {
		case !present:
			deleted = append(deleted, m)
		case headVer != ver:
			changed = append(changed, m)
		}
	}
	sort.Strings(changed)
	sort.Strings(deleted)
	return changed, deleted, false
}

// splitModuleVersion cuts "module@version" at the last @.
func splitModuleVersion(a string) (mod, ver string, ok bool) {
	i := strings.LastIndex(a, "@")
	if i <= 0 || i == len(a)-1 {
		return "", "", false
	}
	return a[:i], a[i+1:], true
}

// anchorExistsAtHEAD reports whether anchor (a relative path) exists at HEAD
// as a file or a directory.
func anchorExistsAtHEAD(ctx context.Context, workDir, anchor string) bool {
	if err := runGitErr(ctx, workDir, "cat-file", "-e", "HEAD:"+anchor); err == nil {
		return true
	}
	// cat-file can miss trees on some git versions; ls-tree is the fallback.
	out, err := runGit(ctx, workDir, "ls-tree", "--name-only", "HEAD", "--", anchor)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// headFileBasenames lists the basenames of every file at HEAD.
func headFileBasenames(ctx context.Context, workDir string) (map[string]bool, error) {
	out, err := runGit(ctx, workDir, "ls-tree", "-r", "--name-only", "HEAD", "--")
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		names[filepath.Base(line)] = true
	}
	return names, nil
}

// observedCommitDate reads the committer date of commit, zero on failure.
func observedCommitDate(ctx context.Context, workDir, commit string) time.Time {
	out, err := runGit(ctx, workDir, "show", "-s", "--format=%cI", commit)
	if err != nil {
		return time.Time{}
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, out); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02T15:04:05-07:00", out); err == nil {
		return t
	}
	return time.Time{}
}

// runGit runs git in dir and returns trimmed stdout.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// runGitErr runs git for its exit status only.
func runGitErr(ctx context.Context, dir string, args ...string) error {
	_, err := runGit(ctx, dir, args...)
	return err
}
