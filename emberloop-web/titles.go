package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// titleSystemPrompt is the whole system prompt of a title call. For claude it
// replaces the adapter's claude_code preset (about 4x fewer tokens in the
// ticket probe); for pig it is the only context besides the user message.
const titleSystemPrompt = "You write session titles. Reply with only a 3-6 word title, no quotes, no punctuation."

const (
	// titleInputRunes caps how much of the first prompt a title call sees.
	titleInputRunes = 600
	// titleMaxRunes caps the stored title.
	titleMaxRunes = 60
	// titleMaxTokens leaves room for a reasoning model to think before it
	// writes the title; a tiny cap ends the reply with only thinking.
	titleMaxTokens = 2048
	// titleCallTimeout bounds one title call; a title is best-effort.
	titleCallTimeout = time.Minute
	// titleDefaultClaudeModel titles claude Sessions when no title role is set.
	titleDefaultClaudeModel = "haiku"
	// pigTitleRole is the settings role naming the model that titles pig Sessions.
	pigTitleRole = "pig-title"
)

// titleBackend is the one-off completion seam. Claude implements it with a
// throwaway ACP session, pig with a bare SDK completion: no tools, no history.
// dirs are the owner's dirs, so a throwaway session writes to the owner's
// claude config dir and never the host's.
type titleBackend interface {
	Title(dirs ownerDirs, model, prompt string) (string, error)
}

// titlePrompt wraps the first user message for the title call.
func titlePrompt(first string) string {
	first = strings.Join(strings.Fields(first), " ")
	if r := []rune(first); len(r) > titleInputRunes {
		first = string(r[:titleInputRunes])
	}
	return "Title this chat: '" + first + "'"
}

// cleanTitle reduces a model reply to a plain one-line title: the first
// non-empty line, markdown and quote marks stripped, capped in length.
func cleanTitle(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.Map(func(r rune) rune {
			switch r {
			case '*', '#', '`', '"', '\'', '_':
				return -1
			}
			return r
		}, line)
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
		line = strings.TrimRight(line, ".:;,! ")
		line = strings.Join(strings.Fields(line), " ")
		if r := []rune(line); len(r) > titleMaxRunes {
			line = strings.TrimSpace(string(r[:titleMaxRunes]))
		}
		return line
	}
	return ""
}

// titleTarget picks the harness and model that title a Session. Claude
// Sessions title with haiku while the claude naming switch is on. Pig
// Sessions title with the "pig-title" role's pig model and stay untitled
// without one, so pig never silently spends its own. Other harnesses never
// title.
func titleTarget(sessionHarness string) (string, string) {
	switch sessionHarness {
	case "claude":
		if titlesEnabled() {
			return "claude", titleDefaultClaudeModel
		}
	case "pig":
		if titlesEnvOff() {
			return "", ""
		}
		if h, m := roleFor(pigTitleRole); h == "pig" {
			return h, m
		}
	}
	return "", ""
}

// maybeTitle starts the one-off title call for a root Session's first prompt.
// It runs once per Session per Daemon lifetime and in the background: a
// failure or an empty reply leaves the label as it is.
func (d *Daemon) maybeTitle(id, text string) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok || s.meta.Title != "" || s.titleTried || s.meta.Scout || s.meta.ParentID != "" {
		d.mu.Unlock()
		return
	}
	s.titleTried = true
	harness := s.meta.Harness
	owner := s.meta.Owner
	d.mu.Unlock()
	roleHarness, model := titleTarget(harness)
	if roleHarness == "" {
		return
	}
	d.mu.Lock()
	tb, _ := d.backendForLocked(roleHarness).(titleBackend)
	d.mu.Unlock()
	if tb == nil {
		return
	}
	dirs, err := d.ownerDirsFor(owner)
	if err != nil {
		return
	}
	go func() {
		raw, err := tb.Title(dirs, model, titlePrompt(text))
		if err != nil {
			return
		}
		if title := cleanTitle(raw); title != "" {
			d.setTitle(id, title)
		}
	}()
}

// setTitle stores the title and persists the meta, which also tells the live
// hub the Session list changed. A Session deleted meanwhile is skipped.
func (d *Daemon) setTitle(id, title string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok || s.meta.Title != "" {
		return
	}
	s.meta.Title = title
	d.persistMeta(s.meta)
}

