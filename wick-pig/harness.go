package wick

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// systemPrompt is the rule set every wick session carries. The tools are
// wick's own; background exists because a blocking bash call on a dev server
// never returns. The skill sentence names the one address the read tool serves
// the library under, so an open session can load a skill the user names.
var systemPrompt = "Your file and shell tools are wick's: read, find, edit, write, bash, background. " +
	"Never start a dev server, watcher or log tail with bash: it blocks until timeout. Use background(action=start) and read or stop it later. " +
	"For facts about this repo, spawn a scout when you have one: spawn(agents=[{type: \"scout\", prompt: \"...\"}]). Scouts report facts only; decisions stay with you and the user. " +
	"Skills are listed by reading wick://" + skillsNamespace + "; when the user asks for one by name " +
	"(grill, research, wayfinder, tdd, to-spec), read the matching skill and follow it exactly. " +
	"When a spawn starts agents, end your turn: one notice with every result arrives when the last agent of that call has submitted. Never poll with sleep or bash while agents run." +
	" A spawned agent yields right after submit: end the turn at once, with no text." +
	" " + voiceRegister

// Session is everything an adapter needs to start one agent.
type Session struct {
	Role  string // picks the tool set served by wick-mcp
	Model string
	// Effort is the reasoning effort in the harness's own vocabulary; empty
	// means the harness's default.
	Effort   string
	Cwd      string // the directory it runs in; trusted before launch when set
	Prompt   string // first message
	Headless bool   // print one answer and exit
	ID       string // session id; names the transcript so the reply can be read back
	Dir      string // where a harness that takes a session dir should keep it
	Label    string // names the session so herdr can address it
	Spec     string // the spec an implement session runs; its workers record it
	// AgentType and AgentID are set for a spawned agent: its type's tools and
	// its submit state are keyed by them.
	AgentType string
	AgentID   string
}

// Command is a ready-to-exec invocation.
type Command struct {
	Bin  string
	Args []string
	Env  []string // appended to the parent environment
}

// Harness turns a Session into the command that starts it. A new agent CLI is
// one type implementing this and one Register call; nothing else in wick
// (launcher, workers, reviewers, scouts) names a harness.
type Harness interface {
	Name() string
	Command(Session) (Command, error)
	// SessionRoot is the directory the harness writes this session's
	// transcript under; the file name carries Session.ID.
	SessionRoot(Session) string
	// LastReply is the text of the final assistant message of a finished
	// turn, and false while the agent is still mid-turn.
	LastReply(transcriptPath string) (string, bool)
	// TurnState is TurnDone, TurnFailed (with the error text) or
	// TurnRunning, from the last record that says which.
	TurnState(transcriptPath string) (state, detail string)
	// Trust marks dir as trusted so an interactive session starts without a
	// folder-trust prompt that would sit unanswered in a pane.
	Trust(dir string) error
	// LatePrompt reports that the first message must be sent once the agent
	// is ready, not as a startup argument. claude connects its MCP servers
	// after it starts, so an argument prompt runs with no tools and the model
	// invents tool calls.
	LatePrompt() bool
	// Models lists the models this harness offers, and the effort levels each
	// takes. It is the one source for pickers and for validation, and it runs
	// through the given Exec so a test can feed it captured CLI output.
	Models(run Exec) ([]Model, error)
	// Gate is how this harness can hold an agent at a stop, so one decision
	// serves every harness.
	Gate() Gate
}

// Gate is a harness's stop-gate capability: the part of the decision the
// harness can enforce. Only a harness that can pin the submit tool gets the
// 3rd-attempt pin; claude cannot, so its 3rd block is its last.
type Gate interface {
	CanPin() bool
}

var registry = map[string]Harness{}

// Register adds a harness. The built-in ones register in init.
func Register(h Harness) { registry[h.Name()] = h }

func init() {
	Register(pigHarness{})
	Register(claudeHarness{})
}

