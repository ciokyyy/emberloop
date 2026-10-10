package el

import (
	goflag "flag"
	"fmt"
	"io"
	"net/http"
	"os"
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
	return requestRestart(base, *wait, out, 6*time.Minute)
}

func requestRestart(base string, wait bool, out io.Writer, timeout time.Duration) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(base+"/api/restart", "application/json", nil)
	if err != nil {
		return fmt.Errorf("el restart: Daemon at %s unreachable: %w", base, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("el restart: Daemon answered %s (is it an older build? run el update)", resp.Status)
	}
	fmt.Fprintln(out, "restart requested: running Runs pause, Sessions drain, the Daemon re-executes and resumes the Runs")
	if !wait {
		return nil
	}
	deadline := time.Now().Add(timeout)
	down := false
	for time.Now().Before(deadline) {
		ok := healthy(client, base)
		if !ok {
			down = true
		} else if down {
			fmt.Fprintln(out, "Daemon is back")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("el restart: Daemon did not come back within %s", timeout)
}

func healthy(c *http.Client, base string) bool {
	resp, err := c.Get(base + "/api/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