// titleWorkDir is the fixed directory every claude title session runs in, so
// the adapter's session files land under one project instead of the user's.
// EL_TITLES_DIR overrides it for tests, so title tests never touch the
// shared temp dir.
func titleWorkDir() string {
	if v := strings.TrimSpace(os.Getenv("EL_TITLES_DIR")); v != "" {
		_ = os.MkdirAll(v, 0o700)
		return v
	}
	dir := filepath.Join(os.TempDir(), "el-titles")
	_ = os.MkdirAll(dir, 0o700)
	return dir
}

// Title runs one throwaway ACP session: the stripped option set, no MCP
// servers, the title system prompt in place of the preset, one prompt. The
// adapter process dies with the call; Sessions' own adapters are untouched
// because ACP callbacks are per connection.
func (b *ClaudeBackend) Title(dirs ownerDirs, model, prompt string) (string, error) {
	if !validClaudeModel(model) {
		return "", fmt.Errorf("unknown claude model (want opus, sonnet or haiku)")
	}
	newConn := b.newConn
	if newConn == nil {
		cfg := append([]string{}, b.adapterCommand...)
		newConn = func() (acpConn, error) { return spawnACPConnEnv(cfg, claudeEnv(dirs)) }
	}
	conn, err := newConn()
	if err != nil {
		return "", fmt.Errorf("start claude adapter: %w", err)
	}
	defer conn.Close()
	b.applyTimeouts(conn)
	if pc, ok := conn.(*acpProcessConn); ok && b.promptTimeout == 0 {
		pc.promptTimeout = titleCallTimeout
	}
	if err := conn.Initialize(); err != nil {
		return "", fmt.Errorf("acp initialize: %w", err)
	}
	meta := b.strippingMeta(model)
	meta["systemPrompt"] = titleSystemPrompt
	// A title is one bare reply: no per-turn register reminder.
	delete(meta["claudeCode"].(map[string]any)["options"].(map[string]any), "settings")
	acpID, err := conn.NewSession(titleWorkDir(), []acpMCPServer{}, meta)
	if err != nil {
		return "", fmt.Errorf("acp session/new: %w", err)
	}
	var sb strings.Builder
	_, err = conn.Prompt(acpID, prompt, acpCallbacks{OnUpdate: func(u map[string]any) {
		if kind, _ := u["sessionUpdate"].(string); kind == "agent_message_chunk" {
			sb.WriteString(acpText(u["content"]))
		}
	}})
	if err != nil {
		return "", err
	}
	return sb.String(), nil
}

// Title is one bare SDK completion: the model's own provider, the title
// system prompt, one user message, a tiny token cap. No Runtime, no Session,
// no tools, no session file.
func (b *PigBackend) Title(dirs ownerDirs, model, prompt string) (string, error) {
	agentDir := dirs.pig
	if agentDir == "" {
		b.mu.Lock()
		agentDir = b.AgentDir
		b.mu.Unlock()
	}
	return pigOneShot(agentDir, model, titleSystemPrompt, prompt, titleMaxTokens, titleCallTimeout)
}

// titlesEnvOff reports EL_SESSION_TITLES=0: the one switch that also
// silences pig naming, which is otherwise driven by the pig-title role alone.
func titlesEnvOff() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EL_SESSION_TITLES"))) {
	case "0", "false", "off":
		return true
	}
	return false
}

// titlesEnabled reports whether claude Sessions title themselves. The
// EL_SESSION_TITLES environment variable (0 or 1) wins over the
// session_titles settings key, which defaults to on.
func titlesEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EL_SESSION_TITLES"))) {
	case "0", "false", "off":
		return false
	case "1", "true", "on":
		return true
	}
	all, err := readSettingsMap()
	if err != nil {
		return true
	}
	raw, ok := all["session_titles"]
	if !ok {
		return true
	}
	var on bool
	if json.Unmarshal(raw, &on) != nil {
		return true
	}
	return on
}

// sessionTitlesSetting is the session_titles settings key alone (default on),
// which the settings page edits; titlesFromEnv says the environment overrides
// it, the same locked-field pattern as max_tickets.
func sessionTitlesSetting() bool {
	all, err := readSettingsMap()
	if err != nil {
		return true
	}
	var on bool
	if raw, ok := all["session_titles"]; ok && json.Unmarshal(raw, &on) == nil {
		return on
	}
	return true
}

func titlesFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EL_SESSION_TITLES"))) {
	case "0", "false", "off", "1", "true", "on":
		return true
	}
	return false
}