// HarnessNames lists the registered harnesses.
func HarnessNames() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// withAgentID gives every session an agent id, so its wick-mcp knows who it
// serves. An explicit id wins; otherwise the harness session id names it, then
// the label (a worker's ticket id), then a fresh id for a bare session with
// neither. It is applied in the session builder (BuildCommand/PrepareLaunch),
// so top-level sessions get one too.
func (s Session) withAgentID() Session {
	if s.AgentID != "" {
		return s
	}
	switch {
	case s.ID != "":
		s.AgentID = s.ID
	case s.Label != "":
		s.AgentID = s.Label
	default:
		s.AgentID = newSessionID()
	}
	return s
}

// BuildCommand renders a session for one harness.
func BuildCommand(harness string, s Session) (Command, error) {
	h, ok := registry[harness]
	if !ok {
		return Command{}, fmt.Errorf("unknown harness %q (have %v)", harness, HarnessNames())
	}
	s = s.withAgentID()
	if s.Cwd != "" {
		if err := h.Trust(s.Cwd); err != nil {
			return Command{}, fmt.Errorf("trust %s for %s: %w", s.Cwd, harness, err)
		}
	}
	return h.Command(s)
}

// Line is the command as one shell line, which is what pane run types:
// env assignments first, then the program.
func (c Command) Line() string {
	words := make([]string, 0, len(c.Env)+1+len(c.Args))
	for _, e := range c.Env {
		k, v, _ := strings.Cut(e, "=")
		words = append(words, k+"="+shellArg(v))
	}
	words = append(words, c.Bin)
	for _, a := range c.Args {
		words = append(words, shellArg(a))
	}
	return strings.Join(words, " ")
}

// roleEnv is what the session's wick process needs to know: its role, the
// agent type and id of a spawned agent, for an implement session the spec
// (which spawn hands to every worker), and the session id naming its harness
// transcript. The session dir (pig only: claude takes none) and the launch
// choice (harness, model, effort) come from the harness command beside it;
// together they let the session's own wick process (the wick-mcp attached to
// the harness) snapshot the session's blobs and tail its transcript into the
// archive.
func roleEnv(s Session) []string {
	env := []string{"WICK_ROLE=" + s.Role}
	if s.AgentType != "" {
		env = append(env, "WICK_AGENT_TYPE="+s.AgentType)
	}
	if s.AgentID != "" {
		env = append(env, "WICK_AGENT_ID="+s.AgentID)
	}
	if s.Spec != "" {
		env = append(env, "WICK_SPEC="+s.Spec)
	}
	if s.ID != "" {
		env = append(env, "WICK_SESSION_ID="+s.ID)
	}
	return env
}

// harnessEnv is the launch choice the session's wick process records in its
// flags blob: the harness, model and effort the session started with.
func harnessEnv(harness string, s Session) []string {
	env := []string{"WICK_HARNESS=" + harness}
	if s.Model != "" {
		env = append(env, "WICK_MODEL="+s.Model)
	}
	if s.Effort != "" {
		env = append(env, "WICK_EFFORT="+s.Effort)
	}
	return env
}

// pigHarness: pig loads wick-mcp from its mcp.json, so the built-in tools are
// switched off and the role's MCP tool set is all that is left.
type pigHarness struct{}

func (pigHarness) Name() string { return "pig" }

// pigSessionFileID is the id pig names the transcript file after: the
// session id, else the ticket label, mirroring --session-id above.
func pigSessionFileID(s Session) string {
	if s.ID != "" {
		return s.ID
	}
	return s.Label
}

