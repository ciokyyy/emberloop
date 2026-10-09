package el

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// agentTypeDir is where a repo adds or overrides agent types.
const agentTypeDir = ".emberloop/agents"

//go:embed agenttypes/scout.md
var builtinScout string

//go:embed agenttypes/worker.md
var builtinWorker string

//go:embed agenttypes/reviewer-standards.md
var builtinReviewerStandards string

//go:embed agenttypes/reviewer-spec.md
var builtinReviewerSpec string

// AgentType is one kind of spawned agent, defined by a markdown file with
// frontmatter rather than by code. A type says what the agent may do (tools),
// where it runs (checkout), whether spawning it blocks, what it may report
// (statuses) and what it may spawn itself.
type AgentType struct {
	Name        string
	Description string
	Tools       []string
	// Checkout is worktree (its own git worktree), none (runs in the repo,
	// read-only tools only) or parent (runs in the spawning agent's worktree).
	Checkout   string
	Blocking   bool
	Statuses   []string
	Spawns     []string
	Persistent bool
	// PassHolds makes a pass outlive later commits: the reviewer is only
	// re-prompted while its last verdict is not a pass. A type whose pass is
	// about the ticket, not the code's shape, sets it.
	PassHolds      bool
	RequiresReview []string
	Harness        string
	Model          string
	Effort         string
	// Body is the brief or prompt template the agent is launched with.
	Body string
	// file is where the definition came from, for error messages.
	file string
}

// builtinAgentSources are the types el embeds. A repo file with the same
// name overrides one.
var builtinAgentSources = map[string]string{
	"scout":              builtinScout,
	"worker":             builtinWorker,
	"reviewer-standards": builtinReviewerStandards,
	"reviewer-spec":      builtinReviewerSpec,
}

// hostToolNames are the MCP tools a type's `tools` field may name. `submit` is
// added to every spawned agent by the runtime, so it is not listed here.
// grep, git and check live behind execute (their default placement), so a
// type lists them to reach them inside scripts without serving them in main.
var hostToolNames = []string{"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "grep", "git", "check", "execute"}

// readOnlyToolNames are the host tools a `none` (read-only) type may use. A
// scout reaches grep and git only inside its sandboxed execute, never check.
var readOnlyToolNames = []string{"read", "find", "websearch", "ctx_search", "grep", "git", "execute"}

// LoadAgentTypes returns the built-in types, then every type in
// <root>/.emberloop/agents/*.md, a repo file overriding a built-in of the same name.
// Every definition is validated here; an invalid one is an error naming its
// file and the field at fault.
func LoadAgentTypes(root string) (map[string]AgentType, error) {
	types := map[string]AgentType{}
	for _, name := range sortedKeys(builtinAgentSources) {
		t, err := parseAgentType(builtinAgentSources[name], "built-in "+name+".md")
		if err != nil {
			return nil, err
		}
		types[t.Name] = t
	}

	dir := filepath.Join(root, agentTypeDir)
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		t, err := parseAgentType(string(raw), path)
		if err != nil {
			return nil, err
		}
		types[t.Name] = t
	}

	for _, name := range sortedTypeNames(types) {
		t := types[name]
		for _, ref := range append(append([]string{}, t.Spawns...), t.RequiresReview...) {
			if _, ok := types[ref]; !ok {
				return nil, fmt.Errorf("%s: %s names unknown type %q", t.file, "spawns/requires_review", ref)
			}
		}
	}
	return types, nil
}

// mustLoadAgentTypes loads the types, falling back to the built-ins when a repo
// file is invalid: the callers that must report that error (RoleTools, the MCP
// server start) call LoadAgentTypes directly.
func mustLoadAgentTypes(root string) map[string]AgentType {
	types, err := LoadAgentTypes(root)
	if err != nil {
		types = map[string]AgentType{}
		for name, src := range builtinAgentSources {
			if t, err := parseAgentType(src, "built-in "+name+".md"); err == nil {
				types[t.Name] = t
			}
		}
	}
	return types
}

