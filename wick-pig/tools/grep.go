package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// grepTool is exact-string and regex search over the workspace: ripgrep when
// installed, a Go fallback otherwise. It respects .gitignore, skips binary
// files, and stays inside the workspace like find.
type grepTool struct {
	// fallbackOnly skips the rg lookup, for tests that must give the same
	// answers with and without ripgrep installed.
	fallbackOnly bool
}

func (grepTool) Name() string   { return "grep" }
func (grepTool) ReadOnly() bool { return true }
func (grepTool) Description() string {
	return "Exact-string and regex search over the workspace: ripgrep when installed, a Go fallback otherwise. Respects .gitignore, skips binary files, stays inside the working directory. Matches read `path:line:text`, context lines `path-line-text`, with a total match count."
}
func (grepTool) Schema() map[string]any {
	return obj(map[string]any{
		"pattern":     map[string]any{"type": "string", "description": "Regex to search for, or a fixed string when fixed is true."},
		"fixed":       map[string]any{"type": "boolean", "description": "Treat the pattern as a fixed string, not a regex."},
		"path":        map[string]any{"type": "string", "description": "Optional subdirectory to narrow the search scope."},
		"glob":        map[string]any{"type": "string", "description": "Optional glob over file paths, e.g. \"*.go\"."},
		"context":     map[string]any{"type": "integer", "description": "Lines of context around each match."},
		"max_results": map[string]any{"type": "integer", "description": "Maximum matches returned (default 100); context lines ride along."},
		"ignore_case": map[string]any{"type": "boolean", "description": "Case-insensitive matching."},
	}, "pattern")
}

// defaultGrepMax is the result cap when max_results is absent or not positive.
const defaultGrepMax = 100

// grepMatch is one output line: the workspace-relative path, its 1-based line
// number, and the line's text.
type grepMatch struct {
	rel  string
	line int
	text string
}

// grepHit is one match plus its context lines. The total counts hits; the cap
// keeps whole hits, context included.
type grepHit struct {
	match   grepMatch
	context []grepMatch
}

