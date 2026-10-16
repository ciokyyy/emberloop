package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserSessionEnvAndDomainValidation(t *testing.T) {
	bin := tempDir(t)
	t.Setenv("EL_BIN_DIR", bin)
	browser := filepath.Join(bin, "agent-browser")
	if err := os.WriteFile(browser, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	d := &Daemon{stateDir: filepath.Join(tempDir(t), "daemon-a")}
	env := d.browserSessionEnv("session-123")
	values := map[string]string{}
	for _, pair := range env {
		values[pair[0]] = pair[1]
	}
	for _, key := range []string{"AGENT_BROWSER_SESSION", "AGENT_BROWSER_NAMESPACE", "AGENT_BROWSER_SCREENSHOT_DIR", "AGENT_BROWSER_ALLOWED_DOMAINS", "AGENT_BROWSER_MAX_OUTPUT", "AGENT_BROWSER_CONTENT_BOUNDARIES", "AGENT_BROWSER_IDLE_TIMEOUT_MS", "PATH"} {
		if values[key] == "" {
			t.Errorf("missing %s in %v", key, values)
		}
	}
	if values["AGENT_BROWSER_SESSION"] != "session-123" || !strings.HasSuffix(values["AGENT_BROWSER_SCREENSHOT_DIR"], "session-123") {
		t.Fatalf("session env mismatch: %v", values)
	}
	if values["AGENT_BROWSER_ALLOWED_DOMAINS"] != "localhost,127.0.0.1" {
		t.Fatalf("domains = %q", values["AGENT_BROWSER_ALLOWED_DOMAINS"])
	}
	if validateBrowserDomains(json.RawMessage(`["localhost","*.example.com"]`)) != "" {
		t.Fatal("valid domain list rejected")
	}
	if validateBrowserDomains(json.RawMessage(`["https://example.com"]`)) == "" {
		t.Fatal("URL accepted as a domain")
	}
	if browserNamespace(filepath.Join(tempDir(t), "daemon-a")) == browserNamespace(filepath.Join(tempDir(t), "other", "daemon-a")) {
		t.Fatal("namespace collision")
	}
}

func TestBrowserScreenshotJanitorKeepsActiveAndRecent(t *testing.T) {
	state := tempDir(t)
	root := filepath.Join(state, "agent-browser", "screenshots")
	old := filepath.Join(root, "ended")
	recent := filepath.Join(root, "recent")
	active := filepath.Join(root, "active")
	for _, dir := range []string{old, recent, active} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := os.Chtimes(old, now.Add(-diskTempMaxAge-time.Hour), now.Add(-diskTempMaxAge-time.Hour)); err != nil {
		t.Fatal(err)
	}
	d := &Daemon{stateDir: state, sessions: map[string]*session{"active": {live: true}}}
	fs := defaultDiskFS()
	fs.now = func() time.Time { return now }
	if got := d.cleanBrowserScreenshotDirs(fs); got != 1 {
		t.Fatalf("removed %d dirs, want 1", got)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old screenshot dir remains: %v", err)
	}
	for _, dir := range []string{recent, active} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("expected dir %s to remain: %v", dir, err)
		}
	}
}

func TestBrowserCloseUsesSessionAndNamespace(t *testing.T) {
	bin := tempDir(t)
	logPath := filepath.Join(tempDir(t), "calls")
	t.Setenv("EL_BIN_DIR", bin)
	script := "#!/bin/sh\nprintf '%s|%s|%s\\n' \"$AGENT_BROWSER_SESSION\" \"$AGENT_BROWSER_NAMESPACE\" \"$*\" >> \"" + logPath + "\"\n"
	if err := os.WriteFile(filepath.Join(bin, "agent-browser"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	d := &Daemon{stateDir: filepath.Join(tempDir(t), "state")}
	d.closeBrowser("s-test", false)
	d.closeBrowser("", true)
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "s-test|") || !strings.HasSuffix(lines[0], "|close") || !strings.HasSuffix(lines[1], "|close --all") {
		t.Fatalf("browser calls = %q", calls)
	}
}

func TestBrowserSessionEnvAbsentBinary(t *testing.T) {
	t.Setenv("EL_BIN_DIR", tempDir(t))
	if got := (&Daemon{stateDir: tempDir(t)}).browserSessionEnv("s"); len(got) != 0 {
		t.Fatalf("environment without binary = %v", got)
	}
}

func TestBrowserDisabledSuppressesEnvironmentButStillCleans(t *testing.T) {
	bin := tempDir(t)
	t.Setenv("EL_BIN_DIR", bin)
	callLog := filepath.Join(bin, "calls")
	script := "#!/bin/sh\necho \"$*\" >> \"" + callLog + "\"\n"
	if err := os.WriteFile(filepath.Join(bin, "agent-browser"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(tempDir(t), "settings.json")
	if err := os.WriteFile(settings, []byte(`{"browser.enabled":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)
	d := &Daemon{stateDir: filepath.Join(tempDir(t), "state")}
	if got := d.browserSessionEnv("s"); len(got) != 0 {
		t.Fatalf("disabled environment = %v", got)
	}
	d.closeBrowser("s", false)
	if _, err := os.Stat(callLog); err != nil {
		t.Fatalf("cleanup skipped while disabled: %v", err)
	}
}