// allTypeNames lists the names of types in sorted order, so a description built
// from them is stable.
func allTypeNames(types map[string]AgentType) []string { return sortedTypeNames(types) }

func sortedTypeNames(types map[string]AgentType) []string {
	names := make([]string, 0, len(types))
	for n := range types {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// parseAgentType reads one markdown definition and validates it, naming file
// and field on any fault.
func parseAgentType(src, file string) (AgentType, error) {
	head, body, err := splitFrontmatter(src)
	if err != nil {
		return AgentType{}, fmt.Errorf("%s: %w", file, err)
	}
	fields, err := parseFields(head)
	if err != nil {
		return AgentType{}, fmt.Errorf("%s: %w", file, err)
	}

	t := AgentType{file: file, Body: strings.TrimSpace(body)}

	name, ok := fields.scalar("name")
	if !ok || name == "" {
		return AgentType{}, fmt.Errorf("%s: name is required", file)
	}
	t.Name = name

	desc, ok := fields.scalar("description")
	if !ok || desc == "" {
		return AgentType{}, fmt.Errorf("%s: description is required", file)
	}
	t.Description = desc

	tools, ok := fields.list("tools")
	if !ok || len(tools) == 0 {
		return AgentType{}, fmt.Errorf("%s: tools is required", file)
	}
	t.Tools = tools

	checkout, ok := fields.scalar("checkout")
	if !ok || checkout == "" {
		return AgentType{}, fmt.Errorf("%s: checkout is required", file)
	}
	switch checkout {
	case "worktree", "none", "parent":
	default:
		return AgentType{}, fmt.Errorf("%s: checkout must be worktree, none or parent, got %q", file, checkout)
	}
	t.Checkout = checkout

	blockingRaw, ok := fields.scalar("blocking")
	if !ok {
		return AgentType{}, fmt.Errorf("%s: blocking is required", file)
	}
	blocking, err := parseBool(blockingRaw)
	if err != nil {
		return AgentType{}, fmt.Errorf("%s: blocking %v", file, err)
	}
	t.Blocking = blocking

	statuses, ok := fields.list("statuses")
	if !ok || len(statuses) == 0 {
		return AgentType{}, fmt.Errorf("%s: statuses is required", file)
	}
	t.Statuses = statuses

	for _, name := range t.Tools {
		if !contains(hostToolNames, name) {
			return AgentType{}, fmt.Errorf("%s: tools names unknown tool %q", file, name)
		}
	}
	if t.Checkout == "none" {
		for _, name := range t.Tools {
			if !contains(readOnlyToolNames, name) {
				return AgentType{}, fmt.Errorf("%s: a none (read-only) type may not use tool %q", file, name)
			}
		}
	}

	if v, ok := fields.list("spawns"); ok {
		t.Spawns = v
	}
	if v, ok := fields.scalar("persistent"); ok {
		p, err := parseBool(v)
		if err != nil {
			return AgentType{}, fmt.Errorf("%s: persistent %v", file, err)
		}
		t.Persistent = p
	}
	if v, ok := fields.scalar("pass_holds"); ok {
		p, err := parseBool(v)
		if err != nil {
			return AgentType{}, fmt.Errorf("%s: pass_holds %v", file, err)
		}
		t.PassHolds = p
	}
	if v, ok := fields.list("requires_review"); ok {
		t.RequiresReview = v
	}
	if v, ok := fields.scalar("harness"); ok {
		t.Harness = v
	}
	if v, ok := fields.scalar("model"); ok {
		t.Model = v
	}
	if v, ok := fields.scalar("effort"); ok {
		t.Effort = v
	}
	return t, nil
}

func parseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("must be true or false, got %q", v)
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

func (f frontmatterFields) list(key string) ([]string, bool) {
	v, ok := f.lists[key]
	return v, ok
}

// parseFields reads the small YAML subset a type definition uses: `key: value`,
// `key: [a, b]`, and block lists of `- item` under a bare `key:`. Unknown keys
// are ignored, so a definition can carry fields a later ticket adds.
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