func (pigHarness) Command(s Session) (Command, error) {
	// --no-skills keeps the harness from loading the user's global library:
	// a session reaches skills only by the wick:// addresses its prompt names.
	args := []string{"--no-builtin-tools", "--no-skills", "--append-system-prompt", systemPrompt}
	if ext := reportExtension(runningExe(), os.Getenv("HOME")); ext != "" && !s.Headless {
		args = append(args, "-e", ext)
	}
	if s.Headless {
		args = append(args, "-p", "--no-session")
	} else {
		// A named session is what makes the pane addressable by herdr, and a
		// known id and dir are what let the reply be read back.
		id := pigSessionFileID(s)
		if id != "" {
			args = append(args, "--session-id", id)
		}
		if s.Dir != "" {
			args = append(args, "--session-dir", s.Dir)
		}
	}
	if s.Model != "" {
		args = append(args, "--model", s.Model)
	}
	if s.Effort != "" {
		args = append(args, "--thinking", s.Effort)
	}
	args = appendPrompt(args, s.Prompt)
	env := append(roleEnv(s), harnessEnv("pig", s)...)
	if s.Label != "" {
		env = append(env, "WICK_AGENT_LABEL="+s.Label)
	}
	// The session's wick-mcp tails this transcript for the usage log; the
	// id is what the transcript file carries, the dir where it looks.
	env = append(env, usageTailEnv("pig", pigSessionFileID(s), s.Dir)...)
	return Command{Bin: pigBin(), Args: args, Env: env}, nil
}

func (pigHarness) SessionRoot(s Session) string {
	if s.Dir != "" {
		return s.Dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pig", "agent", "sessions")
}

func (pigHarness) LastReply(path string) (string, bool) {
	return lastRecord(path, func(rec map[string]any) (string, bool) {
		m, _ := rec["message"].(map[string]any)
		if rec["type"] != "message" || m["role"] != "assistant" || m["stopReason"] != "stop" {
			return "", false
		}
		return textOf(m["content"]), true
	})
}

func (pigHarness) TurnState(path string) (string, string) {
	return lastTurn(path, func(rec map[string]any) (string, string, bool) {
		m, _ := rec["message"].(map[string]any)
		if rec["type"] != "message" {
			return "", "", false
		}
		switch {
		case m["role"] == "assistant" && m["stopReason"] == "error":
			detail, _ := m["errorMessage"].(string)
			return TurnFailed, detail, true
		case m["role"] == "assistant" && m["stopReason"] == "stop":
			return TurnDone, "", true
		}
		return TurnRunning, "", true
	})
}

func (claudeHarness) TurnState(path string) (string, string) {
	return lastTurn(path, func(rec map[string]any) (string, string, bool) {
		m, _ := rec["message"].(map[string]any)
		switch {
		case rec["type"] == "assistant" && rec["isApiErrorMessage"] == true:
			return TurnFailed, textOf(m["content"]), true
		case rec["type"] == "assistant" && m["stop_reason"] == "end_turn":
			return TurnDone, "", true
		case rec["type"] == "assistant" || rec["type"] == "user":
			return TurnRunning, "", true
		}
		return "", "", false
	})
}

func (claudeHarness) SessionRoot(Session) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

func (claudeHarness) LastReply(path string) (string, bool) {
	return lastRecord(path, func(rec map[string]any) (string, bool) {
		m, _ := rec["message"].(map[string]any)
		if rec["type"] != "assistant" || m["stop_reason"] != "end_turn" {
			return "", false
		}
		return textOf(m["content"]), true
	})
}

// claudeStopGate is the settings claude is launched with: a `Stop` hook that
// runs wick's shared gate, so a claude agent cannot end a turn without
// submitting, and a `UserPromptSubmit` hook that adds the register's per-turn
// reminder to every user message, as the caveman plugin does, and turns off
// the away recap (awaySummaryEnabled; the env var does the same), hides spinner tips and skips the bypass-mode prompt. It is passed
// inline with --settings, which still applies under --setting-sources "".
const claudeStopGate = `{"awaySummaryEnabled":false,"spinnerTipsEnabled":false,"skipDangerousModePermissionPrompt":true,"hooks":{"Stop":[{"hooks":[{"type":"command","command":"wick-pig stop-gate"}]}],` +
	`"UserPromptSubmit":[{"hooks":[{"type":"command","command":"wick-pig voice-reminder hook"}]}]}}`

type claudeHarness struct{}

func (claudeHarness) Name() string { return "claude" }

func (claudeHarness) Command(s Session) (Command, error) {
	// Claude starts wick-mcp from this config, not from its own environment:
	// an MCP stdio server gets the default env merged with this map, so the
	// session identity the tailer reads (like the WICK_AGENT_ID the tools
	// already rely on) travels here, as does the launch choice the archiver
	// stores in the flags blob. Pig's wick-mcp instead inherits its process
	// env, which the pig command covers the same way.
	mcpEnv := append(roleEnv(s), harnessEnv("claude", s)...)
	mcpEnv = append(mcpEnv, usageTailEnv("claude", s.ID, "")...)
	cfg, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"wick": map[string]any{
		"command": "wick-mcp", "env": envMap(mcpEnv),
	}}})
	if err != nil {
		return Command{}, err
	}
	path := mcpConfigPath(s)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Command{}, err
	}
	if err := os.WriteFile(path, cfg, 0o600); err != nil {
		return Command{}, err
	}
	args := []string{
		"--tools", "", "--setting-sources", "", "--settings", claudeStopGate,
		"--strict-mcp-config",
		"--mcp-config", path, "--system-prompt", systemPrompt,
		// -p cannot prompt for permission; the served tool set is the policy.
		// Bypass mode never stops to ask, and replaces the auto mode footer.
		// Slash commands stay on so /model and /resume work; --tools "" keeps
		// the Skill tool away from the model.
		"--allowedTools", "mcp__wick", "--permission-mode", "bypassPermissions",
	}
	if s.Headless {
		args = append(args, "-p", "--no-session-persistence")
	} else if s.ID != "" {
		args = append(args, "--session-id", s.ID)
	}
	if s.Model != "" {
		args = append(args, "--model", s.Model)
	}
	if s.Effort != "" {
		args = append(args, "--effort", s.Effort)
	}
	args = appendPrompt(args, s.Prompt)
	// Claude's transcript is named by its session id; without one there is
	// nothing for the session's wick-mcp to tail. Claude takes no session
	// dir, so none is passed. The process env carries the same identity as
	// the MCP config above, plus claude's own switches.
	return Command{Bin: "claude", Args: args, Env: append(mcpEnv,
		"CLAUDE_CODE_DISABLE_AUTO_MEMORY=1", "CLAUDE_CODE_DISABLE_CLAUDE_MDS=1",
		"CLAUDE_CODE_DISABLE_GIT_INSTRUCTIONS=1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1", "CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0",
		"CLAUDE_CODE_ENABLE_AWAY_SUMMARY=0",
		"DISABLE_TELEMETRY=1", "DISABLE_ERROR_REPORTING=1")}, nil
}

