package el

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"example.com/emberloop/tools"
)

// SpecRun is what `el implement <spec>` works from: the spec, where its
// tickets live, and the integration branch every worker of the run merges into.
type SpecRun struct {
	Spec    string // absolute path of the spec file
	Tickets string // directory holding its tickets
	Branch  string // the integration branch, spec/<slug>
}

// ResolveSpec turns the implement argument into a spec file. A directory is
// accepted when it holds a spec.md, or when it is the issues directory beside
// one, so `implement .scratch/x` and `implement .scratch/x/issues` both work.
func ResolveSpec(arg string) (SpecRun, error) {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return SpecRun{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return SpecRun{}, err
	}
	spec := abs
	if info.IsDir() {
		spec = ""
		for _, c := range []string{filepath.Join(abs, "spec.md"), filepath.Join(filepath.Dir(abs), "spec.md")} {
			if fi, err := os.Stat(c); err == nil && fi.Mode().IsRegular() {
				spec = c
				break
			}
		}
		if spec == "" {
			return SpecRun{}, fmt.Errorf("%s holds no spec.md; give the spec file", arg)
		}
	}
	dir := filepath.Dir(spec)
	tickets := dir
	if fi, err := os.Stat(filepath.Join(dir, "issues")); err == nil && fi.IsDir() {
		tickets = filepath.Join(dir, "issues")
	}
	return SpecRun{Spec: spec, Tickets: tickets, Branch: "spec/" + specSlug(spec)}, nil
}

// IsSpecFileName reports whether a markdown file name is a spec file rather
// than a ticket: the classic spec.md or a local spec-*.md plan (for example
// docs/plans/spec-01-voice.md). Ticket scans skip these, so a spec that lives
// beside its tickets never parses as one.
func IsSpecFileName(name string) bool {
	base := filepath.Base(name)
	return base == "spec.md" || (strings.HasPrefix(base, "spec-") && strings.HasSuffix(base, ".md"))
}

// SpecMaxTickets reads a spec file's per-spec ticket cap from its frontmatter
// max_tickets key (max_agents is the pre-rename alias). It reports false when
// the file cannot be read, has no frontmatter, or sets no positive cap, so
// specs without the key run under the global cap.
func SpecMaxTickets(specPath string) (int, bool) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return 0, false
	}
	head, _, err := splitFrontmatter(string(raw))
	if err != nil {
		return 0, false
	}
	fields, err := parseFields(head)
	if err != nil {
		return 0, false
	}
	v, ok := fields.scalar("max_tickets")
	if !ok {
		v, ok = fields.scalar("max_agents")
	}
	if !ok {
		return 0, false
	}
	m, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || m <= 0 {
		return 0, false
	}
	return m, true
}

// MaxTicketsForSpec is the ticket cap in force for one spec run: the global
// cap, lowered by the spec file's own max_tickets frontmatter when it sets
// one. A spec cap never raises the global one, and an empty spec leaves it
// untouched. The web Daemon passes its own global; the CLI passes the
// settings file's.
func MaxTicketsForSpec(spec string, global int) int {
	if global <= 0 {
		global = tools.DefaultMaxTickets
	}
	if spec == "" {
		return global
	}
	if m, ok := SpecMaxTickets(spec); ok && m < global {
		return m
	}
	return global
}

// effectiveTicketCapForSpec is the ticket cap in force for one CLI spec run:
// the settings file's global, lowered by the spec file's own max_tickets
// frontmatter when it sets one.
func effectiveTicketCapForSpec(spec string) int {
	return MaxTicketsForSpec(spec, tools.LoadSettings().EffectiveMaxTickets())
}

// specSlug names the run after the spec: the directory of a spec.md, or the
// spec file's own name otherwise.
func specSlug(spec string) string {
	name := strings.TrimSuffix(filepath.Base(spec), filepath.Ext(spec))
	if name == "spec" {
		name = filepath.Base(filepath.Dir(spec))
	}
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// EnterIntegrationBranch checks the repo out on branch, creating it from the
// current HEAD when it does not exist yet. A branch that exists is reused as it
// stands, which is what lets an interrupted run resume where it stopped.
// Workers spawned afterwards record it as their target, and finish
// fast-forwards it, which is why the repo has to stay on it for the run.
func EnterIntegrationBranch(run Exec, repo, branch string) (created bool, err error) {
	git := func(args ...string) (Result, error) {
		return run("git", args, ExecOpts{Cwd: repo, Timeout: 30 * time.Second})
	}
	res, err := git("show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil {
		return false, err
	}
	args := []string{"switch", branch}
	if res.Code != 0 {
		args = []string{"switch", "-c", branch}
		created = true
	}
	res, err = git(args...)
	if err != nil {
		return false, err
	}
	if res.Code != 0 {
		return false, fmt.Errorf("git %s failed: %s", strings.Join(args, " "), firstLine(res.Stderr, res.Stdout))
	}
	return created, nil
}
