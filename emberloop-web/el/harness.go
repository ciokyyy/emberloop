package el

import (
	"os"
	"path/filepath"
	"sort"
)

// Session names one agent session for transcript lookup: the harness that
// wrote it, and the session and directory ids its transcript file carries.
type Session struct {
	Role  string // picks the tool set served by el-mcp
	Model string
	// Effort is the reasoning effort in the harness's own vocabulary; empty
	// means the harness's default.
	Effort   string
	Cwd      string // the directory it runs in
	Prompt   string // first message
	Headless bool   // print one answer and exit
	ID       string // session id; names the transcript so the reply can be read back
	Dir      string // where a harness that takes a session dir should keep it
	Label    string // names the session
	Spec     string // the spec a run session works from
	// AgentType and AgentID identify a spawned agent.
	AgentType string
	AgentID   string
}

// Harness is the transcript layout of one agent CLI, plus its model list:
// where it writes a session's transcript, and which models `el settings
// models` can offer. The archiver and the usage tailer resolve transcripts
// through the registry, so they never name a harness.
type Harness interface {
	Name() string
	// SessionRoot is the directory the harness writes this session's
	// transcript under; the file name carries Session.ID.
	SessionRoot(Session) string
	// Models lists the models this harness offers, and the effort levels each
	// takes. It is the one source for pickers and for validation, and it runs
	// through the given Exec so a test can feed it captured CLI output.
	Models(run Exec) ([]Model, error)
}

var registry = map[string]Harness{}

// Register adds a harness. The built-in ones register in init.
func Register(h Harness) { registry[h.Name()] = h }

func init() {
	Register(pigHarness{})
	Register(claudeHarness{})
}

// Registered returns the harness with this name, or nil.
func Registered(name string) Harness { return registry[name] }

// HarnessNames lists the registered harnesses.
func HarnessNames() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

type pigHarness struct{}

func (pigHarness) Name() string { return "pig" }

func (pigHarness) SessionRoot(s Session) string {
	if s.Dir != "" {
		return s.Dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pig", "agent", "sessions")
}

type claudeHarness struct{}

func (claudeHarness) Name() string { return "claude" }

func (claudeHarness) SessionRoot(Session) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}