// mcpConfigPath is where the MCP config of one claude session is written: under
// wick's state dir, named for the session, so whoever owns the session's
// lifetime can delete it. A temp file outlives every launch that makes one.
func mcpConfigPath(s Session) string {
	key := s.ID
	if key == "" {
		key = s.Label
	}
	if key == "" {
		key = s.Role
	}
	return filepath.Join(filepath.Dir(statePath), "mcp", key+".json")
}

// CleanupSession removes the per-session files a harness launch left behind:
// the MCP config claude reads at startup and the launch file `wick-pig exec`
// reads, both under wick's state dir. Call it when the session ends; a file
// that is already gone is the wanted end state, not a failure. `wick-pig
// exec` already consumes the launch file once it has read it, so this is the
// backup for a launch that never started.
func CleanupSession(s Session) { _ = os.Remove(mcpConfigPath(s)); _ = os.Remove(launchPath(s)) }

// cleanupMCP removes only the MCP config, for launches whose launch file the
// pane's `wick-pig exec` still has to consume: deleting it right after pane
// run would race the agent's startup.
func cleanupMCP(s Session) { _ = os.Remove(mcpConfigPath(s)) }

// Skill lookups for the launcher.

// SkillEntry is one wayfinder-route entry point.
type SkillEntry struct {
	Name, Skill string
	Scout       bool
}

