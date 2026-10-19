package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
	shotDir := filepath.Join(d.stateDir, "agent-browser", "screenshots", id)
	_ = os.MkdirAll(shotDir, 0o700)
	env := [][2]string{
		{"AGENT_BROWSER_SESSION", id},
		{"AGENT_BROWSER_NAMESPACE", browserNamespace(d.stateDir)},
		{"AGENT_BROWSER_SCREENSHOT_DIR", shotDir},
		{"AGENT_BROWSER_ALLOWED_DOMAINS", strings.Join(d.browserAllowedDomains(), ",")},
		{"AGENT_BROWSER_MAX_OUTPUT", "10000"},
		{"AGENT_BROWSER_CONTENT_BOUNDARIES", "true"},
		{"AGENT_BROWSER_IDLE_TIMEOUT_MS", browserIdleTimeoutMS},
		{"PATH", tools.BinDir() + string(os.PathListSeparator) + os.Getenv("PATH")},
	}
	for _, candidate := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome"} {
		if path, err := exec.LookPath(candidate); err == nil {
			env = append(env, [2]string{"AGENT_BROWSER_EXECUTABLE_PATH", path})
			break
		}
	}
	return env
}
func (d *Daemon) sessionToolEnv(id string) [][2]string {
	env := d.browserSessionEnv(id)
	if port, ok := d.previewPortFor(id); ok {
		env = append(env, [2]string{"PORT", strconv.Itoa(port)}, [2]string{previewSessionEnv, id})
	}
	return env
}

func browserNamespace(stateDir string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(stateDir)))
	return hex.EncodeToString(sum[:8])
}

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

func (d *Daemon) closeBrowser(id string, all bool) {
	bin := filepath.Join(tools.BinDir(), "agent-browser")
	if _, err := os.Stat(bin); err != nil {
		return
	}
	args := []string{"close"}
	if all {
		args = append(args, "--all")
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "AGENT_BROWSER_NAMESPACE="+browserNamespace(d.stateDir))
	if !all {
		cmd.Env = append(cmd.Env, "AGENT_BROWSER_SESSION="+id)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("el-web: close browser session %s: %v: %s", id, err, output)
	}
}

var browserDomainPattern = regexp.MustCompile(`^(\*\.)?([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)(\.([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?))*$`)

func validateBrowserDomains(raw json.RawMessage) string {
	var domains []string
	if err := json.Unmarshal(raw, &domains); err != nil || len(domains) == 0 {
		return "browser.allowed_domains must be a non-empty list of domains"
	}
	seen := map[string]bool{}
	for _, domain := range domains {
		if domain != strings.TrimSpace(domain) || domain == "" || !browserDomainPattern.MatchString(domain) {
			return "browser.allowed_domains must contain valid domain names without surrounding whitespace"
		}
		if seen[strings.ToLower(domain)] {
			return "browser.allowed_domains must not contain duplicates"
		}
		seen[strings.ToLower(domain)] = true
	}
	return ""
}