func (g grepTool) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	var a struct {
		Pattern    string `json:"pattern"`
		Fixed      bool   `json:"fixed"`
		Path       string `json:"path"`
		Glob       string `json:"glob"`
		Context    int    `json:"context"`
		MaxResults int    `json:"max_results"`
		IgnoreCase bool   `json:"ignore_case"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid grep arguments: %w", err)
	}
	if strings.TrimSpace(a.Pattern) == "" {
		return "", errors.New("pattern cannot be empty")
	}
	searchRoot := root
	if a.Path != "" {
		// Confined like find: a path a model chose is untrusted.
		dir, err := resolveInside(root, a.Path)
		if err != nil {
			return "", err
		}
		searchRoot = dir
	}
	limit := a.MaxResults
	if limit <= 0 {
		limit = defaultGrepMax
	}
	wantContext := a.Context
	if wantContext < 0 {
		wantContext = 0
	}
	query := grepQuery{pattern: a.Pattern, fixed: a.Fixed, glob: a.Glob, context: wantContext, ignoreCase: a.IgnoreCase}
	if !query.fixed {
		if _, err := regexp.Compile(query.regexSource()); err != nil {
			return "", fmt.Errorf("invalid pattern: %w", err)
		}
	}

	hits, err := g.search(ctx, root, searchRoot, query)
	if err != nil {
		return "", err
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].match.rel != hits[j].match.rel {
			return hits[i].match.rel < hits[j].match.rel
		}
		return hits[i].match.line < hits[j].match.line
	})
	if len(hits) == 0 {
		return fmt.Sprintf("no matches for %q", a.Pattern), nil
	}
	shown := hits
	capped := ""
	if len(hits) > limit {
		shown = hits[:limit]
		capped = fmt.Sprintf("\n... showing first %d of %d (more results)", limit, len(hits))
	}
	return renderGrepHits(hits, shown) + capped, nil
}

// renderGrepHits renders the shown hits: one match line (`path:line:text`)
// per hit with its context lines (`path-line-text`), each source line exactly
// once. A line that matches in any hit is marked a match; context lines never
// match, in either backend.
func renderGrepHits(all, shown []grepHit) string {
	isMatch := map[grepMatch]bool{}
	for _, h := range all {
		isMatch[h.match] = true
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d match(es)\n", len(all))
	emitted := map[grepMatch]bool{}
	for _, h := range shown {
		lines := append([]grepMatch{h.match}, h.context...)
		sort.Slice(lines, func(i, j int) bool { return lines[i].line < lines[j].line })
		for _, m := range lines {
			if emitted[m] {
				continue
			}
			emitted[m] = true
			if isMatch[m] {
				fmt.Fprintf(&sb, "%s:%d:%s\n", m.rel, m.line, m.text)
			} else {
				fmt.Fprintf(&sb, "%s-%d-%s\n", m.rel, m.line, m.text)
			}
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// grepQuery is one search: the pattern plus how to match and filter it. Both
// backends take the same query, so they answer the same question.
type grepQuery struct {
	pattern    string
	fixed      bool
	glob       string
	context    int
	ignoreCase bool
}

// regexSource is the Go regex for a non-fixed query, folded when asked.
func (q grepQuery) regexSource() string {
	if q.ignoreCase {
		return "(?i)" + q.pattern
	}
	return q.pattern
}

// search runs ripgrep when it resolves, and the Go fallback otherwise. A
// failed rg run is a downgrade, not a failure: the walk below answers the
// same question, so a missing or failing rg only costs speed.
func (g grepTool) search(ctx context.Context, root, searchRoot string, q grepQuery) ([]grepHit, error) {
	if !g.fallbackOnly {
		if hits, err := grepWithRg(ctx, root, searchRoot, q); err == nil {
			return hits, nil
		}
	}
	return grepWithGo(ctx, root, searchRoot, q)
}

// rgPath resolves the ripgrep binary: the wick bin directory first, then
// PATH. Empty means no ripgrep is installed.
func rgPath() string {
	if dir := os.Getenv("WICK_BIN_DIR"); dir != "" {
		if p := filepath.Join(dir, "rg"); isExecutable(p) {
			return p
		}
	}
	if home := os.Getenv("HOME"); home != "" {
		if p := filepath.Join(home, ".wick-pig", "bin", "rg"); isExecutable(p) {
			return p
		}
	}
	if p, err := exec.LookPath("rg"); err == nil {
		return p
	}
	return ""
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

// grepWithRg runs rg for one query over searchRoot. Paths come back relative
// to root. An rg failure is an error to the caller, which downgrades to the
// Go search; exit 1 (no match) is an empty result, not a failure.
func grepWithRg(ctx context.Context, root, searchRoot string, q grepQuery) ([]grepHit, error) {
	bin := rgPath()
	if bin == "" {
		return nil, errors.New("no ripgrep installed")
	}
	args := []string{"--no-config", "--color", "never", "-n", "--no-heading", "--with-filename",
		"--max-filesize", "4M",
		// Hidden files are searched (config files live there), but .gitignore
		// applies even outside a git repository, and .git itself never is:
		// it holds objects, not source. The NUL path separator keeps paths
		// with `:` or `-<digits>-` unambiguous.
		"--hidden", "--no-require-git", "-g", "!.git", "-g", "!.git/**", "--null"}
	if q.fixed {
		args = append(args, "-F")
	}
	if q.ignoreCase {
		args = append(args, "-i")
	}
	if q.glob != "" {
		args = append(args, "-g", q.glob)
	}
	if q.context > 0 {
		args = append(args, "-C", strconv.Itoa(q.context))
	}
	args = append(args, "-e", q.pattern, "--", searchRoot)
	cmd := exec.CommandContext(ctx, bin, args...)
	// rg matches slash globs against its working directory, so it runs in
	// the workspace root: `-g sub/*.txt` then means what the Go fallback
	// means, wherever the wick process started.
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	var hits []grepHit
	pending := map[string][]grepMatch{}
	lastIdx := map[string]int{}
	cur := -1
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		// A group separator ends the running hit: the context lines after
		// it belong to the next match, not the last one.
		if line == "--" {
			cur = -1
			continue
		}
		if strings.HasPrefix(line, "Binary file ") {
			continue
		}
		rel, num, text, isMatch, ok := splitRgLine(root, line)
		if !ok {
			continue
		}
		m := grepMatch{rel: rel, line: num, text: text}
		if !isMatch {
			if cur >= 0 && hits[cur].match.rel == rel {
				hits[cur].context = append(hits[cur].context, m)
			} else {
				pending[rel] = append(pending[rel], m)
			}
			continue
		}
		h := grepHit{match: m, context: pending[rel]}
		delete(pending, rel)
		hits = append(hits, h)
		cur = len(hits) - 1
		lastIdx[rel] = cur
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	// Trailing context with no match after it cannot come out of rg, but a
	// partial read still attaches it to the file's last hit rather than
	// dropping a line rg printed.
	for rel, ctx := range pending {
		if idx, ok := lastIdx[rel]; ok {
			hits[idx].context = append(hits[idx].context, ctx...)
		}
	}
	return hits, nil
}

// rgLineRe breaks the `line:text` tail of one rg `--null` output line: the
// line number, then `:` for a match or `-` for context, then the text. The
// path never reaches this regex: `--null` terminates it with a NUL byte,
// which no file name can hold.
var rgLineRe = regexp.MustCompile(`^(\d+)([:-])(.*)$`)

// splitRgLine breaks one rg `--null` output line (`path\0line:text` or
// `path\0line-text`) into its parts, with path made relative to root.
func splitRgLine(root, line string) (rel string, num int, text string, isMatch bool, ok bool) {
	raw, tail, found := strings.Cut(line, "\x00")
	if !found {
		return "", 0, "", false, false
	}
	m := rgLineRe.FindStringSubmatch(tail)
	if m == nil {
		return "", 0, "", false, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		return "", 0, "", false, false
	}
	rel = raw
	if filepath.IsAbs(raw) {
		if r, err := filepath.Rel(root, raw); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			rel = r
		}
	}
	return filepath.ToSlash(rel), n, m[3], m[2] == ":", true
}

// grepWithGo walks searchRoot matching each line against the query. Symlinks
// are never followed, the .git directory is never entered, and binary files
// are skipped.
func grepWithGo(ctx context.Context, root, searchRoot string, q grepQuery) ([]grepHit, error) {
	match := q.matcher()
	if match == nil {
		return nil, errors.New("invalid pattern")
	}
	ignore := newGrepIgnorer(root)
	var hits []grepHit
	walkErr := filepath.WalkDir(searchRoot, func(path string, d os.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			// .git holds objects, not source, and an ignored directory takes
			// its whole subtree with it: git never re-includes under an
			// excluded directory, so pruning here matches rg.
			if d.Name() == ".git" || ignore.ignored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignore.ignored(rel, false) {
			return nil
		}
		if !matchGrepGlob(q.glob, rel) {
			return nil
		}
		data, err := readGrepFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		// A trailing newline is a terminator, not another line: without this
		// a file ending in "\n" reports one line number past its end when
		// context reaches for the line after the last match.
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		hit := make([]bool, len(lines))
		for i, line := range lines {
			hit[i] = match(line)
		}
		for i, line := range lines {
			if !hit[i] {
				continue
			}
			h := grepHit{match: grepMatch{rel: rel, line: i + 1, text: line}}
			// Context is non-matching lines only, like rg: a line that
			// matches is its own hit, never another hit's context. Shared
			// context still reaches every hit; renderGrepHits emits each
			// source line once.
			for j := max(0, i-q.context); j <= min(len(lines)-1, i+q.context); j++ {
				if j != i && !hit[j] {
					h.context = append(h.context, grepMatch{rel: rel, line: j + 1, text: lines[j]})
				}
			}
			hits = append(hits, h)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return hits, nil
}

// matcher builds the line predicate for a query. A fixed query is a substring
// search; otherwise the pattern is already known to compile (Execute checked),
// so a nil return only happens when it does not.
func (q grepQuery) matcher() func(string) bool {
	if q.fixed {
		if q.ignoreCase {
			needle := strings.ToLower(q.pattern)
			return func(line string) bool { return strings.Contains(strings.ToLower(line), needle) }
		}
		return func(line string) bool { return strings.Contains(line, q.pattern) }
	}
	re, err := regexp.Compile(q.regexSource())
	if err != nil {
		return nil
	}
	return re.MatchString
}

// matchGrepGlob reports whether rel passes the glob filter. Empty means no
// filter. A pattern without a slash matches the basename (so "*.txt" reaches
// every directory, like rg -g); otherwise the whole relative path is matched,
// with "**" crossing directories.
func matchGrepGlob(pattern, rel string) bool {
	if pattern == "" {
		return true
	}
	if !strings.Contains(pattern, "/") {
		if ok, _ := path.Match(pattern, path.Base(rel)); ok {
			return true
		}
	}
	if ok, _ := path.Match(pattern, rel); ok {
		return true
	}
	if strings.Contains(pattern, "**") {
		if ok, _ := regexp.MatchString("^"+globToRegexp(pattern)+"$", rel); ok {
			return true
		}
	}
	return false
}

// globToRegexp converts a glob into its regex: "**" crosses directories,
// "*" and "?" stay inside one, and "[...]" classes pass through. Every "**/"
// also matches zero directories, so `sub/**`+`/x` reaches `sub/x` itself.
func globToRegexp(glob string) string {
	var sb strings.Builder
	for i := 0; i < len(glob); i++ {
		switch glob[i] {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				if i+2 < len(glob) && glob[i+2] == '/' {
					sb.WriteString("(.*/)?")
					i += 2
				} else {
					sb.WriteString(".*")
					i++
				}
			} else {
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		case '[':
			end := strings.IndexByte(glob[i:], ']')
			if end < 0 {
				sb.WriteString("\\[")
				continue
			}
			class := glob[i+1 : i+end]
			class = strings.ReplaceAll(class, "\\", "\\\\")
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			sb.WriteString("[" + class + "]")
			i += end
		case '.', '+', '(', ')', '|', '^', '$', ']', '{', '}', '\\':
			sb.WriteByte('\\')
			sb.WriteByte(glob[i])
		default:
			sb.WriteByte(glob[i])
		}
	}
	return sb.String()
}

// readGrepFile reads a file for searching. Files over 4MB (like rg's
// --max-filesize) and binary files are skipped with no match and no error.
func readGrepFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4*1024*1024 {
		return nil, errors.New("skipped")
	}
	// #nosec G304 -- grep intentionally reads files under the caller-selected workspace root
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !isSearchableText(data) {
		return nil, errors.New("binary")
	}
	return data, nil
}