// SkillEntries lists the per-skill subcommands.
func SkillEntries() []SkillEntry {
	out := make([]SkillEntry, len(skillCommands))
	for i, c := range skillCommands {
		out[i] = SkillEntry{c.name, c.skill, c.scout}
	}
	return out
}

// SkillPrompt and ImplementPrompt expose the prompt builders.
func SkillPrompt(skill, args string, scout bool) string {
	return skillPrompt(skill, args, scout)
}

func ImplementPrompt(run SpecRun) string { return implementPrompt(run) }

// appendPrompt adds the first message; an empty one means an open session.
func appendPrompt(args []string, prompt string) []string {
	if prompt == "" {
		return args
	}
	return append(args, "--", prompt)
}

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	return m
}

// pig has no folder-trust prompt for the sessions wick starts.
func (pigHarness) Trust(string) error { return nil }

// Trust sets hasTrustDialogAccepted for dir in ~/.claude.json, the flag the
// dialog writes when a person accepts it.
// ponytail: read-modify-write without a lock; a claude running at the same
// instant can overwrite it, and the dialog would then just appear once more.
func (claudeHarness) Trust(dir string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude.json")
	cfg := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &cfg); err != nil {
			return fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	projects, _ := cfg["projects"].(map[string]any)
	if projects == nil {
		projects = map[string]any{}
	}
	entry, _ := projects[dir].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}
	if entry["hasTrustDialogAccepted"] == true {
		return nil
	}
	entry["hasTrustDialogAccepted"] = true
	projects[dir], cfg["projects"] = entry, projects
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".wick.tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (pigHarness) LatePrompt() bool    { return false }
func (claudeHarness) LatePrompt() bool { return true }

// pig can pin tool_choice at the API level, so its 3rd block forces submit.
func (pigHarness) Gate() Gate { return pigGate{} }

// claude cannot pin a tool, so its gate blocks and reminds only.
func (claudeHarness) Gate() Gate { return claudeGate{} }

type pigGate struct{}
type claudeGate struct{}

func (pigGate) CanPin() bool    { return true }
func (claudeGate) CanPin() bool { return false }

// Launch is a session ready to type into a pane: the command line, and the
// first message when the harness wants it delivered after startup.
type Launch struct {
	Line string
	Late string
}

// PrepareLaunch writes the session's complete launch (binary, args including
// the full prompt, env) to a launch file in wick's state dir and returns the
// one shell-agnostic line the pane runs: `wick-pig exec <launch-file>`. The
// prompt stays out of the typed line, so newlines and quoting survive and the
// line is one line in fish, bash and zsh alike. A harness that takes its
// first message after startup keeps it in Late: the launch file holds the
// command without it, and the typed line is still the single exec line.
func PrepareLaunch(harness string, s Session) (Launch, error) {
	h, ok := registry[harness]
	if !ok {
		return Launch{}, fmt.Errorf("unknown harness %q (have %v)", harness, HarnessNames())
	}
	s = s.withAgentID()
	var l Launch
	if h.LatePrompt() {
		l.Late, s.Prompt = s.Prompt, ""
	}
	cmd, err := BuildCommand(harness, s)
	if err != nil {
		return Launch{}, err
	}
	path, err := writeLaunch(s, cmd)
	if err != nil {
		cleanupMCP(s)
		return Launch{}, err
	}
	l.Line = execLine(path)
	return l, nil
}

// DeliverLate waits for the agent in pane to be ready, then sends the message
// through the shared DeliverPrompt path.
func DeliverLate(run Exec, pane, text string) error {
	if text == "" {
		return nil
	}
	// herdr only knows the pane as an agent once it has seen the process
	// start, so "not found" right after pane run means "not yet".
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, err := herdrJSON(run, []string{"agent", "wait", pane, "--until", "idle", "--timeout", "90000"}, 100*time.Second)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "not found") || time.Now().After(deadline) {
			return fmt.Errorf("agent in %s never became ready: %w", pane, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return DeliverPrompt(run, pane, text)
}

// Registered is the harness registered under name, or nil.
func Registered(name string) Harness { return registry[name] }
