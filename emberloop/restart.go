package el

import (
	"encoding/json"
	goflag "flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	prodDaemonURL = "http://127.0.0.1:7940"
	devDaemonURL  = "http://127.0.0.1:7941"
)

// RunRestart implements `el restart`: it asks the Daemon for the graceful
// restart (POST /api/restart). The Daemon pauses running Runs, drains
// Sessions, re-executes itself, and resumes the Runs. Never use systemctl
// restart for this: it cuts Worker turns and drops the caller's own Session.
// With --wait it polls /api/health until the Daemon went down and came back.
func RunRestart(argv []string, out io.Writer) error {
	fs := goflag.NewFlagSet("el restart", goflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dev := fs.Bool("dev", false, "restart the dev Daemon (port 7941)")
	url := fs.String("url", "", "Daemon base URL (default EL_WEB_URL, else the prod Daemon)")
	wait := fs.Bool("wait", false, "wait until the Daemon is back (do not use inside a Daemon Session)")
	if err := fs.Parse(argv); err != nil {
		return fmt.Errorf("usage: el restart [--dev] [--url URL] [--wait]")
	}
	base := *url
	if base == "" {
		base = os.Getenv("EL_WEB_URL")
	}
	if base == "" {
		base = prodDaemonURL
		if *dev {
			base = devDaemonURL
		}
	}
	base = strings.TrimRight(base, "/")
	return requestRestart(base, restartToken(*dev), *wait, out, 6*time.Minute)
}

// restartState mirrors the Daemon's RestartState (emberloop-web/restart.go):
// what a pending restart still waits on. Parsed loosely so an older Daemon
// that answers without it still works.
type restartState struct {
	Requested bool      `json:"requested"`
	Since     time.Time `json:"since"`
	Drain     string    `json:"drain"`
	Sessions  []string  `json:"sessions"`
	Groups    []string  `json:"groups"`
	Runs      []string  `json:"runs"`
	Starting  int       `json:"starting"`
	Mining    bool      `json:"mining"`
}

// describe renders the state as one line: when the restart was requested and
// what still blocks the drain, or that nothing blocks it.
func (s restartState) describe() string {
	var waiting []string
	if len(s.Sessions) > 0 {
		waiting = append(waiting, "sessions "+strings.Join(s.Sessions, ","))
	}
	if len(s.Groups) > 0 {
		waiting = append(waiting, "groups "+strings.Join(s.Groups, ","))
	}
	if len(s.Runs) > 0 {
		waiting = append(waiting, "runs "+strings.Join(s.Runs, ","))
	}
	if s.Starting > 0 {
		waiting = append(waiting, fmt.Sprintf("starting %d", s.Starting))
	}
	if s.Mining {
		waiting = append(waiting, "mining")
	}
	since := s.Since.Format(time.RFC3339)
	if s.Since.IsZero() {
		since = "unknown time"
	}
	if len(waiting) == 0 {
		return fmt.Sprintf("requested at %s (drain %s), nothing blocking", since, s.Drain)
	}
	return fmt.Sprintf("requested at %s (drain %s), waiting on %s", since, s.Drain, strings.Join(waiting, "; "))
}

// restartPollInterval spaces the /api/health polls while waiting for the
// Daemon to go down and come back. A var so tests can shrink it; production
// keeps the 500ms pace.
var restartPollInterval = 500 * time.Millisecond

func requestRestart(base, token string, wait bool, out io.Writer, timeout time.Duration) error {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPost, base+"/api/restart", nil)
	if err != nil {
		return fmt.Errorf("el restart: %w", err)
	}
	resp, err := client.Do(withDaemonToken(req, token))
	if err != nil {
		return fmt.Errorf("el restart: Daemon at %s unreachable: %w", base, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("el restart: Daemon refused the machine token: set EL_WEB_TOKEN to the token in <state dir>/enderbloom/daemon.token")
	}
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("el restart: Daemon answered %s (is it an older build? run el update)", resp.Status)
	}
	fmt.Fprintln(out, "restart requested: running Runs pause, Sessions drain, the Daemon re-executes and resumes the Runs")
	var res struct {
		State *restartState `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.State != nil {
		fmt.Fprintln(out, "restart state: "+res.State.describe())
	}
	if !wait {
		return nil
	}
	deadline := time.Now().Add(timeout)
	down := false
	for time.Now().Before(deadline) {
		ok := healthy(client, base, token)
		if !ok {
			down = true
		} else if down {
			fmt.Fprintln(out, "Daemon is back")
			return nil
		}
		time.Sleep(restartPollInterval)
	}
	return fmt.Errorf("el restart: Daemon did not come back within %s", timeout)
}

func healthy(c *http.Client, base, token string) bool {
	req, err := http.NewRequest(http.MethodGet, base+"/api/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.Do(withDaemonToken(req, token))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// restartToken is the machine token el restart sends to the target Daemon: EL_WEB_TOKEN,
// else the token file of the target Daemon state dir.
func restartToken(dev bool) string {
	if tok := strings.TrimSpace(os.Getenv("EL_WEB_TOKEN")); tok != "" {
		return tok
	}
	return readTokenFile(defaultTokenPath(dev))
}

// readTokenFile returns the machine token stored at path, or "" if unreadable.
func readTokenFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// defaultTokenPath is the token file of the target Daemon's default state dir:
// pig for prod, pig-dev for --dev (EL_WEB_DEV_STATE overrides it). Both sit
// under XDG_DATA_HOME, else ~/.local/share.
func defaultTokenPath(dev bool) string {
	if dev {
		if dir := os.Getenv("EL_WEB_DEV_STATE"); dir != "" {
			return filepath.Join(dir, "enderbloom", "daemon.token")
		}
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	name := "pig"
	if dev {
		name = "pig-dev"
	}
	return filepath.Join(dataHome, name, "enderbloom", "daemon.token")
}

// withDaemonToken sets the machine token on a request to the Daemon.
func withDaemonToken(req *http.Request, token string) *http.Request {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}