// isSearchableText reports whether data is text worth searching: no NUL byte.
// A NUL check matches rg's binary detection closely enough that both backends
// agree on the fixtures.
func isSearchableText(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// .gitignore
// ---------------------------------------------------------------------------

// gitignoreRule is one parsed .gitignore line: the glob to match, whether it
// re-includes, whether it only matches directories, and whether it anchors at
// the .gitignore's own directory.
type gitignoreRule struct {
	pattern  string
	negated  bool
	dirOnly  bool
	anchored bool
	re       *regexp.Regexp
}

// parseGitignoreRule parses one .gitignore line. Empty lines and comments are
// not rules; an escaped leading "#" or "!" is a literal character, checked
// before the comment rule so it survives it.
func parseGitignoreRule(line string) *gitignoreRule {
	r := gitignoreRule{}
	switch {
	case line == "":
		return nil
	case strings.HasPrefix(line, "\\#") || strings.HasPrefix(line, "\\!"):
		line = line[1:]
	case strings.HasPrefix(line, "#"):
		return nil
	case strings.HasPrefix(line, "!"):
		r.negated = true
		line = line[1:]
	}
	if line == "" {
		return nil
	}
	if strings.HasSuffix(line, "/") {
		r.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	if strings.HasPrefix(line, "/") {
		r.anchored = true
		line = strings.TrimPrefix(line, "/")
	} else if strings.Contains(line, "/") {
		r.anchored = true
	}
	if line == "" {
		return nil
	}
	r.pattern = line
	// A bare name matches at any depth; anything with a slash anchors at the
	// .gitignore's directory.
	source := line
	if !r.anchored {
		source = "**/" + line
	}
	re, err := regexp.Compile("^" + globToRegexp(source) + "$")
	if err != nil {
		return nil
	}
	r.re = re
	return &r
}

// grepIgnorer applies the workspace's .gitignore files to workspace-relative
// paths. Files are loaded lazily per directory and cached for the search.
type grepIgnorer struct {
	root  string
	files map[string][]gitignoreRule
}

func newGrepIgnorer(root string) *grepIgnorer {
	return &grepIgnorer{root: root, files: map[string][]gitignoreRule{}}
}

// rulesFor returns the rules of the .gitignore in dir (workspace-relative,
// "" for the root), loading it once.
func (g *grepIgnorer) rulesFor(dir string) []gitignoreRule {
	if rules, ok := g.files[dir]; ok {
		return rules
	}
	var rules []gitignoreRule
	// #nosec G304 -- grep reads .gitignore files under its own workspace root
	if data, err := os.ReadFile(filepath.Join(g.root, filepath.FromSlash(dir), ".gitignore")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if r := parseGitignoreRule(strings.TrimRight(line, "\r")); r != nil {
				rules = append(rules, *r)
			}
		}
	}
	g.files[dir] = rules
	return rules
}

// ignored reports whether the workspace-relative path is ignored: the last
// matching rule across the .gitignore files from the root down wins, so a
// "!" rule re-includes. dirOnly rules match directories only; anything
// matching a directory prunes its subtree before files are visited.
func (g *grepIgnorer) ignored(rel string, isDir bool) bool {
	ignored := false
	dir := path.Dir(rel)
	chain := []string{""}
	if dir != "." {
		parts := strings.Split(dir, "/")
		for i := range parts {
			chain = append(chain, strings.Join(parts[:i+1], "/"))
		}
	}
	for _, d := range chain {
		under := rel
		if d != "" {
			under = strings.TrimPrefix(rel, d+"/")
		}
		for _, r := range g.rulesFor(d) {
			if r.dirOnly && !isDir {
				continue
			}
			if r.re.MatchString(under) {
				ignored = !r.negated
			}
		}
	}
	return ignored
}
