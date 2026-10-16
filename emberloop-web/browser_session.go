package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"example.com/emberloop-web/el/tools"
)

const browserIdleTimeoutMS = "300000"

func (d *Daemon) browserSessionEnv(id string) [][2]string {
	if id == "" || !browserEnabled() {
		return nil
	}
	bin := filepath.Join(tools.BinDir(), "agent-browser")
	if _, err := os.Stat(bin); err != nil {
		return nil
	}
	allowed := d.browserAllowedDomains()
	shotDir := filepath.Join(d.stateDir, "agent-browser", "screenshots", id)
	_ = os.MkdirAll(shotDir, 0o700)
	return [][2]string{
		{"AGENT_BROWSER_SESSION", id},
		{"AGENT_BROWSER_NAMESPACE", browserNamespace(d.stateDir)},
		{"AGENT_BROWSER_SCREENSHOT_DIR", shotDir},
		{"AGENT_BROWSER_ALLOWED_DOMAINS", strings.Join(allowed, ",")},
		{"AGENT_BROWSER_MAX_OUTPUT", "10000"},
		{"AGENT_BROWSER_CONTENT_BOUNDARIES", "true"},
		{"AGENT_BROWSER_IDLE_TIMEOUT_MS", browserIdleTimeoutMS},
		{"PATH", tools.BinDir() + string(os.PathListSeparator) + os.Getenv("PATH")},
	}
}

func browserNamespace(stateDir string) string { return filepath.Base(filepath.Clean(stateDir)) }

func browserEnabled() bool {
	all, err := readSettingsMap()
	if err != nil {
		return true
	}
	var enabled bool
	raw, ok := all["browser.enabled"]
	if !ok {
		return true
	}
	if json.Unmarshal(raw, &enabled) != nil {
		return false
	}
	return enabled
}

func (d *Daemon) browserAllowedDomains() []string {
	all, _ := d.settingsMap()
	var domains []string
	if raw := all["browser.allowed_domains"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &domains)
	}
	if len(domains) == 0 {
		return []string{"localhost", "127.0.0.1"}
	}
	return domains
}
