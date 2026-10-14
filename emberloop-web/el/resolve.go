package el

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"example.com/emberloop-web/el/tools"
)

// SpecRun is what a Run works from: the spec, where its tickets live, and
// the integration branch every worker of the run merges into.
type SpecRun struct {
	Spec    string // absolute path of the spec file
	Tickets string // directory holding its tickets
	Branch  string // the integration branch, spec/<slug>
}

// ResolveSpec turns a spec argument into a spec file. A directory is
// accepted when it holds a spec.md, or when it is the issues directory beside
// one, so `.scratch/x` and `.scratch/x/issues` both work.
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
// untouched. The web Daemon passes its own global.
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

// splitFrontmatter cuts a definition into its frontmatter and its body. The
// frontmatter is the block between the first two `---` lines.
func splitFrontmatter(src string) (head, body string, err error) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || strings.TrimSpace(lines[i]) != "---" {
		return "", "", fmt.Errorf("missing frontmatter (the file must start with ---)")
	}
	start := i + 1
	j := start
	for j < len(lines) && strings.TrimSpace(lines[j]) != "---" {
		j++
	}
	if j >= len(lines) {
		return "", "", fmt.Errorf("frontmatter is not closed with ---")
	}
	return strings.Join(lines[start:j], "\n"), strings.Join(lines[j+1:], "\n"), nil
}

// frontmatterFields holds one definition's parsed fields: scalar keys and list
// keys kept apart.
type frontmatterFields struct {
	scalars map[string]string
	lists   map[string][]string
}

func (f frontmatterFields) scalar(key string) (string, bool) {
	v, ok := f.scalars[key]
	return v, ok
}

// parseFields reads the small YAML subset a frontmatter block uses:
// `key: value`, `key: [a, b]`, and block lists of `- item` under a bare
// `key:`. Unknown keys are ignored.
func parseFields(head string) (frontmatterFields, error) {
	f := frontmatterFields{scalars: map[string]string{}, lists: map[string][]string{}}
	lastList := ""
	for _, raw := range strings.Split(head, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- ") || line == "-" {
			if lastList == "" {
				return f, fmt.Errorf("a list item (%q) has no key above it", line)
			}
			f.lists[lastList] = append(f.lists[lastList], strings.TrimSpace(strings.TrimPrefix(line, "-")))
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			return f, fmt.Errorf("line %q is not key: value", line)
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		lastList = ""
		switch {
		case val == "":
			if _, ok := f.lists[key]; !ok {
				f.lists[key] = nil
			}
			lastList = key
		case strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]"):
			f.lists[key] = splitInlineList(val)
		default:
			f.scalars[key] = unquote(val)
		}
	}
	return f, nil
}

func splitInlineList(val string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(val, "["), "]")
	if strings.TrimSpace(inner) == "" {
		return nil
	}
	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := unquote(strings.TrimSpace(p)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}
